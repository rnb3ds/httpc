package types

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// mockRequest implements RequestMutator for testing
type mockRequest struct {
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
}

func (m *mockRequest) Method() string              { return m.method }
func (m *mockRequest) URL() string                 { return m.url }
func (m *mockRequest) Headers() map[string]string  { return m.headers }
func (m *mockRequest) QueryParams() map[string]any { return m.queryParams }
func (m *mockRequest) Body() any                   { return m.body }
func (m *mockRequest) Timeout() time.Duration      { return m.timeout }
func (m *mockRequest) MaxRetries() int             { return m.maxRetries }
func (m *mockRequest) Context() context.Context    { return m.context }
func (m *mockRequest) Cookies() []http.Cookie      { return m.cookies }
func (m *mockRequest) FollowRedirects() *bool      { return m.followRedirects }
func (m *mockRequest) MaxRedirects() *int          { return m.maxRedirects }

func (m *mockRequest) SetMethod(v string)             { m.method = v }
func (m *mockRequest) SetURL(v string)                { m.url = v }
func (m *mockRequest) SetHeaders(v map[string]string) { m.headers = v }
func (m *mockRequest) SetHeader(key, value string) {
	if m.headers == nil {
		m.headers = make(map[string]string)
	}
	m.headers[key] = value
}
func (m *mockRequest) SetQueryParams(v map[string]any) { m.queryParams = v }
func (m *mockRequest) SetBody(v any)                   { m.body = v }
func (m *mockRequest) SetTimeout(v time.Duration)      { m.timeout = v }
func (m *mockRequest) SetMaxRetries(v int)             { m.maxRetries = v }
func (m *mockRequest) SetContext(v context.Context)    { m.context = v }
func (m *mockRequest) SetCookies(v []http.Cookie)      { m.cookies = v }
func (m *mockRequest) SetFollowRedirects(v *bool)      { m.followRedirects = v }
func (m *mockRequest) SetMaxRedirects(v *int)          { m.maxRedirects = v }

// mockResponse implements ResponseMutator for testing
type mockResponse struct {
	statusCode     int
	status         string
	proto          string
	headers        http.Header
	body           string
	rawBody        []byte
	contentLength  int64
	duration       time.Duration
	attempts       int
	cookies        []*http.Cookie
	redirectChain  []string
	redirectCount  int
	requestHeaders http.Header
}

func (m *mockResponse) StatusCode() int             { return m.statusCode }
func (m *mockResponse) Status() string              { return m.status }
func (m *mockResponse) Proto() string               { return m.proto }
func (m *mockResponse) Headers() http.Header        { return m.headers }
func (m *mockResponse) Body() string                { return m.body }
func (m *mockResponse) RawBody() []byte             { return m.rawBody }
func (m *mockResponse) ContentLength() int64        { return m.contentLength }
func (m *mockResponse) Duration() time.Duration     { return m.duration }
func (m *mockResponse) Attempts() int               { return m.attempts }
func (m *mockResponse) Cookies() []*http.Cookie     { return m.cookies }
func (m *mockResponse) RedirectChain() []string     { return m.redirectChain }
func (m *mockResponse) RedirectCount() int          { return m.redirectCount }
func (m *mockResponse) RequestHeaders() http.Header { return m.requestHeaders }

func (m *mockResponse) SetStatusCode(v int)             { m.statusCode = v }
func (m *mockResponse) SetStatus(v string)              { m.status = v }
func (m *mockResponse) SetProto(v string)               { m.proto = v }
func (m *mockResponse) SetHeaders(v http.Header)        { m.headers = v }
func (m *mockResponse) SetBody(v string)                { m.body = v }
func (m *mockResponse) SetRawBody(v []byte)             { m.rawBody = v }
func (m *mockResponse) SetContentLength(v int64)        { m.contentLength = v }
func (m *mockResponse) SetDuration(v time.Duration)     { m.duration = v }
func (m *mockResponse) SetAttempts(v int)               { m.attempts = v }
func (m *mockResponse) SetCookies(v []*http.Cookie)     { m.cookies = v }
func (m *mockResponse) SetRedirectChain(v []string)     { m.redirectChain = v }
func (m *mockResponse) SetRedirectCount(v int)          { m.redirectCount = v }
func (m *mockResponse) SetRequestHeaders(v http.Header) { m.requestHeaders = v }
func (m *mockResponse) SetHeader(key string, values ...string) {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	m.headers[key] = values
}

// mockRetryPolicy implements RetryPolicy for testing
type mockRetryPolicy struct {
	shouldRetry bool
	delay       time.Duration
	maxRetries  int
}

func (m *mockRetryPolicy) ShouldRetry(_ ResponseReader, _ error, _ int) bool {
	return m.shouldRetry
}

func (m *mockRetryPolicy) GetDelay(_ int) time.Duration {
	return m.delay
}

func (m *mockRetryPolicy) MaxRetries() int {
	return m.maxRetries
}

// TestRequestMutatorInterface tests that mockRequest implements RequestMutator
func TestRequestMutatorInterface(t *testing.T) {
	var _ RequestMutator = &mockRequest{}
}

// TestResponseMutatorInterface tests that mockResponse implements ResponseMutator
func TestResponseMutatorInterface(t *testing.T) {
	var _ ResponseMutator = &mockResponse{}
}

// TestRetryPolicyInterface tests that mockRetryPolicy implements RetryPolicy
func TestRetryPolicyInterface(t *testing.T) {
	var _ RetryPolicy = &mockRetryPolicy{}
}

// TestRequestReaderMethods tests RequestReader interface methods
func TestRequestReaderMethods(t *testing.T) {
	ctx := context.Background()
	follow := true
	redirects := 5

	req := &mockRequest{
		method:          "GET",
		url:             "https://example.com/test",
		headers:         map[string]string{"Content-Type": "application/json"},
		queryParams:     map[string]any{"page": 1},
		body:            `{"key":"value"}`,
		timeout:         30 * time.Second,
		maxRetries:      3,
		context:         ctx,
		cookies:         []http.Cookie{{Name: "session", Value: "abc123"}},
		followRedirects: &follow,
		maxRedirects:    &redirects,
	}

	// Test all reader methods
	if req.Method() != "GET" {
		t.Errorf("Method() = %q, want %q", req.Method(), "GET")
	}
	if req.URL() != "https://example.com/test" {
		t.Errorf("URL() = %q, want %q", req.URL(), "https://example.com/test")
	}
	if req.Headers()["Content-Type"] != "application/json" {
		t.Errorf("Headers()[Content-Type] = %q, want %q", req.Headers()["Content-Type"], "application/json")
	}
	if req.QueryParams()["page"] != 1 {
		t.Errorf("QueryParams()[page] = %v, want %v", req.QueryParams()["page"], 1)
	}
	if req.Timeout() != 30*time.Second {
		t.Errorf("Timeout() = %v, want %v", req.Timeout(), 30*time.Second)
	}
	if req.MaxRetries() != 3 {
		t.Errorf("MaxRetries() = %v, want %v", req.MaxRetries(), 3)
	}
	if req.Context() != ctx {
		t.Error("Context() returned different context")
	}
	if len(req.Cookies()) != 1 || req.Cookies()[0].Name != "session" {
		t.Errorf("Cookies() = %v, want [{session abc123}]", req.Cookies())
	}
	if *req.FollowRedirects() != true {
		t.Errorf("FollowRedirects() = %v, want %v", *req.FollowRedirects(), true)
	}
	if *req.MaxRedirects() != 5 {
		t.Errorf("MaxRedirects() = %v, want %v", *req.MaxRedirects(), 5)
	}
}

// TestRequestWriterMethods tests RequestWriter interface methods
func TestRequestWriterMethods(t *testing.T) {
	req := &mockRequest{}

	req.SetMethod("POST")
	if req.method != "POST" {
		t.Errorf("SetMethod: method = %q, want %q", req.method, "POST")
	}

	req.SetURL("https://example.com/api")
	if req.url != "https://example.com/api" {
		t.Errorf("SetURL: url = %q, want %q", req.url, "https://example.com/api")
	}

	req.SetHeaders(map[string]string{"X-Custom": "value"})
	if req.headers["X-Custom"] != "value" {
		t.Errorf("SetHeaders: headers[X-Custom] = %q, want %q", req.headers["X-Custom"], "value")
	}

	req.SetHeader("X-Another", "test")
	if req.headers["X-Another"] != "test" {
		t.Errorf("SetHeader: headers[X-Another] = %q, want %q", req.headers["X-Another"], "test")
	}

	req.SetQueryParams(map[string]any{"limit": 10})
	if req.queryParams["limit"] != 10 {
		t.Errorf("SetQueryParams: queryParams[limit] = %v, want %v", req.queryParams["limit"], 10)
	}

	req.SetBody("test body")
	if req.body != "test body" {
		t.Errorf("SetBody: body = %v, want %v", req.body, "test body")
	}

	req.SetTimeout(60 * time.Second)
	if req.timeout != 60*time.Second {
		t.Errorf("SetTimeout: timeout = %v, want %v", req.timeout, 60*time.Second)
	}

	req.SetMaxRetries(5)
	if req.maxRetries != 5 {
		t.Errorf("SetMaxRetries: maxRetries = %v, want %v", req.maxRetries, 5)
	}

	ctx := context.Background()
	req.SetContext(ctx)
	if req.context != ctx {
		t.Error("SetContext: context not set correctly")
	}

	cookies := []http.Cookie{{Name: "test", Value: "value"}}
	req.SetCookies(cookies)
	if len(req.cookies) != 1 || req.cookies[0].Name != "test" {
		t.Errorf("SetCookies: cookies = %v, want [{test value}]", req.cookies)
	}

	follow := false
	req.SetFollowRedirects(&follow)
	if *req.followRedirects != false {
		t.Errorf("SetFollowRedirects: followRedirects = %v, want %v", *req.followRedirects, false)
	}

	redirects := 10
	req.SetMaxRedirects(&redirects)
	if *req.maxRedirects != 10 {
		t.Errorf("SetMaxRedirects: maxRedirects = %v, want %v", *req.maxRedirects, 10)
	}
}

// TestResponseReaderMethods tests ResponseReader interface methods
func TestResponseReaderMethods(t *testing.T) {
	resp := &mockResponse{
		statusCode:     200,
		status:         "OK",
		proto:          "HTTP/1.1",
		headers:        http.Header{"Content-Type": []string{"application/json"}},
		body:           `{"result":"success"}`,
		rawBody:        []byte(`{"result":"success"}`),
		contentLength:  21,
		duration:       150 * time.Millisecond,
		attempts:       2,
		cookies:        []*http.Cookie{{Name: "session", Value: "xyz789"}},
		redirectChain:  []string{"https://a.com", "https://b.com"},
		redirectCount:  2,
		requestHeaders: http.Header{"Authorization": []string{"Bearer token"}},
	}

	if resp.StatusCode() != 200 {
		t.Errorf("StatusCode() = %v, want %v", resp.StatusCode(), 200)
	}
	if resp.Status() != "OK" {
		t.Errorf("Status() = %q, want %q", resp.Status(), "OK")
	}
	if resp.Proto() != "HTTP/1.1" {
		t.Errorf("Proto() = %q, want %q", resp.Proto(), "HTTP/1.1")
	}
	if resp.Headers().Get("Content-Type") != "application/json" {
		t.Errorf("Headers()[Content-Type] = %q, want %q", resp.Headers().Get("Content-Type"), "application/json")
	}
	if resp.Body() != `{"result":"success"}` {
		t.Errorf("Body() = %q, want %q", resp.Body(), `{"result":"success"}`)
	}
	if string(resp.RawBody()) != `{"result":"success"}` {
		t.Errorf("RawBody() = %s, want %s", resp.RawBody(), `{"result":"success"}`)
	}
	if resp.ContentLength() != 21 {
		t.Errorf("ContentLength() = %v, want %v", resp.ContentLength(), 21)
	}
	if resp.Duration() != 150*time.Millisecond {
		t.Errorf("Duration() = %v, want %v", resp.Duration(), 150*time.Millisecond)
	}
	if resp.Attempts() != 2 {
		t.Errorf("Attempts() = %v, want %v", resp.Attempts(), 2)
	}
	if len(resp.Cookies()) != 1 || resp.Cookies()[0].Name != "session" {
		t.Errorf("Cookies() = %v, want [{session xyz789}]", resp.Cookies())
	}
	if len(resp.RedirectChain()) != 2 {
		t.Errorf("RedirectChain() = %v, want 2 elements", resp.RedirectChain())
	}
	if resp.RedirectCount() != 2 {
		t.Errorf("RedirectCount() = %v, want %v", resp.RedirectCount(), 2)
	}
	if resp.RequestHeaders().Get("Authorization") != "Bearer token" {
		t.Errorf("RequestHeaders()[Authorization] = %q, want %q", resp.RequestHeaders().Get("Authorization"), "Bearer token")
	}
}

// TestResponseWriterMethods tests ResponseWriter interface methods
func TestResponseWriterMethods(t *testing.T) {
	resp := &mockResponse{}

	resp.SetStatusCode(404)
	if resp.statusCode != 404 {
		t.Errorf("SetStatusCode: statusCode = %v, want %v", resp.statusCode, 404)
	}

	resp.SetStatus("Not Found")
	if resp.status != "Not Found" {
		t.Errorf("SetStatus: status = %q, want %q", resp.status, "Not Found")
	}

	resp.SetProto("HTTP/2.0")
	if resp.proto != "HTTP/2.0" {
		t.Errorf("SetProto: proto = %q, want %q", resp.proto, "HTTP/2.0")
	}

	resp.SetHeaders(http.Header{"X-Custom": []string{"value"}})
	if resp.headers.Get("X-Custom") != "value" {
		t.Errorf("SetHeaders: headers[X-Custom] = %q, want %q", resp.headers.Get("X-Custom"), "value")
	}

	resp.SetBody("test body")
	if resp.body != "test body" {
		t.Errorf("SetBody: body = %q, want %q", resp.body, "test body")
	}

	resp.SetRawBody([]byte("raw data"))
	if string(resp.rawBody) != "raw data" {
		t.Errorf("SetRawBody: rawBody = %s, want %s", resp.rawBody, "raw data")
	}

	resp.SetContentLength(100)
	if resp.contentLength != 100 {
		t.Errorf("SetContentLength: contentLength = %v, want %v", resp.contentLength, 100)
	}

	resp.SetDuration(200 * time.Millisecond)
	if resp.duration != 200*time.Millisecond {
		t.Errorf("SetDuration: duration = %v, want %v", resp.duration, 200*time.Millisecond)
	}

	resp.SetAttempts(3)
	if resp.attempts != 3 {
		t.Errorf("SetAttempts: attempts = %v, want %v", resp.attempts, 3)
	}

	cookies := []*http.Cookie{{Name: "test", Value: "val"}}
	resp.SetCookies(cookies)
	if len(resp.cookies) != 1 || resp.cookies[0].Name != "test" {
		t.Errorf("SetCookies: cookies = %v, want [{test val}]", resp.cookies)
	}

	resp.SetRedirectChain([]string{"https://a.com", "https://b.com", "https://c.com"})
	if len(resp.redirectChain) != 3 {
		t.Errorf("SetRedirectChain: redirectChain = %v, want 3 elements", resp.redirectChain)
	}

	resp.SetRedirectCount(3)
	if resp.redirectCount != 3 {
		t.Errorf("SetRedirectCount: redirectCount = %v, want %v", resp.redirectCount, 3)
	}

	resp.SetRequestHeaders(http.Header{"X-Request": []string{"header"}})
	if resp.requestHeaders.Get("X-Request") != "header" {
		t.Errorf("SetRequestHeaders: requestHeaders[X-Request] = %q, want %q", resp.requestHeaders.Get("X-Request"), "header")
	}

	resp.SetHeader("X-Multi", "val1", "val2")
	if len(resp.headers["X-Multi"]) != 2 {
		t.Errorf("SetHeader: headers[X-Multi] = %v, want 2 values", resp.headers["X-Multi"])
	}
}

// TestRetryPolicyMethods tests RetryPolicy interface methods
func TestRetryPolicyMethods(t *testing.T) {
	policy := &mockRetryPolicy{
		shouldRetry: true,
		delay:       100 * time.Millisecond,
		maxRetries:  3,
	}

	if !policy.ShouldRetry(nil, nil, 0) {
		t.Error("ShouldRetry() = false, want true")
	}

	if policy.GetDelay(0) != 100*time.Millisecond {
		t.Errorf("GetDelay(0) = %v, want %v", policy.GetDelay(0), 100*time.Millisecond)
	}

	if policy.MaxRetries() != 3 {
		t.Errorf("MaxRetries() = %v, want %v", policy.MaxRetries(), 3)
	}
}

// TestHandlerType tests that Handler is a valid function type
func TestHandlerType(t *testing.T) {
	var handler Handler = func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		return &mockResponse{statusCode: 200}, nil
	}

	if handler == nil {
		t.Error("Handler should not be nil")
	}

	// Call the handler to verify it works
	resp, err := handler(context.Background(), &mockRequest{})
	if err != nil {
		t.Errorf("Handler returned error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Handler response StatusCode() = %v, want %v", resp.StatusCode(), 200)
	}
}

// TestMiddlewareFuncType tests that MiddlewareFunc is a valid function type
func TestMiddlewareFuncType(t *testing.T) {
	var middleware MiddlewareFunc = func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			// Pre-processing
			req.SetHeader("X-Middleware", "processed")
			return next(ctx, req)
		}
	}

	if middleware == nil {
		t.Error("MiddlewareFunc should not be nil")
	}

	// Wrap a handler with middleware
	finalHandler := func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		return &mockResponse{statusCode: 201}, nil
	}

	wrappedHandler := middleware(finalHandler)
	req := &mockRequest{}
	resp, err := wrappedHandler(context.Background(), req)

	if err != nil {
		t.Errorf("Wrapped handler returned error: %v", err)
	}
	if resp.StatusCode() != 201 {
		t.Errorf("Wrapped handler response StatusCode() = %v, want %v", resp.StatusCode(), 201)
	}
	if req.headers["X-Middleware"] != "processed" {
		t.Error("Middleware did not set header")
	}
}

// TestInterfaceComposition tests interface embedding
func TestInterfaceComposition(t *testing.T) {
	// Verify RequestMutator embeds RequestReader and RequestWriter
	var _ RequestReader = (*mockRequest)(nil)
	var _ RequestWriter = (*mockRequest)(nil)
	var _ RequestMutator = (*mockRequest)(nil)

	// Verify ResponseMutator embeds ResponseReader and ResponseWriter
	var _ ResponseReader = (*mockResponse)(nil)
	var _ ResponseWriter = (*mockResponse)(nil)
	var _ ResponseMutator = (*mockResponse)(nil)
}
