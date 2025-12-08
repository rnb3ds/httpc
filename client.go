package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// Client represents the HTTP client interface.
// All methods are safe for concurrent use by multiple goroutines.
type Client interface {
	Get(url string, options ...RequestOption) (*Result, error)
	Post(url string, options ...RequestOption) (*Result, error)
	Put(url string, options ...RequestOption) (*Result, error)
	Patch(url string, options ...RequestOption) (*Result, error)
	Delete(url string, options ...RequestOption) (*Result, error)
	Head(url string, options ...RequestOption) (*Result, error)
	Options(url string, options ...RequestOption) (*Result, error)

	Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error)

	DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error)
	DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error)

	Close() error
}

// clientImpl implements the Client interface using the engine
type clientImpl struct {
	engine *engine.Client
}

func New(config ...*Config) (Client, error) {
	var cfg *Config
	if len(config) > 0 && config[0] != nil {
		if err := ValidateConfig(config[0]); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
		cfg = deepCopyConfig(config[0])
	} else {
		cfg = DefaultConfig()
	}

	engineConfig, err := convertToEngineConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert configuration: %w", err)
	}

	engineClient, err := engine.NewClient(engineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &clientImpl{engine: engineClient}, nil
}

func deepCopyConfig(src *Config) *Config {
	dst := *src

	if src.Headers != nil {
		dst.Headers = make(map[string]string, len(src.Headers))
		maps.Copy(dst.Headers, src.Headers)
	}

	// TLSConfig is already immutable after client creation per guidelines
	// No need to clone - just reference the same config
	// This saves significant memory and CPU on client creation

	return &dst
}

// NewSecure creates a new client with security-focused configuration.
// This preset prioritizes security over performance with strict validation,
// minimal retries, and conservative connection limits.
func NewSecure() (Client, error) {
	return New(SecureConfig())
}

// NewPerformance creates a new client optimized for high-throughput scenarios.
// This preset uses aggressive connection pooling, longer timeouts, and
// faster retry intervals for maximum performance.
func NewPerformance() (Client, error) {
	return New(PerformanceConfig())
}

// NewMinimal creates a new client with minimal features and lightweight configuration.
// This preset is ideal for simple, one-off requests where you don't need
// retries or advanced features.
func NewMinimal() (Client, error) {
	return New(MinimalConfig())
}

func (c *clientImpl) Get(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("GET", url, options)
}

func (c *clientImpl) Post(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("POST", url, options)
}

func (c *clientImpl) Put(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("PUT", url, options)
}

func (c *clientImpl) Patch(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("PATCH", url, options)
}

func (c *clientImpl) Delete(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("DELETE", url, options)
}

func (c *clientImpl) Head(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("HEAD", url, options)
}

func (c *clientImpl) Options(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("OPTIONS", url, options)
}

func (c *clientImpl) doRequest(method, url string, options []RequestOption) (*Result, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Request(context.Background(), method, url, engineOptions...)
	if err != nil {
		return nil, err
	}
	return convertEngineResponseToResult(resp), nil
}

func (c *clientImpl) Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Request(ctx, method, url, engineOptions...)
	if err != nil {
		return nil, err
	}
	return convertEngineResponseToResult(resp), nil
}

func (c *clientImpl) Close() error {
	return c.engine.Close()
}

var (
	defaultClient   atomic.Pointer[clientImpl]
	defaultClientMu sync.Mutex
	defaultOnce     sync.Once
	defaultInitErr  error
)

func getDefaultClient() (Client, error) {
	defaultOnce.Do(func() {
		newClient, err := New()
		if err != nil {
			defaultInitErr = fmt.Errorf("failed to initialize default client: %w", err)
			return
		}

		impl, ok := newClient.(*clientImpl)
		if !ok {
			defaultInitErr = fmt.Errorf("unexpected client type")
			return
		}

		defaultClient.Store(impl)
	})

	if defaultInitErr != nil {
		return nil, defaultInitErr
	}

	return defaultClient.Load(), nil
}

// CloseDefaultClient closes the default client and resets it.
// After calling this, the next package-level function call will create a new client.
// This function is safe for concurrent use.
func CloseDefaultClient() error {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	client := defaultClient.Load()
	if client == nil {
		return nil
	}

	err := client.Close()
	defaultClient.Store(nil)
	defaultOnce = sync.Once{}
	defaultInitErr = nil

	return err
}

// Get executes a GET request using the default client and returns a Result.
func Get(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Get(url, options...)
}

// Post executes a POST request using the default client and returns a Result.
func Post(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Post(url, options...)
}

// Put executes a PUT request using the default client and returns a Result.
func Put(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Put(url, options...)
}

// Patch executes a PATCH request using the default client and returns a Result.
func Patch(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Patch(url, options...)
}

// Delete executes a DELETE request using the default client and returns a Result.
func Delete(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Delete(url, options...)
}

// Head executes a HEAD request using the default client and returns a Result.
func Head(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Head(url, options...)
}

// Options executes an OPTIONS request using the default client and returns a Result.
func Options(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Options(url, options...)
}

// SetDefaultClient sets a custom client as the default for package-level functions.
// The previous default client is closed automatically.
// Returns an error if the client is nil or not created with httpc.New().
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
	defaultOnce = sync.Once{}
	defaultInitErr = nil
	return closeErr
}

const (
	minIdleConnsPerHost    = 2
	maxIdleConnsPerHostCap = 10
	defaultDialTimeout     = 10 * time.Second
	defaultKeepAlive       = 30 * time.Second
	defaultTLSHandshake    = 10 * time.Second
	defaultResponseHeader  = 30 * time.Second
	defaultIdleConnTimeout = 90 * time.Second
	defaultMaxRetryDelay   = 5 * time.Second
	absoluteMaxRetryDelay  = 30 * time.Second
	retryDelayMultiplier   = 3
)

func convertToEngineConfig(cfg *Config) (*engine.Config, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	idleConnsPerHost := cfg.MaxConnsPerHost / 2
	if idleConnsPerHost < minIdleConnsPerHost {
		idleConnsPerHost = minIdleConnsPerHost
	} else if idleConnsPerHost > maxIdleConnsPerHostCap {
		idleConnsPerHost = maxIdleConnsPerHostCap
	}

	minTLSVersion := cfg.MinTLSVersion
	if minTLSVersion == 0 {
		minTLSVersion = tls.VersionTLS12
	}

	maxTLSVersion := cfg.MaxTLSVersion
	if maxTLSVersion == 0 {
		maxTLSVersion = tls.VersionTLS13
	}

	maxRetryDelay := defaultMaxRetryDelay
	if cfg.RetryDelay > 0 && cfg.BackoffFactor > 0 {
		calculated := time.Duration(float64(cfg.RetryDelay) * cfg.BackoffFactor * retryDelayMultiplier)
		maxRetryDelay = min(calculated, absoluteMaxRetryDelay)
	}

	cookieJar, err := createCookieJar(cfg.EnableCookies)
	if err != nil {
		return nil, err
	}

	return &engine.Config{
		Timeout:               cfg.Timeout,
		DialTimeout:           defaultDialTimeout,
		KeepAlive:             defaultKeepAlive,
		TLSHandshakeTimeout:   defaultTLSHandshake,
		ResponseHeaderTimeout: defaultResponseHeader,
		IdleConnTimeout:       defaultIdleConnTimeout,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   idleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		ProxyURL:              cfg.ProxyURL,
		TLSConfig:             cfg.TLSConfig,
		MinTLSVersion:         minTLSVersion,
		MaxTLSVersion:         maxTLSVersion,
		InsecureSkipVerify:    cfg.InsecureSkipVerify,
		MaxResponseBodySize:   cfg.MaxResponseBodySize,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       cfg.AllowPrivateIPs,
		StrictContentLength:   cfg.StrictContentLength,
		MaxRetries:            cfg.MaxRetries,
		RetryDelay:            cfg.RetryDelay,
		MaxRetryDelay:         maxRetryDelay,
		BackoffFactor:         cfg.BackoffFactor,
		Jitter:                true,
		UserAgent:             cfg.UserAgent,
		Headers:               cfg.Headers,
		FollowRedirects:       cfg.FollowRedirects,
		MaxRedirects:          cfg.MaxRedirects,
		EnableHTTP2:           cfg.EnableHTTP2,
		CookieJar:             cookieJar,
		EnableCookies:         cfg.EnableCookies,
	}, nil
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
		engineOptions = append(engineOptions, func(req *engine.Request) error {
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

			if err := opt(publicReq); err != nil {
				return err
			}

			req.Method = publicReq.Method
			req.URL = publicReq.URL
			req.Headers = publicReq.Headers
			req.QueryParams = publicReq.QueryParams
			req.Body = publicReq.Body
			req.Context = publicReq.Context
			req.Timeout = publicReq.Timeout
			req.MaxRetries = publicReq.MaxRetries
			req.Cookies = publicReq.Cookies
			req.FollowRedirects = publicReq.FollowRedirects
			req.MaxRedirects = publicReq.MaxRedirects

			return nil
		})
	}
	return engineOptions
}

func convertEngineResponseToResult(engineResp *engine.Response) *Result {
	if engineResp == nil {
		return nil
	}

	requestCookies := extractRequestCookies(engineResp.RequestHeaders)

	return &Result{
		Request: &RequestInfo{
			Headers: engineResp.RequestHeaders,
			Cookies: requestCookies,
		},
		Response: &ResponseInfo{
			StatusCode:    engineResp.StatusCode,
			Status:        engineResp.Status,
			Headers:       engineResp.Headers,
			Body:          engineResp.Body,
			RawBody:       engineResp.RawBody,
			ContentLength: engineResp.ContentLength,
			Cookies:       engineResp.Cookies,
		},
		Meta: &RequestMeta{
			Duration:      engineResp.Duration,
			Attempts:      engineResp.Attempts,
			RedirectChain: engineResp.RedirectChain,
			RedirectCount: engineResp.RedirectCount,
		},
	}
}

func extractRequestCookies(headers http.Header) []*http.Cookie {
	if headers == nil {
		return nil
	}
	cookieHeader := headers.Get("Cookie")
	if cookieHeader == "" {
		return nil
	}
	return parseCookieHeader(cookieHeader)
}

func createCookieJar(enableCookies bool) (any, error) {
	if !enableCookies {
		return nil, nil
	}
	jar, err := NewCookieJar()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	return jar, nil
}
