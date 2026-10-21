// Package engine implements the core HTTP client engine with request processing,
// response handling, connection pooling, retry logic, and middleware support.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/security"
	"github.com/cybergodev/httpc/internal/types"
	"github.com/cybergodev/httpc/internal/validation"
)

// httpcDebug caches the HTTPC_DEBUG environment-variable check to avoid a
// syscall on every retry attempt and proxy selection. Evaluated once at first
// use via sync.OnceValue; subsequent calls return the cached bool.
var httpcDebug = sync.OnceValue(func() bool {
	return os.Getenv("HTTPC_DEBUG") != ""
})

// Client is the internal HTTP client that manages requests, responses, retries,
// and connection pooling.
type Client struct {
	config *Config

	transport         transportManager
	requestProcessor  *requestProcessor
	responseProcessor *responseProcessor
	retryEngine       *retryEngine
	validator         *security.Validator

	connectionPool *connection.PoolManager

	// requestPool reduces allocations for Request objects
	requestPool requestPool
	// execRequestPool reduces allocations for Request copies in executeRequest
	execRequestPool requestPool
	// securityRequestPool reduces allocations for security.Request objects
	securityRequestPool sync.Pool

	closed atomic.Int32

	closeOnce sync.Once
}

// Config defines the HTTP client configuration.
// Config should be treated as immutable after creation.
//
// Connection holds the transport and connection-pool settings; the engine adds
// request, security, and retry policy on top. Sharing one connection.Config
// value (rather than mirroring its fields here) removes the field-by-field copy
// that previously mapped engine.Config onto connection.Config by hand — see
// buildConnectionConfig and convertToEngineConfig.
type Config struct {
	// Connection carries the transport, TLS, proxy, cookie-jar, and connection-
	// pool configuration. convertToEngineConfig initializes it from
	// connection.DefaultConfig() and overrides the shared fields, so the
	// connection-only fields (MaxTotalConns, ExpectContinueTimeout,
	// DisableKeepAlives, ForceAttemptHTTP2) keep their defaults with no
	// engine-side merge.
	Connection connection.Config

	// Timeout is the overall request timeout, applied at the engine level as
	// the total budget across retries.
	Timeout time.Duration

	// ProxyRotatePerRequest closes idle connections before each request so the
	// transport re-evaluates the proxy pool. It is engine-only (not a
	// connection.Config field): rotation is an engine/retry concern, not a
	// transport setting.
	ProxyRotatePerRequest bool

	// TLSFingerprint enables full-stack browser transport fingerprinting. The
	// fingerprint adapter replaces the transport, so this is engine-only.
	TLSFingerprint string

	// TLSFingerprintDisableSessionTickets forces a full TLS handshake on
	// every fingerprinted connection (see the public SecurityConfig field).
	TLSFingerprintDisableSessionTickets bool

	MaxResponseBodySize     int64
	MaxRequestBodySize      int64
	MaxDecompressedBodySize int64
	ValidateURL             bool
	ValidateHeaders         bool
	StrictContentLength     bool

	MaxRetries    int
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	BackoffFactor float64
	Jitter        bool

	// RetryNonIdempotent allows retrying non-idempotent methods (POST, PATCH,
	// and custom methods). Default false: a timeout or retryable 5xx may
	// arrive after the server already committed the operation, and replaying
	// the buffered body would duplicate it (duplicate payment, double insert,
	// ...). See the retry guard in executeWithRetry.
	RetryNonIdempotent bool

	// ExtraRetryableStatusCodes are HTTP status codes beyond the built-in set
	// (408/429/500/502/503/504) that should trigger a retry. Seeded from
	// Connection.ProxyRotateOnStatus to rotate proxies on WAF/CF blocks.
	ExtraRetryableStatusCodes []int

	// CustomRetryPolicy allows providing a custom retry policy implementation.
	// If set, it overrides the built-in retry logic.
	CustomRetryPolicy types.RetryPolicy

	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	MaxRedirects    int

	// EnableCookies enables automatic cookie handling; the jar itself lives on
	// Connection.CookieJar.
	EnableCookies bool

	// Redirect whitelist configuration
	RedirectWhitelist *security.DomainWhitelist

	// SensitiveHeaders extends the built-in cross-origin redirect strip set
	// (redirectSensitiveHeaders) with caller-declared credential headers, in
	// canonical (http.CanonicalHeaderKey) form. Empty = built-ins only.
	SensitiveHeaders []string

	// Certificate pinning
	CertificatePinner security.CertificatePinner
}

// NewClient creates a new engine Client with the given configuration and optional client options.
func NewClient(config *Config, opts ...clientOption) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Process options
	options := &clientOptions{}
	for _, opt := range opts {
		opt(options)
	}

	client := &Client{
		config:          config,
		requestPool:     newRequestPool(),
		execRequestPool: newRequestPool(),
		securityRequestPool: sync.Pool{
			New: func() any {
				return &security.Request{}
			},
		},
	}

	var err error

	// Use custom transport if provided, otherwise create default
	if options.customTransport != nil {
		client.transport = options.customTransport
		// Connection pool not needed for custom transport
	} else {
		client.connectionPool, err = connection.NewPoolManager(buildConnectionConfig(config))
		if err != nil {
			return nil, fmt.Errorf("failed to create connection pool: %w", err)
		}

		client.transport, err = newTransport(config, client.connectionPool)
		if err != nil {
			_ = client.connectionPool.Close() // best-effort cleanup on init failure
			return nil, fmt.Errorf("failed to create transport: %w", err)
		}
	}

	client.requestProcessor = newRequestProcessor(config)
	client.responseProcessor = newResponseProcessor(config)
	client.retryEngine = newRetryEngine(config)

	validatorConfig := &security.Config{
		ValidateURL:        config.ValidateURL,
		ValidateHeaders:    config.ValidateHeaders,
		MaxRequestBodySize: config.MaxRequestBodySize,
		AllowPrivateIPs:    config.Connection.AllowPrivateIPs,
		ExemptNets:         config.Connection.ExemptNets,
	}
	client.validator = security.NewValidatorWithConfig(validatorConfig)

	return client, nil
}

// buildConnectionConfig returns the engine's embedded connection config,
// starting from connection defaults and overriding any field convertToEngineConfig
// already populated. It is the public layer (not the engine) that owns the
// value: convertToEngineConfig seeds Config.Connection from
// connection.DefaultConfig() and copies its public ConnectionConfig fields into
// it, so by the time a Config reaches the engine the connection settings are
// already final and the engine simply carries them through.
//
// connection-only fields (MaxTotalConns, ExpectContinueTimeout, DisableKeepAlives,
// ForceAttemptHTTP2) have no public knob; they keep their connection.DefaultConfig
// values from convertToEngineConfig. The CertificatePinner is assigned to the
// exported Config.Connection.CertPinner field directly (no more SetCertPinner
// side channel).
func buildConnectionConfig(config *Config) *connection.Config {
	// Copy, then write the pinner into the copy: Config is documented as
	// immutable after creation, and writing through &config.Connection would
	// race a concurrent New sharing the same Config value.
	connConfig := new(connection.Config)
	*connConfig = config.Connection
	if config.CertificatePinner != nil {
		connConfig.CertPinner = config.CertificatePinner
	}
	return connConfig
}

// closeBodyReader closes r when it is an io.Closer (best-effort). Used by the
// retry loop once a caller-supplied io.Reader body has been buffered (or the
// request is aborting): on the non-retry path the transports own and close the
// request body (the standard stack in net/http's send; the fingerprint
// adapter's fail path closes it on its early errors), so an *os.File passed
// via WithBody would otherwise leak one handle per request until the GC
// finalizer runs.
func closeBodyReader(r io.Reader) {
	if rc, ok := r.(io.Closer); ok {
		_ = rc.Close() // best-effort: fully buffered or the request is aborting
	}
}

// transferCancelOwnership moves overallCancel into resp.cancelFunc so the
// deferred overallCancel in executeWithRetry cannot fire: streaming responses
// need the cancel to stay alive until ReleaseResponse. It CHAINS onto any
// cancelFunc already stored by executeRequest (a per-attempt streamCancel)
// instead of overwriting it — overwriting would orphan that cancel func and
// pin its timer until the deadline. Both call sites (success path and the
// defensive (nil, nil) fallback) previously carried verbatim copies of this
// block; the single definition keeps them from drifting.
func transferCancelOwnership(resp *Response, overallCancel *context.CancelFunc) {
	if *overallCancel == nil || resp.rawBodyReader == nil {
		return
	}
	if prev := resp.cancelFunc; prev != nil {
		inner, outer := prev, *overallCancel
		resp.cancelFunc = func() { inner(); outer() }
	} else {
		resp.cancelFunc = *overallCancel
	}
	*overallCancel = nil
}

// effectiveRequestTimeout resolves the per-request timeout budget: the explicit
// WithTimeout value when set, else the client-level Config.Timeout, unless the
// request opts out via NoTimeout. A non-positive result means "no budget".
func effectiveRequestTimeout(c *Client, req *Request) time.Duration {
	if req.NoTimeout() {
		return 0
	}
	if t := req.Timeout(); t > 0 {
		return t
	}
	return c.config.Timeout
}

// wrapCtxIfFarther returns a context carrying a timeout deadline when timeout
// is positive AND the parent either has no deadline or a farther one; any
// nearer parent deadline wins untouched. Otherwise the parent is returned
// unchanged with a nil CancelFunc, and callers must defer the cancel only
// when non-nil. Both timeout application sites — the shared retry budget in
// executeWithRetry and the per-attempt budget in executeRequest — previously
// carried byte-identical copies of this judgment; keeping it in one place
// prevents the two paths from drifting apart on exactly the invariant
// timeout semantics depend on (a nearer caller deadline always wins).
func wrapCtxIfFarther(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, nil
	}
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline && time.Until(deadline) <= timeout {
		return ctx, nil
	}
	return context.WithTimeout(ctx, timeout)
}

// ErrClientClosed is returned when attempting to use a closed client.
var ErrClientClosed = errors.New("client is closed")

// Request executes an HTTP request with the given method, URL, and options.
// It acquires a pooled Request, applies the options, processes it through the
// transport layer with retry support, and returns the resulting Response.
// Returns ErrClientClosed if the client has been closed.
func (c *Client) Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	if c.closed.Load() == 1 {
		return nil, ErrClientClosed
	}

	startTime := time.Now()

	// Get Request from pool (already zeroed by putRequest via resetRequest)
	req := c.getRequest()
	req.SetMethod(method)
	req.SetURL(url)
	req.SetContext(ctx)

	// Ensure request is returned to pool after processing
	defer c.putRequest(req)

	if err := ApplyOptions(req, options); err != nil {
		// Caller-side configuration error, not a transport failure.
		return nil, fmt.Errorf("failed to apply request option: %w", err)
	}

	// Propagate a per-request AllowPrivateIPs override to the connection dialer
	// and the redirect-target validator via the request context. The pre-flight
	// validator reads the same override directly from secReq below.
	// A nil context must be tolerated here like everywhere else in the engine
	// (context.WithValue(nil, ...) panics).
	if override := req.AllowPrivateIPs(); override != nil {
		reqCtx := req.Context()
		if reqCtx == nil {
			reqCtx = backgroundCtx
		}
		req.SetContext(connection.WithAllowPrivateIPsOverride(reqCtx, *override))
	}

	// Use pooled security.Request for validation
	secReq := c.getSecurityRequest()
	secReq.URL = req.URL()
	secReq.Headers = req.Headers()
	secReq.Body = req.Body()
	secReq.AllowPrivateIPs = req.AllowPrivateIPs()

	validationErr := c.validator.ValidateRequest(secReq)
	c.putSecurityRequest(secReq)

	if validationErr != nil {
		// Caller-side validation error, not a transport failure.
		return nil, fmt.Errorf("request validation failed: %w", validationErr)
	}

	response, err := c.executeWithRetry(req)
	duration := time.Since(startTime)

	if err != nil {
		return nil, err
	}

	response.SetDuration(duration)
	return response, nil
}

// getRequest retrieves a Request object from the pool with safe type assertion
func (c *Client) getRequest() *Request {
	return c.requestPool.get()
}

// putRequest returns a Request object to the pool
func (c *Client) putRequest(req *Request) {
	c.requestPool.put(req)
}

// getSecurityRequest retrieves a security.Request from the pool
func (c *Client) getSecurityRequest() *security.Request {
	req, ok := c.securityRequestPool.Get().(*security.Request)
	if !ok || req == nil {
		return &security.Request{}
	}
	return req
}

// putSecurityRequest returns a security.Request to the pool after clearing its fields
func (c *Client) putSecurityRequest(req *security.Request) {
	if req == nil {
		return
	}
	*req = security.Request{}
	c.securityRequestPool.Put(req)
}

// getExecRequest retrieves a Request object from the exec pool for request copies with safe type assertion
func (c *Client) getExecRequest() *Request {
	return c.execRequestPool.get()
}

// putExecRequest returns a Request object to the exec pool
func (c *Client) putExecRequest(req *Request) {
	c.execRequestPool.put(req)
}

func (c *Client) sleepWithContext(ctx context.Context, duration time.Duration) error {
	if ctx == nil {
		time.Sleep(duration)
		return nil
	}

	// Go 1.23+ made time.Timer garbage-collectable and no longer requires the
	// explicit channel drain after Stop; the pooled-timer micro-optimization
	// that predates that change is needless complexity for a backoff sleep that
	// runs a handful of times per request. A plain timer is simpler and correct.
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// drainAndCloseBody drains a bounded prefix of an unwanted response body and
// closes it, letting net/http reuse the underlying connection. The drain cap
// (defaultMaxDrain, tightened to MaxResponseBodySize when smaller) prevents a
// huge body from stalling cleanup. Shared by the buffered-path cleanup defer
// and the streaming Content-Encoding guard.
func (c *Client) drainAndCloseBody(body io.ReadCloser) {
	if body == nil {
		return
	}
	maxDrain := defaultMaxDrain
	if c.config.MaxResponseBodySize > 0 && c.config.MaxResponseBodySize < maxDrain {
		maxDrain = c.config.MaxResponseBodySize
	}
	// io.Discard implements io.ReaderFrom with its own internal pooled buffer,
	// so io.Copy(io.Discard, ...) already avoids per-call heap allocation.
	drainLr := getLimitReader(body, maxDrain)
	_, _ = io.Copy(io.Discard, drainLr) // best-effort drain for connection reuse
	putLimitReader(drainLr)
	_ = body.Close() // best-effort cleanup
}

// executeWithRetry executes a request with intelligent retry logic.
// Optimized for performance with minimal allocations and efficient error handling.
func (c *Client) executeWithRetry(req *Request) (*Response, error) {
	// Determine max retries: maxRetriesUnset = not configured (use config default), 0 = explicitly disabled
	maxRetries := req.MaxRetries()
	if maxRetries < 0 {
		if c.config.CustomRetryPolicy != nil {
			maxRetries = c.config.CustomRetryPolicy.MaxRetries()
		} else {
			maxRetries = c.retryEngine.MaxRetries()
		}
	}
	// A custom policy returning a negative MaxRetries violates the RetryPolicy
	// contract. Clamp to 0 so the request still executes once via the fast
	// path below — without this it fell into a never-entered retry loop and
	// returned "request failed after N attempts" without ever sending.
	if maxRetries < 0 {
		maxRetries = 0
	}

	// SAFETY: non-idempotent methods (POST, PATCH, custom methods) are not
	// retried by default. A timeout or retryable 5xx may arrive after the
	// server already committed the operation, and replaying the buffered body
	// would duplicate it (duplicate payment, double insert, ...). Opt in via
	// Config.RetryNonIdempotent or the WithRetryNonIdempotent per-request
	// option. Zeroing the budget here also takes the fast path below, which
	// skips the 100MB retry body buffering — large POST bodies stream instead
	// of being buffered solely for replay.
	if maxRetries > 0 && !isIdempotentMethod(req.Method()) {
		allowNonIdempotent := c.config.RetryNonIdempotent
		if override := req.RetryNonIdempotent(); override != nil {
			allowNonIdempotent = *override
		}
		if !allowNonIdempotent {
			maxRetries = 0
		}
	}

	// Fast path: no retries configured (most common case)
	// Skip deep copy since request is only executed once — original req
	// is returned to pool by caller's defer putRequest regardless.
	if maxRetries == 0 {
		// Attach a per-request proxy recorder so the transport's Proxy
		// callback can report which proxy it selected. Only when a proxy is
		// actually configured — keeps the direct-connection path allocation-free.
		var proxyRec *connection.ProxyRecorder
		if c.connectionPool != nil && c.connectionPool.HasProxy() {
			proxyRec = &connection.ProxyRecorder{}
			baseCtx := req.Context()
			if baseCtx == nil {
				baseCtx = backgroundCtx
			}
			// Per-request proxy rotation: even without retries, reserve a unique
			// proxy index and close idle connections so the transport calls Proxy()
			// instead of reusing the previous request's tunnel.
			if c.config.ProxyRotatePerRequest {
				if baseIdx, ok := c.connectionPool.NextProxyIndex(); ok {
					baseCtx = connection.WithProxyAttempt(baseCtx, baseIdx)
					c.connectionPool.CloseIdleConnections()
				}
			}
			req.SetContext(connection.WithProxyRecorder(baseCtx, proxyRec))
		}
		resp, err := c.executeRequest(req, true)
		if err != nil {
			return nil, classifyError(err, req.URL(), req.Method(), 1)
		}
		if resp == nil {
			// Defensive: executeRequest must not return (nil, nil); the slow
			// path guards the same contract (see its fallback below), and
			// SetAttempts on a nil resp would panic here.
			return nil, classifyError(errors.New("no response from transport"), req.URL(), req.Method(), 1)
		}
		resp.SetAttempts(1)
		resp.SetProxyURL(proxyRec.Last())
		return resp, nil
	}

	// Slow path: retry loop
	policy := types.RetryPolicy(c.retryEngine)
	if c.config.CustomRetryPolicy != nil {
		policy = c.config.CustomRetryPolicy
	}

	// Shared timeout context for all retry attempts: WithTimeout is a total
	// budget across retries, not per-attempt. Without this, each retry gets a
	// fresh full timeout, allowing WithTimeout(900s)+3 retries to take ~3600s
	// instead of 900s.
	retryCtx := req.Context()
	if retryCtx == nil {
		retryCtx = backgroundCtx
	}
	retryTimeout := effectiveRequestTimeout(c, req)
	var overallCancel context.CancelFunc
	retryCtx, overallCancel = wrapCtxIfFarther(retryCtx, retryTimeout)
	req.SetContext(retryCtx)
	// Ensure overallCancel is called on all non-success exit paths.
	// For streaming success, overallCancel is transferred to the response
	// and set to nil before this defer runs.
	if overallCancel != nil {
		defer func() {
			if overallCancel != nil {
				overallCancel()
			}
		}()
	}

	// Cache sanitized URL once — URL doesn't change across retry attempts
	sanitizedURL := validation.SanitizeURL(req.URL())
	reqMethod := req.Method()

	var lastErr error
	var lastResp *Response

	// Buffer io.Reader body for retry safety. io.Reader is consumed on
	// first use, so subsequent retry attempts would send an empty body.
	// Converting to []byte allows Build() to create a fresh reader per attempt.
	// SECURITY: Cap at maxRetryBodySize to prevent OOM from large streams.
	if req.body != nil {
		if r, ok := req.body.(io.Reader); ok {
			// Fail fast on an already-cancelled context BEFORE buffering up to
			// 100MB: ctx cancellation does not interrupt reading a caller
			// supplied source (e.g. a file), and the request can only fail
			// with the same error class after the read.
			if retryCtx != nil {
				if err := retryCtx.Err(); err != nil {
					if overallCancel != nil {
						overallCancel()
					}
					return nil, classifyError(err, req.URL(), req.Method(), 0)
				}
			}

			const maxRetryBodySize int64 = 100 * 1024 * 1024 // 100MB
			limited := io.LimitReader(r, maxRetryBodySize+1)
			buf, readErr := io.ReadAll(limited)
			if readErr != nil {
				closeBodyReader(r) // aborting: the caller's reader is not reused
				if overallCancel != nil {
					overallCancel()
				}
				return nil, classifyError(fmt.Errorf("buffer request body failed: %w", readErr), req.URL(), req.Method(), 0)
			}
			if int64(len(buf)) > maxRetryBodySize {
				closeBodyReader(r) // aborting (body too large to retry)
				if overallCancel != nil {
					overallCancel()
				}
				return nil, classifyError(
					fmt.Errorf("retry not supported for streaming bodies exceeding %d bytes", maxRetryBodySize),
					req.URL(), req.Method(), 0,
				)
			}
			// The buffered copy replaces the caller's reader from here on —
			// close the original so it cannot leak (see closeBodyReader).
			closeBodyReader(r)
			req.body = buf
		}
	}

	// Deterministic proxy rotation: when a proxy pool is configured with
	// status-based rotation (ExtraRetryableStatusCodes), reserve a unique base
	// proxy index per request via NextProxyIndex. Each retry attempt then uses
	// baseProxyIdx + attempt as the SelectIndex argument, guaranteeing:
	//   - Inter-request rotation: different requests get different base indices.
	//   - Intra-request rotation: retry N lands on a different proxy than N-1.
	//   - Redirect-safe: all Proxy calls within one attempt share the same index,
	//     so redirect-following cannot desynchronize the rotation.
	retryBaseCtx := req.Context()
	// Attach the proxy recorder before the loop so every attempt (and each
	// redirect hop within an attempt) records its selected proxy; Last()
	// after the final attempt yields the proxy that produced the response.
	// As in the fast path, only when a proxy is actually configured.
	var proxyRec *connection.ProxyRecorder
	if c.connectionPool != nil && c.connectionPool.HasProxy() {
		proxyRec = &connection.ProxyRecorder{}
		retryBaseCtx = connection.WithProxyRecorder(retryBaseCtx, proxyRec)
	}
	baseProxyIdx := 0
	// Rotation requires an actual proxy pool: with a custom transport
	// (connectionPool == nil) the flag must zero out, or every retry would
	// attach a WithProxyAttempt value no consumer reads while debug logs
	// printed phantom proxy indexes.
	useProxyRotation := (len(c.config.ExtraRetryableStatusCodes) > 0 || c.config.ProxyRotatePerRequest) && c.connectionPool != nil
	if useProxyRotation {
		var ok bool
		baseProxyIdx, ok = c.connectionPool.NextProxyIndex()
		useProxyRotation = ok // false if pool wasn't actually created
		// Per-request rotation: close idle connections from previous requests
		// so the transport re-evaluates Proxy() instead of reusing the
		// previous request's tunnel.
		if useProxyRotation && c.config.ProxyRotatePerRequest {
			c.connectionPool.CloseIdleConnections()
		}
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if useProxyRotation {
			req.SetContext(connection.WithProxyAttempt(retryBaseCtx, baseProxyIdx+attempt))
		} else {
			req.SetContext(retryBaseCtx)
		}

		resp, err := c.executeRequest(req, false)

		// Computed once and reused by the debug log and the retry decision
		// below: RetryPolicy implementations are caller-supplied and may carry
		// side effects (counters, circuit breakers), so HTTPC_DEBUG must not
		// cause a second ShouldRetry call per attempt.
		respShouldRetry := false
		if resp != nil {
			respShouldRetry = policy.ShouldRetry(resp, nil, attempt)
		}

		if httpcDebug() {
			if err != nil {
				// SECURITY (SEC-001 M-1): print the sanitized classification,
				// not the raw error — net/http *url.Error embeds the full
				// request URL (query string included) in its message.
				dbgErr := classifyErrorWithSanitizedURL(err, sanitizedURL, reqMethod, attempt+1)
				fmt.Fprintf(os.Stderr, "[httpc] attempt=%d ERROR: %s (retryable=%v)\n",
					attempt, dbgErr.Error(), dbgErr.IsRetryable())
			} else if resp != nil {
				fmt.Fprintf(os.Stderr, "[httpc] attempt=%d status=%d shouldRetry=%v attemptLtMax=%v proxyIdx=%d\n",
					attempt, resp.StatusCode(), respShouldRetry, attempt < maxRetries,
					func() int {
						if useProxyRotation {
							return baseProxyIdx + attempt
						}
						return -1
					}())
			}
		}

		if err != nil {
			clientErr := classifyErrorWithSanitizedURL(err, sanitizedURL, reqMethod, attempt+1)
			lastErr = clientErr

			// When proxy rotation is active, proxy connection failures are
			// always retryable regardless of the normal retryability
			// classification — the next attempt uses SelectIndex to pick a
			// different proxy. This handles permanently broken proxies (invalid
			// port, unreachable host) without manual pre-filtering.
			proxyConnFailed := useProxyRotation && attempt < maxRetries &&
				errors.Is(err, connection.ErrProxyConnectionFailed)

			// Fast path: the retry budget always wins.
			if attempt >= maxRetries {
				releaseLastResp(&lastResp)
				clientErr.Attempts = attempt + 1
				return nil, clientErr
			}

			// The retry policy decides. A custom RetryPolicy overrides the
			// built-in classification (Config docs: "If set, it overrides the
			// built-in retry logic") — previously the built-in IsRetryable()
			// veto ran first, so a custom policy was never even consulted for
			// an error the classifier had marked non-retryable. For the
			// built-in engine policy this is behavior-preserving:
			// retryEngine.ShouldRetry(nil, err, ·) delegates to the exact
			// same classifyError call the veto used.
			if !proxyConnFailed && !policy.ShouldRetry(nil, err, attempt) {
				releaseLastResp(&lastResp)
				clientErr.Attempts = attempt + 1
				return nil, clientErr
			}

			// Calculate delay and sleep
			delay := policy.GetDelay(attempt)

			// Close idle connections to force Proxy() re-evaluation on retry
			// (see comment in the response-path retry block below).
			if useProxyRotation && c.connectionPool != nil {
				c.connectionPool.CloseIdleConnections()
			}

			if sleepErr := c.sleepWithContext(req.Context(), delay); sleepErr != nil {
				releaseLastResp(&lastResp)
				return nil, classifyErrorWithSanitizedURL(sleepErr, sanitizedURL, reqMethod, attempt+1)
			}
			continue
		}

		if resp != nil {
			// Release previous intermediate response to prevent pool leak.
			// Defensive: per-attempt flow (return on success, release-and-nil
			// in the retry branch) leaves lastResp nil here in practice —
			// kept as a guard, not a live path.
			if lastResp != nil && lastResp != resp {
				ReleaseResponse(lastResp)
			}
			lastResp = resp

			// Check if response status is retryable, reusing the decision
			// computed above the debug block (single policy call per attempt).
			if respShouldRetry && attempt < maxRetries {
				// Compute the delay while resp is still valid — policies
				// implementing responseDelayPolicy (e.g. the built-in engine
				// policy's Retry-After support) may read headers from it.
				var delay time.Duration
				if dp, ok := policy.(responseDelayPolicy); ok {
					delay = dp.GetDelayWithResponse(attempt, resp)
				} else {
					delay = policy.GetDelay(attempt)
				}

				if httpcDebug() {
					fmt.Fprintf(os.Stderr, "[httpc] attempt=%d retrying after %v delay\n", attempt, delay)
				}

				// Release this discarded response now rather than carrying it as
				// lastResp across the backoff sleep and the next attempt. For a
				// streaming response the body reader pins a connection until
				// ReleaseResponse closes it, so releasing before the sleep frees
				// that slot for the full delay instead of holding it idle.
				ReleaseResponse(resp)
				lastResp = nil

				// Close idle connections so the next attempt's transport
				// MUST call Proxy() again to select a (different) proxy.
				// Without this, HTTP/2 connection reuse through a CONNECT
				// tunnel silently reuses the failed attempt's tunnel,
				// bypassing Proxy() and defeating proxy rotation.
				if useProxyRotation && c.connectionPool != nil {
					c.connectionPool.CloseIdleConnections()
				}

				if sleepErr := c.sleepWithContext(req.Context(), delay); sleepErr != nil {
					return nil, classifyErrorWithSanitizedURL(sleepErr, sanitizedURL, reqMethod, attempt+1)
				}
				continue
			}

			// Success - set attempt count and return
			resp.SetAttempts(attempt + 1)
			resp.SetProxyURL(proxyRec.Last())
			// Streaming responses need the cancel to stay alive until
			// ReleaseResponse; currently defensive (the retry deadline makes
			// the per-attempt streamCancel unreachable), but chaining is cheap.
			transferCancelOwnership(resp, &overallCancel)
			return resp, nil
		}
	}

	// Defensive fallback: loop exited without early return.
	// This can only happen if executeRequest returns (nil, nil), which should
	// never occur with the current implementation. Included for robustness.
	if lastResp != nil {
		lastResp.SetAttempts(maxRetries + 1)
		lastResp.SetProxyURL(proxyRec.Last())
		// Same cancel ownership transfer as the success path above.
		transferCancelOwnership(lastResp, &overallCancel)
		return lastResp, nil
	}

	if lastErr != nil {
		if clientErr, ok := lastErr.(*ClientError); ok {
			clientErr.Attempts = maxRetries + 1
			clientErr.Type = ErrorTypeRetryExhausted
			return nil, clientErr
		}
		return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
	}

	return nil, fmt.Errorf("request failed after %d attempts", maxRetries+1)
}

// captureRequestHeaders builds a complete header map from an http.Request by
// taking ownership of httpReq.Header and enriching it with Content-Length
// (which net/http stores as a Request field, not in the Header map).
//
// Rather than cloning the pooled header map built by requestProcessor.Build, it
// transfers ownership to the caller (which retains it via the Result). This
// mirrors the response-header transfer pattern (Response.TransferHeaders) and
// removes the per-request CloneHeader copy — saving the clone's map allocation,
// its backing-array allocation, and the value-copy loop.
//
// httpReq.Header is set to nil so the caller's deferred putHTTPHeader observes
// nil at function-exit and skips recycling a map still referenced by the Result.
// On error paths where this function is never reached, httpReq.Header stays
// non-nil and is recycled by the deferred putHTTPHeader as before. Consuming one
// pooled map per success-path request is safe: sync.Pool simply allocates a fresh
// map on the next get when the pool is empty — no leak, no double-recycle.
//
// Uses direct map lookups with pre-canonicalized keys ("Content-Length" is
// already canonical) to avoid the textproto.CanonicalMIMEHeaderKey overhead of
// h.Get. Host is intentionally NOT captured: net/http writes the Host header
// from Request.Host (Host is in reqWriteExcludeHeader, never read from the
// Header map), so synthesizing a Host entry here would misrepresent the wire
// as a caller-supplied header while costing a backing-slice allocation on
// every request.
func captureRequestHeaders(httpReq *http.Request) http.Header {
	if httpReq == nil {
		return nil
	}
	h := httpReq.Header
	httpReq.Header = nil // transfer ownership; deferred putHTTPHeader reads nil and skips
	if h == nil {
		h = make(http.Header, 4)
	}
	// Synthesize Content-Length as a single-value entry built directly with
	// http.Header.Set's semantics (a one-element slice). It needs no batch
	// backing array: on the common GET path ContentLength is 0 and the
	// previous make([]string, 0, 2) was a pure per-request allocation.
	if httpReq.ContentLength > 0 && headerValueIsEmpty(h, hdrContentLength) {
		var numBuf [20]byte
		h[hdrContentLength] = []string{string(strconv.AppendInt(numBuf[:0], httpReq.ContentLength, 10))}
	}
	return h
}

// releaseLastResp releases the intermediate response object back to the pool.
// Takes a pointer to the caller's lastResp so it can nil the reference after release,
// preventing double-release in deferred cleanup paths.
func releaseLastResp(lastResp **Response) {
	if *lastResp != nil {
		ReleaseResponse(*lastResp)
		*lastResp = nil
	}
}

const (
	defaultMaxDrain int64 = 10 * 1024 * 1024 // 10MB
)

// backgroundCtx is a convenience alias for context.Background(), used as the
// default context when no context is provided. context.Background() is an
// immutable singleton, so this is purely stylistic — it does not violate
// the "do not store context in struct" guideline.
// Note: the public httpc package also defines its own backgroundCtx — this is
// intentional since unexported vars cannot be shared across packages.
var backgroundCtx = context.Background()

// executeRequest executes a single HTTP request with comprehensive error handling.
// When skipCopy is true, the request is used directly without deep copy (safe when
// the caller guarantees single-use, i.e., no retries).
func (c *Client) executeRequest(req *Request, skipCopy bool) (*Response, error) {
	// Context setup with timeout handling
	execCtx := req.Context()
	if execCtx == nil {
		execCtx = backgroundCtx
	}

	timeout := effectiveRequestTimeout(c, req)

	// Optimized: only create new context if absolutely necessary
	var streamCancel context.CancelFunc
	execCtx, streamCancel = wrapCtxIfFarther(execCtx, timeout)
	// Always defer cancel to prevent timer leaks on early error returns.
	// For streaming requests, cancelFunc is stored in the Response and the
	// defer becomes a no-op after setCancelFuncToNil is called.
	cancelCtx := streamCancel
	if cancelCtx != nil {
		defer func() {
			if cancelCtx != nil {
				cancelCtx()
			}
		}()
	}
	// setCancelFuncToNil is called after storing the cancel in the streaming
	// response, preventing the deferred cleanup from double-cancelling.
	setCancelFuncToNil := func() { cancelCtx = nil }

	// Fast pre-flight cancellation check. Err() instead of a select on
	// Done(): the select forces cancelCtx to materialize its done channel
	// eagerly (net/http allocates it later during the round trip anyway),
	// and the direct error read expresses the "already canceled or
	// expired" intent without the select machinery.
	if err := execCtx.Err(); err != nil {
		return nil, classifyErrorWithSanitizedURL(err, validation.SanitizeURL(req.URL()), req.Method(), 0)
	}

	// Get a pooled Request copy — deep copy reference types to prevent cross-retry mutation
	// Optimization: skip deep copy in no-retry fast path (skipCopy=true) to avoid
	// unnecessary map/slice allocations for headers, query params, and cookies.
	var reqCopy *Request
	if skipCopy {
		req.context = execCtx
		reqCopy = req
	} else {
		reqCopy = c.getExecRequest()
		*reqCopy = *req
		reqCopy.context = execCtx
		if req.headers != nil {
			reqCopy.headers = getHeadersMap()
			maps.Copy(reqCopy.headers, req.headers)
		}
		if req.queryParams != nil {
			reqCopy.queryParams = getQueryParamsMap()
			maps.Copy(reqCopy.queryParams, req.queryParams)
		}
		if len(req.cookies) > 0 {
			reqCopy.cookies = make([]http.Cookie, len(req.cookies))
			copy(reqCopy.cookies, req.cookies)
		}
		defer c.putExecRequest(reqCopy)
	}

	followRedirects := effectiveFollowRedirects(c.config, req)
	maxRedirects := c.config.MaxRedirects
	if req.MaxRedirects() != nil {
		maxRedirects = *req.MaxRedirects()
	}
	// Set redirect policy via context for thread-safety
	// SECURITY: settings must be returned to pool to prevent memory leak
	var redirectSettings *redirectSettings
	reqCopy.context, redirectSettings = c.transport.SetRedirectPolicy(execCtx, followRedirects, maxRedirects)
	if redirectSettings != nil {
		defer putRedirectSettings(redirectSettings)
	}

	// Lazy sanitized URL: only compute when an error occurs.
	// Most requests succeed, so this avoids the SanitizeURL allocation entirely
	// on the happy path.
	var sanitizedURL string
	sanitizeOnce := func() string {
		if sanitizedURL == "" {
			sanitizedURL = validation.SanitizeURL(req.URL())
		}
		return sanitizedURL
	}

	// Invoke OnRequest callback before building the HTTP request
	if reqCopy.onRequest != nil {
		if err := reqCopy.onRequest(reqCopy); err != nil {
			return nil, classifyErrorWithSanitizedURL(fmt.Errorf("onRequest callback failed: %w", err), sanitizeOnce(), req.Method(), 0)
		}
	}

	httpReq, err := c.requestProcessor.Build(reqCopy)
	if err != nil {
		return nil, classifyErrorWithSanitizedURL(err, sanitizeOnce(), req.Method(), 0)
	}
	// Read httpReq.Header at defer-run time, not at registration: captureRequestHeaders
	// transfers header ownership to the Result on success and nils the field, so the
	// closure observes nil and skips recycling. On error paths captureRequestHeaders is
	// never reached, the field stays non-nil, and the header is recycled normally.
	defer func() { putHTTPHeader(httpReq.Header) }()

	httpResp, err := c.transport.RoundTrip(httpReq)

	if err != nil {
		return nil, classifyErrorWithSanitizedURL(err, sanitizeOnce(), req.Method(), 0)
	}
	if httpResp == nil {
		// Defensive: a caller-supplied RoundTripper may return (nil, nil).
		// The buffered-path defer and responseProcessor.Process both
		// dereference httpResp, so fail here instead of panicking. This keeps
		// the (nil, nil) guard in executeWithRetry reachable.
		return nil, classifyErrorWithSanitizedURL(errors.New("no response from transport"), sanitizeOnce(), req.Method(), 0)
	}

	// Streaming mode: skip body buffering, hand raw reader to caller.
	// Caller is responsible for closing the body reader.
	if reqCopy.StreamBody() {
		// The streaming path hands raw bytes to the caller, bypassing the
		// decompression applied to buffered bodies. Build() requests
		// "Accept-Encoding: identity" for streaming requests, but a
		// non-conforming server may compress anyway — the caller would then
		// silently persist compressed bytes as the "downloaded" file (e.g. a
		// gzip archive with the original's name). Fail loudly instead. A
		// caller-supplied Accept-Encoding (anything other than the engine's
		// identity hint) means the encoding was explicitly requested, so the
		// raw bytes are passed through untouched.
		if enc := strings.TrimSpace(httpResp.Header.Get("Content-Encoding")); enc != "" && !strings.EqualFold(enc, "identity") {
			if ae := httpReq.Header.Get("Accept-Encoding"); ae == "" || ae == "identity" {
				c.drainAndCloseBody(httpResp.Body)
				return nil, classifyErrorWithSanitizedURL(
					fmt.Errorf("streaming response has Content-Encoding %q: server ignored Accept-Encoding: identity; use a buffered (non-streaming) request so the client can decompress it", enc),
					sanitizeOnce(), req.Method(), 0)
			}
		}

		resp := getResponse()
		resp.SetStatusCode(httpResp.StatusCode)
		resp.SetStatus(httpResp.Status)
		resp.SetHeaders(httpResp.Header)
		resp.SetContentLength(httpResp.ContentLength)
		resp.SetProto(httpResp.Proto)
		resp.SetCookies(httpResp.Cookies())
		streamLimit := c.config.MaxResponseBodySize
		if streamLimit <= 0 {
			streamLimit = defaultMaxDecompressedSize
		}
		// Budget one byte beyond the limit so streamBodyReader can distinguish
		// an oversize body (error) from one ending exactly at the limit (clean
		// EOF) — see its Read implementation.
		lr := getLimitReader(httpResp.Body, streamLimit+1)
		resp.rawBodyReader = &streamBodyReader{reader: lr, source: httpResp.Body, limit: streamLimit}
		resp.cancelFunc = streamCancel
		setCancelFuncToNil() // Prevent deferred cancel; ReleaseResponse handles cleanup

		if httpResp.Request != nil {
			resp.SetRequestHeaders(captureRequestHeaders(httpResp.Request))
			if httpResp.Request.URL != nil {
				resp.SetRequestURL(httpResp.Request.URL.String())
			}
			resp.SetRequestMethod(httpResp.Request.Method)
		}

		// Capture redirect metadata for streaming responses. Reading the chain
		// straight off the settings object avoids a ctx.Value chain walk (the
		// settings pointer is already in scope from SetRedirectPolicy).
		if redirectSettings != nil {
			if redirectChain := redirectSettings.getChain(); len(redirectChain) > 0 {
				resp.SetRedirectChain(redirectChain)
				resp.SetRedirectCount(len(redirectChain))
			}
		}

		// Invoke OnResponse callback for streaming responses
		if reqCopy.onResponse != nil {
			if err := reqCopy.onResponse(resp); err != nil {
				ReleaseResponse(resp)
				return nil, classifyErrorWithSanitizedURL(fmt.Errorf("onResponse callback failed: %w", err), sanitizeOnce(), req.Method(), 0)
			}
		}

		return resp, nil
	}

	defer func() {
		if httpResp != nil && httpResp.Body != nil {
			c.drainAndCloseBody(httpResp.Body)
		}
	}()

	resp, err := c.responseProcessor.Process(httpResp)
	if err != nil {
		return nil, classifyErrorWithSanitizedURL(err, sanitizeOnce(), req.Method(), 0)
	}

	// Read the chain straight off the settings object (see the streaming-path
	// note above) rather than walking the request context.
	if redirectSettings != nil {
		if redirectChain := redirectSettings.getChain(); len(redirectChain) > 0 {
			resp.SetRedirectChain(redirectChain)
			resp.SetRedirectCount(len(redirectChain))
		}
	}

	if httpResp.Request != nil {
		resp.SetRequestHeaders(captureRequestHeaders(httpResp.Request))
		// Set the actual request URL and method
		if httpResp.Request.URL != nil {
			resp.SetRequestURL(httpResp.Request.URL.String())
		}
		resp.SetRequestMethod(httpResp.Request.Method)
	}

	// Invoke OnResponse callback after response processing
	if reqCopy.onResponse != nil {
		if err := reqCopy.onResponse(resp); err != nil {
			ReleaseResponse(resp)
			return nil, classifyErrorWithSanitizedURL(fmt.Errorf("onResponse callback failed: %w", err), sanitizeOnce(), req.Method(), 0)
		}
	}

	return resp, nil
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return c.closed.Load() == 1
}

// Close releases the client's resources, including the connection pool and
// transport. It is safe to call multiple times; subsequent calls are no-ops.
// Returns any error encountered while closing sub-components.
func (c *Client) Close() error {
	var closeErr error

	c.closeOnce.Do(func() {
		c.closed.Store(1)

		if c.connectionPool != nil {
			if err := c.connectionPool.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("failed to close connection pool: %w", err))
			}
		}

		if c.transport != nil {
			if err := c.transport.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("failed to close transport: %w", err))
			}
		}
	})

	return closeErr
}

// RequestOption is a function that modifies a pooled *Request before it is
// sent. It takes types.RequestMutator (rather than the concrete *Request) so
// options compose through the same abstraction as middleware. The facade's
// public RequestOption is an interface; the facade adapts it to this func type
// at the boundary (see httpc.toEngineOptions), so the engine's
// option-application loop stays a plain function call. Built-in facade options
// that need engine-specific fields reach them via a type assertion on the
// concrete *Request.
type RequestOption func(types.RequestMutator) error

// ApplyOptions runs a list of options against a concrete *Request. The
// concrete type is what the engine pools and passes onward; RequestOption's
// interface parameter lets external options stay decoupled from it.
//
// Exported (rather than unexported) because the httpc facade applies options
// to its own pooled *Request before running the middleware chain — see
// clientImpl.executeRequest.
func ApplyOptions(req *Request, options []RequestOption) error {
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(req); err != nil {
			return err
		}
	}
	return nil
}

// clientOption is a functional option for configuring the Client.
// It exists solely as a seam for tests to inject a custom transportManager;
// it is not instance configuration — new client settings belong on Config
// (the engine follows the same Config-struct convention as the public API).
type clientOption func(*clientOptions)

type clientOptions struct {
	customTransport transportManager
}
