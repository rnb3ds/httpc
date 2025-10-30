package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// Helper functions for min/max operations
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
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
	cfg := DefaultConfig()

	// Enforce strict security settings
	cfg.InsecureSkipVerify = false
	cfg.AllowPrivateIPs = false
	cfg.MaxResponseBodySize = 10 * 1024 * 1024 // Reduce to 10MB for security
	cfg.MaxRetries = 1                         // Reduce retries to prevent abuse
	cfg.Timeout = 30 * time.Second             // Shorter timeout
	cfg.FollowRedirects = false                // Disable redirects for security

	return New(cfg)
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
	defaultClient Client
	defaultOnce   sync.Once
	defaultErr    error
	defaultMu     sync.RWMutex
)

// getDefaultClient returns the default client, creating it if necessary
func getDefaultClient() (Client, error) {
	defaultMu.RLock()
	if defaultClient != nil {
		client := defaultClient
		err := defaultErr
		defaultMu.RUnlock()
		return client, err
	}
	defaultMu.RUnlock()

	// Use a separate mutex for initialization to avoid deadlock
	defaultMu.Lock()
	defer defaultMu.Unlock()

	// Double-check pattern
	if defaultClient != nil {
		return defaultClient, defaultErr
	}

	// Create new client with error handling
	client, err := New()
	if err != nil {
		defaultErr = err
		return nil, err
	}

	defaultClient = client
	defaultErr = nil
	return defaultClient, nil
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

// SetDefaultClient sets the default client used by package-level functions
func SetDefaultClient(client Client) {
	if client == nil {
		return
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()

	// Close previous client safely
	if defaultClient != nil {
		if err := defaultClient.Close(); err != nil {
			// Log error but don't fail the operation
			// In production, you might want to use a proper logger here
		}
	}

	defaultClient = client
	defaultErr = nil // Reset error state
}

func convertToEngineConfig(cfg *Config) *engine.Config {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Optimized connection pool calculations
	maxIdleConnsPerHost := calculateOptimalIdleConnsPerHost(cfg.MaxIdleConns, cfg.MaxConnsPerHost)

	// Intelligent concurrent request limits based on system resources and configuration
	maxConcurrent := calculateOptimalConcurrency(cfg.MaxConnsPerHost, cfg.MaxIdleConns)

	// Adaptive timeout settings based on configuration
	timeouts := calculateOptimalTimeouts(cfg.Timeout)

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
		MinTLSVersion:         tls.VersionTLS12, // Force minimum TLS 1.2
		MaxTLSVersion:         tls.VersionTLS13,
		InsecureSkipVerify:    cfg.InsecureSkipVerify,
		MaxResponseBodySize:   cfg.MaxResponseBodySize,
		MaxConcurrentRequests: maxConcurrent,
		ValidateURL:           true, // Force enable URL validation
		ValidateHeaders:       true, // Force enable header validation
		AllowPrivateIPs:       cfg.AllowPrivateIPs,
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

// calculateOptimalIdleConnsPerHost calculates the optimal idle connections per host
func calculateOptimalIdleConnsPerHost(maxIdleConns, maxConnsPerHost int) int {
	if maxConnsPerHost <= 0 {
		// Default calculation based on total idle connections
		result := max(2, min(20, maxIdleConns/10))
		return result
	}

	// Base it on MaxConnsPerHost but ensure reasonable limits
	result := max(2, min(maxConnsPerHost/2, 50))

	// Ensure it doesn't exceed total idle connections
	if maxIdleConns > 0 {
		result = min(result, maxIdleConns/5) // Allow up to 5 hosts to use all idle connections
		if result < 2 {
			result = 2
		}
	}

	return result
}

// calculateOptimalConcurrency calculates the optimal concurrent request limit
func calculateOptimalConcurrency(maxConnsPerHost, maxIdleConns int) int {
	baseLimit := 500

	if maxConnsPerHost > 0 {
		// Scale based on connection limits, but be more aggressive for performance
		hostBasedLimit := maxConnsPerHost * 10
		baseLimit = max(100, min(2000, hostBasedLimit))
	}

	if maxIdleConns > 0 {
		// Also consider total connection capacity
		idleBasedLimit := maxIdleConns * 2
		baseLimit = max(baseLimit, min(2000, idleBasedLimit))
	}

	return baseLimit
}

// calculateOptimalTimeouts calculates optimal timeout values based on overall timeout
func calculateOptimalTimeouts(overallTimeout time.Duration) TimeoutConfig {
	config := TimeoutConfig{
		Dial:           15 * time.Second,
		TLS:            15 * time.Second,
		ResponseHeader: 30 * time.Second,
		KeepAlive:      30 * time.Second,
		IdleConn:       90 * time.Second,
	}

	// If overall timeout is specified and reasonable, scale component timeouts
	if overallTimeout > 0 && overallTimeout < 5*time.Minute {
		if overallTimeout < 30*time.Second {
			// For short timeouts, scale proportionally but maintain minimums
			ratio := float64(overallTimeout) / float64(30*time.Second)

			config.Dial = maxDuration(2*time.Second, time.Duration(float64(config.Dial)*ratio))
			config.TLS = maxDuration(3*time.Second, time.Duration(float64(config.TLS)*ratio))
			config.ResponseHeader = maxDuration(5*time.Second, time.Duration(float64(config.ResponseHeader)*ratio))
		} else {
			// For longer timeouts, use more generous values
			config.Dial = minDuration(30*time.Second, overallTimeout/4)
			config.TLS = minDuration(30*time.Second, overallTimeout/4)
			config.ResponseHeader = minDuration(60*time.Second, overallTimeout/2)
		}
	}

	return config
}

// calculateMaxRetryDelay calculates the maximum retry delay to prevent excessive waits
func calculateMaxRetryDelay(baseDelay time.Duration, backoffFactor float64) time.Duration {
	if baseDelay <= 0 {
		return 5 * time.Second // Reduced default
	}

	// For very high backoff factors, be more conservative
	if backoffFactor >= 5.0 {
		// Cap at a much lower value for high backoff factors
		return 2 * time.Second
	}

	// Calculate what the delay would be after 3 iterations (reasonable for most cases)
	maxDelay := time.Duration(float64(baseDelay) * backoffFactor * backoffFactor * backoffFactor)

	// Cap at reasonable limits based on base delay
	maxLimit := 10 * time.Second
	if baseDelay > 1*time.Second {
		maxLimit = 30 * time.Second
	}

	if maxDelay > maxLimit {
		maxDelay = maxLimit
	}
	if maxDelay < baseDelay {
		maxDelay = baseDelay
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
