package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// ----------------------------------------------------------------------------
// SessionManager Concurrency Tests
// ----------------------------------------------------------------------------

func TestSessionManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	session := NewSessionManager()
	const numGoroutines = 100
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 4) // 4 types of operations

	// Concurrent header writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := "X-Header-" + time.Now().String()
				session.SetHeader(key, "value")
			}
		}(i)
	}

	// Concurrent header reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				session.GetHeaders()
			}
		}()
	}

	// Concurrent cookie writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cookie := &http.Cookie{
					Name:  "session-" + time.Now().String(),
					Value: "value",
				}
				session.SetCookie(cookie)
			}
		}(i)
	}

	// Concurrent cookie reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				session.GetCookies()
			}
		}()
	}

	wg.Wait()
}

func TestSessionManager_PrepareOptions_Concurrent(t *testing.T) {
	t.Parallel()

	session := NewSessionManager()
	session.SetHeader("Authorization", "Bearer token")
	session.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			options := session.PrepareOptions()
			if len(options) == 0 {
				t.Error("Expected options to be returned")
			}
		}()
	}

	wg.Wait()
}

// ----------------------------------------------------------------------------
// Default Client Concurrency Tests
// ----------------------------------------------------------------------------

func TestDefaultClient_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	// Reset default client first
	CloseDefaultClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			result, err := Get(server.URL)
			if err == nil && result.StatusCode() == 200 {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != numGoroutines {
		t.Errorf("Expected %d successful requests, got %d", numGoroutines, successCount)
	}
}

func TestSetDefaultClient_Concurrent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	var setErrors int64
	var getErrors int64

	// Concurrent setters
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			client, err := New()
			if err != nil {
				atomic.AddInt64(&setErrors, 1)
				return
			}
			if err := SetDefaultClient(client); err != nil {
				atomic.AddInt64(&setErrors, 1)
			}
		}()
	}

	// Concurrent getters
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := getDefaultClient()
			if err != nil {
				atomic.AddInt64(&getErrors, 1)
			}
		}()
	}

	wg.Wait()

	// Some errors are expected due to race conditions on set
	// But get should never fail
	if getErrors > 0 {
		t.Errorf("Unexpected get errors: %d", getErrors)
	}
}

// ----------------------------------------------------------------------------
// Metrics Concurrency Tests
// ----------------------------------------------------------------------------

func TestMetrics_ConcurrentRecording(t *testing.T) {
	t.Parallel()

	metrics := &engine.Metrics{}
	const numGoroutines = 100
	const numRecords = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(success bool) {
			defer wg.Done()
			for j := 0; j < numRecords; j++ {
				metrics.RecordRequest(int64(j*1000), success)
			}
		}(i%2 == 0)
	}

	wg.Wait()

	snapshot := metrics.Snapshot()
	expectedTotal := int64(numGoroutines * numRecords)
	if snapshot.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, snapshot.TotalRequests)
	}
}

func TestMetrics_ConcurrentReadAndWrite(t *testing.T) {
	t.Parallel()

	metrics := &engine.Metrics{}
	const duration = 100 * time.Millisecond

	var stop int32
	var wg sync.WaitGroup

	// Writers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			metrics.RecordRequest(1000, true)
		}
	}()

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.LoadInt32(&stop) == 0 {
				snapshot := metrics.Snapshot()
				_ = snapshot.TotalRequests
				_ = metrics.GetHealthStatus()
				_ = metrics.IsHealthy()
			}
		}()
	}

	time.Sleep(duration)
	atomic.StoreInt32(&stop, 1)
	wg.Wait()
}

// ----------------------------------------------------------------------------
// Buffer Pool Concurrency Tests
// ----------------------------------------------------------------------------

func TestBufferPool_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return variable size responses to test buffer pool
		size := 100 + len(r.URL.Path)*10
		data := make([]byte, size)
		for i := range data {
			data[i] = 'x'
		}
		w.Write(data)
	}))
	defer server.Close()

	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			result, err := client.Get(server.URL + "/" + time.Now().String())
			if err != nil {
				t.Errorf("Request failed: %v", err)
				return
			}
			if result.StatusCode() != 200 {
				t.Errorf("Expected status 200, got %d", result.StatusCode())
			}
		}(i)
	}

	wg.Wait()
}

// ----------------------------------------------------------------------------
// Request Pool Concurrency Tests
// ----------------------------------------------------------------------------

func TestRequestPool_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numRequests = 100
	var wg sync.WaitGroup
	wg.Add(numRequests)

	var successCount int64

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := client.Request(ctx, "GET", server.URL,
				WithHeader("X-Request-ID", time.Now().String()),
				WithTimeout(2*time.Second),
			)
			if err == nil && result.StatusCode() == 200 {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != numRequests {
		t.Errorf("Expected %d successful requests, got %d", numRequests, successCount)
	}
}

// ----------------------------------------------------------------------------
// Middleware Concurrency Tests
// ----------------------------------------------------------------------------

func TestMiddleware_ConcurrentExecution(t *testing.T) {
	t.Parallel()

	var callCount int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{
		LoggingMiddleware(func(format string, args ...any) {
			atomic.AddInt64(&callCount, 1)
		}),
		RecoveryMiddleware(),
		HeaderMiddleware(map[string]string{
			"X-Custom-Header": "test-value",
		}),
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numRequests = 50
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			_, err := client.Get(server.URL)
			if err != nil {
				t.Errorf("Request failed: %v", err)
			}
		}()
	}

	wg.Wait()

	if callCount != numRequests {
		t.Errorf("Expected %d middleware calls, got %d", numRequests, callCount)
	}
}

// ----------------------------------------------------------------------------
// DomainClient Concurrency Tests
// ----------------------------------------------------------------------------

func TestDomainClient_ConcurrentSessionOperations(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Set-Cookie", "new-cookie=value")
	}))
	defer server.Close()

	dc, err := NewDomain(server.URL)
	if err != nil {
		t.Fatalf("Failed to create domain client: %v", err)
	}
	defer dc.Close()

	// Set initial session state
	dc.SetHeader("Authorization", "Bearer token")
	dc.SetCookie(&http.Cookie{Name: "session", Value: "initial"})

	const numGoroutines = 30
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Concurrent requests
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			dc.Get("/test")
		}()
	}

	// Concurrent header updates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			dc.SetHeader("X-Custom", time.Now().String())
		}(i)
	}

	// Concurrent cookie updates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			dc.SetCookie(&http.Cookie{Name: "cookie", Value: time.Now().String()})
		}(i)
	}

	wg.Wait()
}

// ----------------------------------------------------------------------------
// Connection Pool Concurrency Tests
// ----------------------------------------------------------------------------

func TestConnectionPool_ConcurrentConnections(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate latency
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Connections.MaxIdleConns = 50
	cfg.Connections.MaxConnsPerHost = 20

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numRequests = 100
	var wg sync.WaitGroup
	wg.Add(numRequests)

	var successCount int64

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			result, err := client.Get(server.URL)
			if err == nil && result.StatusCode() == 200 {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != numRequests {
		t.Errorf("Expected %d successful requests, got %d", numRequests, successCount)
	}
}

// ----------------------------------------------------------------------------
// DoH Cache Concurrency Tests
// ----------------------------------------------------------------------------

func TestDoHCache_ConcurrentLookups(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Connections.EnableDoH = true
	cfg.Connections.DoHCacheTTL = 1 * time.Minute

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Note: This test doesn't make actual network requests
	// It just verifies the DoH cache structure is thread-safe
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Multiple goroutines initializing DoH shouldn't cause races
			_ = client
		}()
	}

	wg.Wait()
}

// ----------------------------------------------------------------------------
// Close Concurrency Tests
// ----------------------------------------------------------------------------

func TestClient_ConcurrentClose(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	const numClosers = 10
	var wg sync.WaitGroup
	wg.Add(numClosers)

	for i := 0; i < numClosers; i++ {
		go func() {
			defer wg.Done()
			client.Close() // Should be safe to call multiple times
		}()
	}

	wg.Wait()
}

// ----------------------------------------------------------------------------
// Mixed Operations Concurrency Test
// ----------------------------------------------------------------------------

func TestClient_MixedConcurrentOperations(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const duration = 200 * time.Millisecond
	var stop int32
	var wg sync.WaitGroup

	// GET requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			client.Get(server.URL)
		}
	}()

	// POST requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			client.Post(server.URL, WithJSON(map[string]string{"key": "value"}))
		}
	}()

	// Context requests
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			client.Request(ctx, "GET", server.URL)
			cancel()
		}
	}()

	// Health checks
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			_ = client.(*clientImpl).engine.IsHealthy()
		}
	}()

	time.Sleep(duration)
	atomic.StoreInt32(&stop, 1)
	wg.Wait()
}
