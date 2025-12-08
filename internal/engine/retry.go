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

func (r *RetryEngine) isRetryableError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTimeout || dnsErr.IsTemporary
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil {
			if errors.Is(opErr.Err, context.Canceled) || errors.Is(opErr.Err, context.DeadlineExceeded) {
				return false
			}
		}
		return opErr.Timeout()
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	errMsgLower := strings.ToLower(err.Error())

	if strings.Contains(errMsgLower, "context canceled") || strings.Contains(errMsgLower, "context deadline exceeded") {
		return false
	}

	return strings.Contains(errMsgLower, "connection refused") ||
		strings.Contains(errMsgLower, "connection reset") ||
		strings.Contains(errMsgLower, "broken pipe") ||
		strings.Contains(errMsgLower, "network unreachable") ||
		strings.Contains(errMsgLower, "host unreachable") ||
		strings.Contains(errMsgLower, "no route to host") ||
		strings.Contains(errMsgLower, "no such host") ||
		(strings.Contains(errMsgLower, "timeout") && !strings.Contains(errMsgLower, "context"))
}

func (r *RetryEngine) getSecureJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	// Use atomic counter with time mixing for lock-free jitter
	// This provides variation without mutex contention
	count := atomic.AddInt64(&r.counter, 1)
	nanos := time.Now().UnixNano()

	// Mix counter and time for better distribution
	mixed := (count ^ nanos) * 1103515245
	mixed = (mixed ^ (mixed >> 30)) * 1664525

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
