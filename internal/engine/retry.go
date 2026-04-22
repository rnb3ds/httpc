package engine

import (
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/cybergodev/httpc/internal/types"
)

type RetryEngine struct {
	config *Config
}

// Compile-time interface check
var _ types.RetryPolicy = (*RetryEngine)(nil)

func NewRetryEngine(config *Config) *RetryEngine {
	return &RetryEngine{
		config: config,
	}
}

// ShouldRetry implements types.RetryPolicy interface.
func (r *RetryEngine) ShouldRetry(resp types.ResponseReader, err error, attempt int) bool {
	if err != nil {
		return r.isRetryableError(err)
	}

	if resp != nil {
		return r.isRetryableStatus(resp.StatusCode())
	}

	return false
}

func (r *RetryEngine) GetDelay(attempt int) time.Duration {
	return r.GetDelayWithResponse(attempt, nil)
}

// GetDelayWithResponse returns the delay for the given attempt, considering response headers.
// It first checks for Retry-After header, then falls back to exponential backoff.
func (r *RetryEngine) GetDelayWithResponse(attempt int, resp *Response) time.Duration {
	// Check Retry-After header first
	if resp != nil {
		if retryAfterDelay := parseRetryAfterHeader(resp.Headers()); retryAfterDelay > 0 {
			return retryAfterDelay
		}
	}

	return r.calculateExponentialDelay(attempt)
}

// parseRetryAfterHeader parses the Retry-After header and returns the delay duration.
// Returns 0 if the header is not present or cannot be parsed.
// Supports both delta-seconds and HTTP-date formats per RFC 7231.
func parseRetryAfterHeader(headers http.Header) time.Duration {
	if headers == nil {
		return 0
	}

	retryAfterValues := headers["Retry-After"]
	if len(retryAfterValues) == 0 {
		return 0
	}

	retryAfter := retryAfterValues[0]

	// Try parsing as seconds (delta-seconds format)
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date (RFC1123 format)
	if retryTime, err := time.Parse(time.RFC1123, retryAfter); err == nil {
		if delay := time.Until(retryTime); delay > 0 {
			return delay
		}
	}

	return 0
}

// calculateExponentialDelay calculates the exponential backoff delay with optional jitter.
func (r *RetryEngine) calculateExponentialDelay(attempt int) time.Duration {
	delay := r.config.RetryDelay
	if delay <= 0 {
		delay = time.Second
	}

	backoffFactor := r.config.BackoffFactor
	if backoffFactor <= 0 {
		backoffFactor = 2.0
	}

	exponentialDelay := time.Duration(float64(delay) * math.Pow(backoffFactor, float64(attempt)))

	// Apply max delay cap
	if r.config.MaxRetryDelay > 0 && exponentialDelay > r.config.MaxRetryDelay {
		exponentialDelay = r.config.MaxRetryDelay
	}

	// Apply jitter to prevent thundering herd
	if r.config.Jitter {
		exponentialDelay = r.applyJitter(exponentialDelay)
	}

	return exponentialDelay
}

// applyJitter adds randomization to the delay to prevent thundering herd problems.
func (r *RetryEngine) applyJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}

	jitterRange := delay / 10
	jitter := r.getJitter(jitterRange * 2)
	return delay - jitterRange + jitter
}

func (r *RetryEngine) MaxRetries() int {
	return r.config.MaxRetries
}

// isRetryableError determines if an error is retryable by delegating to
// the centralized error classification in ClientError.IsRetryable().
// This ensures consistent retry behavior across the codebase.
func (r *RetryEngine) isRetryableError(err error) bool {
	clientErr := ClassifyError(err, "", "", 0)
	if clientErr == nil {
		return false
	}
	return clientErr.IsRetryable()
}

// getJitter generates pseudo-random jitter for retry delays.
// Uses math/rand/v2 for high-quality randomness without security concerns.
func (r *RetryEngine) getJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(maxJitter)))
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
