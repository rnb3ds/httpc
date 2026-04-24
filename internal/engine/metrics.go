package engine

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsSnapshot represents a point-in-time snapshot of client Metrics.
type MetricsSnapshot struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
}

// HealthStatus represents basic health Metrics for the client.
type HealthStatus struct {
	Healthy            bool
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	ErrorRate          float64
}

// Metrics collects and tracks HTTP client performance Metrics.
// All methods are safe for concurrent use.
type Metrics struct {
	totalRequests      atomic.Int64
	successfulRequests atomic.Int64
	failedRequests     atomic.Int64
	latencyMu          sync.Mutex
	averageLatency     int64 // stored as nanoseconds
}

// RecordRequest records the result of a single request.
// It updates the request counters and rolling average latency.
func (m *Metrics) RecordRequest(latencyNs int64, success bool) {
	m.totalRequests.Add(1)
	if success {
		m.successfulRequests.Add(1)
	} else {
		m.failedRequests.Add(1)
	}
	m.updateLatency(latencyNs)
}

// updateLatency updates the rolling average latency.
func (m *Metrics) updateLatency(latency int64) {
	m.latencyMu.Lock()
	current := m.averageLatency
	if current == 0 {
		m.averageLatency = latency
	} else {
		m.averageLatency = (current*9 + latency) / 10
	}
	m.latencyMu.Unlock()
}

// Snapshot returns a point-in-time copy of the current Metrics.
// Each field is individually atomic, but the snapshot is not transactionally
// consistent — concurrent calls may cause total != success + failed.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalRequests:      m.totalRequests.Load(),
		SuccessfulRequests: m.successfulRequests.Load(),
		FailedRequests:     m.failedRequests.Load(),
		AverageLatency:     time.Duration(m.getAverageLatency()),
	}
}

// Reset resets all Metrics to zero.
func (m *Metrics) Reset() {
	m.totalRequests.Store(0)
	m.successfulRequests.Store(0)
	m.failedRequests.Store(0)
	m.latencyMu.Lock()
	m.averageLatency = 0
	m.latencyMu.Unlock()
}

// GetHealthStatus returns the current health status of the client.
// A client is considered healthy if its error rate is below 10%.
func (m *Metrics) GetHealthStatus() HealthStatus {
	total := m.totalRequests.Load()
	failed := m.failedRequests.Load()
	success := m.successfulRequests.Load()
	avgLatNs := m.getAverageLatency()

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

// getAverageLatency returns the current average latency in nanoseconds.
func (m *Metrics) getAverageLatency() int64 {
	m.latencyMu.Lock()
	defer m.latencyMu.Unlock()
	return m.averageLatency
}
