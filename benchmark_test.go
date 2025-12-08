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
// RESULT OBJECT BENCHMARKS
// ============================================================================

func BenchmarkResult_ConvenienceMethods(b *testing.B) {
	result := &Result{
		Request: &RequestInfo{
			Cookies: []*http.Cookie{
				{Name: "session", Value: "abc123"},
				{Name: "token", Value: "xyz789"},
			},
		},
		Response: &ResponseInfo{
			StatusCode: 200,
			Body:       "test body content",
			RawBody:    []byte("test body content"),
			Cookies: []*http.Cookie{
				{Name: "resp1", Value: "val1"},
				{Name: "resp2", Value: "val2"},
			},
		},
		Meta: &RequestMeta{
			Duration: 100 * time.Millisecond,
			Attempts: 1,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = result.Body()
		_ = result.StatusCode()
		_ = result.RequestCookies()
		_ = result.ResponseCookies()
		_ = result.IsSuccess()
	}
}

func BenchmarkResult_CookieAccess(b *testing.B) {
	result := &Result{
		Response: &ResponseInfo{
			Cookies: []*http.Cookie{
				{Name: "cookie1", Value: "value1"},
				{Name: "cookie2", Value: "value2"},
				{Name: "cookie3", Value: "value3"},
				{Name: "session", Value: "abc123"},
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = result.GetCookie("session")
		_ = result.HasCookie("session")
	}
}

func BenchmarkResult_JSON(b *testing.B) {
	result := &Result{
		Response: &ResponseInfo{
			RawBody: []byte(`{"name":"John","age":30,"email":"john@example.com"}`),
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var data map[string]interface{}
		_ = result.JSON(&data)
	}
}

func BenchmarkResult_String(b *testing.B) {
	result := &Result{
		Response: &ResponseInfo{
			StatusCode:    200,
			Status:        "200 OK",
			ContentLength: 1024,
			Body:          "test response body",
		},
		Meta: &RequestMeta{
			Duration: 50 * time.Millisecond,
			Attempts: 1,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = result.String()
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
