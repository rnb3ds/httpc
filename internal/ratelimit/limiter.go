package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter provides simple rate limiting functionality
type Limiter struct {
	rate     int           // requests per second
	interval time.Duration // time window
	tokens   int           // current tokens
	lastTime time.Time     // last refill time
	mu       sync.Mutex
}

// NewLimiter creates a new rate limiter
func NewLimiter(requestsPerSecond int) *Limiter {
	return &Limiter{
		rate:     requestsPerSecond,
		interval: time.Second,
		tokens:   requestsPerSecond,
		lastTime: time.Now(),
	}
}

// Allow checks if a request is allowed under the rate limit
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastTime)

	// Refill tokens based on elapsed time
	if elapsed >= l.interval {
		l.tokens = l.rate
		l.lastTime = now
	}

	// Check if we have tokens available
	if l.tokens > 0 {
		l.tokens--
		return true
	}

	return false
}

// Wait blocks until a request is allowed
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		if l.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Continue checking
		}
	}
}
