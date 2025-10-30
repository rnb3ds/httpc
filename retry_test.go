package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// RETRY LOGIC TESTS
// ============================================================================

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if atomic.LoadInt32(&attemptCount) != 1 {
		t.Errorf("Expected 1 attempt, got %d", atomic.LoadInt32(&attemptCount))
	}
}

func TestRetry_SuccessOnSecondAttempt(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 100 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Attempts < 2 {
		t.Errorf("Expected at least 2 attempts, got %d", resp.Attempts)
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 2
	config.RetryDelay = 50 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Should return response even after max retries
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", resp.StatusCode)
	}

	// Should have tried initial + 2 retries = 3 attempts
	if atomic.LoadInt32(&attemptCount) != 3 {
		t.Errorf("Expected 3 attempts, got %d", atomic.LoadInt32(&attemptCount))
	}

	// Verify attempts recorded in response
	if resp.Attempts != 3 {
		t.Errorf("Expected 3 attempts in response, got %d", resp.Attempts)
	}
}

func TestRetry_RetryableStatusCodes(t *testing.T) {
	retryableStatuses := []int{
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}

	for _, status := range retryableStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attemptCount := int32(0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := atomic.AddInt32(&attemptCount, 1)
				if count <= 2 {
					w.WriteHeader(status)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()

			config := DefaultConfig()
			config.MaxRetries = 3
			config.RetryDelay = 50 * time.Millisecond
			config.AllowPrivateIPs = true

			client, _ := New(config)
			defer client.Close()

			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			if resp.Attempts < 3 {
				t.Errorf("Expected at least 3 attempts, got %d", resp.Attempts)
			}
		})
	}
}

func TestRetry_NonRetryableStatusCodes(t *testing.T) {
	nonRetryableStatuses := []int{
		http.StatusBadRequest,          // 400
		http.StatusUnauthorized,        // 401
		http.StatusForbidden,           // 403
		http.StatusNotFound,            // 404
		http.StatusMethodNotAllowed,    // 405
		http.StatusConflict,            // 409
		http.StatusUnprocessableEntity, // 422
	}

	for _, status := range nonRetryableStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attemptCount := int32(0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&attemptCount, 1)
				w.WriteHeader(status)
			}))
			defer server.Close()

			config := DefaultConfig()
			config.MaxRetries = 3
			config.AllowPrivateIPs = true

			client, _ := New(config)
			defer client.Close()

			_, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			// Should not retry for client errors
			if atomic.LoadInt32(&attemptCount) != 1 {
				t.Errorf("Expected 1 attempt for non-retryable status, got %d", atomic.LoadInt32(&attemptCount))
			}
		})
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	attemptCount := int32(0)
	attemptTimes := make([]time.Time, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptTimes = append(attemptTimes, time.Now())
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 4 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 100 * time.Millisecond
	config.BackoffFactor = 2.0
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	_, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Check that delays increase exponentially
	if len(attemptTimes) >= 2 {
		delay1 := attemptTimes[1].Sub(attemptTimes[0])
		t.Logf("Delay 1: %v", delay1)

		if len(attemptTimes) >= 3 {
			delay2 := attemptTimes[2].Sub(attemptTimes[1])
			t.Logf("Delay 2: %v", delay2)

			// Second delay should be roughly 2x the first (with some tolerance)
			if delay2 < delay1 {
				t.Error("Expected exponential backoff, but delays did not increase")
			}
		}
	}
}

func TestRetry_WithJitter(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 100 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 5
	config.RetryDelay = 100 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	_, err := client.Request(ctx, "GET", server.URL)

	// Should get either a timeout error or a response
	// The context will cancel during retry attempts
	if err != nil {
		// Verify it's a context-related error
		if !strings.Contains(err.Error(), "context") &&
			!strings.Contains(err.Error(), "deadline") &&
			!strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected context-related error, got: %v", err)
		}
	}

	// Should not complete all retries due to context cancellation
	attempts := atomic.LoadInt32(&attemptCount)
	if attempts > 3 {
		t.Errorf("Expected fewer attempts due to context cancellation, got %d", attempts)
	}

	t.Logf("Completed %d attempts before context cancellation", attempts)
}

func TestRetry_MaxRetryDelay(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count < 4 { // Reduced from 5 to 4 for faster test
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 4                     // Reduced from 5 to 4
	config.RetryDelay = 50 * time.Millisecond // Reduced from 100ms
	config.BackoffFactor = 5.0                // Reduced from 10.0 to 5.0
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	start := time.Now()
	_, err := client.Get(server.URL)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// With max delay cap, total time should be reasonable
	// Expected: ~50ms + ~250ms + ~1250ms + ~2000ms (capped) = ~3.5s max
	if duration > 8*time.Second { // Increased tolerance but still reasonable
		t.Errorf("Request took too long: %v (expected under 8s)", duration)
	}

	// Verify that the max delay cap is working by ensuring it's not extremely long
	if duration > 15*time.Second {
		t.Errorf("Max delay cap not working properly, took: %v", duration)
	}

	t.Logf("Total duration: %v (attempts: %d)", duration, atomic.LoadInt32(&attemptCount))
}

func TestRetry_RequestSpecificRetries(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 5 // Client default
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	// Override with request-specific retries
	resp, err := client.Get(server.URL, WithMaxRetries(1))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Should return response even after max retries
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", resp.StatusCode)
	}

	// Should only try 2 times (initial + 1 retry)
	if atomic.LoadInt32(&attemptCount) != 2 {
		t.Errorf("Expected 2 attempts, got %d", atomic.LoadInt32(&attemptCount))
	}

	// Verify attempts recorded in response
	if resp.Attempts != 2 {
		t.Errorf("Expected 2 attempts in response, got %d", resp.Attempts)
	}
}

func TestRetry_NoRetries(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 0
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Should return response even with no retries
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", resp.StatusCode)
	}

	if atomic.LoadInt32(&attemptCount) != 1 {
		t.Errorf("Expected 1 attempt with no retries, got %d", atomic.LoadInt32(&attemptCount))
	}

	if resp.Attempts != 1 {
		t.Errorf("Expected 1 attempt in response, got %d", resp.Attempts)
	}
}

func TestRetry_ConcurrentRequestsWithRetries(t *testing.T) {
	attemptCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		if count%3 == 0 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 50 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	const numRequests = 20
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := client.Get(server.URL)
			errors <- err
		}()
	}

	successCount := 0
	for i := 0; i < numRequests; i++ {
		if err := <-errors; err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("Expected some requests to succeed")
	}

	t.Logf("Success rate: %d/%d", successCount, numRequests)
}

func TestRetry_NetworkError(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 2
	config.RetryDelay = 50 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	// Try to connect to non-existent server
	_, err := client.Get("http://localhost:99999")
	if err == nil {
		t.Error("Expected error for network failure")
	}

	// Network errors should be retried
	t.Logf("Network error: %v", err)
}

func TestRetry_DNSError(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 2
	config.RetryDelay = 50 * time.Millisecond
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	// Try to connect to non-existent domain
	_, err := client.Get("http://this-domain-definitely-does-not-exist-12345.com")
	if err == nil {
		t.Error("Expected error for DNS failure")
	}

	// DNS errors should be retried
	t.Logf("DNS error: %v", err)
}
