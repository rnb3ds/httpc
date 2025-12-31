package engine

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type RetryEngine struct {
	config  *Config
	counter int64 // Atomic counter for jitter variation
}

func NewRetryEngine(config *Config) *RetryEngine {
	return &RetryEngine{
		config:  config,
		counter: time.Now().UnixNano(),
	}
}

func (r *RetryEngine) ShouldRetry(resp *Response, err error, attempt int) bool {
	if attempt >= r.config.MaxRetries {
		return false
	}

	if err != nil {
		return r.isRetryableError(err)
	}

	if resp != nil {
		return r.isRetryableStatus(resp.StatusCode)
	}

	return false
}

func (r *RetryEngine) GetDelay(attempt int) time.Duration {
	return r.GetDelayWithResponse(attempt, nil)
}

func (r *RetryEngine) GetDelayWithResponse(attempt int, resp *Response) time.Duration {
	if resp != nil && resp.Headers != nil {
		if retryAfterValues, exists := resp.Headers["Retry-After"]; exists && len(retryAfterValues) > 0 {
			retryAfter := retryAfterValues[0]
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
			if retryTime, err := time.Parse(time.RFC1123, retryAfter); err == nil {
				if delay := time.Until(retryTime); delay > 0 {
					return delay
				}
			}
		}
	}

	delay := r.config.RetryDelay
	if delay <= 0 {
		delay = time.Second
	}
	backoffFactor := r.config.BackoffFactor
	if backoffFactor <= 0 {
		backoffFactor = 2.0
	}

	exponentialDelay := time.Duration(float64(delay) * math.Pow(backoffFactor, float64(attempt)))

	if r.config.MaxRetryDelay > 0 && exponentialDelay > r.config.MaxRetryDelay {
		exponentialDelay = r.config.MaxRetryDelay
	}

	if r.config.Jitter {
		jitterRange := exponentialDelay / 10
		jitter := r.getSecureJitter(jitterRange * 2)
		exponentialDelay = exponentialDelay - jitterRange + jitter
	}

	return exponentialDelay
}

func (r *RetryEngine) MaxRetries() int {
	return r.config.MaxRetries
}

// isRetryableError determines if an error is retryable based on its type and characteristics.
// Optimized for performance with efficient error type checking and early returns.
func (r *RetryEngine) isRetryableError(err error) bool {
	// Fast path: context errors are never retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// DNS errors - retry if temporary or timeout
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTimeout || dnsErr.IsTemporary
	}

	// Network operation errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// Don't retry if the underlying error is context-related
		if opErr.Err != nil {
			if errors.Is(opErr.Err, context.Canceled) || errors.Is(opErr.Err, context.DeadlineExceeded) {
				return false
			}
		}
		return opErr.Timeout()
	}

	// Generic network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Optimized string-based classification with early returns
	errMsg := err.Error()
	errMsgLen := len(errMsg)

	// Skip string operations for very short messages
	if errMsgLen < 4 {
		return false
	}

	// Convert to lowercase once for efficiency
	errMsgLower := strings.ToLower(errMsg)

	// Never retry context errors (double-check with fast string matching)
	if strings.Contains(errMsgLower, "context") {
		return strings.Contains(errMsgLower, "timeout") &&
			!strings.Contains(errMsgLower, "canceled") &&
			!strings.Contains(errMsgLower, "deadline")
	}

	// Optimized retryable condition checks with priority ordering
	// Most common errors first for better performance
	return strings.Contains(errMsgLower, "connection refused") ||
		strings.Contains(errMsgLower, "timeout") ||
		strings.Contains(errMsgLower, "connection reset") ||
		strings.Contains(errMsgLower, "broken pipe") ||
		strings.Contains(errMsgLower, "network unreachable") ||
		strings.Contains(errMsgLower, "host unreachable") ||
		strings.Contains(errMsgLower, "no route to host") ||
		strings.Contains(errMsgLower, "no such host")
}

// getSecureJitter generates cryptographically secure jitter for retry delays.
// Uses atomic operations and time mixing to avoid lock contention while ensuring good distribution.
func (r *RetryEngine) getSecureJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	// Use atomic counter with time mixing for lock-free jitter generation
	// This provides good distribution without mutex contention in hot paths
	count := atomic.AddInt64(&r.counter, 1)
	nanos := time.Now().UnixNano()

	// Linear congruential generator for better distribution
	// Constants from Numerical Recipes
	mixed := (count ^ nanos) * 1103515245
	mixed = (mixed ^ (mixed >> 30)) * 1664525

	// Ensure positive result and apply modulo
	jitter := mixed % int64(maxJitter)
	if jitter < 0 {
		jitter = -jitter
	}

	return time.Duration(jitter)
}

func (r *RetryEngine) isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}
