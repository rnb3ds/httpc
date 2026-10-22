package types

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// This file pins the shared interface contracts with local fakes. The
// interfaces in this package are implemented by the engine's concrete
// request/response types and consumed across the library; a renamed method,
// a changed signature, or a dropped method would break every consumer at
// once. The compile-time assertions below make such a change fail HERE
// first, inside the package that owns the contract, instead of surfacing as
// scattered compile errors across engine/fingerprint and the facade.

type fakeRequest struct {
	method  string
	url     string
	headers map[string]string
	query   map[string]any
	body    any
	timeout time.Duration
	retries int
	ctx     context.Context
	cookies []http.Cookie
	follow  *bool
	maxRed  *int
	stream  bool
}

var _ RequestMutator = (*fakeRequest)(nil)

func (f *fakeRequest) Method() string                  { return f.method }
func (f *fakeRequest) URL() string                     { return f.url }
func (f *fakeRequest) Headers() map[string]string      { return f.headers }
func (f *fakeRequest) QueryParams() map[string]any     { return f.query }
func (f *fakeRequest) Body() any                       { return f.body }
func (f *fakeRequest) Timeout() time.Duration          { return f.timeout }
func (f *fakeRequest) MaxRetries() int                 { return f.retries }
func (f *fakeRequest) Context() context.Context        { return f.ctx }
func (f *fakeRequest) Cookies() []http.Cookie          { return f.cookies }
func (f *fakeRequest) FollowRedirects() *bool          { return f.follow }
func (f *fakeRequest) MaxRedirects() *int              { return f.maxRed }
func (f *fakeRequest) StreamBody() bool                { return f.stream }
func (f *fakeRequest) SetMethod(v string)              { f.method = v }
func (f *fakeRequest) SetURL(v string)                 { f.url = v }
func (f *fakeRequest) SetHeaders(v map[string]string)  { f.headers = v }
func (f *fakeRequest) SetHeader(k, v string)           { f.headers[k] = v }
func (f *fakeRequest) SetQueryParams(v map[string]any) { f.query = v }
func (f *fakeRequest) SetBody(v any)                   { f.body = v }
func (f *fakeRequest) SetTimeout(v time.Duration)      { f.timeout = v }
func (f *fakeRequest) SetMaxRetries(v int)             { f.retries = v }
func (f *fakeRequest) SetContext(v context.Context)    { f.ctx = v }
func (f *fakeRequest) SetCookies(v []http.Cookie)      { f.cookies = v }
func (f *fakeRequest) SetFollowRedirects(v *bool)      { f.follow = v }
func (f *fakeRequest) SetMaxRedirects(v *int)          { f.maxRed = v }
func (f *fakeRequest) SetStreamBody(v bool)            { f.stream = v }

// EnsureQueryParams satisfies QueryParamsEnsurer: WithQuery/WithQueryMap
// reach the request through this narrow contract, lazily creating the map.
func (f *fakeRequest) EnsureQueryParams() map[string]any {
	if f.query == nil {
		f.query = make(map[string]any)
	}
	return f.query
}

var _ QueryParamsEnsurer = (*fakeRequest)(nil)

type fakeResponse struct {
	status    int
	statusTxt string
	proto     string
	headers   http.Header
	body      string
	rawBody   []byte
	length    int64
	duration  time.Duration
	attempts  int
	cookies   []*http.Cookie
	chain     []string
	chainLen  int
	reqHdrs   http.Header
	reqURL    string
	reqMethod string
}

var _ ResponseMutator = (*fakeResponse)(nil)

func (f *fakeResponse) StatusCode() int                        { return f.status }
func (f *fakeResponse) Status() string                         { return f.statusTxt }
func (f *fakeResponse) Proto() string                          { return f.proto }
func (f *fakeResponse) Headers() http.Header                   { return f.headers }
func (f *fakeResponse) Body() string                           { return f.body }
func (f *fakeResponse) RawBody() []byte                        { return f.rawBody }
func (f *fakeResponse) ContentLength() int64                   { return f.length }
func (f *fakeResponse) Duration() time.Duration                { return f.duration }
func (f *fakeResponse) Attempts() int                          { return f.attempts }
func (f *fakeResponse) Cookies() []*http.Cookie                { return f.cookies }
func (f *fakeResponse) RedirectChain() []string                { return f.chain }
func (f *fakeResponse) RedirectCount() int                     { return f.chainLen }
func (f *fakeResponse) RequestHeaders() http.Header            { return f.reqHdrs }
func (f *fakeResponse) RequestURL() string                     { return f.reqURL }
func (f *fakeResponse) RequestMethod() string                  { return f.reqMethod }
func (f *fakeResponse) SetStatusCode(v int)                    { f.status = v }
func (f *fakeResponse) SetStatus(v string)                     { f.statusTxt = v }
func (f *fakeResponse) SetProto(v string)                      { f.proto = v }
func (f *fakeResponse) SetHeaders(v http.Header)               { f.headers = v }
func (f *fakeResponse) SetBody(v string)                       { f.body = v }
func (f *fakeResponse) SetRawBody(v []byte)                    { f.rawBody = v }
func (f *fakeResponse) SetContentLength(v int64)               { f.length = v }
func (f *fakeResponse) SetDuration(v time.Duration)            { f.duration = v }
func (f *fakeResponse) SetAttempts(v int)                      { f.attempts = v }
func (f *fakeResponse) SetCookies(v []*http.Cookie)            { f.cookies = v }
func (f *fakeResponse) SetRedirectChain(v []string)            { f.chain = v }
func (f *fakeResponse) SetRedirectCount(v int)                 { f.chainLen = v }
func (f *fakeResponse) SetRequestHeaders(v http.Header)        { f.reqHdrs = v }
func (f *fakeResponse) SetRequestURL(v string)                 { f.reqURL = v }
func (f *fakeResponse) SetRequestMethod(v string)              { f.reqMethod = v }
func (f *fakeResponse) SetHeader(key string, values ...string) { f.headers[key] = values }

type fakeRetryPolicy struct{}

var _ RetryPolicy = fakeRetryPolicy{}

func (fakeRetryPolicy) ShouldRetry(resp ResponseReader, err error, attempt int) bool { return false }
func (fakeRetryPolicy) GetDelay(attempt int) time.Duration                           { return 0 }
func (fakeRetryPolicy) MaxRetries() int                                              { return 0 }

// The narrow capability interfaces the facade type-asserts onto requests.
// They must stay implementable by a type that does NOT import the engine.
var (
	_ PrivateIPSetter          = (*fakeRequest)(nil)
	_ RetryNonIdempotentSetter = (*fakeRequest)(nil)
	_ NoTimeoutSetter          = (*fakeRequest)(nil)
)

func (f *fakeRequest) SetAllowPrivateIPs(v *bool)    {}
func (f *fakeRequest) SetRetryNonIdempotent(v *bool) {}
func (f *fakeRequest) SetNoTimeout(v bool)           {}

var _ RequestCallbackSetter = (*fakeRequest)(nil)

func (f *fakeRequest) OnRequestMutator() func(RequestMutator) error      { return nil }
func (f *fakeRequest) SetOnRequestMutator(cb func(RequestMutator) error) {}
func (f *fakeRequest) OnResponseMutator() func(ResponseMutator) error    { return nil }
func (f *fakeRequest) SetOnResponseMutator(cb func(ResponseMutator) error) {
}

// TestHandlerShape exercises the Handler/MiddlewareFunc function types end
// to end: a middleware wraps a terminal handler and the request flows
// through both mutator surfaces. This is the same shape the facade's
// Chain/buildMiddlewareChain relies on.
func TestHandlerShape(t *testing.T) {
	terminal := func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		req.SetHeader("X-Stage", "terminal")
		resp := &fakeResponse{status: 200, headers: make(http.Header)}
		resp.SetRequestMethod(req.Method())
		return resp, nil
	}
	var mw MiddlewareFunc = func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			req.SetMethod("POST")
			return next(ctx, req)
		}
	}

	req := &fakeRequest{method: "GET", headers: make(map[string]string)}
	resp, err := mw(terminal)(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method() != "POST" {
		t.Errorf("middleware method mutation lost: %q", req.Method())
	}
	if req.Headers()["X-Stage"] != "terminal" {
		t.Errorf("terminal handler header mutation lost: %v", req.Headers())
	}
	if resp.RequestMethod() != "POST" {
		t.Errorf("response must observe the mutated method, got %q", resp.RequestMethod())
	}
}

// TestFormDataShape pins the multipart payload types' field layout — the
// engine's form writer reads exactly these fields.
func TestFormDataShape(t *testing.T) {
	fd := &FormData{
		Fields: map[string]string{"k": "v"},
		Files: map[string]*FileData{
			"doc": {Filename: "a.txt", Content: []byte("hi"), ContentType: "text/plain"},
		},
	}
	if fd.Fields["k"] != "v" {
		t.Errorf("Fields roundtrip failed")
	}
	file := fd.Files["doc"]
	if file.Filename != "a.txt" || string(file.Content) != "hi" || file.ContentType != "text/plain" {
		t.Errorf("FileData roundtrip failed: %+v", file)
	}
	// Zero values must be representable: the engine's form writer handles a
	// FormData with no fields or files.
	empty := &FormData{}
	if empty.Fields != nil || empty.Files != nil {
		t.Errorf("zero FormData should have nil maps, got %+v", empty)
	}
}
