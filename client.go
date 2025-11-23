package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
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
	defaultClient   atomic.Pointer[clientImpl]
	defaultClientMu sync.Mutex
)

// getDefaultClient returns the default client, creating it if necessary
func getDefaultClient() (Client, error) {
	// Fast path: check if client already exists
	if client := defaultClient.Load(); client != nil {
		return client, nil
	}

	// Slow path: acquire lock and initialize
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	// Double-check after acquiring lock
	if client := defaultClient.Load(); client != nil {
		return client, nil
	}

	// Create new client
	newClient, err := New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize default client: %w", err)
	}

	impl, ok := newClient.(*clientImpl)
	if !ok {
		return nil, fmt.Errorf("unexpected client type")
	}

	defaultClient.Store(impl)
	return impl, nil
}

// CloseDefaultClient closes the default client and resets it.
// After calling this, the next package-level function call will create a new client.
func CloseDefaultClient() error {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	client := defaultClient.Load()
	if client == nil {
		return nil
	}

	err := client.Close()
	defaultClient.Store(nil)
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

	impl, ok := client.(*clientImpl)
	if !ok {
		return fmt.Errorf("client must be created with httpc.New()")
	}

	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	var closeErr error
	if oldClient := defaultClient.Load(); oldClient != nil {
		closeErr = oldClient.Close()
	}

	defaultClient.Store(impl)
	return closeErr
}

func convertToEngineConfig(cfg *Config) *engine.Config {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	maxIdleConnsPerHost := calculateOptimalIdleConnsPerHost(cfg.MaxIdleConns, cfg.MaxConnsPerHost)

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
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		ProxyURL:              cfg.ProxyURL,
		TLSConfig:             cfg.TLSConfig,
		MinTLSVersion:       minTLSVersion,
		MaxTLSVersion:       maxTLSVersion,
		InsecureSkipVerify:  cfg.InsecureSkipVerify,
		MaxResponseBodySize: cfg.MaxResponseBodySize,
		ValidateURL:         true, // Force enable URL validation
		ValidateHeaders:     true, // Force enable header validation
		AllowPrivateIPs:     cfg.AllowPrivateIPs,
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



func calculateOptimalIdleConnsPerHost(maxIdleConns, maxConnsPerHost int) int {
	if maxConnsPerHost > 0 {
		result := maxConnsPerHost / 2
		if result < 2 {
			return 2
		}
		if result > 10 {
			return 10
		}
		return result
	}
	
	result := maxIdleConns / 2
	if result < 2 {
		return 2
	}
	if result > 10 {
		return 10
	}
	return result
}

func calculateMaxRetryDelay(baseDelay time.Duration, backoffFactor float64) time.Duration {
	if baseDelay <= 0 {
		return 5 * time.Second
	}
	maxDelay := time.Duration(float64(baseDelay) * backoffFactor * 3)
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
		currentOpt := opt
		engineOptions = append(engineOptions, func(req *engine.Request) error {
			// Convert engine.Request to public Request
			publicReq := &Request{
				Method:      req.Method,
				URL:         req.URL,
				Headers:     req.Headers,
				QueryParams: req.QueryParams,
				Body:        req.Body,
				Context:     req.Context,
				Timeout:     req.Timeout,
				MaxRetries:  req.MaxRetries,
				Cookies:     req.Cookies,
			}
			
			// Apply the option
			if err := currentOpt(publicReq); err != nil {
				return err
			}
			
			// Copy back the modified values
			req.Method = publicReq.Method
			req.URL = publicReq.URL
			req.Headers = publicReq.Headers
			req.QueryParams = publicReq.QueryParams
			req.Body = publicReq.Body
			req.Context = publicReq.Context
			req.Timeout = publicReq.Timeout
			req.MaxRetries = publicReq.MaxRetries
			req.Cookies = publicReq.Cookies
			
			return nil
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
