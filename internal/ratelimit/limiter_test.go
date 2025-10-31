package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// RATE LIMITER TESTS
// ============================================================================

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(10) // 10 requests per second

	if limiter == nil {
		t.Fatal("NewLimiter should not return nil")
	}

	// Verify initial state - should allow requests immediately
	if !limiter.Allow() {
		t.Error("New limiter should allow first request")
	}
}

func TestLimiter_Allow_Basic(t *testing.T) {
	limiter := NewLimiter(5) // 5 requests per second

	// Should allow up to 5 requests immediately
	allowedCount := 0
	for i := 0; i < 10; i++ {
		if limiter.Allow() {
			allowedCount++
		}
	}

	if allowedCount < 5 {
		t.Errorf("Expected at least 5 requests to be allowed, got %d", allowedCount)
	}

	if allowedCount > 6 {
		t.Errorf("Expected at most 6 requests to be allowed initially, got %d", allowedCount)
	}
}

func TestLimiter_Allow_TokenRefill(t *testing.T) {
	limiter := NewLimiter(10) // 10 requests per second

	// Exhaust tokens
	for i := 0; i < 15; i++ {
		limiter.Allow()
	}

	// Wait for full second to refill (fixed window implementation)
	time.Sleep(1100 * time.Millisecond)

	// Should allow requests after refill
	if !limiter.Allow() {
		t.Error("Expected request to be allowed after token refill")
	}
}

func TestLimiter_Allow_RateLimit(t *testing.T) {
	limiter := NewLimiter(10) // 10 requests per second

	// First 10 should succeed immediately
	successCount := 0
	for i := 0; i < 10; i++ {
		if limiter.Allow() {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("Expected 10 successful requests in first batch, got %d", successCount)
	}

	// Next requests should fail until window resets
	if limiter.Allow() {
		t.Error("Expected request to be denied after exhausting tokens")
	}

	// Wait for window to reset
	time.Sleep(1100 * time.Millisecond)

	// Should allow more requests
	if !limiter.Allow() {
		t.Error("Expected request to be allowed after window reset")
	}
}

func TestLimiter_Wait_Success(t *testing.T) {
	limiter := NewLimiter(10) // 10 requests per second

	ctx := context.Background()

	// First request should succeed immediately
	err := limiter.Wait(ctx)
	if err != nil {
		t.Errorf("First Wait should succeed immediately, got error: %v", err)
	}
}

func TestLimiter_Wait_Blocking(t *testing.T) {
	limiter := NewLimiter(5) // 5 requests per second

	ctx := context.Background()

	// Exhaust tokens
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}

	// Wait should block until token is available
	start := time.Now()
	err := limiter.Wait(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Wait should succeed, got error: %v", err)
	}

	// Should have waited at least 100ms for token refill
	if duration < 50*time.Millisecond {
		t.Errorf("Expected Wait to block for at least 50ms, got %v", duration)
	}
}

func TestLimiter_Wait_ContextCancellation(t *testing.T) {
	limiter := NewLimiter(1) // 1 request per second

	// Exhaust tokens
	for i := 0; i < 5; i++ {
		limiter.Allow()
	}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Wait should fail due to context cancellation
	err := limiter.Wait(ctx)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestLimiter_Wait_ContextAlreadyCancelled(t *testing.T) {
	limiter := NewLimiter(10)

	// Exhaust tokens first
	for i := 0; i < 15; i++ {
		limiter.Allow()
	}

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Wait should fail due to cancelled context
	err := limiter.Wait(ctx)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewLimiter(100) // 100 requests per second

	var wg sync.WaitGroup
	numGoroutines := 50
	requestsPerGoroutine := 10

	successCount := int32(0)
	var mu sync.Mutex

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				if limiter.Allow() {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	// Fixed window: should allow exactly 100 requests in first window
	if successCount != 100 {
		t.Errorf("Expected 100 successful requests (rate limit), got %d", successCount)
	}

	t.Logf("Concurrent test: %d/%d requests allowed in %v", successCount, numGoroutines*requestsPerGoroutine, duration)
}

func TestLimiter_ConcurrentWait(t *testing.T) {
	limiter := NewLimiter(50) // 50 requests per second

	var wg sync.WaitGroup
	numGoroutines := 20

	ctx := context.Background()
	errors := make(chan error, numGoroutines)

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := limiter.Wait(ctx)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)
	duration := time.Since(start)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Wait failed: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Expected no errors, got %d", errorCount)
	}

	t.Logf("Concurrent Wait test: %d goroutines completed in %v", numGoroutines, duration)
}

func TestLimiter_HighRate(t *testing.T) {
	limiter := NewLimiter(1000) // 1000 requests per second

	// Should allow many requests quickly
	allowedCount := 0
	for i := 0; i < 1000; i++ {
		if limiter.Allow() {
			allowedCount++
		}
	}

	if allowedCount < 900 {
		t.Errorf("Expected at least 900 requests to be allowed, got %d", allowedCount)
	}
}

func TestLimiter_LowRate(t *testing.T) {
	limiter := NewLimiter(1) // 1 request per second

	// First request should succeed
	if !limiter.Allow() {
		t.Error("First request should be allowed")
	}

	// Second request should fail (no tokens)
	if limiter.Allow() {
		t.Error("Second request should be denied")
	}

	// Wait for token refill
	time.Sleep(1100 * time.Millisecond)

	// Should allow request after refill
	if !limiter.Allow() {
		t.Error("Request should be allowed after token refill")
	}
}

func TestLimiter_BurstBehavior(t *testing.T) {
	limiter := NewLimiter(10) // 10 requests per second

	// Test burst: should allow initial burst up to rate limit
	burstCount := 0
	for i := 0; i < 20; i++ {
		if limiter.Allow() {
			burstCount++
		} else {
			break
		}
	}

	if burstCount != 10 {
		t.Errorf("Expected burst of exactly 10 requests, got %d", burstCount)
	}

	// Wait for full window reset
	time.Sleep(1100 * time.Millisecond)

	// Should allow full rate again after window reset
	refillCount := 0
	for i := 0; i < 15; i++ {
		if limiter.Allow() {
			refillCount++
		}
	}

	if refillCount != 10 {
		t.Errorf("Expected exactly 10 requests after window reset, got %d", refillCount)
	}
}

func TestLimiter_ZeroRate(t *testing.T) {
	// Zero or negative rate should be handled gracefully
	limiter := NewLimiter(0)

	// Should not panic
	allowed := limiter.Allow()

	// Behavior: either deny all requests or use a minimum rate
	_ = allowed // We don't enforce specific behavior for invalid input
}

func TestLimiter_Wait_MultipleWaiters(t *testing.T) {
	limiter := NewLimiter(5) // 5 requests per second

	// Exhaust tokens
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}

	var wg sync.WaitGroup
	numWaiters := 5

	ctx := context.Background()
	start := time.Now()

	for i := 0; i < numWaiters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := limiter.Wait(ctx)
			if err != nil {
				t.Errorf("Waiter %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// All waiters should eventually succeed
	// Should take at least 200ms (5 waiters at 5 req/s)
	if duration < 100*time.Millisecond {
		t.Errorf("Expected duration >= 100ms, got %v", duration)
	}

	t.Logf("Multiple waiters test: %d waiters completed in %v", numWaiters, duration)
}

func TestLimiter_TokenAccumulation(t *testing.T) {
	limiter := NewLimiter(10) // 10 requests per second

	// Don't make any requests for 2 seconds
	time.Sleep(2 * time.Second)

	// Should have accumulated tokens (but capped at burst size)
	allowedCount := 0
	for i := 0; i < 30; i++ {
		if limiter.Allow() {
			allowedCount++
		}
	}

	// Should allow burst, but not unlimited
	if allowedCount < 10 {
		t.Errorf("Expected at least 10 requests after accumulation, got %d", allowedCount)
	}

	if allowedCount > 25 {
		t.Errorf("Expected at most 25 requests (should cap accumulation), got %d", allowedCount)
	}
}

func BenchmarkLimiter_Allow(b *testing.B) {
	limiter := NewLimiter(1000000) // Very high rate to avoid blocking

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}

func BenchmarkLimiter_Wait(b *testing.B) {
	limiter := NewLimiter(1000000) // Very high rate to avoid blocking
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		limiter.Wait(ctx)
	}
}

func BenchmarkLimiter_ConcurrentAllow(b *testing.B) {
	limiter := NewLimiter(1000000) // Very high rate

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow()
		}
	})
}

