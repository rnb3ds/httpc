package httpc

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"github.com/cybergodev/httpc/internal/engine"
)

// backgroundCtx is a convenience alias for context.Background(), used as the
// default context in methods that don't accept one (e.g., Get, Post, doRequest).
// context.Background() returns the same immutable singleton on every call,
// so caching it in a package variable is purely stylistic.
// Note: internal/engine also defines its own backgroundCtx — this is intentional
// since unexported vars cannot be shared across packages.
var backgroundCtx = context.Background()

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
	DownloadFileWithContext(ctx context.Context, url string, filePath string, options ...RequestOption) (*DownloadResult, error)
	DownloadWithOptionsWithContext(ctx context.Context, url string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error)

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

// engineClient defines the interface for the internal engine.Client.
// This enables testing clientImpl without a real engine.Client.
type engineClient interface {
	Request(ctx context.Context, method, url string, opts ...engine.RequestOption) (*engine.Response, error)
	Close() error
	IsClosed() bool
}

// Compile-time check that engine.Client satisfies engineClient.
var _ engineClient = (*engine.Client)(nil)

type clientImpl struct {
	engine          engineClient
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
//	cfg.Timeouts.Request = 60 * time.Second
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
		cfg = config[0]
	} else {
		cfg = DefaultConfig()
	}

	engineConfig, err := convertToEngineConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert configuration: %w", err)
	}

	// Warn if InsecureSkipVerify is enabled outside test environment.
	if cfg.Security.InsecureSkipVerify && !isTestEnvironment() {
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING] InsecureSkipVerify is enabled - TLS certificate verification is DISABLED\n")
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING] This should only be used in testing. Use SecureConfig() for production.\n")
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
//
// Note: RetryConfig.CustomPolicy is NOT deep-copied. If the policy
// implementation contains mutable state, do not share the same Config
// instance across multiple clients concurrently.
func deepCopyConfig(src *Config) *Config {
	dst := *src

	// Deep copy middleware headers
	if src.Middleware.Headers != nil {
		dst.Middleware.Headers = make(map[string]string, len(src.Middleware.Headers))
		maps.Copy(dst.Middleware.Headers, src.Middleware.Headers)
	}

	// Deep copy middlewares slice
	if len(src.Middleware.Middlewares) > 0 {
		dst.Middleware.Middlewares = make([]MiddlewareFunc, len(src.Middleware.Middlewares))
		copy(dst.Middleware.Middlewares, src.Middleware.Middlewares)
	}

	// Deep copy redirect whitelist
	if len(src.Security.RedirectWhitelist) > 0 {
		dst.Security.RedirectWhitelist = make([]string, len(src.Security.RedirectWhitelist))
		copy(dst.Security.RedirectWhitelist, src.Security.RedirectWhitelist)
	}

	// Clone TLS config if present
	if src.Security.TLSConfig != nil {
		dst.Security.TLSConfig = src.Security.TLSConfig.Clone()
	}

	// Deep copy cookie security config if present
	if src.Security.CookieSecurity != nil {
		cookieSec := *src.Security.CookieSecurity
		dst.Security.CookieSecurity = &cookieSec
	}

	// Deep copy SSRF exempt CIDRs
	if len(src.Security.SSRFExemptCIDRs) > 0 {
		dst.Security.SSRFExemptCIDRs = make([]string, len(src.Security.SSRFExemptCIDRs))
		copy(dst.Security.SSRFExemptCIDRs, src.Security.SSRFExemptCIDRs)
	}

	return &dst
}

// buildMiddlewareChain constructs a middleware chain from the provided middlewares.
// The final handler copies the middleware-modified request fields into a fresh engine
// request and executes it. This avoids re-applying user options (double execution) and
// uses a single option closure to forward all mutable state including callbacks.
//
// Callbacks (OnRequest/OnResponse) are extracted before the chain runs and forwarded
// via closure, avoiding a direct dependency on the engine.Request concrete type.
func (c *clientImpl) buildMiddlewareChain(middlewares []MiddlewareFunc) Handler {
	finalHandler := func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		reqCtx := req.Context()
		if reqCtx == nil {
			reqCtx = ctx
		}

		// Extract callbacks from the concrete engine.Request before forwarding.
		// We capture them in the closure so the final handler doesn't need
		// a type assertion on each invocation — only once at chain entry.
		var onRequest func(*engine.Request) error
		var onResponse func(*engine.Response) error
		if engReq, ok := req.(*engine.Request); ok {
			if cb := engReq.OnRequest(); cb != nil {
				onRequest = cb
			}
			if cb := engReq.OnResponse(); cb != nil {
				onResponse = cb
			}
		}

		// Single option closure forwards all mutable fields from the middleware-modified request.
		resp, err := c.engine.Request(reqCtx, req.Method(), req.URL(),
			func(r *engine.Request) error {
				r.SetHeaders(req.Headers())
				r.SetQueryParams(req.QueryParams())
				r.SetBody(req.Body())
				r.SetTimeout(req.Timeout())
				r.SetMaxRetries(req.MaxRetries())
				r.SetCookies(req.Cookies())
				if fr := req.FollowRedirects(); fr != nil {
					r.SetFollowRedirects(fr)
				}
				if mr := req.MaxRedirects(); mr != nil {
					r.SetMaxRedirects(mr)
				}
				r.SetStreamBody(req.StreamBody())
				// Forward pre-extracted callbacks
				if onRequest != nil {
					r.SetOnRequest(onRequest)
				}
				if onResponse != nil {
					r.SetOnResponse(onResponse)
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	return Chain(middlewares...)(finalHandler)
}

// Get makes a GET request to the specified URL using the client's configuration.
func (c *clientImpl) Get(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("GET", url, options)
}

// Post makes a POST request to the specified URL using the client's configuration.
func (c *clientImpl) Post(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("POST", url, options)
}

// Put makes a PUT request to the specified URL using the client's configuration.
func (c *clientImpl) Put(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("PUT", url, options)
}

// Patch makes a PATCH request to the specified URL using the client's configuration.
func (c *clientImpl) Patch(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("PATCH", url, options)
}

// Delete makes a DELETE request to the specified URL using the client's configuration.
func (c *clientImpl) Delete(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("DELETE", url, options)
}

// Head makes a HEAD request to the specified URL using the client's configuration.
func (c *clientImpl) Head(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("HEAD", url, options)
}

// Options makes an OPTIONS request to the specified URL using the client's configuration.
func (c *clientImpl) Options(url string, options ...RequestOption) (*Result, error) {
	return c.doRequest("OPTIONS", url, options)
}

// doRequest executes an HTTP request with the given method and options.
// It delegates to Request with a background context for convenience methods.
func (c *clientImpl) doRequest(method, url string, options []RequestOption) (*Result, error) {
	return c.Request(backgroundCtx, method, url, options...)
}

// Request executes an HTTP request with the given context, method, URL, and options.
// The context parameter allows for timeout and cancellation control.
func (c *clientImpl) Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error) {
	resp, err := c.executeRequest(ctx, method, url, options)
	if err != nil {
		return nil, err
	}
	defer releaseResponseMutator(resp)
	return convertResponseToResult(resp), nil
}

// releaseResponseMutator safely releases a ResponseMutator back to the engine pool.
// If the response is an *engine.Response, it is returned via ReleaseResponse.
// Custom ResponseMutator implementations (e.g., from middleware wrapping) are not
// pooled — middleware authors are responsible for their own resource cleanup.
func releaseResponseMutator(resp ResponseMutator) {
	if resp == nil {
		return
	}
	if engineResp, ok := resp.(*engine.Response); ok {
		engine.ReleaseResponse(engineResp)
	}
}

// acquireMiddlewareRequest gets a Request from the engine's shared pool.
func acquireMiddlewareRequest() *engine.Request {
	return engine.AcquireRequest()
}

// releaseMiddlewareRequest returns a Request to the engine's shared pool.
func releaseMiddlewareRequest(req *engine.Request) {
	engine.ReleaseRequest(req)
}

// executeRequest executes an HTTP request through the middleware chain (if configured)
// or directly via the engine. Returns the raw ResponseMutator; the caller must
// release the response via engine.ReleaseResponse() or convert it via convertResponseToResult().
//
// Middleware contract: if a middleware calls next() and obtains a non-nil response,
// it must either return that response (directly or via a later next() call) or
// explicitly release it via releaseResponseMutator(). Returning (nil, error) while
// holding an unreleased response will cause a pool leak.
func (c *clientImpl) executeRequest(ctx context.Context, method, url string, options []RequestOption) (ResponseMutator, error) {
	if !c.hasMiddlewares {
		return c.engine.Request(ctx, method, url, options...)
	}

	engineReq := acquireMiddlewareRequest()
	// Clear sensitive data (cookies, headers, auth tokens) before returning to pool.
	// SAFETY: middlewareChain executes synchronously — defer runs only after
	// the chain returns. If async middleware is ever introduced, this pool
	// pattern will cause data races and must be redesigned.
	defer releaseMiddlewareRequest(engineReq)

	engineReq.SetMethod(method)
	engineReq.SetURL(url)
	engineReq.SetContext(ctx)

	for _, opt := range options {
		if opt != nil {
			if err := opt(engineReq); err != nil {
				return nil, fmt.Errorf("failed to apply request option: %w", err)
			}
		}
	}

	resp, err := c.middlewareChain(ctx, engineReq)
	// Safety net: if middleware returned an error but also a response,
	// release the response to prevent pool leaks. This handles user-written
	// middlewares that call next() (obtaining a response) then return an error
	// without passing it along.
	if err != nil && resp != nil {
		releaseResponseMutator(resp)
		return nil, err
	}
	return resp, err
}

// Close releases resources held by the client including connection pools and transport.
// After calling Close, the client must not be used for further requests.
func (c *clientImpl) Close() error {
	if c.engine == nil {
		return nil
	}
	if err := c.engine.Close(); err != nil {
		return fmt.Errorf("failed to close client: %w", err)
	}
	return nil
}

var (
	defaultClient   atomic.Pointer[clientImpl]
	defaultClientMu sync.Mutex
)

func getDefaultClient() (Client, error) {
	// Fast path: check if already initialized (lock-free)
	if client := defaultClient.Load(); client != nil {
		return client, nil
	}

	// Slow path: mutex-protected initialization ensures exactly one client
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	// Double-check after acquiring lock
	if client := defaultClient.Load(); client != nil {
		return client, nil
	}

	newClient, err := New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize default client: %w", err)
	}

	impl, ok := newClient.(*clientImpl)
	if !ok {
		_ = newClient.Close() // Prevent resource leak on unexpected type
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
	defaultClient.Store(nil)
	return client.Close()
}

// doPackage is a helper for package-level HTTP verb functions.
func doPackage(fn func(Client, string, ...RequestOption) (*Result, error), url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return fn(client, url, options...)
}

// Get makes a GET request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Get(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Get, url, options...)
}

// Post makes a POST request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Post(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Post, url, options...)
}

// Put makes a PUT request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Put(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Put, url, options...)
}

// Patch makes a PATCH request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Patch(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Patch, url, options...)
}

// Delete makes a DELETE request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Delete(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Delete, url, options...)
}

// Head makes a HEAD request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Head(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Head, url, options...)
}

// Options makes an OPTIONS request to the specified URL using the default client. Call ReleaseResult when done to reduce GC pressure.
func Options(url string, options ...RequestOption) (*Result, error) {
	return doPackage(Client.Options, url, options...)
}

// doPackageRequest is a helper for the package-level Request function.
// Unlike doPackage, it accepts a context parameter for timeout and cancellation control.
func doPackageRequest(ctx context.Context, method, url string, options ...RequestOption) (*Result, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Request(ctx, method, url, options...)
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
	return doPackageRequest(ctx, method, url, options...)
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

	if impl.engine.IsClosed() {
		return fmt.Errorf("cannot set a closed client as default")
	}

	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	// Swap the old client with the new one
	var closeErr error
	oldClient := defaultClient.Load()
	defaultClient.Store(impl)
	if oldClient != nil && oldClient != impl {
		closeErr = oldClient.Close()
	}
	return closeErr
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

	// Guard against manually constructed Results with nil sub-structs.
	if r.Response == nil {
		r.Response = &ResponseInfo{}
	}
	if r.Request == nil {
		r.Request = &RequestInfo{}
	}
	if r.Meta == nil {
		r.Meta = &RequestMeta{}
	}

	// Sanitize sensitive body data before returning to pool.
	// Clear the entire backing array to prevent sensitive data (tokens, PII)
	// from persisting in pooled objects.
	body := r.Response.RawBody
	if len(body) > 0 {
		clear(body)
		r.Response.RawBody = nil
	}
	*r.Request = RequestInfo{}
	*r.Response = ResponseInfo{}
	*r.Meta = RequestMeta{}
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
	// Convert body directly from raw bytes, bypassing the engine's lazy
	// string conversion (sync.Once). The engine Response is released right
	// after this call, so its cached body string would be wasted.
	result.Response.RawBody = resp.RawBody()
	if len(result.Response.RawBody) > 0 {
		result.Response.Body = string(result.Response.RawBody)
	}
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

	// Fast path: avoid map lookup when no Cookie header exists
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
	jar, err := newCookieJar()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	return jar, nil
}
