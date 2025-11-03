package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// RETRY MECHANISM COMPREHENSIVE TESTS
// ============================================================================

func TestRetryEngine_BasicRetry(t *testing.T) {
	config := &Config{
		MaxRetries:    3,
		RetryDelay:    100 * time.Millisecond,
		MaxRetryDelay: 1 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        false, // Disable jitter for testing
	}

	engine := NewRetryEngine(config)

	tests := []struct {
		name          string
		statusCode    int
		shouldRetry   bool
		maxAttempts   int
		expectedDelay time.Duration
	}{
		{
			name:          "500 Internal Server Error - should retry",
			statusCode:    500,
			shouldRetry:   true,
			maxAttempts:   3,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name:          "502 Bad Gateway - should retry",
			statusCode:    502,
			shouldRetry:   true,
			maxAttempts:   3,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name:          "503 Service Unavailable - should retry",
			statusCode:    503,
			shouldRetry:   true,
			maxAttempts:   3,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name:          "504 Gateway Timeout - should retry",
			statusCode:    504,
			shouldRetry:   true,
			maxAttempts:   3,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name:          "429 Too Many Requests - should retry",
			statusCode:    429,
			shouldRetry:   true,
			maxAttempts:   3,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name:        "400 Bad Request - should not retry",
			statusCode:  400,
			shouldRetry: false,
		},
		{
			name:        "401 Unauthorized - should not retry",
			statusCode:  401,
			shouldRetry: false,
		},
		{
			name:        "404 Not Found - should not retry",
			statusCode:  404,
			shouldRetry: false,
		},
		{
			name:        "200 OK - should not retry",
			statusCode:  200,
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				StatusCode: tt.statusCode,
			}

			shouldRetry := engine.ShouldRetry(resp, nil, 0)
			if shouldRetry != tt.shouldRetry {
				t.Errorf("Expected shouldRetry=%v, got %v", tt.shouldRetry, shouldRetry)
			}

			if tt.shouldRetry {
				delay := engine.GetDelay(0)
				if delay != tt.expectedDelay {
					t.Errorf("Expected delay %v, got %v", tt.expectedDelay, delay)
				}
			}
		})
	}
}

func TestRetryEngine_ExponentialBackoff(t *testing.T) {
	config := &Config{
		MaxRetries:    5,
		RetryDelay:    100 * time.Millisecond,
		MaxRetryDelay: 2 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        false,
	}

	engine := NewRetryEngine(config)

	expectedDelays := []time.Duration{
		100 * time.Millisecond,  // attempt 0
		200 * time.Millisecond,  // attempt 1
		400 * time.Millisecond,  // attempt 2
		800 * time.Millisecond,  // attempt 3
		1600 * time.Millisecond, // attempt 4 (capped at MaxRetryDelay)
	}

	for attempt, expectedDelay := range expectedDelays {
		actualDelay := engine.GetDelay(attempt)

		// For cases exceeding MaxRetryDelay, should be capped
		if expectedDelay > config.MaxRetryDelay {
			expectedDelay = config.MaxRetryDelay
		}

		if actualDelay != expectedDelay {
			t.Errorf("Attempt %d: expected delay %v, got %v", attempt, expectedDelay, actualDelay)
		}
	}
}

func TestRetryEngine_WithJitter(t *testing.T) {
	config := &Config{
		MaxRetries:    3,
		RetryDelay:    100 * time.Millisecond,
		MaxRetryDelay: 1 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,
	}

	engine := NewRetryEngine(config)

	baseDelay := 100 * time.Millisecond

	// Test multiple times to ensure jitter takes effect
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = engine.GetDelay(0)
	}

	// Check if there are variations (jitter should produce different delays)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Expected jitter to produce different delays, but all delays were the same")
	}

	// Check if delays are within reasonable range (90%-110% of base delay)
	minExpected := time.Duration(float64(baseDelay) * 0.9)
	maxExpected := time.Duration(float64(baseDelay) * 1.1)

	for i, delay := range delays {
		if delay < minExpected || delay > maxExpected {
			t.Errorf("Delay %d (%v) is outside expected range [%v, %v]", i, delay, minExpected, maxExpected)
		}
	}
}

func TestRetryEngine_NetworkErrors(t *testing.T) {
	config := &Config{
		MaxRetries:    3,
		RetryDelay:    50 * time.Millisecond,
		MaxRetryDelay: 1 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        false,
	}

	engine := NewRetryEngine(config)

	networkErrors := []error{
		fmt.Errorf("dial tcp: connection refused"),
		fmt.Errorf("dial tcp: no such host"),
		fmt.Errorf("dial tcp: timeout"),
		fmt.Errorf("read tcp: connection reset by peer"),
		fmt.Errorf("write tcp: broken pipe"),
	}

	for _, err := range networkErrors {
		t.Run(fmt.Sprintf("Error: %v", err), func(t *testing.T) {
			shouldRetry := engine.ShouldRetry(nil, err, 0)
			if !shouldRetry {
				t.Errorf("Expected network error to be retryable: %v", err)
			}
		})
	}

	// Test non-network errors
	nonNetworkErrors := []error{
		fmt.Errorf("invalid JSON"),
		fmt.Errorf("permission denied"),
		context.Canceled,
		context.DeadlineExceeded,
	}

	for _, err := range nonNetworkErrors {
		t.Run(fmt.Sprintf("Non-network error: %v", err), func(t *testing.T) {
			shouldRetry := engine.ShouldRetry(nil, err, 0)
			if shouldRetry {
				t.Errorf("Expected non-network error to not be retryable: %v", err)
			}
		})
	}
}

func TestRetryEngine_MaxRetriesLimit(t *testing.T) {
	config := &Config{
		MaxRetries:    2,
		RetryDelay:    10 * time.Millisecond,
		MaxRetryDelay: 1 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        false,
	}

	engine := NewRetryEngine(config)

	resp := &Response{StatusCode: 500}

	// Test within maximum retry attempts
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		shouldRetry := engine.ShouldRetry(resp, nil, attempt)
		if !shouldRetry {
			t.Errorf("Attempt %d: expected retry, got no retry", attempt)
		}
	}

	// Test exceeding maximum retry attempts
	shouldRetry := engine.ShouldRetry(resp, nil, config.MaxRetries)
	if shouldRetry {
		t.Errorf("Attempt %d: expected no retry (exceeded max), got retry", config.MaxRetries)
	}
}

func TestRetryEngine_IntegrationWithClient(t *testing.T) {
	attemptCount := int32(0)
	successAfterAttempts := int32(3) // Success on 3rd attempt

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attemptCount, 1)

		if attempt < successAfterAttempts {
			// Return server error for first few attempts
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Server Error"))
		} else {
			// Finally succeed
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		}
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      5,
		RetryDelay:      50 * time.Millisecond,
		MaxRetryDelay:   1 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          false,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := client.Request(ctx, "GET", server.URL)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Body != "Success" {
		t.Errorf("Expected body 'Success', got '%s'", resp.Body)
	}

	// Check retry count
	if resp.Attempts != int(successAfterAttempts) {
		t.Errorf("Expected %d attempts, got %d", successAfterAttempts, resp.Attempts)
	}

	// Check number of requests received by server
	finalAttemptCount := atomic.LoadInt32(&attemptCount)
	if finalAttemptCount != successAfterAttempts {
		t.Errorf("Expected server to receive %d requests, got %d", successAfterAttempts, finalAttemptCount)
	}
}

func TestRetryEngine_ContextCancellation(t *testing.T) {
	attemptCount := int32(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		// Always return error to trigger retry
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      10, // Set higher retry count
		RetryDelay:      200 * time.Millisecond,
		MaxRetryDelay:   2 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          false,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	resp, err := client.Request(ctx, "GET", server.URL)
	duration := time.Since(start)

	// Should fail due to context cancellation
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
		if resp != nil {
			t.Logf("Unexpected success response: %d", resp.StatusCode)
		}
	}

	// Check if cancelled within reasonable time
	if duration > 1*time.Second {
		t.Errorf("Request took too long to cancel: %v", duration)
	}

	// Check attempt count (should be less than max retries)
	attempts := atomic.LoadInt32(&attemptCount)
	if attempts >= 10 {
		t.Errorf("Too many attempts before cancellation: %d", attempts)
	}

	t.Logf("Request cancelled after %d attempts in %v", attempts, duration)
}

func TestRetryEngine_RetryAfterHeader(t *testing.T) {
	attemptCount := int32(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attemptCount, 1)

		if attempt == 1 {
			// First request returns 429 and sets Retry-After header
			w.Header().Set("Retry-After", "1") // Retry after 1 second
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate Limited"))
		} else {
			// Subsequent requests succeed
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		}
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      3,
		RetryDelay:      100 * time.Millisecond,
		MaxRetryDelay:   5 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          false,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := client.Request(ctx, "GET", server.URL)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Body != "Success" {
		t.Errorf("Expected body 'Success', got '%s'", resp.Body)
	}

	// Check if retry was performed
	if resp.Attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", resp.Attempts)
	}

	// Check total time (should have waited at least the time specified by Retry-After)
	if duration < 1*time.Second {
		t.Errorf("Expected to wait at least 1 second due to Retry-After, but took %v", duration)
	}

	t.Logf("Request completed in %v with %d attempts", duration, resp.Attempts)
}

func TestRetryEngine_CustomRetryConditions(t *testing.T) {
	config := &Config{
		MaxRetries:    3,
		RetryDelay:    50 * time.Millisecond,
		MaxRetryDelay: 1 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        false,
	}

	engine := NewRetryEngine(config)

	tests := []struct {
		name        string
		statusCode  int
		error       error
		attempt     int
		shouldRetry bool
	}{
		{
			name:        "First attempt with 500 error",
			statusCode:  500,
			error:       nil,
			attempt:     0,
			shouldRetry: true,
		},
		{
			name:        "Third attempt with 500 error",
			statusCode:  500,
			error:       nil,
			attempt:     2,
			shouldRetry: true,
		},
		{
			name:        "Fourth attempt with 500 error (exceeds max)",
			statusCode:  500,
			error:       nil,
			attempt:     3,
			shouldRetry: false,
		},
		{
			name:        "Network error on first attempt",
			statusCode:  0,
			error:       fmt.Errorf("dial tcp: connection refused"),
			attempt:     0,
			shouldRetry: true,
		},
		{
			name:        "Context cancelled",
			statusCode:  0,
			error:       context.Canceled,
			attempt:     0,
			shouldRetry: false,
		},
		{
			name:        "Success status",
			statusCode:  200,
			error:       nil,
			attempt:     0,
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *Response
			if tt.statusCode > 0 {
				resp = &Response{StatusCode: tt.statusCode}
			}

			shouldRetry := engine.ShouldRetry(resp, tt.error, tt.attempt)
			if shouldRetry != tt.shouldRetry {
				t.Errorf("Expected shouldRetry=%v, got %v", tt.shouldRetry, shouldRetry)
			}
		})
	}
}
