package monitoring

import (
	"sync/atomic"
	"time"
)

// HealthChecker provides comprehensive health monitoring for the HTTP client
type HealthChecker struct {
	// Request metrics
	totalRequests  int64
	successfulReqs int64
	failedReqs     int64
	timeoutReqs    int64

	// Performance metrics
	avgLatency int64 // nanoseconds
	maxLatency int64 // nanoseconds
	minLatency int64 // nanoseconds

	// Resource metrics
	activeConnections int64
	poolUtilization   int64 // percentage (0-100)
	memoryUsage       int64 // bytes

	// Health status
	lastHealthCheck int64 // unix timestamp
	healthScore     int64 // 0-100
	isHealthy       int32 // 0 or 1

	// Configuration
	maxFailureRate      float64
	maxLatencyThreshold int64 // nanoseconds
	maxPoolUtilization  float64
}

// HealthStatus represents the current health status
type HealthStatus struct {
	IsHealthy         bool
	HealthScore       int
	TotalRequests     int64
	SuccessRate       float64
	FailureRate       float64
	TimeoutRate       float64
	AverageLatency    time.Duration
	MaxLatency        time.Duration
	MinLatency        time.Duration
	ActiveConnections int64
	PoolUtilization   float64
	MemoryUsage       int64
	LastCheck         time.Time
	Issues            []string
}

// NewHealthChecker creates a new health checker with default thresholds
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		maxFailureRate:      0.05,                   // 5% failure rate threshold
		maxLatencyThreshold: int64(5 * time.Second), // 5 second latency threshold
		maxPoolUtilization:  0.8,                    // 80% pool utilization threshold
		minLatency:          int64(time.Hour),       // Initialize to high value
	}
}

// RecordRequest records a request completion
func (h *HealthChecker) RecordRequest(success bool, latency time.Duration, isTimeout bool) {
	atomic.AddInt64(&h.totalRequests, 1)

	if success {
		atomic.AddInt64(&h.successfulReqs, 1)
	} else {
		atomic.AddInt64(&h.failedReqs, 1)
	}

	if isTimeout {
		atomic.AddInt64(&h.timeoutReqs, 1)
	}

	// Update latency metrics
	latencyNs := latency.Nanoseconds()
	h.updateLatencyMetrics(latencyNs)
}

// updateLatencyMetrics updates latency statistics atomically
func (h *HealthChecker) updateLatencyMetrics(latencyNs int64) {
	// Update average latency using exponential moving average
	for {
		current := atomic.LoadInt64(&h.avgLatency)
		newAvg := (current*9 + latencyNs) / 10
		if atomic.CompareAndSwapInt64(&h.avgLatency, current, newAvg) {
			break
		}
	}

	// Update max latency
	for {
		current := atomic.LoadInt64(&h.maxLatency)
		if latencyNs <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&h.maxLatency, current, latencyNs) {
			break
		}
	}

	// Update min latency
	for {
		current := atomic.LoadInt64(&h.minLatency)
		if latencyNs >= current && current != int64(time.Hour) {
			break
		}
		if atomic.CompareAndSwapInt64(&h.minLatency, current, latencyNs) {
			break
		}
	}
}

// UpdateResourceMetrics updates resource utilization metrics
func (h *HealthChecker) UpdateResourceMetrics(activeConns int64, poolUtil float64, memUsage int64) {
	atomic.StoreInt64(&h.activeConnections, activeConns)
	atomic.StoreInt64(&h.poolUtilization, int64(poolUtil*100))
	atomic.StoreInt64(&h.memoryUsage, memUsage)
}

// CheckHealth performs a comprehensive health check
func (h *HealthChecker) CheckHealth() HealthStatus {
	now := time.Now()
	atomic.StoreInt64(&h.lastHealthCheck, now.Unix())

	total := atomic.LoadInt64(&h.totalRequests)
	successful := atomic.LoadInt64(&h.successfulReqs)
	failed := atomic.LoadInt64(&h.failedReqs)
	timeouts := atomic.LoadInt64(&h.timeoutReqs)

	var successRate, failureRate, timeoutRate float64
	if total > 0 {
		successRate = float64(successful) / float64(total)
		failureRate = float64(failed) / float64(total)
		timeoutRate = float64(timeouts) / float64(total)
	}

	avgLatency := time.Duration(atomic.LoadInt64(&h.avgLatency))
	maxLatency := time.Duration(atomic.LoadInt64(&h.maxLatency))
	minLatency := time.Duration(atomic.LoadInt64(&h.minLatency))
	if minLatency == time.Hour {
		minLatency = 0
	}

	activeConns := atomic.LoadInt64(&h.activeConnections)
	poolUtil := float64(atomic.LoadInt64(&h.poolUtilization)) / 100
	memUsage := atomic.LoadInt64(&h.memoryUsage)

	// Calculate health score and identify issues
	score, issues := h.calculateHealthScore(failureRate, timeoutRate, avgLatency, poolUtil)

	isHealthy := score >= 70 // Healthy if score is 70 or above
	atomic.StoreInt32(&h.isHealthy, boolToInt32(isHealthy))
	atomic.StoreInt64(&h.healthScore, int64(score))

	return HealthStatus{
		IsHealthy:         isHealthy,
		HealthScore:       score,
		TotalRequests:     total,
		SuccessRate:       successRate,
		FailureRate:       failureRate,
		TimeoutRate:       timeoutRate,
		AverageLatency:    avgLatency,
		MaxLatency:        maxLatency,
		MinLatency:        minLatency,
		ActiveConnections: activeConns,
		PoolUtilization:   poolUtil,
		MemoryUsage:       memUsage,
		LastCheck:         now,
		Issues:            issues,
	}
}

// calculateHealthScore calculates a health score from 0-100 and identifies issues
func (h *HealthChecker) calculateHealthScore(failureRate, timeoutRate float64, avgLatency time.Duration, poolUtil float64) (int, []string) {
	score := 100
	var issues []string

	// Penalize high failure rate
	if failureRate > h.maxFailureRate {
		penalty := int((failureRate - h.maxFailureRate) * 1000) // Heavy penalty for failures
		score -= penalty
		issues = append(issues, "High failure rate detected")
	}

	// Penalize high timeout rate
	if timeoutRate > 0.02 { // 2% timeout threshold
		penalty := int(timeoutRate * 500) // Moderate penalty for timeouts
		score -= penalty
		issues = append(issues, "High timeout rate detected")
	}

	// Penalize high latency
	if avgLatency.Nanoseconds() > h.maxLatencyThreshold {
		penalty := int((avgLatency.Nanoseconds() - h.maxLatencyThreshold) / int64(time.Millisecond) / 10)
		score -= penalty
		issues = append(issues, "High average latency detected")
	}

	// Penalize high pool utilization
	if poolUtil > h.maxPoolUtilization {
		penalty := int((poolUtil - h.maxPoolUtilization) * 200)
		score -= penalty
		issues = append(issues, "High connection pool utilization")
	}

	// Ensure score is within bounds
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score, issues
}

// IsHealthy returns the current health status
func (h *HealthChecker) IsHealthy() bool {
	return atomic.LoadInt32(&h.isHealthy) == 1
}

// GetHealthScore returns the current health score (0-100)
func (h *HealthChecker) GetHealthScore() int {
	return int(atomic.LoadInt64(&h.healthScore))
}

// Reset resets all metrics (useful for testing)
func (h *HealthChecker) Reset() {
	atomic.StoreInt64(&h.totalRequests, 0)
	atomic.StoreInt64(&h.successfulReqs, 0)
	atomic.StoreInt64(&h.failedReqs, 0)
	atomic.StoreInt64(&h.timeoutReqs, 0)
	atomic.StoreInt64(&h.avgLatency, 0)
	atomic.StoreInt64(&h.maxLatency, 0)
	atomic.StoreInt64(&h.minLatency, int64(time.Hour))
	atomic.StoreInt64(&h.activeConnections, 0)
	atomic.StoreInt64(&h.poolUtilization, 0)
	atomic.StoreInt64(&h.memoryUsage, 0)
	atomic.StoreInt32(&h.isHealthy, 1)
	atomic.StoreInt64(&h.healthScore, 100)
}

func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
