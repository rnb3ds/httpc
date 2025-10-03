package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient creates a client with AllowPrivateIPs enabled for testing
func newTestClient() (Client, error) {
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	return New(config)
}

// ============================================================================
// BENCHMARK TESTS - Basic Operations
// ============================================================================

func BenchmarkClient_Get(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Get(server.URL)
			if err != nil {
				b.Errorf("Request failed: %v", err)
			}
		}
	})
}

func BenchmarkClient_Post(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	data := map[string]string{"key": "value"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Post(server.URL, WithJSON(data))
			if err != nil {
				b.Errorf("Request failed: %v", err)
			}
		}
	})
}

func BenchmarkClient_GetWithOptions(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Get(server.URL,
				WithHeader("X-Custom", "value"),
				WithQuery("param", "value"),
				WithTimeout(30*time.Second),
			)
			if err != nil {
				b.Errorf("Request failed: %v", err)
			}
		}
	})
}

// ============================================================================
// BENCHMARK TESTS - JSON Operations
// ============================================================================

func BenchmarkClient_JSONMarshal(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	type TestStruct struct {
		Name    string
		Age     int
		Email   string
		Active  bool
		Tags    []string
		Metadata map[string]interface{}
	}

	data := TestStruct{
		Name:   "John Doe",
		Age:    30,
		Email:  "john@example.com",
		Active: true,
		Tags:   []string{"tag1", "tag2", "tag3"},
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Post(server.URL, WithJSON(data))
		if err != nil {
			b.Errorf("Request failed: %v", err)
		}
	}
}

func BenchmarkClient_JSONUnmarshal(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"John","age":30,"email":"john@example.com","active":true}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	type TestStruct struct {
		Name   string `json:"name"`
		Age    int    `json:"age"`
		Email  string `json:"email"`
		Active bool   `json:"active"`
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			b.Errorf("Request failed: %v", err)
			continue
		}

		var data TestStruct
		if err := resp.JSON(&data); err != nil {
			b.Errorf("JSON unmarshal failed: %v", err)
		}
	}
}

// ============================================================================
// BENCHMARK TESTS - Concurrency
// ============================================================================

func BenchmarkClient_ConcurrentRequests_10(b *testing.B) {
	benchmarkConcurrentRequests(b, 10)
}

func BenchmarkClient_ConcurrentRequests_50(b *testing.B) {
	benchmarkConcurrentRequests(b, 50)
}

func BenchmarkClient_ConcurrentRequests_100(b *testing.B) {
	benchmarkConcurrentRequests(b, 100)
}

func BenchmarkClient_ConcurrentRequests_500(b *testing.B) {
	benchmarkConcurrentRequests(b, 500)
}

func benchmarkConcurrentRequests(b *testing.B, concurrency int) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	b.ResetTimer()
	b.SetParallelism(concurrency)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Get(server.URL)
			if err != nil {
				b.Errorf("Request failed: %v", err)
			}
		}
	})
}

// ============================================================================
// BENCHMARK TESTS - Different Payload Sizes
// ============================================================================

func BenchmarkClient_SmallPayload(b *testing.B) {
	benchmarkPayloadSize(b, 100) // 100 bytes
}

func BenchmarkClient_MediumPayload(b *testing.B) {
	benchmarkPayloadSize(b, 10*1024) // 10KB
}

func BenchmarkClient_LargePayload(b *testing.B) {
	benchmarkPayloadSize(b, 1024*1024) // 1MB
}

func benchmarkPayloadSize(b *testing.B, size int) {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(payload)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			b.Errorf("Request failed: %v", err)
			continue
		}
		_ = resp.RawBody
	}
}

// ============================================================================
// BENCHMARK TESTS - Connection Pooling
// ============================================================================

func BenchmarkClient_ConnectionReuse(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxIdleConns = 100
	config.MaxIdleConnsPerHost = 10

	client, _ := New(config)
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.Get(server.URL)
		if err != nil {
			b.Errorf("Request failed: %v", err)
		}
	}
}

func BenchmarkClient_NoConnectionReuse(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create new client for each request (no connection reuse)
		client, _ := newTestClient()
		_, err := client.Get(server.URL)
		client.Close()
		if err != nil {
			b.Errorf("Request failed: %v", err)
		}
	}
}

// ============================================================================
// BENCHMARK TESTS - Request Options
// ============================================================================

func BenchmarkRequestOptions_WithHeader(b *testing.B) {
	req := &Request{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opt := WithHeader("X-Test", "value")
		opt(req)
	}
}

func BenchmarkRequestOptions_WithJSON(b *testing.B) {
	data := map[string]string{"key": "value"}
	req := &Request{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opt := WithJSON(data)
		opt(req)
	}
}

func BenchmarkRequestOptions_WithQuery(b *testing.B) {
	req := &Request{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opt := WithQuery("key", "value")
		opt(req)
	}
}

func BenchmarkRequestOptions_Multiple(b *testing.B) {
	req := &Request{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := []RequestOption{
			WithHeader("X-Test", "value"),
			WithQuery("key", "value"),
			WithTimeout(30 * time.Second),
			WithUserAgent("TestAgent"),
		}
		for _, opt := range opts {
			opt(req)
		}
	}
}

// ============================================================================
// PERFORMANCE TESTS - Throughput
// ============================================================================

func TestPerformance_Throughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	duration := 10 * time.Second
	var requestCount int64
	var errorCount int64

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Launch multiple workers
	numWorkers := 50
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_, err := client.Get(server.URL)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
					} else {
						atomic.AddInt64(&requestCount, 1)
					}
				}
			}
		}()
	}

	wg.Wait()

	rps := float64(requestCount) / duration.Seconds()
	t.Logf("Throughput: %.2f requests/second", rps)
	t.Logf("Total requests: %d", requestCount)
	t.Logf("Total errors: %d", errorCount)
	t.Logf("Error rate: %.2f%%", float64(errorCount)/float64(requestCount)*100)
}

func TestPerformance_Latency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(1 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	numRequests := 1000
	latencies := make([]time.Duration, numRequests)

	for i := 0; i < numRequests; i++ {
		start := time.Now()
		_, err := client.Get(server.URL)
		latencies[i] = time.Since(start)
		if err != nil {
			t.Errorf("Request %d failed: %v", i, err)
		}
	}

	// Calculate statistics
	var total time.Duration
	var min, max time.Duration = latencies[0], latencies[0]

	for _, lat := range latencies {
		total += lat
		if lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
	}

	avg := total / time.Duration(numRequests)

	t.Logf("Latency Statistics:")
	t.Logf("  Min: %v", min)
	t.Logf("  Max: %v", max)
	t.Logf("  Avg: %v", avg)
}

