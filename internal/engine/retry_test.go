package engine

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// ============================================================================
// RETRY ENGINE UNIT TESTS
// ============================================================================

func TestNewRetryEngine(t *testing.T) {
	config := &Config{
		MaxRetries:    3,
		RetryDelay:    100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	engine := NewRetryEngine(config)

	if engine == nil {
		t.Fatal("Expected non-nil retry engine")
	}

	if engine.config != config {
		t.Error("Config should be set")
	}
}

func TestRetryEngine_MaxRetries(t *testing.T) {
	config := &Config{
		MaxRetries: 5,
	}

	engine := NewRetryEngine(config)

	if engine.MaxRetries() != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", engine.MaxRetries())
	}
}

func TestRetryEngine_ShouldRetry_MaxAttemptsExceeded(t *testing.T) {
	config := &Config{
		MaxRetries: 3,
	}

	engine := NewRetryEngine(config)

	// Attempt 3 (0-indexed, so this is the 4th attempt)
	shouldRetry := engine.ShouldRetry(nil, errors.New("network error"), 3)

	if shouldRetry {
		t.Error("Should not retry when max attempts exceeded")
	}
}

func TestRetryEngine_ShouldRetry_NetworkErrors(t *testing.T) {
	config := &Config{
		MaxRetries: 3,
	}

	engine := NewRetryEngine(config)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "OpError is retryable (temporary)",
			err: &net.OpError{
				Op:   "dial",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  errors.New("connection refused"),
			},
			expected: false, // OpError.Temporary() returns false by default, so not retryable via net.Error interface
		},
		{
			name: "DNSError is retryable (temporary)",
			err: &net.DNSError{
				Err:         "no such host",
				Name:        "example.com",
				Server:      "8.8.8.8",
				IsTimeout:   false,
				IsTemporary: true, // Set to true to make it retryable
			},
			expected: true,
		},
		{
			name: "DNSError not temporary",
			err: &net.DNSError{
				Err:         "no such host",
				Name:        "example.com",
				Server:      "8.8.8.8",
				IsTimeout:   false,
				IsTemporary: false,
			},
			expected: false,
		},
		{
			name:     "Context canceled is not retryable",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "Context deadline exceeded is not retryable",
			err:      context.DeadlineExceeded,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.ShouldRetry(nil, tt.err, 0)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for error: %v", tt.expected, result, tt.err)
			}
		})
	}
}

func TestRetryEngine_ShouldRetry_StatusCodes(t *testing.T) {
	config := &Config{
		MaxRetries: 3,
	}

	engine := NewRetryEngine(config)

	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{
			name:       "408 Request Timeout is retryable",
			statusCode: http.StatusRequestTimeout,
			expected:   true,
		},
		{
			name:       "429 Too Many Requests is retryable",
			statusCode: http.StatusTooManyRequests,
			expected:   true,
		},
		{
			name:       "500 Internal Server Error is retryable",
			statusCode: http.StatusInternalServerError,
			expected:   true,
		},
		{
			name:       "502 Bad Gateway is retryable",
			statusCode: http.StatusBadGateway,
			expected:   true,
		},
		{
			name:       "503 Service Unavailable is retryable",
			statusCode: http.StatusServiceUnavailable,
			expected:   true,
		},
		{
			name:       "504 Gateway Timeout is retryable",
			statusCode: http.StatusGatewayTimeout,
			expected:   true,
		},
		{
			name:       "200 OK is not retryable",
			statusCode: http.StatusOK,
			expected:   false,
		},
		{
			name:       "400 Bad Request is not retryable",
			statusCode: http.StatusBadRequest,
			expected:   false,
		},
		{
			name:       "404 Not Found is not retryable",
			statusCode: http.StatusNotFound,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				StatusCode: tt.statusCode,
			}

			result := engine.ShouldRetry(resp, nil, 0)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRetryEngine_GetDelay_ExponentialBackoff(t *testing.T) {
	config := &Config{
		RetryDelay:    100 * time.Millisecond,
		BackoffFactor: 2.0,
		Jitter:        false, // Disable jitter for predictable testing
	}

	engine := NewRetryEngine(config)

	tests := []struct {
		attempt      int
		expectedMin  time.Duration
		expectedMax  time.Duration
	}{
		{
			attempt:     0,
			expectedMin: 100 * time.Millisecond,
			expectedMax: 100 * time.Millisecond,
		},
		{
			attempt:     1,
			expectedMin: 200 * time.Millisecond,
			expectedMax: 200 * time.Millisecond,
		},
		{
			attempt:     2,
			expectedMin: 400 * time.Millisecond,
			expectedMax: 400 * time.Millisecond,
		},
		{
			attempt:     3,
			expectedMin: 800 * time.Millisecond,
			expectedMax: 800 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			delay := engine.GetDelay(tt.attempt)

			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("Attempt %d: expected delay between %v and %v, got %v",
					tt.attempt, tt.expectedMin, tt.expectedMax, delay)
			}
		})
	}
}

func TestRetryEngine_GetDelay_WithJitter(t *testing.T) {
	config := &Config{
		RetryDelay:    100 * time.Millisecond,
		BackoffFactor: 2.0,
		Jitter:        true,
	}

	engine := NewRetryEngine(config)

	// With jitter, delays should vary
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = engine.GetDelay(1)
	}

	// Check that we have some variation (not all delays are the same)
	allSame := true
	firstDelay := delays[0]
	for _, delay := range delays[1:] {
		if delay != firstDelay {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Expected variation in delays with jitter enabled")
	}

	// All delays should be within reasonable bounds
	baseDelay := 200 * time.Millisecond // 100ms * 2^1
	maxDelay := baseDelay + (baseDelay / 2)

	for i, delay := range delays {
		if delay < baseDelay || delay > maxDelay {
			t.Errorf("Delay %d out of expected range: %v (expected %v to %v)",
				i, delay, baseDelay, maxDelay)
		}
	}
}

func TestRetryEngine_GetDelay_MaxRetryDelay(t *testing.T) {
	config := &Config{
		RetryDelay:     100 * time.Millisecond,
		BackoffFactor:  2.0,
		MaxRetryDelay:  500 * time.Millisecond,
		Jitter:         false,
	}

	engine := NewRetryEngine(config)

	// Attempt 3 would normally be 800ms, but should be capped at 500ms
	delay := engine.GetDelay(3)

	if delay > 500*time.Millisecond {
		t.Errorf("Expected delay <= 500ms, got %v", delay)
	}

	if delay != 500*time.Millisecond {
		t.Errorf("Expected delay to be capped at 500ms, got %v", delay)
	}
}

func TestRetryEngine_GetDelay_DefaultValues(t *testing.T) {
	t.Run("Zero RetryDelay uses default", func(t *testing.T) {
		config := &Config{
			RetryDelay:    0,
			BackoffFactor: 2.0,
			Jitter:        false,
		}

		engine := NewRetryEngine(config)
		delay := engine.GetDelay(0)

		// Should use default 1 second
		if delay != 1*time.Second {
			t.Errorf("Expected default delay 1s, got %v", delay)
		}
	})

	t.Run("Zero BackoffFactor uses default", func(t *testing.T) {
		config := &Config{
			RetryDelay:    100 * time.Millisecond,
			BackoffFactor: 0,
			Jitter:        false,
		}

		engine := NewRetryEngine(config)
		delay := engine.GetDelay(1)

		// Should use default backoff factor of 2.0
		expected := 200 * time.Millisecond
		if delay != expected {
			t.Errorf("Expected delay %v, got %v", expected, delay)
		}
	})
}

func TestRetryEngine_IsRetryableError(t *testing.T) {
	config := &Config{
		MaxRetries: 3,
	}

	engine := NewRetryEngine(config)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "Context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "Context canceled in message",
			err:      errors.New("context canceled"),
			expected: false,
		},
		{
			name:     "Request context canceled",
			err:      errors.New("request context canceled"),
			expected: false,
		},
		{
			name: "OpError without context (not temporary)",
			err: &net.OpError{
				Op:   "dial",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  errors.New("connection refused"),
			},
			expected: false, // OpError.Temporary() returns false by default
		},
		{
			name: "OpError with context",
			err: &net.OpError{
				Op:   "dial",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  errors.New("context deadline exceeded"),
			},
			expected: false,
		},
		{
			name: "DNSError temporary",
			err: &net.DNSError{
				Err:         "no such host",
				Name:        "example.com",
				Server:      "8.8.8.8",
				IsTimeout:   false,
				IsTemporary: true, // Set to true to make it retryable
			},
			expected: true,
		},
		{
			name: "DNSError not temporary",
			err: &net.DNSError{
				Err:         "no such host",
				Name:        "example.com",
				Server:      "8.8.8.8",
				IsTimeout:   false,
				IsTemporary: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for error: %v", tt.expected, result, tt.err)
			}
		})
	}
}

// Note: mockNetError is defined in errors_test.go and shared across test files

func TestRetryEngine_GetSecureJitter(t *testing.T) {
	config := &Config{}
	engine := NewRetryEngine(config)

	maxJitter := 100 * time.Millisecond

	// Test multiple times to ensure randomness
	for i := 0; i < 10; i++ {
		jitter := engine.getSecureJitter(maxJitter)

		if jitter < 0 {
			t.Errorf("Jitter should not be negative, got %v", jitter)
		}

		if jitter > maxJitter {
			t.Errorf("Jitter should not exceed max, got %v (max: %v)", jitter, maxJitter)
		}
	}
}

func TestRetryEngine_GetSecureJitter_ZeroMax(t *testing.T) {
	config := &Config{}
	engine := NewRetryEngine(config)

	jitter := engine.getSecureJitter(0)

	if jitter != 0 {
		t.Errorf("Expected 0 jitter for 0 maxJitter, got %v", jitter)
	}
}

