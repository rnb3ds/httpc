package httpc

// Package httpc provides a high-performance HTTP client library with enterprise-grade
// security, zero external dependencies, and production-ready defaults.
//
// Key Features:
//   - Secure by default with TLS 1.2+, SSRF protection, CRLF injection prevention
//   - High performance with connection pooling, HTTP/2, and goroutine-safe operations
//   - Built-in resilience with smart retry and exponential backoff
//   - Clean API with functional options and comprehensive error handling
//
// Basic Usage:
//
//	result, err := httpc.Get("https://api.example.com/data")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Body())
//
// Advanced Usage:
//
//	// Create a configured client
//	client, err := httpc.New(httpc.SecureConfig())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Make authenticated requests
//	result, err := client.Get("https://api.example.com/protected",
//	    httpc.WithBearerToken(token),
//	)
//
//	// Automatic state management with DomainClient
//	domainClient, err := httpc.NewDomain("https://api.example.com")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer domainClient.Close()
//
//	domainClient.SetHeader("Authorization", "Bearer "+token)
//	result, err = domainClient.Get("/profile")  // Header automatically included
//
// For more information, see https://github.com/cybergodev/httpc
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

// deepCopyConfig creates a deep copy of the configuration to prevent
// accidental mutation of shared config state. This is called internally
// when creating a new client to ensure each client has its own
// independent configuration.
func deepCopyConfig(src *Config) *Config {
	dst := *src

	if src.Headers != nil {
		dst.Headers = make(map[string]string, len(src.Headers))
		maps.Copy(dst.Headers, src.Headers)
	}

	return &dst
}

// NewSecure creates a new client with security-focused configuration.
func NewSecure() (Client, error) {
	return New(SecureConfig())
}

// NewPerformance creates a new client optimized for high-throughput scenarios.
func NewPerformance() (Client, error) {
	return New(PerformanceConfig())
}

// NewMinimal creates a new client with minimal features and lightweight configuration.
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

// doRequest executes an HTTP request with the given method and options.
func (c *clientImpl) doRequest(method, url string, options []RequestOption) (*Result, error) {
	ctx := context.Background()
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Request(ctx, method, url, engineOptions...)
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

	client := defaultClient.Load()
	if client == nil {
		return nil, fmt.Errorf("default client not initialized")
	}

	return client, nil
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

func Get(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Get(url, options...)
}

func Post(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Post(url, options...)
}

func Put(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Put(url, options...)
}

func Patch(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Patch(url, options...)
}

func Delete(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Delete(url, options...)
}

func Head(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Head(url, options...)
}

func Options(url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Options(url, options...)
}

// SetDefaultClient sets a custom client as the default for package-level functions.
// The previous default client is closed automatically.
// Only *clientImpl instances created by this package are supported.
func SetDefaultClient(client Client) error {
	if client == nil {
		return fmt.Errorf("cannot set nil client as default")
	}

	impl, ok := client.(*clientImpl)
	if !ok {
		return fmt.Errorf("only clients created by this package are supported")
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

	const (
		defaultMaxRetryDelay  = 5 * time.Second
		absoluteMaxRetryDelay = 30 * time.Second
	)
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
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   idleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		ProxyURL:              cfg.ProxyURL,
		EnableSystemProxy:     cfg.EnableSystemProxy,
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
		EnableDoH:             cfg.EnableDoH,
		DoHCacheTTL:           cfg.DoHCacheTTL,
	}, nil
}

// convertRequestOptions converts public RequestOptions to internal engine options.
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
			if publicReq.FollowRedirects != nil {
				req.FollowRedirects = publicReq.FollowRedirects
			}
			if publicReq.MaxRedirects != nil {
				req.MaxRedirects = publicReq.MaxRedirects
			}

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
