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

type RetryEngine struct {
	config *Config
}

func NewRetryEngine(config *Config) *RetryEngine {
	return &RetryEngine{
		config: config,
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
		delay = 1 * time.Second
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

	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "context canceled") || strings.Contains(errMsg, "context deadline exceeded") {
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

	retryablePatterns := []string{
		"connection refused", "connection reset", "broken pipe",
		"network unreachable", "host unreachable", "no route to host", "no such host",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	if strings.Contains(errMsg, "timeout") && !strings.Contains(errMsg, "context") {
		return true
	}

	return false
}

func (r *RetryEngine) getSecureJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	maxInt := big.NewInt(int64(maxJitter))
	n, err := rand.Int(rand.Reader, maxInt)
	if err != nil {
		return time.Duration(time.Now().UnixNano() % int64(maxJitter))
	}

	return time.Duration(n.Int64())
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
