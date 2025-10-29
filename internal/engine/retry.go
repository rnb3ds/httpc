package engine

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryEngine handles retry logic with exponential backoff
type RetryEngine struct {
	config *Config
}

// NewRetryEngine creates a new retry engine
func NewRetryEngine(config *Config) *RetryEngine {
	return &RetryEngine{
		config: config,
	}
}

// ShouldRetry determines if a request should be retried
func (r *RetryEngine) ShouldRetry(resp *Response, err error, attempt int) bool {
	// Don't retry if we've exceeded max attempts
	if attempt >= r.config.MaxRetries {
		return false
	}

	// Retry on network errors
	if err != nil {
		return r.isRetryableError(err)
	}

	// Retry on specific HTTP status codes
	if resp != nil {
		return r.isRetryableStatus(resp.StatusCode)
	}

	return false
}

// GetDelay calculates the delay for the next retry attempt
func (r *RetryEngine) GetDelay(attempt int) time.Duration {
	return r.GetDelayWithResponse(attempt, nil)
}

// GetDelayWithResponse calculates the delay for the next retry attempt, considering Retry-After header
func (r *RetryEngine) GetDelayWithResponse(attempt int, resp *Response) time.Duration {
	// Check for Retry-After header first
	if resp != nil && resp.Headers != nil {
		if retryAfterValues, exists := resp.Headers["Retry-After"]; exists && len(retryAfterValues) > 0 {
			retryAfter := retryAfterValues[0]

			// Try to parse as seconds (integer)
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}

			// Try to parse as HTTP date (RFC 1123)
			if retryTime, err := time.Parse(time.RFC1123, retryAfter); err == nil {
				delay := time.Until(retryTime)
				if delay > 0 {
					return delay
				}
			}
		}
	}
	// Base delay
	delay := r.config.RetryDelay
	if delay <= 0 {
		delay = 1 * time.Second
	}

	// Apply exponential backoff
	backoffFactor := r.config.BackoffFactor
	if backoffFactor <= 0 {
		backoffFactor = 2.0
	}

	exponentialDelay := time.Duration(float64(delay) * math.Pow(backoffFactor, float64(attempt)))

	// Apply maximum delay limit
	if r.config.MaxRetryDelay > 0 && exponentialDelay > r.config.MaxRetryDelay {
		exponentialDelay = r.config.MaxRetryDelay
	}

	// Add jitter if enabled (±10% variation)
	if r.config.Jitter {
		jitterRange := exponentialDelay / 10                       // 10% of the delay
		jitter := r.getSecureJitter(jitterRange * 2)               // 0 to 20%
		exponentialDelay = exponentialDelay - jitterRange + jitter // ±10%
	}

	return exponentialDelay
}

// MaxRetries returns the maximum number of retry attempts
func (r *RetryEngine) MaxRetries() int {
	return r.config.MaxRetries
}

// isRetryableError checks if an error is retryable
func (r *RetryEngine) isRetryableError(err error) bool {
	// Context cancellation should never be retried
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Context deadline exceeded should not be retried (indicates overall timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for context cancellation in error message
	errMsg := err.Error()
	if strings.Contains(errMsg, "context canceled") ||
		strings.Contains(errMsg, "request context canceled") {
		return false
	}

	if dnsErr, ok := err.(*net.DNSError); ok {
		return dnsErr.IsTimeout || dnsErr.Temporary()
	}

	if opErr, ok := err.(*net.OpError); ok {
		if strings.Contains(errMsg, "context") {
			return false
		}
		return opErr.Temporary()
	}

	if netErr, ok := err.(net.Error); ok {
		if strings.Contains(errMsg, "context") {
			return false
		}
		return netErr.Timeout()
	}

	// Check for common network error patterns in error messages
	networkPatterns := []string{
		"connection refused",
		"no such host",
		"timeout",
		"connection reset by peer",
		"broken pipe",
		"network unreachable",
		"host unreachable",
	}

	for _, pattern := range networkPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// getSecureJitter generates cryptographically secure random jitter
func (r *RetryEngine) getSecureJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	// Use crypto/rand for secure random number generation
	maxInt := big.NewInt(int64(maxJitter))
	n, err := rand.Int(rand.Reader, maxInt)
	if err != nil {
		// Fallback to a simple calculation if crypto/rand fails
		return maxJitter / 4
	}

	return time.Duration(n.Int64())
}

// isRetryableStatus checks if an HTTP status code is retryable
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
