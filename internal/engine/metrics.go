package engine

import (
	"sync/atomic"
	"time"
)

// MetricsSnapshot represents a point-in-time snapshot of client metrics.
type MetricsSnapshot struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
}

// Metrics collects and tracks HTTP client performance metrics.
// All methods are safe for concurrent use.
type Metrics struct {
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	averageLatency     int64 // stored as nanoseconds for atomic operations
}

// RecordRequest records the result of a single request.
// It updates the request counters and rolling average latency.
func (m *Metrics) RecordRequest(latencyNs int64, success bool) {
	atomic.AddInt64(&m.totalRequests, 1)
	if success {
		atomic.AddInt64(&m.successfulRequests, 1)
	} else {
		atomic.AddInt64(&m.failedRequests, 1)
	}
	m.updateLatencyMetrics(latencyNs)
}

// updateLatencyMetrics updates the rolling average latency using lock-free atomic operations.
func (m *Metrics) updateLatencyMetrics(latency int64) {
	for {
		current := atomic.LoadInt64(&m.averageLatency)
		newAvg := (current*9 + latency) / 10
		if atomic.CompareAndSwapInt64(&m.averageLatency, current, newAvg) {
			break
		}
	}
}

// Snapshot returns a point-in-time copy of the current metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalRequests:      atomic.LoadInt64(&m.totalRequests),
		SuccessfulRequests: atomic.LoadInt64(&m.successfulRequests),
		FailedRequests:     atomic.LoadInt64(&m.failedRequests),
		AverageLatency:     time.Duration(atomic.LoadInt64(&m.averageLatency)),
	}
}

// Reset resets all metrics to zero.
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.totalRequests, 0)
	atomic.StoreInt64(&m.successfulRequests, 0)
	atomic.StoreInt64(&m.failedRequests, 0)
	atomic.StoreInt64(&m.averageLatency, 0)
}
