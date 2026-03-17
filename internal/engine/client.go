package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/security"
	"github.com/cybergodev/httpc/internal/types"
)

type Client struct {
	config *Config

	transport         TransportManager
	requestProcessor  *RequestProcessor
	responseProcessor *ResponseProcessor
	retryEngine       *RetryEngine
	validator         *security.Validator

	connectionPool *connection.PoolManager

	// requestPool reduces allocations for Request objects
	requestPool sync.Pool
	// execRequestPool reduces allocations for Request copies in executeRequest
	execRequestPool sync.Pool
	// securityRequestPool reduces allocations for security.Request objects
	securityRequestPool sync.Pool

	// metrics tracks request statistics
	metrics *Metrics

	closed int32

	closeOnce sync.Once
}

// Config defines the HTTP client configuration.
// Config should be treated as immutable after creation.
type Config struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	ProxyURL              string

	// System proxy configuration
	EnableSystemProxy bool // Automatically detect and use system proxy settings

	TLSConfig           *tls.Config
	MinTLSVersion       uint16
	MaxTLSVersion       uint16
	InsecureSkipVerify  bool
	MaxResponseBodySize int64
	ValidateURL         bool
	ValidateHeaders     bool
	AllowPrivateIPs     bool
	StrictContentLength bool

	MaxRetries    int
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	BackoffFactor float64
	Jitter        bool

	// CustomRetryPolicy allows providing a custom retry policy implementation.
	// If set, it overrides the built-in retry logic.
	CustomRetryPolicy types.RetryPolicy

	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	MaxRedirects    int
	EnableHTTP2     bool

	CookieJar     http.CookieJar
	EnableCookies bool

	// DNS configuration
	EnableDoH   bool
	DoHCacheTTL time.Duration

	// Redirect whitelist configuration
	RedirectWhitelist *security.DomainWhitelist

	// Certificate pinning
	CertificatePinner security.CertificatePinner
}

// RequestCallback is a callback function invoked before a request is sent.
type RequestCallback func(req *Request) error

// ResponseCallback is a callback function invoked after a response is received.
type ResponseCallback func(resp *Response) error

type Request struct {
	method          string
	url             string
	headers         map[string]string
	queryParams     map[string]any
	body            any
	timeout         time.Duration
	maxRetries      int
	context         context.Context
	cookies         []http.Cookie
	followRedirects *bool
	maxRedirects    *int
	onRequest       RequestCallback
	onResponse      ResponseCallback
}

// Compile-time interface check
var _ types.RequestMutator = (*Request)(nil)

// Accessors (implement RequestMutator)
func (r *Request) Method() string              { return r.method }
func (r *Request) URL() string                 { return r.url }
func (r *Request) Headers() map[string]string  { return r.headers }
func (r *Request) QueryParams() map[string]any { return r.queryParams }
func (r *Request) Body() any                   { return r.body }
func (r *Request) Timeout() time.Duration      { return r.timeout }
func (r *Request) MaxRetries() int             { return r.maxRetries }
func (r *Request) Context() context.Context    { return r.context }
func (r *Request) Cookies() []http.Cookie      { return r.cookies }
func (r *Request) FollowRedirects() *bool      { return r.followRedirects }
func (r *Request) MaxRedirects() *int          { return r.maxRedirects }

// Mutators
func (r *Request) SetMethod(v string)             { r.method = v }
func (r *Request) SetURL(v string)                { r.url = v }
func (r *Request) SetHeaders(v map[string]string) { r.headers = v }
func (r *Request) SetHeader(key, value string) {
	if r.headers == nil {
		r.headers = make(map[string]string, 4) // Pre-allocate with capacity for common headers
	}
	r.headers[key] = value
}
func (r *Request) SetQueryParams(v map[string]any) { r.queryParams = v }
func (r *Request) SetBody(v any)                   { r.body = v }
func (r *Request) SetTimeout(v time.Duration)      { r.timeout = v }
func (r *Request) SetMaxRetries(v int)             { r.maxRetries = v }
func (r *Request) SetContext(v context.Context)    { r.context = v }
func (r *Request) SetCookies(v []http.Cookie)      { r.cookies = v }
func (r *Request) SetFollowRedirects(v *bool)      { r.followRedirects = v }
func (r *Request) SetMaxRedirects(v *int)          { r.maxRedirects = v }

// Callback accessors
func (r *Request) OnRequest() RequestCallback        { return r.onRequest }
func (r *Request) OnResponse() ResponseCallback      { return r.onResponse }
func (r *Request) SetOnRequest(cb RequestCallback)   { r.onRequest = cb }
func (r *Request) SetOnResponse(cb ResponseCallback) { r.onResponse = cb }

// Response represents an HTTP response.
// Response objects are safe to read from multiple goroutines after they are returned.
type Response struct {
	statusCode     int
	status         string
	headers        http.Header
	body           string
	rawBody        []byte
	contentLength  int64
	proto          string
	duration       time.Duration
	attempts       int
	cookies        []*http.Cookie
	redirectChain  []string
	redirectCount  int
	requestHeaders http.Header // Actual headers sent with the request
	requestURL     string      // The actual URL that was requested (with query params)
	requestMethod  string      // The HTTP method used
}

// Compile-time interface check
var _ types.ResponseMutator = (*Response)(nil)

// Accessors (implement ResponseAccessor)
func (r *Response) StatusCode() int             { return r.statusCode }
func (r *Response) Status() string              { return r.status }
func (r *Response) Headers() http.Header        { return r.headers }
func (r *Response) Body() string                { return r.body }
func (r *Response) RawBody() []byte             { return r.rawBody }
func (r *Response) ContentLength() int64        { return r.contentLength }
func (r *Response) Proto() string               { return r.proto }
func (r *Response) Duration() time.Duration     { return r.duration }
func (r *Response) Attempts() int               { return r.attempts }
func (r *Response) Cookies() []*http.Cookie     { return r.cookies }
func (r *Response) RedirectChain() []string     { return r.redirectChain }
func (r *Response) RedirectCount() int          { return r.redirectCount }
func (r *Response) RequestHeaders() http.Header { return r.requestHeaders }
func (r *Response) RequestURL() string          { return r.requestURL }
func (r *Response) RequestMethod() string       { return r.requestMethod }

// Mutators (implement ResponseMutator)
func (r *Response) SetStatusCode(v int)             { r.statusCode = v }
func (r *Response) SetStatus(v string)              { r.status = v }
func (r *Response) SetHeaders(v http.Header)        { r.headers = v }
func (r *Response) SetBody(v string)                { r.body = v }
func (r *Response) SetRawBody(v []byte)             { r.rawBody = v }
func (r *Response) SetContentLength(v int64)        { r.contentLength = v }
func (r *Response) SetProto(v string)               { r.proto = v }
func (r *Response) SetDuration(v time.Duration)     { r.duration = v }
func (r *Response) SetAttempts(v int)               { r.attempts = v }
func (r *Response) SetCookies(v []*http.Cookie)     { r.cookies = v }
func (r *Response) SetRedirectChain(v []string)     { r.redirectChain = v }
func (r *Response) SetRedirectCount(v int)          { r.redirectCount = v }
func (r *Response) SetRequestHeaders(v http.Header) { r.requestHeaders = v }
func (r *Response) SetRequestURL(v string)          { r.requestURL = v }
func (r *Response) SetRequestMethod(v string)       { r.requestMethod = v }

// SetHeader sets a header with multiple values (implements ResponseMutator)
func (r *Response) SetHeader(key string, values ...string) {
	if r.headers == nil {
		r.headers = make(http.Header)
	}
	r.headers[key] = values
}

func NewClient(config *Config, opts ...ClientOption) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Process options
	options := &clientOptions{}
	for _, opt := range opts {
		opt(options)
	}

	client := &Client{
		config:  config,
		metrics: &Metrics{},
		requestPool: sync.Pool{
			New: func() any {
				return &Request{}
			},
		},
		execRequestPool: sync.Pool{
			New: func() any {
				return &Request{}
			},
		},
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
		connConfig := connection.DefaultConfig()
		connConfig.MaxIdleConns = config.MaxIdleConns
		connConfig.MaxIdleConnsPerHost = config.MaxIdleConnsPerHost
		connConfig.MaxConnsPerHost = config.MaxConnsPerHost
		connConfig.DialTimeout = config.DialTimeout
		connConfig.KeepAlive = config.KeepAlive
		connConfig.TLSHandshakeTimeout = config.TLSHandshakeTimeout
		connConfig.ResponseHeaderTimeout = config.ResponseHeaderTimeout
		connConfig.IdleConnTimeout = config.IdleConnTimeout
		connConfig.MinTLSVersion = config.MinTLSVersion
		connConfig.MaxTLSVersion = config.MaxTLSVersion
		connConfig.InsecureSkipVerify = config.InsecureSkipVerify
		connConfig.EnableHTTP2 = config.EnableHTTP2
		connConfig.ProxyURL = config.ProxyURL
		connConfig.EnableSystemProxy = config.EnableSystemProxy
		connConfig.CookieJar = config.CookieJar
		connConfig.AllowPrivateIPs = config.AllowPrivateIPs
		connConfig.EnableDoH = config.EnableDoH
		connConfig.DoHCacheTTL = config.DoHCacheTTL

		client.connectionPool, err = connection.NewPoolManager(connConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection pool: %w", err)
		}

		client.transport, err = NewTransport(config, client.connectionPool)
		if err != nil {
			return nil, fmt.Errorf("failed to create transport: %w", err)
		}
	}

	client.requestProcessor = NewRequestProcessor(config)
	client.responseProcessor = NewResponseProcessor(config)
	client.retryEngine = NewRetryEngine(config)

	validatorConfig := &security.Config{
		ValidateURL:         config.ValidateURL,
		ValidateHeaders:     config.ValidateHeaders,
		MaxResponseBodySize: config.MaxResponseBodySize,
		AllowPrivateIPs:     config.AllowPrivateIPs,
	}
	client.validator = security.NewValidatorWithConfig(validatorConfig)

	return client, nil
}

func (c *Client) Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, fmt.Errorf("client is closed")
	}

	startTime := time.Now()

	// Get Request from pool and reset fields
	req := c.getRequest()
	req.SetMethod(method)
	req.SetURL(url)
	req.SetContext(ctx)
	// Reset other fields to zero values
	req.SetHeaders(nil)
	req.SetQueryParams(nil)
	req.SetBody(nil)
	req.SetTimeout(0)
	req.SetMaxRetries(0)
	req.SetCookies(nil)
	req.SetFollowRedirects(nil)
	req.SetMaxRedirects(nil)

	// Ensure request is returned to pool after processing
	defer c.putRequest(req)

	for _, option := range options {
		if option != nil {
			if err := option(req); err != nil {
				c.metrics.RecordRequest(time.Since(startTime).Nanoseconds(), false)
				return nil, fmt.Errorf("failed to apply request option: %w", err)
			}
		}
	}

	// Use pooled security.Request for validation
	secReq := c.getSecurityRequest()
	secReq.Method = req.Method()
	secReq.URL = req.URL()
	secReq.Headers = req.Headers()
	secReq.QueryParams = req.QueryParams()
	secReq.Body = req.Body()

	validationErr := c.validator.ValidateRequest(secReq)
	c.putSecurityRequest(secReq)

	if validationErr != nil {
		c.metrics.RecordRequest(time.Since(startTime).Nanoseconds(), false)
		return nil, fmt.Errorf("request validation failed: %w", validationErr)
	}

	response, err := c.executeWithRetry(req)
	duration := time.Since(startTime)

	if err != nil {
		c.metrics.RecordRequest(duration.Nanoseconds(), false)
		return nil, err
	}

	c.metrics.RecordRequest(duration.Nanoseconds(), true)
	response.SetDuration(duration)
	return response, nil
}

// getRequest retrieves a Request object from the pool with safe type assertion
func (c *Client) getRequest() *Request {
	req, ok := c.requestPool.Get().(*Request)
	if !ok || req == nil {
		// Fallback to new allocation if pool returns wrong type (defensive)
		return &Request{}
	}
	return req
}

// putRequest returns a Request object to the pool
func (c *Client) putRequest(req *Request) {
	// Clear sensitive data and callbacks before returning to pool
	// to prevent memory leaks from callback closures
	req.SetContext(context.Background())
	req.SetBody(nil)
	req.SetOnRequest(nil)
	req.SetOnResponse(nil)
	c.requestPool.Put(req)
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
	// Clear fields to prevent memory leaks
	req.Method = ""
	req.URL = ""
	req.Headers = nil
	req.QueryParams = nil
	req.Body = nil
	c.securityRequestPool.Put(req)
}

// getExecRequest retrieves a Request object from the exec pool for request copies with safe type assertion
func (c *Client) getExecRequest() *Request {
	req, ok := c.execRequestPool.Get().(*Request)
	if !ok || req == nil {
		// Fallback to new allocation if pool returns wrong type (defensive)
		return &Request{}
	}
	return req
}

// putExecRequest returns a Request object to the exec pool
func (c *Client) putExecRequest(req *Request) {
	// Clear all fields to prevent memory leaks
	req.method = ""
	req.url = ""
	req.headers = nil
	req.queryParams = nil
	req.body = nil
	req.timeout = 0
	req.maxRetries = 0
	req.context = nil
	req.cookies = nil
	req.followRedirects = nil
	req.maxRedirects = nil
	req.onRequest = nil
	req.onResponse = nil
	c.execRequestPool.Put(req)
}

func (c *Client) Get(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("GET", url, options...)
}

func (c *Client) Post(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("POST", url, options...)
}

func (c *Client) Put(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("PUT", url, options...)
}

func (c *Client) Patch(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("PATCH", url, options...)
}

func (c *Client) Delete(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("DELETE", url, options...)
}

func (c *Client) Head(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("HEAD", url, options...)
}

func (c *Client) Options(url string, options ...RequestOption) (*Response, error) {
	return c.executeWithDefaultContext("OPTIONS", url, options...)
}

func (c *Client) executeWithDefaultContext(method, url string, options ...RequestOption) (*Response, error) {
	ctx, cancel := c.createDefaultContext()
	defer cancel()
	return c.Request(ctx, method, url, options...)
}

// nopCancelFunc is a no-op cancel function that avoids allocation
// when no timeout is configured
var nopCancelFunc = func() {}

func (c *Client) createDefaultContext() (context.Context, context.CancelFunc) {
	if c.config.Timeout > 0 {
		return context.WithTimeout(context.Background(), c.config.Timeout)
	}
	// Return background context with no-op cancel to avoid allocation
	return context.Background(), nopCancelFunc
}

func (c *Client) sleepWithContext(ctx context.Context, duration time.Duration) error {
	if ctx == nil {
		time.Sleep(duration)
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// executeWithRetry executes a request with intelligent retry logic.
// Optimized for performance with minimal allocations and efficient error handling.
func (c *Client) executeWithRetry(req *Request) (*Response, error) {
	// Determine which retry policy to use
	policy := types.RetryPolicy(c.retryEngine)
	if c.config.CustomRetryPolicy != nil {
		policy = c.config.CustomRetryPolicy
	}

	maxRetries := policy.MaxRetries()
	if req.MaxRetries() > 0 {
		maxRetries = req.MaxRetries()
	}

	var lastErr error
	var lastResp *Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.executeRequest(req)

		if err != nil {
			clientErr := ClassifyError(err, req.URL(), req.Method(), attempt+1)
			lastErr = clientErr

			// Fast path: non-retryable errors
			if !clientErr.IsRetryable() || attempt >= maxRetries {
				clientErr.Attempts = attempt + 1
				return nil, clientErr
			}

			// Check retry policy
			if !policy.ShouldRetry(nil, err, attempt) {
				clientErr.Attempts = attempt + 1
				return nil, clientErr
			}

			// Calculate delay and sleep
			delay := policy.GetDelay(attempt)
			if sleepErr := c.sleepWithContext(req.Context(), delay); sleepErr != nil {
				return nil, ClassifyError(sleepErr, req.URL(), req.Method(), attempt+1)
			}
			continue
		}

		if resp != nil {
			lastResp = resp

			// Check if response status is retryable using policy
			if c.retryEngine.isRetryableStatus(resp.StatusCode()) && attempt < maxRetries {
				if policy.ShouldRetry(resp, nil, attempt) {
					delay := c.retryEngine.GetDelayWithResponse(attempt, resp)
					if sleepErr := c.sleepWithContext(req.Context(), delay); sleepErr != nil {
						return nil, ClassifyError(sleepErr, req.URL(), req.Method(), attempt+1)
					}
					continue
				}
			}

			// Success - set attempt count and return
			resp.SetAttempts(attempt + 1)
			return resp, nil
		}
	}

	// Handle final failure cases
	if lastResp != nil {
		lastResp.SetAttempts(maxRetries + 1)
		return lastResp, nil
	}

	if lastErr != nil {
		if clientErr, ok := lastErr.(*ClientError); ok {
			clientErr.Attempts = maxRetries + 1
			return nil, clientErr
		}
		return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
	}

	return nil, fmt.Errorf("request failed after %d attempts", maxRetries+1)
}

const (
	defaultMaxDrain int64 = 10 * 1024 * 1024 // 10MB
)

// backgroundCtx is a cached background context to avoid repeated allocations
var backgroundCtx = context.Background()

// executeRequest executes a single HTTP request with comprehensive error handling.
func (c *Client) executeRequest(req *Request) (*Response, error) {
	// Context setup with timeout handling
	execCtx := req.Context()
	if execCtx == nil {
		execCtx = backgroundCtx
	}

	timeout := req.Timeout()
	if timeout <= 0 && c.config.Timeout > 0 {
		timeout = c.config.Timeout
	}

	// Optimized: only create new context if absolutely necessary
	if timeout > 0 {
		if existingDeadline, hasDeadline := execCtx.Deadline(); !hasDeadline {
			// No existing deadline - create one
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(execCtx, timeout)
			defer cancel()
		} else if timeUntil := time.Until(existingDeadline); timeUntil > timeout {
			// Existing deadline is further than our timeout - create tighter one
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(execCtx, timeout)
			defer cancel()
		}
		// else: existing deadline is within our timeout - reuse it
	}

	select {
	case <-execCtx.Done():
		return nil, ClassifyError(execCtx.Err(), req.URL(), req.Method(), 0)
	default:
	}

	// Get a pooled Request copy and populate it
	reqCopy := c.getExecRequest()
	reqCopy.method = req.method
	reqCopy.url = req.url
	reqCopy.headers = req.headers
	reqCopy.queryParams = req.queryParams
	reqCopy.body = req.body
	reqCopy.timeout = req.timeout
	reqCopy.maxRetries = req.maxRetries
	reqCopy.context = execCtx
	reqCopy.cookies = req.cookies
	reqCopy.followRedirects = req.followRedirects
	reqCopy.maxRedirects = req.maxRedirects
	reqCopy.onRequest = req.onRequest
	reqCopy.onResponse = req.onResponse

	// Ensure request copy is returned to pool after processing
	defer c.putExecRequest(reqCopy)

	followRedirects := c.config.FollowRedirects
	if req.FollowRedirects() != nil {
		followRedirects = *req.FollowRedirects()
	}
	maxRedirects := c.config.MaxRedirects
	if req.MaxRedirects() != nil {
		maxRedirects = *req.MaxRedirects()
	}
	// Set redirect policy via context for thread-safety
	reqCopy.context = c.transport.SetRedirectPolicy(execCtx, followRedirects, maxRedirects)
	// Ensure redirect settings are returned to pool to prevent memory leak
	defer c.transport.CleanupRedirectSettings(reqCopy.context)

	// Invoke OnRequest callback before building the HTTP request
	if reqCopy.onRequest != nil {
		if err := reqCopy.onRequest(reqCopy); err != nil {
			return nil, ClassifyError(fmt.Errorf("onRequest callback failed: %w", err), req.URL(), req.Method(), 0)
		}
	}

	httpReq, err := c.requestProcessor.Build(reqCopy)
	if err != nil {
		return nil, ClassifyError(fmt.Errorf("build request failed: %w", err), req.URL(), req.Method(), 0)
	}

	start := time.Now()
	httpResp, err := c.transport.RoundTrip(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, ClassifyError(fmt.Errorf("transport failed: %w", err), req.URL(), req.Method(), 0)
	}

	defer func() {
		if httpResp.Body != nil {
			maxDrain := defaultMaxDrain
			if c.config.MaxResponseBodySize > 0 && c.config.MaxResponseBodySize < maxDrain {
				maxDrain = c.config.MaxResponseBodySize
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(httpResp.Body, maxDrain))
			_ = httpResp.Body.Close()
		}
	}()

	resp, err := c.responseProcessor.Process(httpResp)
	if err != nil {
		return nil, ClassifyError(fmt.Errorf("process response failed: %w", err), req.URL(), req.Method(), 0)
	}

	if redirectChain := c.transport.GetRedirectChain(reqCopy.context); len(redirectChain) > 0 {
		resp.SetRedirectChain(redirectChain)
		resp.SetRedirectCount(len(redirectChain))
	}

	if httpResp.Request != nil {
		// Copy headers - this is necessary since the underlying map may be reused
		// Optimized: pre-calculate total values count and allocate once
		headerLen := len(httpResp.Request.Header)
		if headerLen > 0 {
			// First pass: count total values to allocate once
			totalValues := 0
			for _, values := range httpResp.Request.Header {
				totalValues += len(values)
			}

			// Allocate backing array for all values at once
			valuesBacking := make([]string, 0, totalValues)
			requestHeaders := make(http.Header, headerLen)

			// Second pass: copy headers using slices from backing array
			for key, values := range httpResp.Request.Header {
				valuesLen := len(values)
				// Extend backing slice and copy values
				start := len(valuesBacking)
				valuesBacking = valuesBacking[:start+valuesLen]
				copy(valuesBacking[start:], values)
				requestHeaders[key] = valuesBacking[start : start+valuesLen]
			}
			resp.SetRequestHeaders(requestHeaders)
		}
		// Set the actual request URL and method
		if httpResp.Request.URL != nil {
			resp.SetRequestURL(httpResp.Request.URL.String())
		}
		resp.SetRequestMethod(httpResp.Request.Method)
	}

	resp.SetDuration(duration)

	// Invoke OnResponse callback after response processing
	if reqCopy.onResponse != nil {
		if err := reqCopy.onResponse(resp); err != nil {
			return nil, ClassifyError(fmt.Errorf("onResponse callback failed: %w", err), req.URL(), req.Method(), 0)
		}
	}

	return resp, nil
}

// GetHealthStatus returns the current health status of the client.
func (c *Client) GetHealthStatus() HealthStatus {
	return c.metrics.GetHealthStatus()
}

// IsHealthy returns true if the client is healthy (error rate < 10%).
func (c *Client) IsHealthy() bool {
	return c.metrics.IsHealthy()
}

func (c *Client) Close() error {
	var closeErr error

	c.closeOnce.Do(func() {
		atomic.StoreInt32(&c.closed, 1)

		if c.connectionPool != nil {
			if err := c.connectionPool.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close connection pool: %w", err)
			}
		}

		if c.transport != nil {
			if err := c.transport.Close(); err != nil {
				closeErr = fmt.Errorf("failed to close transport: %w", err)
			}
		}
	})

	return closeErr
}

type RequestOption func(*Request) error

// ClientOption is a functional option for configuring the Client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	customTransport TransportManager
}

// WithTransport sets a custom transport for the client.
// This is useful for testing with mock transports or for custom protocols.
func WithTransport(transport TransportManager) ClientOption {
	return func(opts *clientOptions) {
		opts.customTransport = transport
	}
}
