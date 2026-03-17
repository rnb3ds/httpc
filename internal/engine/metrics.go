package engine

import (
	"runtime"
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

// HealthStatus represents basic health metrics for the client.
type HealthStatus struct {
	Healthy            bool
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	ErrorRate          float64
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
// Includes backoff strategy to prevent CPU spinning under high contention.
func (m *Metrics) updateLatencyMetrics(latency int64) {
	const maxRetries = 100
	for i := 0; i < maxRetries; i++ {
		current := atomic.LoadInt64(&m.averageLatency)
		// BUGFIX: Handle initial case where current is 0
		// First measurement should be the actual latency, not latency/10
		var newAvg int64
		if current == 0 {
			newAvg = latency
		} else {
			newAvg = (current*9 + latency) / 10
		}
		if atomic.CompareAndSwapInt64(&m.averageLatency, current, newAvg) {
			return
		}
		// Backoff: yield CPU after several failed attempts to reduce contention
		if i > 10 {
			runtime.Gosched()
		}
	}
	// If we exhausted retries, the update is skipped (acceptable for metrics)
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

// GetHealthStatus returns the current health status of the client.
// A client is considered healthy if its error rate is below 10%.
func (m *Metrics) GetHealthStatus() HealthStatus {
	total := atomic.LoadInt64(&m.totalRequests)
	success := atomic.LoadInt64(&m.successfulRequests)
	failed := atomic.LoadInt64(&m.failedRequests)
	avgLatNs := atomic.LoadInt64(&m.averageLatency)

	var errorRate float64
	if total > 0 {
		errorRate = float64(failed) / float64(total)
	}

	healthy := errorRate < 0.1

	return HealthStatus{
		Healthy:            healthy,
		TotalRequests:      total,
		SuccessfulRequests: success,
		FailedRequests:     failed,
		AverageLatency:     time.Duration(avgLatNs),
		ErrorRate:          errorRate,
	}
}

// IsHealthy returns true if the client is healthy (error rate < 10%).
func (m *Metrics) IsHealthy() bool {
	return m.GetHealthStatus().Healthy
}
