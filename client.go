package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// Client represents the HTTP client interface
type Client interface {
	// HTTP methods
	Get(url string, options ...RequestOption) (*Response, error)
	Post(url string, options ...RequestOption) (*Response, error)
	Put(url string, options ...RequestOption) (*Response, error)
	Patch(url string, options ...RequestOption) (*Response, error)
	Delete(url string, options ...RequestOption) (*Response, error)
	Head(url string, options ...RequestOption) (*Response, error)
	Options(url string, options ...RequestOption) (*Response, error)

	// Generic request method
	Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error)

	// File download
	DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error)
	DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error)

	// Client management
	Close() error
}

// clientImpl implements the Client interface using the engine
type clientImpl struct {
	engine *engine.Client
}

func New(config ...*Config) (Client, error) {
	var cfg *Config
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
		if err := ValidateConfig(cfg); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
	} else {
		cfg = DefaultConfig()
	}

	engineConfig := convertToEngineConfig(cfg)

	engineClient, err := engine.NewClient(engineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &clientImpl{engine: engineClient}, nil
}

func NewSecure() (Client, error) {
	return New(SecureConfig())
}

func NewPerformance() (Client, error) {
	return New(PerformanceConfig())
}

func NewMinimal() (Client, error) {
	return New(MinimalConfig())
}

func (c *clientImpl) Get(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Get(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Post(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Post(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Put(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Put(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Patch(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Patch(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Delete(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Delete(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Head(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Head(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Options(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Options(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Request(ctx, method, url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Close() error {
	return c.engine.Close()
}

// Default client instance for package-level functions
var (
	defaultClient     Client
	defaultClientErr  error
	defaultClientOnce sync.Once
	defaultClientMu   sync.RWMutex
)

// getDefaultClient returns the default client, creating it if necessary
func getDefaultClient() (Client, error) {
	// Fast path: check if client already exists
	defaultClientMu.RLock()
	client := defaultClient
	err := defaultClientErr
	defaultClientMu.RUnlock()

	if client != nil {
		return client, err
	}

	// Slow path: initialize client
	defaultClientOnce.Do(func() {
		defaultClientMu.Lock()
		defer defaultClientMu.Unlock()

		// Double-check in case another goroutine initialized it
		if defaultClient != nil {
			return
		}

		// Create new client with error handling
		newClient, initErr := New()
		if initErr != nil {
			defaultClientErr = initErr
			return
		}

		defaultClient = newClient
		defaultClientErr = nil
	})

	defaultClientMu.RLock()
	client = defaultClient
	err = defaultClientErr
	defaultClientMu.RUnlock()

	return client, err
}

// CloseDefaultClient closes the default client and resets it.
// This should be called when the application is shutting down to prevent resource leaks.
func CloseDefaultClient() error {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	if defaultClient == nil {
		return nil
	}

	err := defaultClient.Close()
	defaultClient = nil
	defaultClientErr = nil
	// Reset sync.Once to allow re-initialization if needed
	defaultClientOnce = sync.Once{}

	return err
}

// Get executes a GET request using the default client
func Get(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Get(url, options...)
}

// Post executes a POST request using the default client
func Post(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Post(url, options...)
}

// Put executes a PUT request using the default client
func Put(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Put(url, options...)
}

// Patch executes a PATCH request using the default client
func Patch(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Patch(url, options...)
}

// Delete executes a DELETE request using the default client
func Delete(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Delete(url, options...)
}

// Head executes a HEAD request using the default client
func Head(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Head(url, options...)
}

// Options executes an OPTIONS request using the default client
func Options(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Options(url, options...)
}

func SetDefaultClient(client Client) error {
	if client == nil {
		return fmt.Errorf("cannot set nil client as default")
	}

	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	var closeErr error
	if defaultClient != nil {
		closeErr = defaultClient.Close()
	}

	defaultClient = client
	defaultClientErr = nil
	defaultClientOnce = sync.Once{}

	return closeErr
}

func convertToEngineConfig(cfg *Config) *engine.Config {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Simplified connection pool calculations
	maxIdleConnsPerHost := calculateOptimalIdleConnsPerHost(cfg.MaxIdleConns, cfg.MaxConnsPerHost)

	// Reasonable concurrent request limits
	maxConcurrent := calculateOptimalConcurrency(cfg.MaxConnsPerHost, cfg.MaxIdleConns)

	// Standard timeout settings
	timeouts := calculateOptimalTimeouts(cfg.Timeout)

	// Determine TLS version settings
	minTLSVersion := cfg.MinTLSVersion
	if minTLSVersion == 0 {
		minTLSVersion = tls.VersionTLS12 // Default to TLS 1.2
	}

	maxTLSVersion := cfg.MaxTLSVersion
	if maxTLSVersion == 0 {
		maxTLSVersion = tls.VersionTLS13 // Default to TLS 1.3
	}

	return &engine.Config{
		Timeout:               cfg.Timeout,
		DialTimeout:           timeouts.Dial,
		KeepAlive:             timeouts.KeepAlive,
		TLSHandshakeTimeout:   timeouts.TLS,
		ResponseHeaderTimeout: timeouts.ResponseHeader,
		IdleConnTimeout:       timeouts.IdleConn,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		ProxyURL:              cfg.ProxyURL,
		TLSConfig:             cfg.TLSConfig,
		MinTLSVersion:         minTLSVersion,
		MaxTLSVersion:         maxTLSVersion,
		InsecureSkipVerify:    cfg.InsecureSkipVerify,
		MaxResponseBodySize:   cfg.MaxResponseBodySize,
		MaxConcurrentRequests: maxConcurrent,
		ValidateURL:           true, // Force enable URL validation
		ValidateHeaders:       true, // Force enable header validation
		AllowPrivateIPs:       cfg.AllowPrivateIPs,
		StrictContentLength:   cfg.StrictContentLength,
		MaxRetries:            cfg.MaxRetries,
		RetryDelay:            cfg.RetryDelay,
		MaxRetryDelay:         calculateMaxRetryDelay(cfg.RetryDelay, cfg.BackoffFactor),
		BackoffFactor:         cfg.BackoffFactor,
		Jitter:                true, // Enable jitter to prevent thundering herd
		UserAgent:             cfg.UserAgent,
		Headers:               cfg.Headers,
		FollowRedirects:       cfg.FollowRedirects,
		EnableHTTP2:           cfg.EnableHTTP2,
		CookieJar:             createCookieJar(cfg.EnableCookies),
		EnableCookies:         cfg.EnableCookies,
	}
}

type timeoutConfig struct {
	Dial           time.Duration
	TLS            time.Duration
	ResponseHeader time.Duration
	KeepAlive      time.Duration
	IdleConn       time.Duration
}

func calculateOptimalIdleConnsPerHost(maxIdleConns, maxConnsPerHost int) int {
	if maxConnsPerHost <= 0 {
		if maxIdleConns/2 < 10 {
			return maxIdleConns / 2
		}
		return 10
	}
	if maxConnsPerHost/2 < 10 {
		return maxConnsPerHost / 2
	}
	return 10
}

func calculateOptimalConcurrency(maxConnsPerHost, maxIdleConns int) int {
	var concurrent int
	if maxConnsPerHost > 0 {
		concurrent = maxConnsPerHost * 2
	} else {
		concurrent = maxIdleConns
	}

	if concurrent > 500 {
		return 500
	}
	if concurrent < 100 {
		return 100
	}
	return concurrent
}

func calculateOptimalTimeouts(overallTimeout time.Duration) timeoutConfig {
	return timeoutConfig{
		Dial:           10 * time.Second,
		TLS:            10 * time.Second,
		ResponseHeader: 30 * time.Second,
		KeepAlive:      30 * time.Second,
		IdleConn:       90 * time.Second,
	}
}

func calculateMaxRetryDelay(baseDelay time.Duration, backoffFactor float64) time.Duration {
	if baseDelay <= 0 {
		return 5 * time.Second
	}
	maxDelay := baseDelay * time.Duration(backoffFactor*3)
	if maxDelay > 30*time.Second {
		return 30 * time.Second
	}
	return maxDelay
}

func convertRequestOptions(options []RequestOption) []engine.RequestOption {
	if len(options) == 0 {
		return nil
	}

	engineOptions := make([]engine.RequestOption, 0, len(options))
	for _, opt := range options {
		if opt == nil {
			continue
		}
		option := opt
		engineOptions = append(engineOptions, func(req *engine.Request) {
			publicReq := &Request{
				Method:      req.Method,
				URL:         req.URL,
				Headers:     req.Headers,
				QueryParams: req.QueryParams,
				Body:        req.Body,
				Context:     req.Context,
				Timeout:     req.Timeout,
				MaxRetries:  req.MaxRetries,
				Cookies:     req.Cookies, // Preserve existing cookies
			}

			option(publicReq)

			req.Method = publicReq.Method
			req.URL = publicReq.URL
			req.Headers = publicReq.Headers
			req.QueryParams = publicReq.QueryParams
			req.Body = publicReq.Body
			req.Context = publicReq.Context
			req.Timeout = publicReq.Timeout
			req.MaxRetries = publicReq.MaxRetries
			req.Cookies = publicReq.Cookies
		})
	}

	return engineOptions
}

func convertEngineResponse(engineResp *engine.Response) *Response {
	if engineResp == nil {
		return nil
	}

	return &Response{
		StatusCode:    engineResp.StatusCode,
		Status:        engineResp.Status,
		Headers:       engineResp.Headers,
		Body:          engineResp.Body,
		RawBody:       engineResp.RawBody,
		ContentLength: engineResp.ContentLength,
		Duration:      engineResp.Duration,
		Attempts:      engineResp.Attempts,
		Cookies:       engineResp.Cookies,
	}
}

// createCookieJar creates a cookie jar if cookies are enabled
func createCookieJar(enableCookies bool) any {
	if !enableCookies {
		return nil
	}

	jar, err := NewCookieJar()
	if err != nil {
		return nil
	}

	return jar
}
