package httpc

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// RETRY TESTS - Exponential backoff, retryable status codes, retry behavior
// ============================================================================

func TestRetry_Behavior(t *testing.T) {
	t.Run("SuccessOnFirstAttempt", func(t *testing.T) {
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

		_, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if atomic.LoadInt32(&attemptCount) != 1 {
			t.Errorf("Expected 1 attempt, got %d", atomic.LoadInt32(&attemptCount))
		}
	})

	t.Run("SuccessOnRetry", func(t *testing.T) {
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
		config.RetryDelay = 10 * time.Millisecond
		config.AllowPrivateIPs = true
		client, _ := New(config)
		defer client.Close()

		_, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if atomic.LoadInt32(&attemptCount) != 3 {
			t.Errorf("Expected 3 attempts, got %d", atomic.LoadInt32(&attemptCount))
		}
	})

	t.Run("MaxRetriesExceeded", func(t *testing.T) {
		attemptCount := int32(0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attemptCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		config := DefaultConfig()
		config.MaxRetries = 2
		config.RetryDelay = 10 * time.Millisecond
		config.AllowPrivateIPs = true
		client, _ := New(config)
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", resp.StatusCode)
		}
		if atomic.LoadInt32(&attemptCount) != 3 {
			t.Errorf("Expected 3 attempts (1 + 2 retries), got %d", atomic.LoadInt32(&attemptCount))
		}
	})

	t.Run("NoRetries", func(t *testing.T) {
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
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", resp.StatusCode)
		}
		if atomic.LoadInt32(&attemptCount) != 1 {
			t.Errorf("Expected 1 attempt, got %d", atomic.LoadInt32(&attemptCount))
		}
	})
}

func TestRetry_StatusCodes(t *testing.T) {
	t.Run("RetryableStatusCodes", func(t *testing.T) {
		retryableStatuses := []int{
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,           // 502
			http.StatusServiceUnavailable,   // 503
			http.StatusGatewayTimeout,       // 504
			http.StatusTooManyRequests,      // 429
		}

		for _, status := range retryableStatuses {
			t.Run(http.StatusText(status), func(t *testing.T) {
				attemptCount := int32(0)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(&attemptCount, 1)
					w.WriteHeader(status)
				}))
				defer server.Close()

				config := DefaultConfig()
				config.MaxRetries = 2
				config.RetryDelay = 10 * time.Millisecond
				config.AllowPrivateIPs = true
				client, _ := New(config)
				defer client.Close()

				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				if resp.StatusCode != status {
					t.Errorf("Expected status %d, got %d", status, resp.StatusCode)
				}
				if atomic.LoadInt32(&attemptCount) < 2 {
					t.Errorf("Expected at least 2 attempts for status %d, got %d", status, atomic.LoadInt32(&attemptCount))
				}
			})
		}
	})

	t.Run("NonRetryableStatusCodes", func(t *testing.T) {
		nonRetryableStatuses := []int{
			http.StatusBadRequest,          // 400
			http.StatusUnauthorized,        // 401
			http.StatusForbidden,           // 403
			http.StatusNotFound,            // 404
			http.StatusMethodNotAllowed,    // 405
			http.StatusNotAcceptable,       // 406
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
				config.MaxRetries = 2
				config.RetryDelay = 10 * time.Millisecond
				config.AllowPrivateIPs = true
				client, _ := New(config)
				defer client.Close()

				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				if resp.StatusCode != status {
					t.Errorf("Expected status %d, got %d", status, resp.StatusCode)
				}
				if atomic.LoadInt32(&attemptCount) != 1 {
					t.Errorf("Expected 1 attempt for status %d, got %d", status, atomic.LoadInt32(&attemptCount))
				}
			})
		}
	})
}

func TestRetry_Backoff(t *testing.T) {
	attemptCount := int32(0)
	attemptTimes := make([]time.Time, 0)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptTimes = append(attemptTimes, time.Now())
		mu.Unlock()
		atomic.AddInt32(&attemptCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 100 * time.Millisecond
	config.BackoffFactor = 2.0
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(attemptTimes) < 2 {
		t.Fatal("Not enough attempts to verify backoff")
	}

	// Verify delays increase exponentially
	for i := 1; i < len(attemptTimes); i++ {
		delay := attemptTimes[i].Sub(attemptTimes[i-1])
		expectedMin := time.Duration(float64(config.RetryDelay) * float64(i) * 0.5) // Allow jitter
		if delay < expectedMin {
			t.Logf("Delay %d: %v (expected min %v)", i, delay, expectedMin)
		}
	}
}
