package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/security"
)

type Client struct {
	config *Config

	transport         *Transport
	requestProcessor  *RequestProcessor
	responseProcessor *ResponseProcessor
	retryEngine       *RetryEngine
	validator         *security.Validator

	connectionPool *connection.PoolManager

	closed             int32
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	averageLatency     int64

	closeOnce sync.Once
}

// Config defines the HTTP client configuration.
// Config should be treated as immutable after creation.
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

	TLSConfig           any
	MinTLSVersion       uint16
	MaxTLSVersion       uint16
	InsecureSkipVerify  bool
	MaxResponseBodySize int64
	ValidateURL         bool
	ValidateHeaders     bool
	AllowPrivateIPs     bool
	StrictContentLength bool

	MaxRetries    int
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	BackoffFactor float64
	Jitter        bool

	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	MaxRedirects    int
	EnableHTTP2     bool

	CookieJar     any
	EnableCookies bool
}

type Request struct {
	Method          string
	URL             string
	Headers         map[string]string
	QueryParams     map[string]any
	Body            any
	Timeout         time.Duration
	MaxRetries      int
	Context         context.Context
	Cookies         []http.Cookie
	FollowRedirects *bool
	MaxRedirects    *int
}

// Response represents an HTTP response.
// Response objects are safe to read from multiple goroutines after they are returned.
type Response struct {
	StatusCode     int
	Status         string
	Headers        map[string][]string
	Body           string
	RawBody        []byte
	ContentLength  int64
	Proto          string
	Duration       time.Duration
	Attempts       int
	Cookies        []*http.Cookie
	RedirectChain  []string
	RedirectCount  int
	RequestHeaders map[string][]string // Actual headers sent with the request
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	client := &Client{
		config: config,
	}

	var err error

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
	connConfig.AllowPrivateIPs = config.AllowPrivateIPs

	client.connectionPool, err = connection.NewPoolManager(connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	client.transport, err = NewTransport(config, client.connectionPool)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	client.requestProcessor = NewRequestProcessor(config)
	client.responseProcessor = NewResponseProcessor(config)
	client.retryEngine = NewRetryEngine(config)

	validatorConfig := &security.Config{
		ValidateURL:         config.ValidateURL,
		ValidateHeaders:     config.ValidateHeaders,
		MaxResponseBodySize: config.MaxResponseBodySize,
		AllowPrivateIPs:     config.AllowPrivateIPs,
	}
	client.validator = security.NewValidatorWithConfig(validatorConfig)

	return client, nil
}

func (c *Client) Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, fmt.Errorf("client is closed")
	}

	atomic.AddInt64(&c.totalRequests, 1)
	startTime := time.Now()

	req := &Request{
		Method:  method,
		URL:     url,
		Context: ctx,
	}

	for _, option := range options {
		if option != nil {
			if err := option(req); err != nil {
				atomic.AddInt64(&c.failedRequests, 1)
				return nil, fmt.Errorf("failed to apply request option: %w", err)
			}
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
		atomic.AddInt64(&c.failedRequests, 1)
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	response, err := c.executeWithRetry(req)
	duration := time.Since(startTime)
	c.updateLatencyMetrics(duration.Nanoseconds())

	if err != nil {
		atomic.AddInt64(&c.failedRequests, 1)
		return nil, err
	}

	atomic.AddInt64(&c.successfulRequests, 1)
	response.Duration = duration
	return response, nil
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

// updateLatencyMetrics updates the rolling average latency using lock-free atomic operations.
func (c *Client) updateLatencyMetrics(latency int64) {
	for {
		current := atomic.LoadInt64(&c.averageLatency)
		newAvg := (current*9 + latency) / 10
		if atomic.CompareAndSwapInt64(&c.averageLatency, current, newAvg) {
			break
		}
	}
}

// executeWithRetry executes a request with intelligent retry logic.
// Optimized for performance with minimal allocations and efficient error handling.
func (c *Client) executeWithRetry(req *Request) (*Response, error) {
	maxRetries := c.config.MaxRetries
	if req.MaxRetries > 0 {
		maxRetries = req.MaxRetries
	}

	var lastErr error
	var lastResp *Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.executeRequest(req)

		if err != nil {
			clientErr := ClassifyError(err, req.URL, req.Method, attempt+1)
			lastErr = clientErr

			// Fast path: non-retryable errors
			if !clientErr.IsRetryable() || attempt >= maxRetries {
				clientErr.Attempts = attempt + 1
				return nil, clientErr
			}

			// Check retry policy
			if !c.retryEngine.ShouldRetry(nil, err, attempt) {
				clientErr.Attempts = attempt + 1
				return nil, clientErr
			}

			// Calculate delay and sleep
			delay := c.retryEngine.GetDelay(attempt)
			if sleepErr := c.sleepWithContext(req.Context, delay); sleepErr != nil {
				return nil, ClassifyError(sleepErr, req.URL, req.Method, attempt+1)
			}
			continue
		}

		if resp != nil {
			lastResp = resp

			// Check if response status is retryable
			if c.retryEngine.isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
				if c.retryEngine.ShouldRetry(resp, nil, attempt) {
					delay := c.retryEngine.GetDelayWithResponse(attempt, resp)
					if sleepErr := c.sleepWithContext(req.Context, delay); sleepErr != nil {
						return nil, ClassifyError(sleepErr, req.URL, req.Method, attempt+1)
					}
					continue
				}
			}

			// Success - set attempt count and return
			resp.Attempts = attempt + 1
			return resp, nil
		}
	}

	// Handle final failure cases
	if lastResp != nil {
		lastResp.Attempts = maxRetries + 1
		return lastResp, nil
	}

	if lastErr != nil {
		if clientErr, ok := lastErr.(*ClientError); ok {
			clientErr.Attempts = maxRetries + 1
			return nil, clientErr
		}
		return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
	}

	return nil, fmt.Errorf("request failed after %d attempts", maxRetries+1)
}

const (
	defaultMaxDrain int64 = 10 * 1024 * 1024 // 10MB
)

// executeRequest executes a single HTTP request with comprehensive error handling.
func (c *Client) executeRequest(req *Request) (*Response, error) {
	// Context setup with timeout handling
	execCtx := req.Context
	if execCtx == nil {
		execCtx = context.Background()
	}

	timeout := req.Timeout
	if timeout <= 0 && c.config.Timeout > 0 {
		timeout = c.config.Timeout
	}

	if timeout > 0 {
		if existingDeadline, hasDeadline := execCtx.Deadline(); !hasDeadline || time.Until(existingDeadline) > timeout {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(execCtx, timeout)
			defer cancel()
		}
	}

	select {
	case <-execCtx.Done():
		return nil, ClassifyError(execCtx.Err(), req.URL, req.Method, 0)
	default:
	}

	reqCopy := *req
	reqCopy.Context = execCtx

	followRedirects := c.config.FollowRedirects
	if req.FollowRedirects != nil {
		followRedirects = *req.FollowRedirects
	}
	maxRedirects := c.config.MaxRedirects
	if req.MaxRedirects != nil {
		maxRedirects = *req.MaxRedirects
	}
	c.transport.SetRedirectPolicy(followRedirects, maxRedirects)

	httpReq, err := c.requestProcessor.Build(&reqCopy)
	if err != nil {
		return nil, ClassifyError(fmt.Errorf("build request failed: %w", err), req.URL, req.Method, 0)
	}

	start := time.Now()
	httpResp, err := c.transport.RoundTrip(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, ClassifyError(fmt.Errorf("transport failed: %w", err), req.URL, req.Method, 0)
	}

	defer func() {
		if httpResp.Body != nil {
			maxDrain := defaultMaxDrain
			if c.config.MaxResponseBodySize > 0 && c.config.MaxResponseBodySize < maxDrain {
				maxDrain = c.config.MaxResponseBodySize
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(httpResp.Body, maxDrain))
			_ = httpResp.Body.Close()
		}
	}()

	resp, err := c.responseProcessor.Process(httpResp)
	if err != nil {
		return nil, ClassifyError(fmt.Errorf("process response failed: %w", err), req.URL, req.Method, 0)
	}

	if redirectChain := c.transport.GetRedirectChain(); len(redirectChain) > 0 {
		resp.RedirectChain = redirectChain
		resp.RedirectCount = len(redirectChain)
	}

	if httpResp.Request != nil {
		resp.RequestHeaders = make(map[string][]string, len(httpResp.Request.Header))
		for key, values := range httpResp.Request.Header {
			resp.RequestHeaders[key] = append([]string(nil), values...)
		}
	}

	resp.Duration = duration
	return resp, nil
}

// HealthStatus represents basic health metrics
type HealthStatus struct {
	Healthy            bool
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	ErrorRate          float64
}

func (c *Client) GetHealthStatus() HealthStatus {
	total := atomic.LoadInt64(&c.totalRequests)
	success := atomic.LoadInt64(&c.successfulRequests)
	failed := atomic.LoadInt64(&c.failedRequests)
	avgLatNs := atomic.LoadInt64(&c.averageLatency)

	var errorRate float64
	if total > 0 {
		errorRate = float64(failed) / float64(total)
	}

	healthy := errorRate < 0.1

	return HealthStatus{
		Healthy:            healthy,
		TotalRequests:      total,
		SuccessfulRequests: success,
		FailedRequests:     failed,
		AverageLatency:     time.Duration(avgLatNs),
		ErrorRate:          errorRate,
	}
}

func (c *Client) IsHealthy() bool {
	return c.GetHealthStatus().Healthy
}

func (c *Client) Close() error {
	var closeErr error

	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.closed, 1)

		if c.connectionPool != nil {
			if err := c.connectionPool.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close connection pool: %w", err)
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

type RequestOption func(*Request) error
