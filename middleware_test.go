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

	"github.com/cybergodev/httpc/internal/engine"
)

func TestChain(t *testing.T) {
	var order []string
	var mu sync.Mutex

	middleware1 := func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
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
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
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

	finalHandler := func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
		return &engine.Response{StatusCode: 200}, nil
	}

	chain := Chain(middleware1, middleware2)
	handler := chain(finalHandler)

	_, _ = handler(context.Background(), &engine.Request{})

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
	cfg.Middlewares = []MiddlewareFunc{
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
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
			panic("test panic")
		}
	}

	cfg := DefaultConfig()
	cfg.Middlewares = []MiddlewareFunc{
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
	cfg.Middlewares = []MiddlewareFunc{
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
	cfg.Timeout = 5 * time.Second
	cfg.Middlewares = []MiddlewareFunc{
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
	cfg.Middlewares = []MiddlewareFunc{
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
	cfg.Middlewares = []MiddlewareFunc{
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
			return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
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
	cfg.Middlewares = []MiddlewareFunc{
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
	cfg.Middlewares = []MiddlewareFunc{
		func(next Handler) Handler {
			return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
				if req.Headers == nil {
					req.Headers = make(map[string]string)
				}
				req.Headers["X-Modified-By-Middleware"] = "modified-value"
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
	cfg.Middlewares = []MiddlewareFunc{
		func(next Handler) Handler {
			return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
				resp, err := next(ctx, req)
				if resp != nil && resp.Headers != nil {
					resp.Headers["X-Modified"] = []string{"modified-value"}
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
		cfg.Middlewares = []MiddlewareFunc{
			func(next Handler) Handler {
				return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
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
		cfg.Middlewares = []MiddlewareFunc{
			func(next Handler) Handler {
				return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
					return next(ctx, req)
				}
			},
			func(next Handler) Handler {
				return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
					return next(ctx, req)
				}
			},
			func(next Handler) Handler {
				return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
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
	cfg.Middlewares = []MiddlewareFunc{
		func(next Handler) Handler {
			return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
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
