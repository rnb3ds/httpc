package pool

import (
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// REQUEST POOL TESTS
// ============================================================================

func TestRequestPool_GetPut(t *testing.T) {
	pool := &RequestPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &PooledRequest{
					Headers:     make(map[string]string),
					QueryParams: make(map[string]any),
				}
			},
		},
	}
	
	// Get from pool
	req := pool.Get()
	if req == nil {
		t.Fatal("Get() returned nil")
	}
	
	// Set some values
	req.Method = "GET"
	req.URL = "https://example.com"
	req.Headers["X-Test"] = "value"
	req.QueryParams["key"] = "value"
	
	// Put back to pool
	pool.Put(req)
	
	// Get again and verify it's reset
	req2 := pool.Get()
	if req2.Method != "" {
		t.Errorf("Expected empty Method after reset, got: %s", req2.Method)
	}
	if req2.URL != "" {
		t.Errorf("Expected empty URL after reset, got: %s", req2.URL)
	}
	if len(req2.Headers) != 0 {
		t.Errorf("Expected empty Headers after reset, got: %d items", len(req2.Headers))
	}
	if len(req2.QueryParams) != 0 {
		t.Errorf("Expected empty QueryParams after reset, got: %d items", len(req2.QueryParams))
	}
}

func TestPooledRequest_Reset(t *testing.T) {
	req := &PooledRequest{
		Method:      "POST",
		URL:         "https://example.com",
		Headers:     map[string]string{"X-Test": "value"},
		QueryParams: map[string]any{"key": "value"},
		Body:        "test body",
		Timeout:     5000,
		MaxRetries:  3,
		BasicAuth:   &BasicAuth{Username: "user", Password: "pass"},
		BearerToken: "token123",
	}
	
	req.Reset()
	
	if req.Method != "" {
		t.Errorf("Expected empty Method, got: %s", req.Method)
	}
	if req.URL != "" {
		t.Errorf("Expected empty URL, got: %s", req.URL)
	}
	if req.Body != nil {
		t.Errorf("Expected nil Body, got: %v", req.Body)
	}
	if req.Timeout != 0 {
		t.Errorf("Expected 0 Timeout, got: %d", req.Timeout)
	}
	if req.MaxRetries != 0 {
		t.Errorf("Expected 0 MaxRetries, got: %d", req.MaxRetries)
	}
	if req.BasicAuth != nil {
		t.Error("Expected nil BasicAuth")
	}
	if req.BearerToken != "" {
		t.Errorf("Expected empty BearerToken, got: %s", req.BearerToken)
	}
	if len(req.Headers) != 0 {
		t.Errorf("Expected empty Headers, got: %d items", len(req.Headers))
	}
	if len(req.QueryParams) != 0 {
		t.Errorf("Expected empty QueryParams, got: %d items", len(req.QueryParams))
	}
}

func TestRequestPool_Concurrent(t *testing.T) {
	pool := &RequestPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &PooledRequest{
					Headers:     make(map[string]string),
					QueryParams: make(map[string]any),
				}
			},
		},
	}
	
	var wg sync.WaitGroup
	concurrency := 100
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// Get from pool
			req := pool.Get()
			
			// Use it
			req.Method = "GET"
			req.URL = "https://example.com"
			
			// Put back
			pool.Put(req)
		}(i)
	}
	
	wg.Wait()
}

// ============================================================================
// BUFFER POOL TESTS
// ============================================================================

func TestBufferPool_GetPut(t *testing.T) {
	pool := &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}
	
	// Get from pool
	buf := pool.Get()
	if buf == nil {
		t.Fatal("Get() returned nil")
	}
	
	// Use buffer
	copy(buf, []byte("test data"))
	
	// Put back to pool
	pool.Put(buf)
	
	// Get again
	buf2 := pool.Get()
	if buf2 == nil {
		t.Fatal("Get() returned nil on second call")
	}
}

func TestBufferPool_SizeFiltering(t *testing.T) {
	pool := &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}
	
	tests := []struct {
		name       string
		bufferSize int
		shouldPool bool
	}{
		{"Too small", 16 * 1024, false},
		{"Min size", 32 * 1024, true},
		{"Normal size", 48 * 1024, true},
		{"Max size", 64 * 1024, true},
		{"Too large", 128 * 1024, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.bufferSize)
			pool.Put(buf)
			// Note: We can't directly test if it was pooled,
			// but we verify the method doesn't panic
		})
	}
}

func TestBufferPool_Concurrent(t *testing.T) {
	pool := &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 32*1024)
			},
		},
	}
	
	var wg sync.WaitGroup
	concurrency := 100
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			buf := pool.Get()
			copy(buf, []byte("test"))
			pool.Put(buf)
		}()
	}
	
	wg.Wait()
}

// ============================================================================
// RESPONSE POOL TESTS
// ============================================================================

func TestResponsePool_GetPut(t *testing.T) {
	pool := &ResponsePool{
		pool: sync.Pool{
			New: func() interface{} {
				return &PooledResponse{
					Headers: make(map[string][]string),
				}
			},
		},
	}
	
	// Get from pool
	resp := pool.Get()
	if resp == nil {
		t.Fatal("Get() returned nil")
	}
	
	// Set some values
	resp.StatusCode = 200
	resp.Status = "OK"
	resp.Body = "response body"
	resp.Headers["Content-Type"] = []string{"application/json"}
	
	// Put back to pool
	pool.Put(resp)
	
	// Get again and verify it's reset
	resp2 := pool.Get()
	if resp2.StatusCode != 0 {
		t.Errorf("Expected 0 StatusCode after reset, got: %d", resp2.StatusCode)
	}
	if resp2.Status != "" {
		t.Errorf("Expected empty Status after reset, got: %s", resp2.Status)
	}
	if resp2.Body != "" {
		t.Errorf("Expected empty Body after reset, got: %s", resp2.Body)
	}
}

func TestPooledResponse_Reset(t *testing.T) {
	resp := &PooledResponse{
		StatusCode:    200,
		Status:        "OK",
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		Body:          "test body",
		RawBody:       []byte("raw body"),
		ContentLength: 100,
		Proto:         "HTTP/1.1",
		Duration:      1000000,
		Attempts:      3,
	}
	
	resp.Reset()
	
	if resp.StatusCode != 0 {
		t.Errorf("Expected 0 StatusCode, got: %d", resp.StatusCode)
	}
	if resp.Status != "" {
		t.Errorf("Expected empty Status, got: %s", resp.Status)
	}
	if resp.Headers != nil {
		t.Error("Expected nil Headers")
	}
	if resp.Body != "" {
		t.Errorf("Expected empty Body, got: %s", resp.Body)
	}
	if resp.RawBody != nil {
		t.Error("Expected nil RawBody")
	}
	if resp.ContentLength != 0 {
		t.Errorf("Expected 0 ContentLength, got: %d", resp.ContentLength)
	}
	if resp.Proto != "" {
		t.Errorf("Expected empty Proto, got: %s", resp.Proto)
	}
	if resp.Duration != 0 {
		t.Errorf("Expected 0 Duration, got: %d", resp.Duration)
	}
	if resp.Attempts != 0 {
		t.Errorf("Expected 0 Attempts, got: %d", resp.Attempts)
	}
}

// ============================================================================
// STRING BUILDER POOL TESTS
// ============================================================================

func TestStringBuilderPool_GetPut(t *testing.T) {
	pool := &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}
	
	// Get from pool
	sb := pool.Get()
	if sb == nil {
		t.Fatal("Get() returned nil")
	}
	
	// Use it
	sb.WriteString("test string")
	if sb.String() != "test string" {
		t.Errorf("Expected 'test string', got: %s", sb.String())
	}
	
	// Put back to pool
	pool.Put(sb)
	
	// Get again and verify it's reset
	sb2 := pool.Get()
	if sb2.Len() != 0 {
		t.Errorf("Expected empty builder after reset, got length: %d", sb2.Len())
	}
}

func TestStringBuilderPool_Concurrent(t *testing.T) {
	pool := &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &strings.Builder{}
			},
		},
	}
	
	var wg sync.WaitGroup
	concurrency := 100
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			sb := pool.Get()
			sb.WriteString("test")
			pool.Put(sb)
		}(i)
	}
	
	wg.Wait()
}

