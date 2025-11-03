package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/concurrency"
	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/memory"
	"github.com/cybergodev/httpc/internal/monitoring"
	"github.com/cybergodev/httpc/internal/security"
)

type Client struct {
	config *Config

	transport         *Transport
	requestProcessor  *RequestProcessor
	responseProcessor *ResponseProcessor
	retryEngine       *RetryEngine
	validator         *security.Validator

	concurrencyManager *concurrency.Manager
	memoryManager      *memory.Manager
	connectionPool     *connection.PoolManager
	healthChecker      *monitoring.HealthChecker

	closed             int32
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	averageLatency     int64

	closeOnce sync.Once
	mu        sync.RWMutex
}

type Config struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	ProxyURL              string

	TLSConfig             interface{}
	MinTLSVersion         uint16
	MaxTLSVersion         uint16
	InsecureSkipVerify    bool
	MaxResponseBodySize   int64
	MaxConcurrentRequests int
	ValidateURL           bool
	ValidateHeaders       bool
	AllowPrivateIPs       bool
	StrictContentLength   bool

	MaxRetries    int
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	BackoffFactor float64
	Jitter        bool

	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	EnableHTTP2     bool

	CookieJar     interface{}
	EnableCookies bool
}

type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]any
	Body        any
	Timeout     time.Duration
	MaxRetries  int
	Context     context.Context
	Cookies     []*http.Cookie
}

type Response struct {
	StatusCode    int
	Status        string
	Headers       map[string][]string
	Body          string
	RawBody       []byte
	ContentLength int64
	Proto         string
	Duration      time.Duration
	Attempts      int
	Request       interface{}
	Response      interface{}
	Cookies       []*http.Cookie
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	client := &Client{
		config: config,
	}

	var err error

	client.memoryManager = memory.NewManager(memory.DefaultConfig())

	connConfig := connection.DefaultConfig()
	connConfig.MaxIdleConns = config.MaxIdleConns
	connConfig.MaxIdleConnsPerHost = config.MaxIdleConnsPerHost
	connConfig.MaxConnsPerHost = config.MaxConnsPerHost
	connConfig.DialTimeout = config.DialTimeout
	connConfig.KeepAlive = config.KeepAlive
	connConfig.TLSHandshakeTimeout = config.TLSHandshakeTimeout
	connConfig.ResponseHeaderTimeout = config.ResponseHeaderTimeout
	connConfig.IdleConnTimeout = config.IdleConnTimeout
	connConfig.MinTLSVersion = config.MinTLSVersion
	connConfig.MaxTLSVersion = config.MaxTLSVersion
	connConfig.InsecureSkipVerify = config.InsecureSkipVerify
	connConfig.EnableHTTP2 = config.EnableHTTP2
	connConfig.ProxyURL = config.ProxyURL
	connConfig.CookieJar = config.CookieJar

	client.connectionPool, err = connection.NewPoolManager(connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	maxConcurrent := config.MaxConcurrentRequests
	if maxConcurrent <= 0 {
		maxConcurrent = 500
	}
	client.concurrencyManager = concurrency.NewManager(maxConcurrent, maxConcurrent*2)

	client.transport, err = NewTransport(config, client.connectionPool)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	client.requestProcessor = NewRequestProcessor(config, client.memoryManager)
	client.responseProcessor = NewResponseProcessor(config, client.memoryManager)
	client.retryEngine = NewRetryEngine(config)

	validatorConfig := &security.Config{
		ValidateURL:           config.ValidateURL,
		ValidateHeaders:       config.ValidateHeaders,
		MaxResponseBodySize:   config.MaxResponseBodySize,
		MaxConcurrentRequests: config.MaxConcurrentRequests,
		AllowPrivateIPs:       config.AllowPrivateIPs,
	}
	client.validator = security.NewValidatorWithConfig(validatorConfig)
	client.healthChecker = monitoring.NewHealthChecker()

	return client, nil
}

func (c *Client) Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, fmt.Errorf("client is closed")
	}

	atomic.AddInt64(&c.totalRequests, 1)
	startTime := time.Now()

	headers := c.memoryManager.GetHeaders()
	defer c.memoryManager.PutHeaders(headers)

	req := &Request{
		Method:      method,
		URL:         url,
		Context:     ctx,
		Headers:     headers,
		QueryParams: make(map[string]any, 4),
	}

	for _, option := range options {
		if option != nil {
			option(req)
		}
	}

	secReq := &security.Request{
		Method:      req.Method,
		URL:         req.URL,
		Headers:     req.Headers,
		QueryParams: req.QueryParams,
		Body:        req.Body,
	}
	if err := c.validator.ValidateRequest(secReq); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	response, err := c.executeWithRetry(req)
	duration := time.Since(startTime)
	c.updateLatencyMetrics(duration.Nanoseconds())

	isTimeout := err != nil && (errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "deadline exceeded"))

	success := err == nil
	c.healthChecker.RecordRequest(success, duration, isTimeout)

	if err != nil {
		atomic.AddInt64(&c.failedRequests, 1)
		return nil, err
	}

	atomic.AddInt64(&c.successfulRequests, 1)

	if response != nil {
		response.Duration = duration
		return response, nil
	}

	return &Response{
		StatusCode: 200,
		Duration:   duration,
	}, nil
}

func (c *Client) Get(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("GET", url, options...)
}

func (c *Client) Post(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("POST", url, options...)
}

func (c *Client) Put(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("PUT", url, options...)
}

func (c *Client) Patch(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("PATCH", url, options...)
}

func (c *Client) Delete(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("DELETE", url, options...)
}

func (c *Client) Head(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("HEAD", url, options...)
}

func (c *Client) Options(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("OPTIONS", url, options...)
}

func (c *Client) executeWithDefaultContext(method, url string, options ...RequestOption) (*Response, error) {
	ctx, cancel := c.createDefaultContext()
	defer cancel()
	return c.Request(ctx, method, url, options...)
}

func (c *Client) createDefaultContext() (context.Context, context.CancelFunc) {
	if c.config.Timeout > 0 {
		return context.WithTimeout(context.Background(), c.config.Timeout)
	}
	return context.WithCancel(context.Background())
}

func (c *Client) sleepWithContext(ctx context.Context, duration time.Duration) error {
	if ctx == nil {
		time.Sleep(duration)
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) updateLatencyMetrics(latency int64) {
	for {
		current := atomic.LoadInt64(&c.averageLatency)
		newAvg := (current*9 + latency) / 10
		if atomic.CompareAndSwapInt64(&c.averageLatency, current, newAvg) {
			break
		}
	}
}

func (c *Client) executeWithRetry(req *Request) (*Response, error) {
	maxRetries := c.config.MaxRetries
	if req.MaxRetries > 0 {
		maxRetries = req.MaxRetries
	}

	var lastErr error
	var lastResp *Response
	var clientErr *ClientError

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.executeRequest(req)

		shouldRetry := false

		if err != nil {
			clientErr = ClassifyError(err, req.URL, req.Method, attempt+1)
			lastErr = clientErr

			if clientErr.IsRetryable() && attempt < maxRetries {
				shouldRetry = c.retryEngine.ShouldRetry(nil, err, attempt)
			}
		} else if resp != nil {
			lastResp = resp

			if c.retryEngine.isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
				shouldRetry = c.retryEngine.ShouldRetry(resp, nil, attempt)
			} else {
				resp.Attempts = attempt + 1
				return resp, nil
			}
		}

		if !shouldRetry {
			break
		}

		delay := c.retryEngine.GetDelayWithResponse(attempt, lastResp)
		if err := c.sleepWithContext(req.Context, delay); err != nil {
			return nil, ClassifyError(err, req.URL, req.Method, attempt+1)
		}
	}

	if lastResp != nil {
		lastResp.Attempts = maxRetries + 1
		return lastResp, nil
	}

	if clientErr != nil {
		clientErr.Attempts = maxRetries + 1
		return nil, clientErr
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
}

func (c *Client) executeRequest(req *Request) (resp *Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ClassifyError(fmt.Errorf("panic during request execution: %v", r), req.URL, req.Method, 0)
		}
	}()

	originalCtx := req.Context
	if originalCtx == nil {
		originalCtx = context.Background()
	}

	var cancel context.CancelFunc
	var execCtx context.Context
	timeout := req.Timeout

	if timeout <= 0 && c.config.Timeout > 0 {
		timeout = c.config.Timeout
	}

	if timeout > 0 {
		existingDeadline, hasDeadline := originalCtx.Deadline()

		if !hasDeadline || (hasDeadline && time.Until(existingDeadline) > timeout) {
			execCtx, cancel = context.WithTimeout(originalCtx, timeout)
			defer cancel()
		} else {
			execCtx = originalCtx
		}
	} else {
		execCtx = originalCtx
	}

	select {
	case <-execCtx.Done():
		return nil, ClassifyError(execCtx.Err(), req.URL, req.Method, 0)
	default:
	}

	reqCopy := *req
	reqCopy.Context = execCtx
	httpReq, err := c.requestProcessor.Build(&reqCopy)
	if err != nil {
		return nil, ClassifyError(fmt.Errorf("failed to build request: %w", err), req.URL, req.Method, 0)
	}

	start := time.Now()
	httpResp, err := c.transport.RoundTrip(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, ClassifyError(fmt.Errorf("transport error: %w", err), req.URL, req.Method, 0)
	}

	defer func() {
		if httpResp != nil && httpResp.Body != nil {
			if resp == nil || resp.RawBody == nil {
				io.Copy(io.Discard, io.LimitReader(httpResp.Body, 64*1024))
			}
			httpResp.Body.Close()
		}
	}()

	resp, err = c.responseProcessor.Process(httpResp)
	if err != nil {
		return nil, ClassifyError(fmt.Errorf("failed to process response: %w", err), req.URL, req.Method, 0)
	}

	resp.Duration = duration
	return resp, nil
}

func (c *Client) GetHealthStatus() monitoring.HealthStatus {
	if c.connectionPool != nil {
		metrics := c.connectionPool.GetMetrics()
		poolUtil := float64(metrics.ActiveConnections) / float64(metrics.TotalConnections+1)
		c.healthChecker.UpdateResourceMetrics(metrics.ActiveConnections, poolUtil, 0)
	}

	return c.healthChecker.CheckHealth()
}

func (c *Client) IsHealthy() bool {
	return c.healthChecker.IsHealthy()
}

func (c *Client) Close() error {
	var closeErr error

	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.closed, 1)

		if c.concurrencyManager != nil {
			if err := c.concurrencyManager.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close concurrency manager: %w", err)
			}
		}

		if c.connectionPool != nil {
			if err := c.connectionPool.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close connection pool: %w", err)
			}
		}

		if c.memoryManager != nil {
			if err := c.memoryManager.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close memory manager: %w", err)
			}
		}

		if c.transport != nil {
			if err := c.transport.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close transport: %w", err)
			}
		}
	})

	return closeErr
}

type RequestOption func(*Request)
