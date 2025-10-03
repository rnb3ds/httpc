package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// CONCURRENCY MANAGER UNIT TESTS
// ============================================================================

func TestManager_New(t *testing.T) {
	tests := []struct {
		name           string
		maxConcurrent  int
		queueSize      int
		expectedMax    int64
		expectedQueue  int
	}{
		{
			name:          "Valid configuration",
			maxConcurrent: 10,
			queueSize:     20,
			expectedMax:   10,
			expectedQueue: 20,
		},
		{
			name:          "Zero maxConcurrent uses default",
			maxConcurrent: 0,
			queueSize:     20,
			expectedMax:   100, // Default
			expectedQueue: 20,
		},
		{
			name:          "Zero queueSize uses default",
			maxConcurrent: 10,
			queueSize:     0,
			expectedMax:   10,
			expectedQueue: 20, // 2x maxConcurrent
		},
		{
			name:          "Negative values use defaults",
			maxConcurrent: -5,
			queueSize:     -10,
			expectedMax:   100,
			expectedQueue: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(tt.maxConcurrent, tt.queueSize)
			defer m.Close()

			if m.maxConcurrent != tt.expectedMax {
				t.Errorf("Expected maxConcurrent %d, got %d", tt.expectedMax, m.maxConcurrent)
			}

			if m.queueSize != tt.expectedQueue {
				t.Errorf("Expected queueSize %d, got %d", tt.expectedQueue, m.queueSize)
			}

			if m.semaphore == nil {
				t.Error("Semaphore should not be nil")
			}

			if m.queue == nil {
				t.Error("Queue should not be nil")
			}

			if m.metrics == nil {
				t.Error("Metrics should not be nil")
			}
		})
	}
}

func TestManager_Execute_Success(t *testing.T) {
	m := NewManager(5, 10)
	defer m.Close()

	ctx := context.Background()
	executed := false

	err := m.Execute(ctx, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !executed {
		t.Error("Function should have been executed")
	}

	metrics := m.GetMetrics()
	if metrics.TotalRequests != 1 {
		t.Errorf("Expected 1 total request, got %d", metrics.TotalRequests)
	}

	if metrics.CompletedRequests != 1 {
		t.Errorf("Expected 1 completed request, got %d", metrics.CompletedRequests)
	}
}

func TestManager_Execute_Error(t *testing.T) {
	m := NewManager(5, 10)
	defer m.Close()

	ctx := context.Background()
	expectedErr := errors.New("test error")

	err := m.Execute(ctx, func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	metrics := m.GetMetrics()
	if metrics.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", metrics.FailedRequests)
	}
}

func TestManager_Execute_ContextCancellation(t *testing.T) {
	m := NewManager(1, 1)
	defer m.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := m.Execute(ctx, func() error {
		t.Error("Function should not be executed")
		return nil
	})

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}

	metrics := m.GetMetrics()
	if metrics.RejectedRequests == 0 {
		t.Error("Expected rejected request due to context cancellation")
	}
}

func TestManager_Execute_QueueFull(t *testing.T) {
	m := NewManager(1, 1)
	defer m.Close()

	// Block the worker
	blockCh := make(chan struct{})
	ctx := context.Background()

	// Fill the queue
	go m.Execute(ctx, func() error {
		<-blockCh
		return nil
	})

	// Wait a bit for the first request to be queued
	time.Sleep(10 * time.Millisecond)

	// Try to add another request (should fill the queue)
	go m.Execute(ctx, func() error {
		<-blockCh
		return nil
	})

	time.Sleep(10 * time.Millisecond)

	// This should fail because queue is full
	err := m.Execute(context.Background(), func() error {
		return nil
	})

	close(blockCh)

	if err == nil {
		t.Error("Expected error for full queue")
	}

	if err.Error() != "request queue is full" {
		t.Errorf("Expected 'request queue is full', got: %v", err)
	}
}

func TestManager_Execute_Concurrent(t *testing.T) {
	maxConcurrent := 10
	m := NewManager(maxConcurrent, 100)
	defer m.Close()

	numRequests := 100
	var wg sync.WaitGroup
	var completed int64
	var failed int64

	ctx := context.Background()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := m.Execute(ctx, func() error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})

			if err != nil {
				atomic.AddInt64(&failed, 1)
			} else {
				atomic.AddInt64(&completed, 1)
			}
		}()
	}

	wg.Wait()

	if completed != int64(numRequests) {
		t.Errorf("Expected %d completed requests, got %d", numRequests, completed)
	}

	if failed != 0 {
		t.Errorf("Expected 0 failed requests, got %d", failed)
	}

	metrics := m.GetMetrics()
	if metrics.TotalRequests != int64(numRequests) {
		t.Errorf("Expected %d total requests, got %d", numRequests, metrics.TotalRequests)
	}
}

func TestManager_GetMetrics(t *testing.T) {
	m := NewManager(5, 10)
	defer m.Close()

	ctx := context.Background()

	// Execute some requests
	for i := 0; i < 5; i++ {
		m.Execute(ctx, func() error {
			time.Sleep(5 * time.Millisecond)
			return nil
		})
	}

	metrics := m.GetMetrics()

	if metrics.TotalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", metrics.TotalRequests)
	}

	if metrics.CompletedRequests != 5 {
		t.Errorf("Expected 5 completed requests, got %d", metrics.CompletedRequests)
	}

	if metrics.LastUpdate == 0 {
		t.Error("LastUpdate should be set")
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager(5, 10)

	err := m.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got: %v", err)
	}

	// Try to close again
	err = m.Close()
	if err == nil {
		t.Error("Expected error on double close")
	}

	// Try to execute after close
	ctx := context.Background()
	err = m.Execute(ctx, func() error {
		return nil
	})

	if err == nil {
		t.Error("Expected error when executing on closed manager")
	}
}

func TestManager_IsHealthy(t *testing.T) {
	m := NewManager(10, 20)
	defer m.Close()

	// Initially should be healthy
	if !m.IsHealthy() {
		t.Error("Manager should be healthy initially")
	}

	ctx := context.Background()

	// Execute some requests
	for i := 0; i < 5; i++ {
		m.Execute(ctx, func() error {
			return nil
		})
	}

	// Should still be healthy
	if !m.IsHealthy() {
		t.Error("Manager should be healthy after normal requests")
	}
}

func TestManager_MetricsUpdate(t *testing.T) {
	m := NewManager(5, 10)
	defer m.Close()

	ctx := context.Background()

	// Execute requests with different outcomes
	m.Execute(ctx, func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	m.Execute(ctx, func() error {
		time.Sleep(20 * time.Millisecond)
		return errors.New("test error")
	})

	metrics := m.GetMetrics()

	if metrics.CompletedRequests != 1 {
		t.Errorf("Expected 1 completed request, got %d", metrics.CompletedRequests)
	}

	if metrics.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", metrics.FailedRequests)
	}

	if metrics.AverageExecTime == 0 {
		t.Error("AverageExecTime should be updated")
	}

	if metrics.MaxExecTime == 0 {
		t.Error("MaxExecTime should be updated")
	}
}

