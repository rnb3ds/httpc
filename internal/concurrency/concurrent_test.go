// Package concurrency_test provides concurrent safety tests for the httpc library.
// These tests verify thread-safety under high concurrency scenarios.
package concurrency_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/httpc"
	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/dns"
	"github.com/cybergodev/httpc/internal/security"
)

// TestConcurrentClientRequests tests concurrent requests using the same client.
func TestConcurrentClientRequests(t *testing.T) {
	// Create test server with mutex for thread safety
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client, err := httpc.New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 50
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	var errors int64
	var success int64

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				result, err := client.Get(server.URL)
				if err != nil {
					atomic.AddInt64(&errors, 1)
					continue
				}
				if result.StatusCode() != 200 {
					atomic.AddInt64(&errors, 1)
					continue
				}
				atomic.AddInt64(&success, 1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Concurrent test completed: %d success, %d errors, %d server requests",
		success, errors, requestCount)
}

// TestConcurrentDomainClientSession tests concurrent session operations.
func TestConcurrentDomainClientSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back received cookies and headers
		cookies := r.Cookies()
		for _, c := range cookies {
			http.SetCookie(w, c)
		}
		w.Header().Set("X-Received", r.Header.Get("X-Session-Header"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dc, err := httpc.NewDomain(server.URL)
	if err != nil {
		t.Fatalf("Failed to create domain client: %v", err)
	}
	defer dc.Close()

	const numGoroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent header operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Set header
				headerKey := fmt.Sprintf("X-Session-Header-%d", id)
				headerValue := fmt.Sprintf("value-%d-%d", id, j)
				if err := dc.SetHeader(headerKey, headerValue); err != nil {
					t.Errorf("Failed to set header: %v", err)
					return
				}

				// Set cookie
				cookie := &http.Cookie{
					Name:  fmt.Sprintf("session-cookie-%d", id),
					Value: fmt.Sprintf("value-%d-%d", id, j),
				}
				if err := dc.SetCookie(cookie); err != nil {
					t.Errorf("Failed to set cookie: %v", err)
					return
				}

				// Read headers
				headers := dc.GetHeaders()
				if headers == nil {
					t.Errorf("GetHeaders returned nil")
					return
				}

				// Read cookies
				cookies := dc.GetCookies()
				_ = cookies // Just verify no panic

				// Delete header
				dc.DeleteHeader(headerKey)

				// Delete cookie
				dc.DeleteCookie(fmt.Sprintf("session-cookie-%d", id))
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentSessionManager tests SessionManager under concurrent access.
func TestConcurrentSessionManager(t *testing.T) {
	sm := httpc.NewSessionManager()

	const numGoroutines = 100
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent operations on SessionManager
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Set header
				if err := sm.SetHeader(fmt.Sprintf("X-Header-%d", id), fmt.Sprintf("value-%d", j)); err != nil {
					t.Errorf("SetHeader failed: %v", err)
					return
				}

				// Set cookie
				cookie := &http.Cookie{
					Name:  fmt.Sprintf("cookie-%d", id),
					Value: fmt.Sprintf("value-%d-%d", id, j),
				}
				if err := sm.SetCookie(cookie); err != nil {
					t.Errorf("SetCookie failed: %v", err)
					return
				}

				// Read operations
				_ = sm.GetHeaders()
				_ = sm.GetCookies()
				_ = sm.GetCookie(fmt.Sprintf("cookie-%d", id))
				_ = sm.PrepareOptions()

				// Delete operations
				sm.DeleteHeader(fmt.Sprintf("X-Header-%d", id))
				sm.DeleteCookie(fmt.Sprintf("cookie-%d", id))
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent
	headers := sm.GetHeaders()
	cookies := sm.GetCookies()

	t.Logf("Final state: %d headers, %d cookies", len(headers), len(cookies))
}

// TestConcurrentDoHResolverCache tests concurrent DNS cache access.
func TestConcurrentDoHResolverCache(t *testing.T) {
	providers := []*dns.DoHProvider{
		{Name: "test", Template: "https://example.com/dns-query?name={name}", Priority: 1},
	}
	resolver := dns.NewDoHResolver(providers, 5*time.Minute)

	const numGoroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Test cache size access (atomic)
				_ = resolver.CacheSize()

				// Test TTL read (atomic)
				_ = resolver.GetCacheTTL()

				// Test cache clearing
				if j%10 == 0 {
					resolver.ClearCache()
				}

				// Test TTL setting (atomic - this was the race condition we fixed)
				if j%5 == 0 {
					resolver.SetCacheTTL(time.Minute)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Final cache size: %d", resolver.CacheSize())
}

// TestConcurrentConnectionPool tests concurrent connection pool operations.
func TestConcurrentConnectionPool(t *testing.T) {
	cfg := connection.DefaultConfig()
	cfg.MaxIdleConns = 100
	cfg.MaxIdleConnsPerHost = 20

	pm, err := connection.NewPoolManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer pm.Close()

	const numGoroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Concurrent metrics access (atomic)
				_ = pm.GetMetrics()
				_ = pm.IsHealthy()

				// Concurrent transport access (read-only after creation)
				transport := pm.GetTransport()
				if transport == nil {
					t.Error("GetTransport returned nil")
					return
				}
			}
		}(i)
	}

	wg.Wait()

	metrics := pm.GetMetrics()
	t.Logf("Final pool metrics: Active=%d, Total=%d, Rejected=%d, HitRate=%.2f",
		metrics.ActiveConnections, metrics.TotalConnections,
		metrics.RejectedConnections, metrics.ConnectionHitRate)
}

// TestConcurrentDomainWhitelist tests concurrent domain whitelist operations.
func TestConcurrentDomainWhitelist(t *testing.T) {
	dw := security.NewDomainWhitelist("example.com", "*.trusted.org")

	const numGoroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var allowedCount int64
	var deniedCount int64

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Concurrent Add operations
				domain := fmt.Sprintf("domain-%d-%d.com", id, j)
				dw.Add(domain)

				// Concurrent IsAllowed checks
				if dw.IsAllowed(domain) {
					atomic.AddInt64(&allowedCount, 1)
				} else {
					atomic.AddInt64(&deniedCount, 1)
				}

				// Concurrent Domains read
				exact, wildcards := dw.Domains()
				_ = exact
				_ = wildcards

				// Concurrent Remove operations
				if j%10 == 0 {
					dw.Remove(domain)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Domain whitelist test: %d allowed, %d denied", allowedCount, deniedCount)
}

// TestConcurrentDefaultClient tests concurrent access to the default client.
func TestConcurrentDefaultClient(t *testing.T) {
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Reset default client
	httpc.CloseDefaultClient()

	const numGoroutines = 10
	const requestsPerGoroutine = 5

	var wg sync.WaitGroup
	var errors int64

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				// Package-level functions use default client
				_, err := httpc.Get(server.URL)
				if err != nil {
					atomic.AddInt64(&errors, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Cleanup
	httpc.CloseDefaultClient()

	t.Logf("Default client test: %d errors, %d server requests", errors, requestCount)
}

// TestConcurrentContextCancellation tests proper handling of concurrent context cancellations.
func TestConcurrentContextCancellation(t *testing.T) {
	var requestCount int64
	// Slow server that takes time to respond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := httpc.New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 10

	var wg sync.WaitGroup
	var cancelledCount int64
	var successCount int64
	var errorCount int64

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Create context with short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			result, err := client.Request(ctx, "GET", server.URL)
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					atomic.AddInt64(&cancelledCount, 1)
				} else {
					atomic.AddInt64(&errorCount, 1)
				}
			} else {
				_ = result
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Context cancellation test: %d cancelled, %d succeeded, %d other errors, %d server requests",
		cancelledCount, successCount, errorCount, requestCount)
}

// TestRaceConditionMetricsUpdate tests for race conditions in metrics updates.
func TestRaceConditionMetricsUpdate(t *testing.T) {
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		// Random latency to test rolling average calculation
		time.Sleep(time.Duration(1+hashString(r.URL.Path)%5) * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := httpc.New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 20
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				// Make requests with varying URLs to exercise metrics code
				url := fmt.Sprintf("%s/path/%d/%d", server.URL, id, j)
				_, _ = client.Get(url)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Race condition test completed: %d server requests", requestCount)
}

// TestConcurrentClientClose tests closing client while requests are in flight.
func TestConcurrentClientClose(t *testing.T) {
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	const numAttempts = 10
	var closeErrors int64
	var requestErrors int64

	var wg sync.WaitGroup
	wg.Add(numAttempts)

	for i := 0; i < numAttempts; i++ {
		go func(id int) {
			defer wg.Done()

			client, err := httpc.New()
			if err != nil {
				atomic.AddInt64(&closeErrors, 1)
				return
			}

			// Start a request in a separate goroutine
			var reqWg sync.WaitGroup
			reqWg.Add(1)
			go func() {
				defer reqWg.Done()
				_, err := client.Get(server.URL)
				if err != nil {
					atomic.AddInt64(&requestErrors, 1)
				}
			}()

			// Close while request might be in flight
			time.Sleep(time.Duration(id%30) * time.Millisecond)
			if err := client.Close(); err != nil {
				atomic.AddInt64(&closeErrors, 1)
			}

			reqWg.Wait()
		}(i)
	}

	wg.Wait()

	// Some request errors are expected when closing during request
	t.Logf("Client close test: %d close errors, %d request errors, %d server requests",
		closeErrors, requestErrors, requestCount)
}

// TestConcurrentResultPool tests concurrent Result pooling operations.
func TestConcurrentResultPool(t *testing.T) {
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer server.Close()

	client, err := httpc.New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 20
	const opsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Get result
				result, err := client.Get(server.URL)
				if err != nil {
					continue
				}

				// Access result data (concurrent read from pool)
				_ = result.StatusCode()
				_ = result.Body()
				// Access headers via Response struct field
				if result.Response != nil {
					_ = result.Response.Headers
				}

				// Release to pool
				httpc.ReleaseResult(result)
			}
		}()
	}

	wg.Wait()

	t.Logf("Result pool test: %d server requests", requestCount)
}

// TestConcurrentCookieJar tests concurrent cookie jar operations.
func TestConcurrentCookieJar(t *testing.T) {
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		// Set cookies and echo back
		http.SetCookie(w, &http.Cookie{
			Name:  "server-cookie",
			Value: "server-value",
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.EnableCookies = true

	client, err := httpc.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 10
	const requestsPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				_, _ = client.Get(server.URL)
			}
		}()
	}

	wg.Wait()

	t.Logf("Cookie jar test: %d server requests", requestCount)
}

// TestConcurrentRedirectHandling tests concurrent redirect handling.
func TestConcurrentRedirectHandling(t *testing.T) {
	var requestCount int64
	var redirectCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		if r.URL.Path == "/redirect" {
			atomic.AddInt64(&redirectCount, 1)
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	client, err := httpc.New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numGoroutines = 10
	const requestsPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				_, _ = client.Get(server.URL + "/redirect")
			}
		}()
	}

	wg.Wait()

	t.Logf("Redirect handling test: %d server requests, %d redirects", requestCount, redirectCount)
}

// Helper function for deterministic pseudo-random
func hashString(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}
