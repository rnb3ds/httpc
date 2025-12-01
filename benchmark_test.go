package httpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// BASELINE BENCHMARKS - Measure current performance
// ============================================================================

func BenchmarkClient_SimpleGET(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer func() { _ = client.Close() }()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Get(server.URL)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_POST_JSON(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer func() { _ = client.Close() }()

	payload := map[string]interface{}{
		"name":  "test",
		"value": 123,
		"tags":  []string{"a", "b", "c"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Post(server.URL, WithJSON(payload))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_Concurrent_Requests(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer func() { _ = client.Close() }()

	concurrency := []int{1, 10, 50, 100}

	for _, c := range concurrency {
		b.Run(fmt.Sprintf("Concurrency_%d", c), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := client.Get(server.URL)
					if err != nil {
						b.Error(err)
					}
				}
			})
		})
	}
}

// ============================================================================
// MEMORY ALLOCATION BENCHMARKS
// ============================================================================

func BenchmarkClient_MemoryAllocation_SmallResponse(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Small response"))
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Body
	}
}

func BenchmarkClient_MemoryAllocation_LargeResponse(b *testing.B) {
	largeBody := strings.Repeat("x", 64*1024) // 64KB

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(largeBody))
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.RawBody
	}
}

// ============================================================================
// RETRY AND TIMEOUT BENCHMARKS
// ============================================================================

func BenchmarkClient_WithRetry(b *testing.B) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts%3 != 0 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 1 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = client.Get(server.URL)
	}
}

func BenchmarkClient_WithTimeout(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Get(server.URL, WithTimeout(100*time.Millisecond))
		if err != nil {
			b.Error(err)
		}
	}
}

// ============================================================================
// CONTEXT CANCELLATION BENCHMARKS
// ============================================================================

func BenchmarkClient_ContextCancellation(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, _ = client.Request(ctx, "GET", server.URL)
		cancel()
	}
}

// ============================================================================
// DEFAULT CLIENT BENCHMARKS
// ============================================================================

func BenchmarkDefaultClient_Get(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Ensure default client is initialized
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	_ = SetDefaultClient(client)
	defer func() { _ = CloseDefaultClient() }()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := Get(server.URL)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// HEADER MANIPULATION BENCHMARKS
// ============================================================================

func BenchmarkClient_WithHeaders(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newBenchmarkClient()
	defer client.Close()

	headers := map[string]string{
		"X-Custom-1": "value1",
		"X-Custom-2": "value2",
		"X-Custom-3": "value3",
		"X-Custom-4": "value4",
		"X-Custom-5": "value5",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Get(server.URL, WithHeaderMap(headers))
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func newBenchmarkClient() (Client, error) {
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.MaxRetries = 0
	return New(config)
}
