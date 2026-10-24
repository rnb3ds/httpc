package httpc

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/cybergodev/httpc/internal/engine"
	"github.com/cybergodev/httpc/internal/fingerprint"
	"github.com/cybergodev/httpc/internal/types"
	"github.com/cybergodev/httpc/internal/validation"
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

	// Convenience methods for common HTTP verbs. Each issues a request with
	// the corresponding method and returns the buffered Result; none of them
	// take a context — use Request for cancellation/deadline control.

	// Get issues a GET request to url.
	Get(url string, options ...RequestOption) (*Result, error)
	// Post issues a POST request to url (non-idempotent: not retried by default).
	Post(url string, options ...RequestOption) (*Result, error)
	// Put issues a PUT request to url.
	Put(url string, options ...RequestOption) (*Result, error)
	// Patch issues a PATCH request to url (non-idempotent: not retried by default).
	Patch(url string, options ...RequestOption) (*Result, error)
	// Delete issues a DELETE request to url.
	Delete(url string, options ...RequestOption) (*Result, error)
	// Head issues a HEAD request to url.
	Head(url string, options ...RequestOption) (*Result, error)
	// Options issues an OPTIONS request to url.
	Options(url string, options ...RequestOption) (*Result, error)

	// Download downloads a file from url to the path specified in cfg.
	// cfg must be non-nil and cfg.FilePath must be set (ErrEmptyFilePath otherwise).
	// Use DefaultDownloadConfig() as the starting point, then set FilePath and any
	// of ProgressCallback / Overwrite / ResumeDownload / Checksum as needed.
	// The download is subject to the client's Security.MaxResponseBodySize
	// (default 10 MB); oversize bodies fail with an error, never a silent
	// truncation — see DownloadConfig.
	Download(ctx context.Context, url string, cfg *DownloadConfig, options ...RequestOption) (*DownloadResult, error)

	// Close releases resources held by the client
	Close() error
}

// DomainClienter extends Client with domain-scoped operations.
// It provides session management for cookies and headers across requests
// to a specific domain.
type DomainClienter interface {
	Client

	// URL accessors

	// URL returns the base URL this client was constructed with.
	URL() string
	// Domain returns the hostname of the base URL (without port). Origin
	// scoping (host:port comparison) is enforced internally by
	// targetsBaseHost — compare against url.URL.Host, not Domain(), when
	// matching origins.
	Domain() string

	// Session header management

	// SetHeader sets a session header applied to every subsequent request.
	SetHeader(key, value string) error
	// SetHeaders adds or updates multiple session headers (matching keys
	// are replaced; other session headers persist).
	SetHeaders(headers map[string]string) error
	// DeleteHeader removes one session header.
	DeleteHeader(key string)
	// ClearHeaders removes all session headers.
	ClearHeaders()
	// GetHeaders returns a copy of the current session headers.
	GetHeaders() map[string]string

	// Session cookie management

	// SetCookie adds a cookie to the domain session.
	SetCookie(cookie *http.Cookie) error
	// SetCookies adds multiple cookies to the domain session.
	SetCookies(cookies []*http.Cookie) error
	// DeleteCookie removes one session cookie by name.
	DeleteCookie(name string)
	// ClearCookies removes all session cookies.
	ClearCookies()
	// GetCookies returns the session cookies.
	GetCookies() []*http.Cookie
	// GetCookie returns one session cookie by name, or nil.
	GetCookie(name string) *http.Cookie

	// Session access

	// Session returns the underlying SessionManager for direct inspection.
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
// Pass DefaultConfig() for sensible defaults, or use a preset like SecureConfig().
//
// Returns an error if the configuration fails validation (e.g., invalid
// Security.SSRFExemptCIDRs, mutually exclusive security settings) or if the
// underlying engine client cannot be created.
//
// Examples:
//
//	// Use default configuration (or call NewDefault() for the same result)
//	client, err := httpc.New(httpc.DefaultConfig())
//
//	// Use custom configuration
//	cfg := httpc.DefaultConfig()
//	cfg.Timeouts.Request = 60 * time.Second
//	client, err := httpc.New(cfg)
//
//	// Use preset configuration
//	client, err := httpc.New(httpc.SecureConfig())
func New(cfg Config) (Client, error) {
	cfg, err := validatedCopy(cfg)
	if err != nil {
		return nil, err
	}
	return newFromConfig(cfg)
}

// validatedCopy runs the constructor pipeline shared by New and NewDomain:
// validate, deep-copy (callers may reuse their Config), and pre-parse the
// SSRF exemption CIDRs. Keeping the sequence in one place prevents the two
// constructors from drifting apart when a step is added.
func validatedCopy(cfg Config) (Config, error) {
	if err := ValidateConfig(&cfg); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	cfg = copyConfig(cfg)
	if err := cfg.parseSSRFExemptCIDRs(); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

// NewDefault creates a new HTTP client with default configuration.
// It is a convenience shortcut for New(DefaultConfig()); see New for possible
// error conditions (default configuration is always valid, so this effectively
// never returns an error in practice).
//
// Example:
//
//	client, err := httpc.NewDefault()
//	defer func() { _ = client.Close() }()
func NewDefault() (Client, error) {
	return New(DefaultConfig())
}

// newFromConfig creates a client from an already-validated and copied config.
// Used internally by New and NewDomain.
func newFromConfig(cfg Config) (Client, error) {
	engineConfig, err := convertToEngineConfig(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert configuration: %w", err)
	}

	// Warn if InsecureSkipVerify is enabled outside test environment.
	// Check both the top-level flag and a custom TLSConfig that sets it —
	// otherwise a Security.TLSConfig{InsecureSkipVerify: true} silently
	// disables verification with no warning.
	insecureVerify := cfg.Security.InsecureSkipVerify ||
		(cfg.Security.TLSConfig != nil && cfg.Security.TLSConfig.InsecureSkipVerify)

	// Hand-built-Config trap (see the Config struct doc): every bool
	// documented "Default: true" is true only in DefaultConfig()/presets. The
	// exact all-off signature below is virtually always accidental — a
	// struct literal that set only e.g. Retry.BackoffFactor passes
	// ValidateConfig with URL pre-validation (including the SSRF host
	// pre-check), header validation, strict Content-Length, redirects,
	// HTTP/2, and the response-size cap all silently disabled. Warn once;
	// deliberate all-off users can ignore it.
	handBuiltZero := !cfg.Security.ValidateURL && !cfg.Security.ValidateHeaders &&
		!cfg.Security.StrictContentLength && !cfg.Security.FollowRedirects &&
		!cfg.Connection.EnableHTTP2 && cfg.Security.MaxResponseBodySize == 0
	if handBuiltZero && !isTestEnvironment() {
		handBuiltConfigWarnOnce.Do(func() {
			w := getSecurityWarnOutput()
			_, _ = fmt.Fprintf(w, "[CONFIG WARNING] hand-built Config detected: URL validation, header validation, strict Content-Length, redirects, HTTP/2 and the response-size cap are ALL off\n") // best-effort warning
			_, _ = fmt.Fprintf(w, "[CONFIG WARNING] 'Default: true' fields are true only in DefaultConfig()/presets; a struct literal zeroes them — start from DefaultConfig() and change fields\n")  // best-effort warning
		})
	}
	if insecureVerify && !isTestEnvironment() {
		insecureSkipVerifyWarnOnce.Do(func() {
			w := getSecurityWarnOutput()
			_, _ = fmt.Fprintf(w, "[SECURITY WARNING] InsecureSkipVerify is enabled - TLS certificate verification is DISABLED\n") // best-effort warning
			_, _ = fmt.Fprintf(w, "[SECURITY WARNING] This should only be used in testing. Use SecureConfig() for production.\n")  // best-effort warning
		})
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

// copyConfig returns an independent copy of cfg. Value-type sub-configs are
// copied by the struct assignment; this function deep-copies the remaining
// reference types (maps, slices, *tls.Config) so the caller cannot mutate the
// client's configuration after construction.
//
// Note: RetryConfig.CustomPolicy is NOT deep-copied. If the policy
// implementation contains mutable state, do not share the same Config
// instance across multiple clients concurrently.
func copyConfig(src Config) Config {
	dst := src // value copy — all sub-config structs are independent

	// Deep copy reference types within sub-configs
	if dst.Defaults.Headers != nil {
		dst.Defaults.Headers = make(map[string]string, len(src.Defaults.Headers))
		maps.Copy(dst.Defaults.Headers, src.Defaults.Headers)
	}

	if len(dst.Middleware.Middlewares) > 0 {
		dst.Middleware.Middlewares = make([]MiddlewareFunc, len(src.Middleware.Middlewares))
		copy(dst.Middleware.Middlewares, src.Middleware.Middlewares)
	}

	if len(dst.Security.RedirectWhitelist) > 0 {
		dst.Security.RedirectWhitelist = make([]string, len(src.Security.RedirectWhitelist))
		copy(dst.Security.RedirectWhitelist, src.Security.RedirectWhitelist)
	}

	if len(dst.Security.SensitiveHeaders) > 0 {
		dst.Security.SensitiveHeaders = make([]string, len(src.Security.SensitiveHeaders))
		copy(dst.Security.SensitiveHeaders, src.Security.SensitiveHeaders)
	}

	if len(dst.Connection.ProxyPool) > 0 {
		dst.Connection.ProxyPool = make([]string, len(src.Connection.ProxyPool))
		copy(dst.Connection.ProxyPool, src.Connection.ProxyPool)
	}

	if len(dst.Connection.ProxyRotateOnStatus) > 0 {
		dst.Connection.ProxyRotateOnStatus = make([]int, len(src.Connection.ProxyRotateOnStatus))
		copy(dst.Connection.ProxyRotateOnStatus, src.Connection.ProxyRotateOnStatus)
	}

	if dst.Security.TLSConfig != nil {
		dst.Security.TLSConfig = src.Security.TLSConfig.Clone()
	}

	if len(dst.Security.SSRFExemptCIDRs) > 0 {
		dst.Security.SSRFExemptCIDRs = make([]string, len(src.Security.SSRFExemptCIDRs))
		copy(dst.Security.SSRFExemptCIDRs, src.Security.SSRFExemptCIDRs)
	}

	if len(src.parsedCIDRs) > 0 {
		dst.parsedCIDRs = make([]*net.IPNet, len(src.parsedCIDRs))
		copy(dst.parsedCIDRs, src.parsedCIDRs)
	}

	return dst
}

// buildMiddlewareChain constructs a middleware chain from the provided middlewares.
// The terminal handler copies the middleware-modified request fields into a fresh engine
// request and executes it. This avoids re-applying user options (double execution) and
// uses a single option closure to forward all mutable state.
//
// Callbacks (OnRequest/OnResponse) and the per-request SSRF override (AllowPrivateIPs)
// live on the concrete *engine.Request, not on the shared RequestMutator interface:
// their signatures reference *engine.Request/*engine.Response, and surfacing them
// through internal/types would create an import cycle (engine -> types). The same
// applies to NoTimeout, RetryNonIdempotent, and the sanitized-URL cache, which the
// shared interface does not expose at all. The terminal handler therefore reads them
// via a concrete-type assertion. This is a deliberate, bounded coupling to
// *engine.Request with a graceful fallback — see finalHandler.
func (c *clientImpl) buildMiddlewareChain(middlewares []MiddlewareFunc) Handler {
	finalHandler := func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		reqCtx := req.Context()
		if reqCtx == nil {
			reqCtx = ctx
		}

		// The concrete *engine.Request carries engine-specific hooks (callbacks,
		// per-request SSRF override) that are not part of RequestMutator (see the
		// buildMiddlewareChain doc above). Apply() forwards them along with every
		// other field when the assertion succeeds. A middleware that replaces req
		// with a non-*engine.Request value loses them — as do NoTimeout,
		// RetryNonIdempotent, and the sanitized-URL cache, none of which are
		// exposed on RequestMutator. Request replacement is therefore unsupported
		// for ALL per-request overrides (mirrors getOrComputeSanitizedURL); a
		// replacing middleware is limited to the fields forwarded by the
		// fallback below.
		engReq, isEngineReq := req.(*engine.Request)

		// Single option closure forwards all mutable fields from the middleware-modified request.
		resp, err := c.engine.Request(reqCtx, req.Method(), req.URL(),
			engine.RequestOption(func(r types.RequestMutator) error {
				engR, ok := r.(*engine.Request)
				if !ok {
					return fmt.Errorf("middleware terminal handler: engine passed %T, want *engine.Request", r)
				}
				// Fast path: src is the concrete *engine.Request (all built-in
				// middleware mutates in place). Apply is owned by the engine, so
				// new Request fields are forwarded here by construction — the
				// facade no longer enumerates them.
				if isEngineReq {
					engR.Apply(engReq)
					return nil
				}
				// Fallback for a middleware that replaced req with a non-*engine.Request
				// RequestMutator: forward via the interface accessors. Callbacks and
				// the SSRF override are unavailable in this case (see doc above).
				//
				// Headers and query params are passed as-is: engine
				// SetHeaders/SetQueryParams COPY into engine-pooled maps, which
				// is exactly what this path needs — a replacing middleware's
				// accessors may return the engineReq's pooled maps, and copying
				// at the engine boundary prevents the same pooled map being
				// returned to the pool twice (double-release → concurrent map
				// writes across future requests). No facade-side copy required;
				// the engine setter is the single copy point.
				if headers := req.Headers(); headers != nil {
					engR.SetHeaders(headers)
				}
				if qp := req.QueryParams(); qp != nil {
					engR.SetQueryParams(qp)
				}
				engR.SetBody(req.Body())
				engR.SetTimeout(req.Timeout())
				engR.SetMaxRetries(req.MaxRetries())
				engR.SetCookies(req.Cookies())
				if fr := req.FollowRedirects(); fr != nil {
					engR.SetFollowRedirects(fr)
				}
				if mr := req.MaxRedirects(); mr != nil {
					engR.SetMaxRedirects(mr)
				}
				engR.SetStreamBody(req.StreamBody())
				return nil
			}))
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
func (c *clientImpl) Request(ctx context.Context, method, url string, options ...RequestOption) (result *Result, err error) {
	// SEC-003: default panic safety net. An unexpected runtime panic anywhere in the
	// execution path (engine, transport, TLS, response conversion) is converted to an
	// error instead of crashing the caller. Pool hygiene is unaffected: executeRequest's
	// own deferred releases and the releaseResponseMutator defer below run during stack
	// unwinding before this recover (LIFO), so no pooled Response is leaked on panic.
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = panicToError(r)
		}
	}()
	resp, err := c.executeRequest(ctx, method, url, options)
	if err != nil {
		return nil, err
	}
	// WithStreamBody has no effect on this path: the body is buffered into a
	// Result below and the stream is closed by releaseResponseMutator, so the
	// caller would silently get an empty body. Fail loudly instead and point
	// at Download, which is the streaming entry point.
	//
	// The type assertion covers both the direct engine path and the middleware
	// path (whose terminal handler returns the engine Response unwrapped). A
	// user middleware that *wraps* the Response in its own type escapes this
	// guard — such authors own the streaming contract (see TimeoutMiddleware
	// for the reject-up-front pattern).
	if engineResp, ok := resp.(*engine.Response); ok && engineResp.RawBodyReader() != nil {
		releaseResponseMutator(resp)
		return nil, ErrStreamBodyRequiresDownload
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
	if c.engine != nil && c.engine.IsClosed() {
		return nil, ErrClientClosed
	}
	if !c.hasMiddlewares {
		return c.engine.Request(ctx, method, url, toEngineOptions(options)...)
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

	if err := engine.ApplyOptions(engineReq, toEngineOptions(options)); err != nil {
		return nil, fmt.Errorf("failed to apply request option: %w", err)
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
	// A middleware contract violation: neither a response nor an error.
	// Without this guard the caller would receive (nil, nil), and the nil-safe
	// Result accessors would silently report StatusCode 0 as a "success".
	// Mirrors the defensive fallback in engine executeWithRetry.
	if resp == nil && err == nil {
		return nil, fmt.Errorf("middleware chain returned neither a response nor an error")
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
	// Fast path: check if already initialized (lock-free).
	// A closed default client is treated as absent so the singleton
	// self-heals after CloseDefaultClient() or a direct client.Close().
	if client := defaultClient.Load(); client != nil && !client.engine.IsClosed() {
		return client, nil
	}

	// Slow path: mutex-protected initialization ensures exactly one client
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	// Double-check after acquiring lock
	if client := defaultClient.Load(); client != nil && !client.engine.IsClosed() {
		return client, nil
	}

	newClient, err := NewDefault()
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

// routingKey identifies one package-level routed client. The zero value is the
// default (direct, no fingerprint) client; a non-zero proxy, systemProxy, or
// fingerprint selects a dedicated client. The fields are the only
// instance-level knobs the routing options can currently express — extend this
// struct when a new routing option is added.
type routingKey struct {
	proxy       string // raw proxy URL string ("" = no proxy)
	systemProxy bool   // enable system proxy detection (false = off)
	fingerprint string // resolved fingerprint preset name ("" = standard stack)
}

// isZero reports whether the key routes to the default client (no routing
// option carried).
func (k routingKey) isZero() bool {
	return k.proxy == "" && !k.systemProxy && k.fingerprint == ""
}

// String renders the routing key with the proxy URL's credentials redacted.
// CloseRoutedClients interpolates the key into a returned error; without this
// method fmt prints the raw proxy string — password included — which apps
// typically log. Every other proxy-URL error path in the codebase already
// redacts (validation.ValidateProxyURL, proxypool, system-proxy detection);
// this closes the last gap. An unparseable URL (unreachable here — creation
// validates it) is rendered as a placeholder rather than echoed raw, so a
// future code path cannot reintroduce the leak through the error branch.
func (k routingKey) String() string {
	proxy := ""
	if k.proxy != "" {
		if u, err := url.Parse(k.proxy); err == nil {
			proxy = u.Redacted()
		} else {
			proxy = "<invalid-proxy-url>"
		}
	}
	return fmt.Sprintf("{proxy:%s systemProxy:%t fingerprint:%s}", proxy, k.systemProxy, k.fingerprint)
}

// routingClientEntry is one cached package-level routed client, keyed by
// routingKey. Created lazily on first use and closed by CloseRoutedClients.
type routingClientEntry struct {
	key    routingKey
	client *clientImpl
}

// routedClients caches the dedicated clients that package-level routing
// options (WithProxy, WithSystemProxy, WithTLSFingerprint) create, keyed by
// routingKey. The proxy field uses the raw (pre-canonicalized) URL string
// because it is the caller's identity for the proxy — two spellings of the same
// proxy (e.g. "host" vs "host:1080") are distinct entries, which is correct:
// they may resolve to different dial addresses. The fingerprint field uses the
// resolved canonical preset name (see ResolvePresetName), so "Chrome" and
// "chrome" share one client.
var routedClients sync.Map // map[routingKey]*routingClientEntry

// routedClientsMu serializes the routed-clients slow path: creation inside
// getOrCreateRoutedClient and the sweep in CloseRoutedClients. The Load fast
// path stays lock-free; the mutex makes create-vs-create and create-vs-close
// interleavings impossible, so the resolution logic can never return a client
// it closed itself (the former LoadOrStore loser branch did exactly that when
// the raced-in winner turned out to be closed) and never strands a live
// client outside the cache for CloseRoutedClients to miss.
var routedClientsMu sync.Mutex

// getOrCreateRoutedClient returns the cached routed client for key, creating it
// on first use. The client is built from DefaultConfig() with the key's proxy
// and fingerprint applied — so every routed client inherits the same secure
// defaults (SSRF protection, TLS, timeouts, retries) as the default client and
// differs only in the routing options that produced the key.
//
// The fingerprint is resolved and canonicalized here (ResolvePresetName), so a
// miss surfaces as ErrInvalidSecurity before any client is built and so that
// aliases ("Chrome" vs "chrome") collapse onto one cache entry. The proxy URL
// is validated here too; both mirrors ValidateConfig's treatment of the
// equivalent Config fields. The zero key returns the default client via
// getDefaultClient.
func getOrCreateRoutedClient(key routingKey) (Client, error) {
	if key.isZero() {
		return getDefaultClient()
	}

	if key.proxy != "" {
		if _, err := validation.ValidateProxyURL(key.proxy); err != nil {
			return nil, fmt.Errorf("%w: WithProxy: %w", ErrInvalidConnection, err)
		}
	}
	if key.fingerprint != "" {
		resolved, ok := fingerprint.ResolvePresetName(key.fingerprint)
		if !ok {
			return nil, fmt.Errorf("%w: WithTLSFingerprint %q is not a known browser preset", ErrInvalidSecurity, key.fingerprint)
		}
		key.fingerprint = resolved // canonicalize the cache key
	}

	// Fast path: a live cached client (lock-free). A closed entry falls
	// through to the locked slow path, which self-heals it.
	if entry, ok := routedClients.Load(key); ok {
		e := entry.(*routingClientEntry)
		if !e.client.engine.IsClosed() {
			return e.client, nil
		}
	}

	// Slow path under routedClientsMu: exactly one creator runs at a time and
	// CloseRoutedClients (same mutex) cannot interleave. The double-check
	// below therefore sees the final state, and the entry returned to the
	// caller is the entry stored — the former LoadOrStore loser branch closed
	// its own client before checking the raced-in winner, then returned that
	// same closed client when the winner turned out dead.
	routedClientsMu.Lock()
	defer routedClientsMu.Unlock()

	if entry, ok := routedClients.Load(key); ok {
		e := entry.(*routingClientEntry)
		if !e.client.engine.IsClosed() {
			return e.client, nil
		}
		// Self-heal a closed client (the caller may have closed it directly).
		routedClients.Delete(key)
	}

	cfg := DefaultConfig()
	if key.proxy != "" {
		cfg.Connection.ProxyURL = key.proxy
	}
	if key.systemProxy {
		cfg.Connection.EnableSystemProxy = true
	}
	if key.fingerprint != "" {
		cfg.Security.TLSFingerprint = key.fingerprint
	}

	client, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create routed client: %w", err)
	}
	impl, ok := client.(*clientImpl)
	if !ok {
		_ = client.Close() // prevent resource leak on unexpected type
		return nil, fmt.Errorf("unexpected client type")
	}

	routedClients.Store(key, &routingClientEntry{key: key, client: impl})
	return impl, nil
}

// CloseRoutedClients closes every cached package-level routed client (the
// WithProxy, WithSystemProxy, and WithTLSFingerprint clients) and clears the
// cache. It is idempotent and safe for concurrent use. It does not close the
// default client — use CloseDefaultClient for that. Unlike CloseDefaultClient it
// does not return the first close error, but closes as many clients as possible
// and returns a joined error (errors.Join) when any close fails.
func CloseRoutedClients() error {
	// Hold routedClientsMu so the sweep cannot interleave with a concurrent
	// getOrCreateRoutedClient creation: a client created before this call is
	// always closed, and a client created after it starts is never swept.
	routedClientsMu.Lock()
	defer routedClientsMu.Unlock()

	var errs error
	routedClients.Range(func(key, value any) bool {
		routedClients.Delete(key)
		entry := value.(*routingClientEntry)
		if err := entry.client.Close(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("routed client %v: %w", entry.key, err))
		}
		return true
	})
	return errs
}

// withDefault resolves the target client (a WithProxy/WithSystemProxy/
// WithTLSFingerprint-routed client, or the default) and dispatches a request
// through it. It is the single dispatch point for every package-level HTTP
// function, so the package-level verbs and Request share one code path instead
// of duplicating the default-client resolution in two separate helpers.
func withDefault(ctx context.Context, method, url string, options []RequestOption) (*Result, error) {
	client, options, err := resolvePackageClient(options)
	if err != nil {
		return nil, err
	}
	return client.Request(ctx, method, url, options...)
}

// extractProxyOption returns the WithProxy option carried on options, or nil.
// WithProxy is a routing option (see proxyOption): it is stripped before the
// request executes and never reaches the engine.
func extractProxyOption(options []RequestOption) *proxyOption {
	for _, opt := range options {
		if po, ok := opt.(*proxyOption); ok {
			return po
		}
	}
	return nil
}

// extractFingerprintOption returns the WithTLSFingerprint option carried on
// options, or nil. Like WithProxy it is a routing option, stripped before the
// request executes.
func extractFingerprintOption(options []RequestOption) *fingerprintOption {
	for _, opt := range options {
		if fo, ok := opt.(*fingerprintOption); ok {
			return fo
		}
	}
	return nil
}

// extractSystemProxyOption reports whether a WithSystemProxy routing option is
// carried on options. Like WithProxy and WithTLSFingerprint it is stripped
// before the request executes.
func extractSystemProxyOption(options []RequestOption) bool {
	for _, opt := range options {
		if _, ok := opt.(*systemProxyOption); ok {
			return true
		}
	}
	return false
}

// stripRoutingOptions returns options with the routing options (proxyOption,
// systemProxyOption, and fingerprintOption) removed. These are handled by the
// dispatch layer and would otherwise fail the request when they reach the
// engine (their apply returns an error — see proxyOption.apply /
// systemProxyOption.apply / fingerprintOption.apply). The input slice is
// returned unchanged when no routing option is present, keeping the common
// (no-routing) path allocation-free.
func stripRoutingOptions(options []RequestOption) []RequestOption {
	hasRouting := false
	for _, opt := range options {
		switch opt.(type) {
		case *proxyOption, *systemProxyOption, *fingerprintOption:
			hasRouting = true
		}
	}
	if !hasRouting {
		return options
	}
	out := make([]RequestOption, 0, len(options)-1)
	for _, opt := range options {
		switch opt.(type) {
		case *proxyOption, *systemProxyOption, *fingerprintOption:
			continue
		}
		out = append(out, opt)
	}
	return out
}

// resolvePackageClient resolves which client a package-level call should run
// on, honoring WithProxy, WithSystemProxy, and WithTLSFingerprint. Returns the
// target client and the options with the routing options stripped. Validation
// of the routing values (proxy URL, fingerprint preset) happens in
// getOrCreateRoutedClient.
func resolvePackageClient(options []RequestOption) (Client, []RequestOption, error) {
	key := routingKey{}
	if po := extractProxyOption(options); po != nil {
		key.proxy = po.url
	}
	// WithProxy takes precedence over WithSystemProxy (mirrors Config: ProxyURL >
	// EnableSystemProxy); when both are present the system-proxy flag is dropped
	// so the cache key collapses onto the explicit-proxy client.
	if key.proxy == "" && extractSystemProxyOption(options) {
		key.systemProxy = true
	}
	if fo := extractFingerprintOption(options); fo != nil {
		key.fingerprint = fo.preset
	}
	options = stripRoutingOptions(options)
	c, err := getOrCreateRoutedClient(key)
	if err != nil {
		return nil, nil, err
	}
	return c, options, nil
}

// Package-level HTTP convenience functions (Get, Post, Put, Patch, Delete,
// Head, Options, Request) use a shared default client managed internally.
// Each delegates to the corresponding method on that singleton client (see
// SetDefaultClient, CloseDefaultClient). For production services, prefer an
// explicit client created with NewDefault() to control configuration
// and lifecycle.

// Get makes a GET request to the specified URL using the default client.
func Get(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "GET", url, options)
}

// Post makes a POST request to the specified URL using the default client.
func Post(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "POST", url, options)
}

// Put makes a PUT request to the specified URL using the default client.
func Put(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "PUT", url, options)
}

// Patch makes a PATCH request to the specified URL using the default client.
func Patch(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "PATCH", url, options)
}

// Delete makes a DELETE request to the specified URL using the default client.
func Delete(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "DELETE", url, options)
}

// Head makes a HEAD request to the specified URL using the default client.
func Head(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "HEAD", url, options)
}

// Options makes an OPTIONS request to the specified URL using the default client.
func Options(url string, options ...RequestOption) (*Result, error) {
	return withDefault(backgroundCtx, "OPTIONS", url, options)
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
	return withDefault(ctx, method, url, options)
}

// SetDefaultClient sets a custom client as the default for package-level functions.
// The previous default client is closed automatically. The new client is
// installed even when closing the previous one fails; in that case the
// returned error reports the close failure, not a failed swap.
//
// Concurrency: closing the previous default aborts any requests still in
// flight on it — goroutines that already resolved the old client may observe
// connection errors. Swap the default only at startup or during a quiescent
// window, never under live traffic.
// Only *clientImpl instances created by this package are supported.
// Returns an error if client is nil, not created by this package, or already closed.
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

	if impl.engine.IsClosed() {
		return fmt.Errorf("cannot set a closed client as default")
	}

	// Swap the old client with the new one. The new client is installed even
	// when an error is returned: the error reports the previous default's
	// close failure, not a failed swap.
	oldClient := defaultClient.Load()
	defaultClient.Store(impl)
	if oldClient != nil && oldClient != impl {
		if closeErr := oldClient.Close(); closeErr != nil {
			return fmt.Errorf("default client replaced, but closing the previous default failed: %w", closeErr)
		}
	}
	return nil
}

// resultBundle co-locates a Result and its three nested info structs in a single
// heap allocation. convertResponseToResult returns &b.result, whose Request,
// Response, and Meta fields point into the same bundle. This collapses four
// separate allocations (Result + RequestInfo + ResponseInfo + RequestMeta) into
// one while remaining transparent to callers — the public API exposes only the
// *Result pointer, and the bundle is reclaimed by GC once the result is.
// Pooling is unsuitable here because callers retain the returned Result
// indefinitely.
type resultBundle struct {
	result Result
	req    RequestInfo
	resp   ResponseInfo
	meta   RequestMeta
}

func convertResponseToResult(resp ResponseMutator) *Result {
	if resp == nil {
		return nil
	}

	// Optimization: transfer request header ownership from the engine Response
	// instead of cloning. Fall back to Headers() for middleware-wrapped responses.
	var requestHeaders http.Header
	if engineResp, ok := resp.(*engine.Response); ok {
		requestHeaders = engineResp.TransferRequestHeaders()
	} else {
		requestHeaders = resp.RequestHeaders()
	}
	requestCookies := extractRequestCookies(requestHeaders)

	// Single allocation for Result and its three nested structs (see resultBundle).
	b := &resultBundle{
		req: RequestInfo{
			URL:     resp.RequestURL(),
			Method:  resp.RequestMethod(),
			Headers: requestHeaders,
			Cookies: requestCookies,
		},
		resp: ResponseInfo{
			StatusCode: resp.StatusCode(),
			Status:     resp.Status(),
			Proto:      resp.Proto(),
		},
		meta: RequestMeta{
			Duration:      resp.Duration(),
			Attempts:      resp.Attempts(),
			RedirectChain: resp.RedirectChain(),
			RedirectCount: resp.RedirectCount(),
		},
	}
	result := &b.result
	result.Request = &b.req
	result.Response = &b.resp
	result.Meta = &b.meta

	// Transfer header ownership from engine Response.
	// Fall back to clone for middleware-wrapped ResponseMutator.
	if engineResp, ok := resp.(*engine.Response); ok {
		result.Response.Headers = engineResp.TransferHeaders()
		b.meta.ProxyURL = engineResp.ProxyURL()
	} else {
		result.Response.Headers = cloneHeaders(resp.Headers())
		// Middleware-wrapped responses keep their proxy attribution when the
		// wrapper forwards it: embedding or delegating to the underlying
		// *engine.Response satisfies this assertion for free. A wrapper that
		// hides the underlying response loses Meta.ProxyURL (documented in the
		// middleware package docs) — previously even a forwarding wrapper
		// lost it, because the attribution was reachable only via the
		// concrete type assertion above.
		if p, ok := resp.(interface{ ProxyURL() string }); ok {
			b.meta.ProxyURL = p.ProxyURL()
		}
	}
	result.Response.RawBody = resp.RawBody()
	if len(result.Response.RawBody) > 0 {
		result.Response.Body = string(result.Response.RawBody)
	}
	result.Response.ContentLength = resp.ContentLength()
	result.Response.Cookies = resp.Cookies()

	return result
}

func extractRequestCookies(headers http.Header) []*http.Cookie {
	if headers == nil {
		return nil
	}

	// Direct map lookup with the pre-canonicalized key "Cookie" avoids the
	// textproto.CanonicalMIMEHeaderKey overhead of headers.Get on every request.
	cookieVals := headers["Cookie"]
	if len(cookieVals) == 0 || cookieVals[0] == "" {
		return nil
	}

	return parseCookieHeader(cookieVals[0])
}

// cloneHeaders returns a deep copy of http.Header. Delegates to the engine's
// batch-allocation CloneHeader to avoid duplicating the logic.
func cloneHeaders(h http.Header) http.Header {
	return engine.CloneHeader(h)
}

func createCookieJar(enableCookies bool, psl cookiejar.PublicSuffixList) (http.CookieJar, error) {
	if !enableCookies {
		return nil, nil
	}
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: psl,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	return jar, nil
}
