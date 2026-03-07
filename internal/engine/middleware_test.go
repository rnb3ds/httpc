package engine

import (
	"context"
	"sync"
	"testing"
)

func TestBuildChain_Empty(t *testing.T) {
	callCount := 0
	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		callCount++
		return &Response{StatusCode: 200}, nil
	}

	// Empty middleware slice should return final handler directly
	chain := BuildChain(nil, finalHandler)
	if chain == nil {
		t.Fatal("chain should not be nil")
	}

	_, _ = chain(context.Background(), &Request{})

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestBuildChain_Single(t *testing.T) {
	var order []string
	var mu sync.Mutex

	middleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			mu.Lock()
			order = append(order, "mw-before")
			mu.Unlock()
			resp, err := next(ctx, req)
			mu.Lock()
			order = append(order, "mw-after")
			mu.Unlock()
			return resp, err
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{middleware}, finalHandler)
	_, _ = chain(context.Background(), &Request{})

	expected := []string{"mw-before", "handler", "mw-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}

	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("position %d: expected %s, got %s", i, exp, order[i])
		}
	}
}

func TestBuildChain_Multiple(t *testing.T) {
	var order []string
	var mu sync.Mutex

	createMiddleware := func(name string) MiddlewareFunc {
		return func(next Handler) Handler {
			return func(ctx context.Context, req *Request) (*Response, error) {
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

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
		return &Response{StatusCode: 200}, nil
	}

	middlewares := []MiddlewareFunc{
		createMiddleware("A"),
		createMiddleware("B"),
		createMiddleware("C"),
	}

	chain := BuildChain(middlewares, finalHandler)
	_, _ = chain(context.Background(), &Request{})

	expected := []string{
		"A-before", "B-before", "C-before",
		"handler",
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

func TestBuildChain_CanModifyRequest(t *testing.T) {
	modifyingMiddleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			req.Headers["X-Modified"] = "true"
			return next(ctx, req)
		}
	}

	var capturedRequest *Request
	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		capturedRequest = req
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{modifyingMiddleware}, finalHandler)
	_, _ = chain(context.Background(), &Request{})

	if capturedRequest == nil {
		t.Fatal("request was not captured")
	}

	if capturedRequest.Headers["X-Modified"] != "true" {
		t.Error("expected header to be modified")
	}
}

func TestBuildChain_CanModifyResponse(t *testing.T) {
	modifyingMiddleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			resp, err := next(ctx, req)
			if resp != nil {
				resp.StatusCode = 201
			}
			return resp, err
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{modifyingMiddleware}, finalHandler)
	resp, _ := chain(context.Background(), &Request{})

	if resp.StatusCode != 201 {
		t.Errorf("expected status code 201, got %d", resp.StatusCode)
	}
}

func TestBuildChain_ErrorPropagation(t *testing.T) {
	expectedErr := context.DeadlineExceeded

	errorMiddleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			_, _ = next(ctx, req) // Discard response and error, return custom error
			return nil, expectedErr
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{errorMiddleware}, finalHandler)
	_, err := chain(context.Background(), &Request{})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestBuildChain_ConcurrentAccess(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	middleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return next(ctx, req)
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{middleware}, finalHandler)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = chain(context.Background(), &Request{})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if callCount != 100 {
		t.Errorf("expected 100 calls, got %d", callCount)
	}
}

func BenchmarkBuildChain_Empty(b *testing.B) {
	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildChain(nil, finalHandler)
	}
}

func BenchmarkBuildChain_Single(b *testing.B) {
	middleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			return next(ctx, req)
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	middlewares := []MiddlewareFunc{middleware}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildChain(middlewares, finalHandler)
	}
}

func BenchmarkBuildChain_Multiple(b *testing.B) {
	middleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			return next(ctx, req)
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	middlewares := []MiddlewareFunc{middleware, middleware, middleware}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildChain(middlewares, finalHandler)
	}
}

func BenchmarkChainExecution_Empty(b *testing.B) {
	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain(nil, finalHandler)
	ctx := context.Background()
	req := &Request{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain(ctx, req)
	}
}

func BenchmarkChainExecution_Single(b *testing.B) {
	middleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			return next(ctx, req)
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{middleware}, finalHandler)
	ctx := context.Background()
	req := &Request{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain(ctx, req)
	}
}

func BenchmarkChainExecution_Multiple(b *testing.B) {
	middleware := func(next Handler) Handler {
		return func(ctx context.Context, req *Request) (*Response, error) {
			return next(ctx, req)
		}
	}

	finalHandler := func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{StatusCode: 200}, nil
	}

	chain := BuildChain([]MiddlewareFunc{middleware, middleware, middleware}, finalHandler)
	ctx := context.Background()
	req := &Request{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain(ctx, req)
	}
}
