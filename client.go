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

// Doer is a minimal interface for executing HTTP requests.
// Users who need custom implementations can implement this smaller interface
// instead of the full Client interface.
type Doer interface {
	// Request executes an HTTP request with the given method and URL.
	Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error)
}

// Client is the main interface for making HTTP requests.
// It extends Doer with convenience methods and lifecycle management.
type Client interface {
	Doer

	// Convenience methods for common HTTP verbs
	Get(url string, options ...RequestOption) (*Result, error)
	Post(url string, options ...RequestOption) (*Result, error)
	Put(url string, options ...RequestOption) (*Result, error)
	Patch(url string, options ...RequestOption) (*Result, error)
	Delete(url string, options ...RequestOption) (*Result, error)
	Head(url string, options ...RequestOption) (*Result, error)
	Options(url string, options ...RequestOption) (*Result, error)

	// File download methods
	DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error)
	DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error)

	// Close releases resources held by the client
	Close() error
}

// DomainClienter extends Client with domain-scoped operations.
// It provides session management for cookies and headers across requests
// to a specific domain.
type DomainClienter interface {
	Client

	// URL accessors
	URL() string
	Domain() string

	// Session header management
	SetHeader(key, value string) error
	SetHeaders(headers map[string]string) error
	DeleteHeader(key string)
	ClearHeaders()
	GetHeaders() map[string]string

	// Session cookie management
	SetCookie(cookie *http.Cookie) error
	SetCookies(cookies []*http.Cookie) error
	DeleteCookie(name string)
	ClearCookies()
	GetCookies() []*http.Cookie
	GetCookie(name string) *http.Cookie

	// Session access
	Session() *SessionManager
}

type clientImpl struct {
	engine          *engine.Client
	middlewareChain Handler
	hasMiddlewares  bool
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

	client := &clientImpl{
		engine:         engineClient,
		hasMiddlewares: len(cfg.Middleware.Middlewares) > 0,
	}

	// Build middleware chain if middlewares are configured
	if client.hasMiddlewares {
		client.middlewareChain = client.buildMiddlewareChain(cfg.Middleware.Middlewares)
	}

	return client, nil
}

// deepCopyConfig creates a deep copy of the configuration to prevent
// accidental mutation of shared config state. This is called internally
// when creating a new client to ensure each client has its own
// independent configuration.
func deepCopyConfig(src *Config) *Config {
	dst := *src

	// Deep copy middleware headers
	if src.Middleware.Headers != nil {
		dst.Middleware.Headers = make(map[string]string, len(src.Middleware.Headers))
		maps.Copy(dst.Middleware.Headers, src.Middleware.Headers)
	}

	// Deep copy middleware slice
	if len(src.Middleware.Middlewares) > 0 {
		dst.Middleware.Middlewares = make([]MiddlewareFunc, len(src.Middleware.Middlewares))
		copy(dst.Middleware.Middlewares, src.Middleware.Middlewares)
	}

	// Clone TLS config if present
	if src.Security.TLSConfig != nil {
		dst.Security.TLSConfig = src.Security.TLSConfig.Clone()
	}

	return &dst
}

// buildMiddlewareChain constructs a middleware chain from the provided middlewares.
// The final handler executes the actual HTTP request via the engine.
func (c *clientImpl) buildMiddlewareChain(middlewares []MiddlewareFunc) Handler {
	// Final handler that executes the actual request
	finalHandler := func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		// Use the context from the request (may have been modified by middleware)
		reqCtx := req.Context()
		if reqCtx == nil {
			reqCtx = ctx
		}

		// Execute the request via engine using interface methods
		resp, err := c.engine.Request(reqCtx, req.Method(), req.URL(),
			func(r *engine.Request) error {
				r.SetHeaders(req.Headers())
				r.SetQueryParams(req.QueryParams())
				r.SetBody(req.Body())
				r.SetTimeout(req.Timeout())
				r.SetMaxRetries(req.MaxRetries())
				r.SetCookies(req.Cookies())
				r.SetFollowRedirects(req.FollowRedirects())
				r.SetMaxRedirects(req.MaxRedirects())
				return nil
			})
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// Build the chain by wrapping middlewares in reverse order
	chain := finalHandler
	for i := len(middlewares) - 1; i >= 0; i-- {
		chain = middlewares[i](chain)
	}
	return chain
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

	// Use middleware chain if configured
	if c.hasMiddlewares {
		return c.executeWithMiddleware(ctx, method, url, options)
	}

	// Direct path when no middlewares (zero overhead)
	resp, err := c.engine.Request(ctx, method, url, options...)
	if err != nil {
		return nil, err
	}
	return convertResponseToResult(resp), nil
}

func (c *clientImpl) Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error) {
	// Use middleware chain if configured
	if c.hasMiddlewares {
		return c.executeWithMiddleware(ctx, method, url, options)
	}

	// Direct path when no middlewares (zero overhead)
	resp, err := c.engine.Request(ctx, method, url, options...)
	if err != nil {
		return nil, err
	}
	return convertResponseToResult(resp), nil
}

// executeWithMiddleware executes a request through the middleware chain.
func (c *clientImpl) executeWithMiddleware(ctx context.Context, method, url string, options []RequestOption) (*Result, error) {
	// Build engine request from options using setters
	engineReq := &engine.Request{}
	engineReq.SetMethod(method)
	engineReq.SetURL(url)
	engineReq.SetContext(ctx)

	// Apply request options directly (no conversion needed with unified types)
	for _, opt := range options {
		if opt != nil {
			if err := opt(engineReq); err != nil {
				return nil, fmt.Errorf("failed to apply request option: %w", err)
			}
		}
	}

	// Execute through middleware chain - engine.Request now implements RequestMutator
	respAccessor, err := c.middlewareChain(ctx, engineReq)
	if err != nil {
		return nil, err
	}

	return convertResponseToResult(respAccessor), nil
}

func (c *clientImpl) Close() error {
	return c.engine.Close()
}

var (
	defaultClient   atomic.Pointer[clientImpl]
	defaultClientMu sync.Mutex
)

func getDefaultClient() (Client, error) {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	if defaultClient.Load() != nil {
		return defaultClient.Load(), nil
	}

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

// Request executes an HTTP request with the given method using the default client.
// The context parameter allows for timeout and cancellation control.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	result, err := httpc.Request(ctx, "GET", "https://api.example.com/data")
func Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Request(ctx, method, url, options...)
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

	// Calculate idle connections per host with proper bounds
	idleConnsPerHost := cfg.Connections.MaxConnsPerHost / 2

	// Handle edge cases for MaxConnsPerHost
	// - If 0 (unlimited), use default idle connections
	// - If very small (< 2*minIdleConnsPerHost), use minimum
	// - Otherwise, use half of MaxConnsPerHost
	if cfg.Connections.MaxConnsPerHost == 0 {
		// Unlimited max connections - use reasonable default for idle
		idleConnsPerHost = maxIdleConnsPerHostCap
	} else if idleConnsPerHost < minIdleConnsPerHost {
		idleConnsPerHost = minIdleConnsPerHost
	} else if idleConnsPerHost > maxIdleConnsPerHostCap {
		idleConnsPerHost = maxIdleConnsPerHostCap
	}

	minTLSVersion := cfg.Security.MinTLSVersion
	if minTLSVersion == 0 {
		minTLSVersion = tls.VersionTLS12
	}

	maxTLSVersion := cfg.Security.MaxTLSVersion
	if maxTLSVersion == 0 {
		maxTLSVersion = tls.VersionTLS13
	}

	const (
		defaultMaxRetryDelay  = 5 * time.Second
		absoluteMaxRetryDelay = 30 * time.Second
	)
	maxRetryDelay := defaultMaxRetryDelay
	if cfg.Retry.Delay > 0 && cfg.Retry.BackoffFactor > 0 {
		calculated := time.Duration(float64(cfg.Retry.Delay) * cfg.Retry.BackoffFactor * retryDelayMultiplier)
		maxRetryDelay = min(calculated, absoluteMaxRetryDelay)
	}

	cookieJar, err := createCookieJar(cfg.Connections.EnableCookies)
	if err != nil {
		return nil, err
	}

	return &engine.Config{
		Timeout:               cfg.Timeouts.Request,
		DialTimeout:           cfg.Timeouts.Dial,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   cfg.Timeouts.TLSHandshake,
		ResponseHeaderTimeout: cfg.Timeouts.ResponseHeader,
		IdleConnTimeout:       cfg.Timeouts.IdleConn,
		MaxIdleConns:          cfg.Connections.MaxIdleConns,
		MaxIdleConnsPerHost:   idleConnsPerHost,
		MaxConnsPerHost:       cfg.Connections.MaxConnsPerHost,
		ProxyURL:              cfg.Connections.ProxyURL,
		EnableSystemProxy:     cfg.Connections.EnableSystemProxy,
		TLSConfig:             cfg.Security.TLSConfig,
		MinTLSVersion:         minTLSVersion,
		MaxTLSVersion:         maxTLSVersion,
		InsecureSkipVerify:    cfg.Security.InsecureSkipVerify,
		MaxResponseBodySize:   cfg.Security.MaxResponseBodySize,
		ValidateURL:           cfg.Security.ValidateURL,
		ValidateHeaders:       cfg.Security.ValidateHeaders,
		AllowPrivateIPs:       cfg.Security.AllowPrivateIPs,
		StrictContentLength:   cfg.Security.StrictContentLength,
		MaxRetries:            cfg.Retry.MaxRetries,
		RetryDelay:            cfg.Retry.Delay,
		MaxRetryDelay:         maxRetryDelay,
		BackoffFactor:         cfg.Retry.BackoffFactor,
		Jitter:                cfg.Retry.EnableJitter,
		CustomRetryPolicy:     cfg.Retry.CustomRetryPolicy,
		UserAgent:             cfg.Middleware.UserAgent,
		Headers:               cfg.Middleware.Headers,
		FollowRedirects:       cfg.Middleware.FollowRedirects,
		MaxRedirects:          cfg.Middleware.MaxRedirects,
		EnableHTTP2:           cfg.Connections.EnableHTTP2,
		CookieJar:             cookieJar,
		EnableCookies:         cfg.Connections.EnableCookies,
		EnableDoH:             cfg.Connections.EnableDoH,
		DoHCacheTTL:           cfg.Connections.DoHCacheTTL,
	}, nil
}

func convertResponseToResult(resp ResponseMutator) *Result {
	if resp == nil {
		return nil
	}

	requestCookies := extractRequestCookies(resp.RequestHeaders())

	return &Result{
		Request: &RequestInfo{
			Headers: resp.RequestHeaders(),
			Cookies: requestCookies,
		},
		Response: &ResponseInfo{
			StatusCode:    resp.StatusCode(),
			Status:        resp.Status(),
			Proto:         resp.Proto(),
			Headers:       resp.Headers(),
			Body:          resp.Body(),
			RawBody:       resp.RawBody(),
			ContentLength: resp.ContentLength(),
			Cookies:       resp.Cookies(),
		},
		Meta: &RequestMeta{
			Duration:      resp.Duration(),
			Attempts:      resp.Attempts(),
			RedirectChain: resp.RedirectChain(),
			RedirectCount: resp.RedirectCount(),
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

func createCookieJar(enableCookies bool) (http.CookieJar, error) {
	if !enableCookies {
		return nil, nil
	}
	jar, err := NewCookieJar()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	return jar, nil
}
