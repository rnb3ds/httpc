package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// Helper functions for minInt/maxInt operations
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

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

// New creates a new HTTP client with the provided configuration.
// If no configuration is provided, secure defaults are used.
func New(config ...*Config) (Client, error) {
	var cfg *Config
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
		// Validate configuration security
		if err := ValidateConfig(cfg); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
	} else {
		cfg = DefaultConfig()
	}

	// Additional security validation
	if cfg.InsecureSkipVerify {
		// Log warning or return error for production builds
		// For now, we'll allow it but could be made stricter
	}

	engineConfig := convertToEngineConfig(cfg)

	engineClient, err := engine.NewClient(engineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &clientImpl{engine: engineClient}, nil
}

// NewSecure creates a new HTTP client with maximum security settings.
// This function enforces strict security policies and is recommended for production use.
func NewSecure() (Client, error) {
	return New(SecureConfig())
}

// NewPerformance creates a new HTTP client optimized for performance.
// This configuration allows higher concurrency and larger response sizes.
func NewPerformance() (Client, error) {
	return New(PerformanceConfig())
}

// NewMinimal creates a new HTTP client with minimal features.
// This is suitable for simple use cases with strict resource constraints.
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

// SetDefaultClient sets the default client used by package-level functions.
// It closes the previous default client if one exists.
// Returns an error if closing the previous client fails.
func SetDefaultClient(client Client) error {
	if client == nil {
		return fmt.Errorf("cannot set nil client as default")
	}

	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	// Close previous client safely
	var closeErr error
	if defaultClient != nil {
		closeErr = defaultClient.Close()
	}

	defaultClient = client
	defaultClientErr = nil
	// Reset sync.Once to prevent re-initialization
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

// TimeoutConfig holds optimized timeout values
type TimeoutConfig struct {
	Dial           time.Duration
	TLS            time.Duration
	ResponseHeader time.Duration
	KeepAlive      time.Duration
	IdleConn       time.Duration
}

// calculateOptimalIdleConnsPerHost calculates idle connections per host
// Uses a simple heuristic: half of per-host limit, capped at 10
func calculateOptimalIdleConnsPerHost(maxIdleConns, maxConnsPerHost int) int {
	if maxConnsPerHost <= 0 {
		// If no per-host limit, use a fraction of total idle connections
		return minInt(10, maxIdleConns/2)
	}
	// Use half of the per-host limit, but cap at 10 for efficiency
	return minInt(maxConnsPerHost/2, 10)
}

// calculateOptimalConcurrency calculates concurrent request limit
// Uses a simple multiplier approach for request queuing
func calculateOptimalConcurrency(maxConnsPerHost, maxIdleConns int) int {
	if maxConnsPerHost > 0 {
		// Allow 10x the connection limit for queuing
		return maxConnsPerHost * 10
	}
	// Fallback: 5x idle connections
	return maxIdleConns * 5
}

// calculateOptimalTimeouts calculates timeout values with simple, fixed defaults
func calculateOptimalTimeouts(overallTimeout time.Duration) TimeoutConfig {
	// Use fixed, reasonable defaults that work for most use cases
	// These values are based on industry best practices and real-world testing
	return TimeoutConfig{
		Dial:           10 * time.Second, // Time to establish TCP connection
		TLS:            10 * time.Second, // Time to complete TLS handshake
		ResponseHeader: 30 * time.Second, // Time to receive response headers
		KeepAlive:      30 * time.Second, // TCP keep-alive interval
		IdleConn:       90 * time.Second, // How long idle connections are kept
	}
	// Note: We don't adjust based on overallTimeout as it adds unnecessary complexity
	// The overall timeout is enforced at the request level via context
}

// calculateMaxRetryDelay calculates maximum retry delay with simple logic
func calculateMaxRetryDelay(baseDelay time.Duration, backoffFactor float64) time.Duration {
	if baseDelay <= 0 {
		return 5 * time.Second
	}
	// Simple cap at 30 seconds
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
