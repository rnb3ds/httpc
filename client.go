package httpc

// Package httpc provides a high-performance HTTP client library with enterprise-grade
// security, zero external dependencies, and production-ready defaults.
//
// # Key Features
//
//   - Secure by default with TLS 1.2+, CRLF injection prevention, header validation
//   - High performance with connection pooling, HTTP/2, and goroutine-safe operations
//   - Built-in resilience with smart retry and exponential backoff
//   - Clean API with simplified request options
//
// # Quick Start
//
// Basic usage with package-level functions:
//
//	result, err := httpc.Get("https://api.example.com/data")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Body())
//
// # Client Creation
//
// Create a client with default configuration:
//
//	client, err := httpc.New()
//	defer client.Close()
//
// Create a client with custom configuration:
//
//	cfg := httpc.DefaultConfig()
//	cfg.Timeout = 60 * time.Second
//	cfg.MaxRetries = 5
//	client, err := httpc.New(cfg)
//
// Use preset configurations:
//
//	client, err := httpc.New(httpc.SecureConfig())      // Security-focused
//	client, err := httpc.New(httpc.PerformanceConfig()) // High-throughput
//	client, err := httpc.New(httpc.TestingConfig())     // Testing only!
//
// # SSRF Protection
//
// By default, AllowPrivateIPs is true for maximum compatibility with VPNs, proxies,
// and corporate networks. If your application makes requests to user-provided URLs,
// enable SSRF protection to block connections to private/reserved IP addresses:
//
//	// Enable SSRF protection
//	cfg := httpc.DefaultConfig()
//	cfg.AllowPrivateIPs = false
//	client, err := httpc.New(cfg)
//
//	// Or use the secure preset (has SSRF protection enabled)
//	client, err := httpc.New(httpc.SecureConfig())
//
// # Request Options
//
// Core options (18 functions):
//
//	// Headers
//	httpc.WithHeader("Authorization", "Bearer token")
//	httpc.WithHeaderMap(map[string]string{"X-Custom": "value"})
//	httpc.WithUserAgent("my-app/1.0")
//
//	// Body
//	httpc.WithJSON(data)
//	httpc.WithXML(data)
//	httpc.WithForm(map[string]string{"key": "value"})
//	httpc.WithFormData(multipartData)
//	httpc.WithFile("file", "document.pdf", fileBytes)
//	httpc.WithBody(rawData)
//	httpc.WithBinary(binaryData)
//
//	// Query parameters
//	httpc.WithQuery("page", 1)
//	httpc.WithQueryMap(map[string]any{"page": 1, "limit": 10})
//
//	// Authentication
//	httpc.WithBearerToken(token)
//	httpc.WithBasicAuth(username, password)
//
//	// Cookies
//	httpc.WithCookie(http.Cookie{Name: "session", Value: "abc"})
//	httpc.WithCookieString("session=abc; token=xyz")
//
//	// Request control
//	httpc.WithContext(ctx)
//	httpc.WithTimeout(30 * time.Second)
//	httpc.WithMaxRetries(3)
//	httpc.WithFollowRedirects(false)
//	httpc.WithMaxRedirects(5)
//
//	// Callbacks
//	httpc.WithOnRequest(callback)
//	httpc.WithOnResponse(callback)
//
// # DomainClient
//
// For session management across requests to the same domain:
//
//	dc, err := httpc.NewDomain("https://api.example.com")
//	defer dc.Close()
//
//	dc.SetHeader("Authorization", "Bearer "+token)
//
//	// Headers automatically included
//	result, err := dc.Request(ctx, "GET", "/users")
//
// # Context Handling
//
// Use context for timeout and cancellation:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	result, err := client.Request(ctx, "GET", "https://api.example.com/data")
//
// # Migration
//
// For migration from older versions, see MIGRATION.md.
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
	DownloadWithOptions(url string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error)

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

// New creates a new HTTP client with the given configuration.
// If no configuration is provided or nil is passed, DefaultConfig() is used.
//
// Examples:
//
//	// Use default configuration
//	client, err := httpc.New()
//
//	// Use default configuration (explicit nil)
//	client, err := httpc.New(nil)
//
//	// Use custom configuration
//	cfg := httpc.DefaultConfig()
//	cfg.Timeout = 60 * time.Second
//	client, err := httpc.New(cfg)
//
//	// Use preset configuration
//	client, err := httpc.New(httpc.SecureConfig())
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
		hasMiddlewares: len(cfg.Middlewares) > 0,
	}

	// Build middleware chain if middlewares are configured
	if client.hasMiddlewares {
		client.middlewareChain = client.buildMiddlewareChain(cfg.Middlewares)
	}

	return client, nil
}

// deepCopyConfig creates a deep copy of the configuration to prevent
// accidental mutation of shared config state. This is called internally
// when creating a new client to ensure each client has its own
// independent configuration.
func deepCopyConfig(src *Config) *Config {
	dst := *src

	// Deep copy headers
	if src.Headers != nil {
		dst.Headers = make(map[string]string, len(src.Headers))
		maps.Copy(dst.Headers, src.Headers)
	}

	// Deep copy middlewares slice
	if len(src.Middlewares) > 0 {
		dst.Middlewares = make([]MiddlewareFunc, len(src.Middlewares))
		copy(dst.Middlewares, src.Middlewares)
	}

	// Clone TLS config if present
	if src.TLSConfig != nil {
		dst.TLSConfig = src.TLSConfig.Clone()
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
// It delegates to Request with a background context for convenience methods.
func (c *clientImpl) doRequest(method, url string, options []RequestOption) (*Result, error) {
	return c.Request(context.Background(), method, url, options...)
}

// Request executes an HTTP request with the given context, method, URL, and options.
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
	// engine.Close() handles all resource cleanup including connection pool and transport
	// If it fails, we wrap the error for better context
	if err := c.engine.Close(); err != nil {
		return fmt.Errorf("failed to close client: %w", err)
	}
	return nil
}

var (
	defaultClient atomic.Pointer[clientImpl]
)

func getDefaultClient() (Client, error) {
	// Fast path: check if already initialized (lock-free)
	if client := defaultClient.Load(); client != nil {
		return client, nil
	}

	// Slow path: initialize with CAS to avoid allocation races
	// Create new client outside of any lock
	newClient, err := New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize default client: %w", err)
	}

	impl, ok := newClient.(*clientImpl)
	if !ok {
		return nil, fmt.Errorf("unexpected client type")
	}

	// CAS: only store if not already set by another goroutine
	// If CAS fails, another goroutine won the race - use their client
	if !defaultClient.CompareAndSwap(nil, impl) {
		// Another goroutine initialized first, close ours and use theirs
		_ = impl.Close() // best-effort cleanup
		return defaultClient.Load(), nil
	}
	return impl, nil
}

// CloseDefaultClient closes the default client and resets it.
// After calling this, the next package-level function call will create a new client.
// This function is safe for concurrent use.
func CloseDefaultClient() error {
	// Atomically swap out the client and close it
	for {
		client := defaultClient.Load()
		if client == nil {
			return nil
		}
		// Try to atomically clear the pointer
		if defaultClient.CompareAndSwap(client, nil) {
			return client.Close()
		}
		// CAS failed - another goroutine modified it, retry
	}
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

	// Atomically swap the old client with the new one
	var closeErr error
	for {
		oldClient := defaultClient.Load()
		// Try to atomically swap the pointer
		if defaultClient.CompareAndSwap(oldClient, impl) {
			// Successfully swapped - close the old client
			if oldClient != nil {
				closeErr = oldClient.Close()
			}
			break
		}
		// CAS failed - another goroutine modified it, retry
	}
	return closeErr
}

const (
	minIdleConnsPerHost    = 2  // Minimum idle connections per host
	maxIdleConnsPerHostCap = 10 // Maximum cap for idle connections per host
	retryDelayMultiplier   = 3  // Retry delay multiplication factor
)

// calculateIdleConnsPerHost calculates the optimal number of idle connections per host
// based on MaxConnsPerHost configuration.
func calculateIdleConnsPerHost(maxConnsPerHost int) int {
	if maxConnsPerHost == 0 {
		// Unlimited max connections - use reasonable default for idle
		return maxIdleConnsPerHostCap
	}
	idleConns := maxConnsPerHost / 2
	if idleConns < minIdleConnsPerHost {
		return minIdleConnsPerHost
	}
	if idleConns > maxIdleConnsPerHostCap {
		return maxIdleConnsPerHostCap
	}
	return idleConns
}

// resolveTLSVersions returns the minimum and maximum TLS versions from config.
// Falls back to TLS 1.2 and TLS 1.3 if not specified.
func resolveTLSVersions(cfg *Config) (min, max uint16) {
	min = cfg.MinTLSVersion
	if min == 0 {
		min = tls.VersionTLS12
	}
	max = cfg.MaxTLSVersion
	if max == 0 {
		max = tls.VersionTLS13
	}
	return min, max
}

// calculateMaxRetryDelay calculates the maximum retry delay based on configuration.
// Formula: min(RetryDelay * BackoffFactor * 3, 30s)
func calculateMaxRetryDelay(cfg *Config) time.Duration {
	const (
		defaultMaxRetryDelay  = 5 * time.Second
		absoluteMaxRetryDelay = 30 * time.Second
	)

	if cfg.RetryDelay <= 0 || cfg.BackoffFactor <= 0 {
		return defaultMaxRetryDelay
	}

	calculated := time.Duration(float64(cfg.RetryDelay) * cfg.BackoffFactor * retryDelayMultiplier)
	if calculated > absoluteMaxRetryDelay {
		return absoluteMaxRetryDelay
	}
	if calculated < defaultMaxRetryDelay {
		return defaultMaxRetryDelay
	}
	return calculated
}

// convertToEngineConfig converts public Config to engine Config.
// It uses helper functions for cleaner separation of concerns.
func convertToEngineConfig(cfg *Config) (*engine.Config, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	idleConnsPerHost := calculateIdleConnsPerHost(cfg.MaxConnsPerHost)
	minTLSVersion, maxTLSVersion := resolveTLSVersions(cfg)
	maxRetryDelay := calculateMaxRetryDelay(cfg)

	cookieJar, err := createCookieJar(cfg.EnableCookies)
	if err != nil {
		return nil, err
	}

	return &engine.Config{
		Timeout:               cfg.Timeout,
		DialTimeout:           cfg.DialTimeout,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		IdleConnTimeout:       cfg.IdleConnTimeout,
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
		ValidateURL:           cfg.ValidateURL,
		ValidateHeaders:       cfg.ValidateHeaders,
		AllowPrivateIPs:       cfg.AllowPrivateIPs,
		StrictContentLength:   cfg.StrictContentLength,
		MaxRetries:            cfg.MaxRetries,
		RetryDelay:            cfg.RetryDelay,
		MaxRetryDelay:         maxRetryDelay,
		BackoffFactor:         cfg.BackoffFactor,
		Jitter:                cfg.EnableJitter,
		CustomRetryPolicy:     cfg.CustomRetryPolicy,
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

// resultPool reduces heap allocations for Result objects.
// Each Result contains RequestInfo, ResponseInfo, and RequestMeta which are
// frequently allocated in the hot path.
var resultPool = sync.Pool{
	New: func() any {
		return &Result{
			Request:  &RequestInfo{},
			Response: &ResponseInfo{},
			Meta:     &RequestMeta{},
		}
	},
}

// getResult retrieves a Result from the pool and resets its fields.
func getResult() *Result {
	r, ok := resultPool.Get().(*Result)
	if !ok || r == nil {
		return &Result{
			Request:  &RequestInfo{},
			Response: &ResponseInfo{},
			Meta:     &RequestMeta{},
		}
	}
	// Reset all fields to zero values
	*r.Request = RequestInfo{}
	*r.Response = ResponseInfo{}
	*r.Meta = RequestMeta{}
	return r
}

// ReleaseResult returns a Result to the pool for reuse.
// Call this when you're done with the Result to reduce garbage collection pressure.
// WARNING: Do not use the Result after calling ReleaseResult.
func ReleaseResult(r *Result) {
	if r == nil {
		return
	}
	// Clear all fields to prevent data leakage and ensure clean state for reuse
	// Request fields
	r.Request.URL = ""
	r.Request.Method = ""
	r.Request.Headers = nil
	r.Request.Cookies = nil

	// Response fields - clear all including sensitive data
	r.Response.StatusCode = 0
	r.Response.Status = ""
	r.Response.Proto = ""
	r.Response.Headers = nil
	r.Response.Body = ""
	r.Response.RawBody = nil
	r.Response.ContentLength = 0
	r.Response.Cookies = nil

	// Meta fields
	r.Meta.Duration = 0
	r.Meta.Attempts = 0
	r.Meta.RedirectChain = nil
	r.Meta.RedirectCount = 0

	resultPool.Put(r)
}

func convertResponseToResult(resp ResponseMutator) *Result {
	if resp == nil {
		return nil
	}

	requestCookies := extractRequestCookies(resp.RequestHeaders())

	// Use pooled Result object
	result := getResult()
	result.Request.URL = resp.RequestURL()
	result.Request.Method = resp.RequestMethod()
	result.Request.Headers = resp.RequestHeaders()
	result.Request.Cookies = requestCookies
	result.Response.StatusCode = resp.StatusCode()
	result.Response.Status = resp.Status()
	result.Response.Proto = resp.Proto()
	result.Response.Headers = resp.Headers()
	result.Response.Body = resp.Body()
	result.Response.RawBody = resp.RawBody()
	result.Response.ContentLength = resp.ContentLength()
	result.Response.Cookies = resp.Cookies()
	result.Meta.Duration = resp.Duration()
	result.Meta.Attempts = resp.Attempts()
	result.Meta.RedirectChain = resp.RedirectChain()
	result.Meta.RedirectCount = resp.RedirectCount()

	return result
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
