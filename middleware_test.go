package httpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChain(t *testing.T) {
	var order []string
	var mu sync.Mutex

	middleware1 := func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			mu.Lock()
			order = append(order, "m1-before")
			mu.Unlock()
			resp, err := next(ctx, req)
			mu.Lock()
			order = append(order, "m1-after")
			mu.Unlock()
			return resp, err
		}
	}

	middleware2 := func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			mu.Lock()
			order = append(order, "m2-before")
			mu.Unlock()
			resp, err := next(ctx, req)
			mu.Lock()
			order = append(order, "m2-after")
			mu.Unlock()
			return resp, err
		}
	}

	finalHandler := func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
		return &mockResponse{statusCode: 200}, nil
	}

	chain := Chain(middleware1, middleware2)
	handler := chain(finalHandler)

	_, _ = handler(context.Background(), &mockRequest{})

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}

	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("position %d: expected %s, got %s", i, exp, order[i])
		}
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var loggedMessages []string
	var mu sync.Mutex

	logger := func(format string, args ...any) {
		mu.Lock()
		loggedMessages = append(loggedMessages, fmt.Sprintf(format, args...))
		mu.Unlock()
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		LoggingMiddleware(logger),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, _ = client.Get(ts.URL)

	mu.Lock()
	if len(loggedMessages) == 0 {
		t.Error("expected log message, got none")
	}
	msg := loggedMessages[0]
	mu.Unlock()

	if !strings.Contains(msg, "GET") {
		t.Errorf("expected log to contain GET, got: %s", msg)
	}
	if !strings.Contains(msg, "200") {
		t.Errorf("expected log to contain 200, got: %s", msg)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	panicMiddleware := func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			panic("test panic")
		}
	}

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		RecoveryMiddleware(),
		panicMiddleware,
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err = client.Get(ts.URL)

	if err == nil {
		t.Error("expected error from panic recovery, got nil")
	}
	if !strings.Contains(err.Error(), "panic recovered") {
		t.Errorf("expected panic recovered error, got: %v", err)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	var receivedID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		RequestIDMiddleware("X-Request-ID", func() string {
			return "test-request-id-123"
		}),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if receivedID != "test-request-id-123" {
		t.Errorf("expected request ID 'test-request-id-123', got: %s", receivedID)
	}
}

func TestTimeoutMiddleware(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Timeouts.Request = 5 * time.Second
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		TimeoutMiddleware(10 * time.Millisecond),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	start := time.Now()
	_, err = client.Get(ts.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error, got nil")
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("request took too long: %v", elapsed)
	}
}

func TestHeaderMiddleware(t *testing.T) {
	var receivedHeaders map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = map[string]string{
			"X-Custom-Header": r.Header.Get("X-Custom-Header"),
			"Authorization":   r.Header.Get("Authorization"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		HeaderMiddleware(map[string]string{
			"X-Custom-Header": "custom-value",
			"Authorization":   "Bearer test-token",
		}),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if receivedHeaders["X-Custom-Header"] != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got: %s", receivedHeaders["X-Custom-Header"])
	}
	if receivedHeaders["Authorization"] != "Bearer test-token" {
		t.Errorf("expected Authorization 'Bearer test-token', got: %s", receivedHeaders["Authorization"])
	}
}

func TestMetricsMiddleware(t *testing.T) {
	var metrics struct {
		method     string
		url        string
		statusCode int
		duration   time.Duration
		err        error
		called     bool
	}
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {
			mu.Lock()
			defer mu.Unlock()
			metrics.method = method
			metrics.url = url
			metrics.statusCode = statusCode
			metrics.duration = duration
			metrics.err = err
			metrics.called = true
		}),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Post(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !metrics.called {
		t.Error("metrics callback was not called")
	}
	if metrics.method != "POST" {
		t.Errorf("expected method POST, got: %s", metrics.method)
	}
	if metrics.statusCode != http.StatusCreated {
		t.Errorf("expected status code %d, got: %d", http.StatusCreated, metrics.statusCode)
	}
	if metrics.duration <= 0 {
		t.Error("expected positive duration")
	}
	if metrics.err != nil {
		t.Errorf("expected no error, got: %v", metrics.err)
	}
}

func TestMultipleMiddlewares(t *testing.T) {
	var order []string
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	createMiddleware := func(name string) MiddlewareFunc {
		return func(next Handler) Handler {
			return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
				mu.Lock()
				order = append(order, name+"-before")
				mu.Unlock()
				resp, err := next(ctx, req)
				mu.Lock()
				order = append(order, name+"-after")
				mu.Unlock()
				return resp, err
			}
		}
	}

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		createMiddleware("A"),
		createMiddleware("B"),
		createMiddleware("C"),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{
		"A-before", "B-before", "C-before",
		"C-after", "B-after", "A-after",
	}

	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}

	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("position %d: expected %s, got %s", i, exp, order[i])
		}
	}
}

func TestZeroOverheadNoMiddleware(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Client without middlewares
	cfg := DefaultConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Should work normally
	_, err = client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

func TestMiddlewareCanModifyRequest(t *testing.T) {
	var receivedValue string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedValue = r.Header.Get("X-Modified-By-Middleware")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		func(next Handler) Handler {
			return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
				req.SetHeader("X-Modified-By-Middleware", "modified-value")
				return next(ctx, req)
			}
		},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if receivedValue != "modified-value" {
		t.Errorf("expected 'modified-value', got: %s", receivedValue)
	}
}

func TestMiddlewareCanModifyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Original", "original-value")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		func(next Handler) Handler {
			return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
				resp, err := next(ctx, req)
				if resp != nil {
					resp.(ResponseMutator).SetHeader("X-Modified", "modified-value")
				}
				return resp, err
			}
		},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	modified := result.Response.Headers["X-Modified"]
	if len(modified) == 0 || modified[0] != "modified-value" {
		t.Errorf("expected modified header, got: %v", modified)
	}
}

func BenchmarkMiddlewareOverhead(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	b.Run("NoMiddleware", func(b *testing.B) {
		cfg := DefaultConfig()
		client, _ := New(cfg)
		defer client.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.Get(ts.URL)
		}
	})

	b.Run("WithMiddleware", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Middleware.Middlewares = []MiddlewareFunc{
			func(next Handler) Handler {
				return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
					return next(ctx, req)
				}
			},
		}
		client, _ := New(cfg)
		defer client.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.Get(ts.URL)
		}
	})

	b.Run("WithThreeMiddlewares", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Middleware.Middlewares = []MiddlewareFunc{
			func(next Handler) Handler {
				return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
					return next(ctx, req)
				}
			},
			func(next Handler) Handler {
				return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
					return next(ctx, req)
				}
			},
			func(next Handler) Handler {
				return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
					return next(ctx, req)
				}
			},
		}
		client, _ := New(cfg)
		defer client.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.Get(ts.URL)
		}
	})
}

func TestConcurrentMiddlewareAccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	var callCount int64

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		func(next Handler) Handler {
			return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
				atomic.AddInt64(&callCount, 1)
				return next(ctx, req)
			}
		},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.Get(ts.URL)
		}()
	}
	wg.Wait()

	count := atomic.LoadInt64(&callCount)
	if count != 10 {
		t.Errorf("expected 10 middleware calls, got %d", count)
	}
}

// mockRequest implements RequestMutator for testing
type mockRequest struct {
	method          string
	url             string
	headers         map[string]string
	queryParams     map[string]any
	body            any
	timeout         time.Duration
	maxRetries      int
	ctx             context.Context
	cookies         []http.Cookie
	followRedirects *bool
	maxRedirects    *int
}

func (m *mockRequest) Method() string                 { return m.method }
func (m *mockRequest) URL() string                    { return m.url }
func (m *mockRequest) Headers() map[string]string     { return m.headers }
func (m *mockRequest) QueryParams() map[string]any    { return m.queryParams }
func (m *mockRequest) Body() any                      { return m.body }
func (m *mockRequest) Timeout() time.Duration         { return m.timeout }
func (m *mockRequest) MaxRetries() int                { return m.maxRetries }
func (m *mockRequest) Context() context.Context       { return m.ctx }
func (m *mockRequest) Cookies() []http.Cookie         { return m.cookies }
func (m *mockRequest) FollowRedirects() *bool         { return m.followRedirects }
func (m *mockRequest) MaxRedirects() *int             { return m.maxRedirects }
func (m *mockRequest) SetMethod(v string)             { m.method = v }
func (m *mockRequest) SetURL(v string)                { m.url = v }
func (m *mockRequest) SetHeaders(v map[string]string) { m.headers = v }
func (m *mockRequest) SetHeader(k, v string) {
	if m.headers == nil {
		m.headers = make(map[string]string)
	}
	m.headers[k] = v
}
func (m *mockRequest) SetQueryParams(v map[string]any) { m.queryParams = v }
func (m *mockRequest) SetBody(v any)                   { m.body = v }
func (m *mockRequest) SetTimeout(v time.Duration)      { m.timeout = v }
func (m *mockRequest) SetMaxRetries(v int)             { m.maxRetries = v }
func (m *mockRequest) SetContext(v context.Context)    { m.ctx = v }
func (m *mockRequest) SetCookies(v []http.Cookie)      { m.cookies = v }
func (m *mockRequest) SetFollowRedirects(v *bool)      { m.followRedirects = v }
func (m *mockRequest) SetMaxRedirects(v *int)          { m.maxRedirects = v }

// mockResponse implements ResponseMutator for testing
type mockResponse struct {
	statusCode     int
	status         string
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
func (m *mockResponse) SetStatusCode(v int)         { m.statusCode = v }
func (m *mockResponse) SetStatus(v string)          { m.status = v }
func (m *mockResponse) SetHeaders(v http.Header)    { m.headers = v }
func (m *mockResponse) SetHeader(k string, v ...string) {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	m.headers[k] = v
}
func (m *mockResponse) SetBody(v string)                { m.body = v }
func (m *mockResponse) SetRawBody(v []byte)             { m.rawBody = v }
func (m *mockResponse) SetContentLength(v int64)        { m.contentLength = v }
func (m *mockResponse) SetDuration(v time.Duration)     { m.duration = v }
func (m *mockResponse) SetAttempts(v int)               { m.attempts = v }
func (m *mockResponse) SetCookies(v []*http.Cookie)     { m.cookies = v }
func (m *mockResponse) SetRedirectChain(v []string)     { m.redirectChain = v }
func (m *mockResponse) SetRedirectCount(v int)          { m.redirectCount = v }
func (m *mockResponse) SetRequestHeaders(v http.Header) { m.requestHeaders = v }
