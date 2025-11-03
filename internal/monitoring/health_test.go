package monitoring

import (
	"sync"
	"testing"
	"time"
)

// ============================================================================
// HEALTH CHECKER TESTS
// ============================================================================

func TestNewHealthChecker(t *testing.T) {
	checker := NewHealthChecker()

	if checker == nil {
		t.Fatal("NewHealthChecker should not return nil")
	}

	// Check initial health status
	status := checker.CheckHealth()

	if !status.IsHealthy {
		t.Error("New health checker should be healthy initially")
	}

	if status.HealthScore != 100 {
		t.Errorf("Expected initial health score 100, got %d", status.HealthScore)
	}
}

func TestHealthChecker_RecordRequest_Success(t *testing.T) {
	checker := NewHealthChecker()

	// Record successful requests
	for i := 0; i < 10; i++ {
		checker.RecordRequest(true, 100*time.Millisecond, false)
	}

	// Check health status
	status := checker.CheckHealth()

	if !status.IsHealthy {
		t.Error("Health checker should be healthy after successful requests")
	}

	if status.HealthScore < 90 {
		t.Errorf("Expected high health score, got %d", status.HealthScore)
	}
}

func TestHealthChecker_RecordRequest_Failures(t *testing.T) {
	checker := NewHealthChecker()

	// Record many failed requests
	for i := 0; i < 50; i++ {
		checker.RecordRequest(false, 100*time.Millisecond, false)
	}

	// Check health - should degrade
	status := checker.CheckHealth()
	if status.HealthScore > 50 {
		t.Errorf("Expected low health score after failures, got %d", status.HealthScore)
	}
}

func TestHealthChecker_RecordRequest_Timeouts(t *testing.T) {
	checker := NewHealthChecker()

	// Record timeout requests
	for i := 0; i < 20; i++ {
		checker.RecordRequest(false, 5*time.Second, true)
	}

	// Check health - timeouts should affect it
	status := checker.CheckHealth()
	if status.HealthScore > 70 {
		t.Errorf("Expected degraded health score after timeouts, got %d", status.HealthScore)
	}
}

func TestHealthChecker_RecordRequest_MixedResults(t *testing.T) {
	checker := NewHealthChecker()

	// Record mixed results: 95% success, 5% failure (at threshold)
	for i := 0; i < 100; i++ {
		success := i < 95
		checker.RecordRequest(success, 100*time.Millisecond, false)
	}

	// Check health - should still be healthy with 95% success rate
	status := checker.CheckHealth()

	if !status.IsHealthy {
		t.Errorf("Health checker should be healthy with 95%% success rate, got score %d", status.HealthScore)
	}

	if status.HealthScore < 90 {
		t.Errorf("Expected health score >= 90, got %d", status.HealthScore)
	}
}

func TestHealthChecker_UpdateResourceMetrics(t *testing.T) {
	checker := NewHealthChecker()

	// Update resource metrics
	checker.UpdateResourceMetrics(50, 0.75, 1024*1024*100) // 50 conns, 75% util, 100MB

	// Verify metrics are recorded (health check should reflect this)
	status := checker.CheckHealth()
	if status.ActiveConnections != 50 {
		t.Errorf("Expected 50 active connections, got %d", status.ActiveConnections)
	}
}

func TestHealthChecker_CheckHealth(t *testing.T) {
	checker := NewHealthChecker()

	// Record some requests
	for i := 0; i < 10; i++ {
		checker.RecordRequest(true, 100*time.Millisecond, false)
	}

	// Update resource metrics
	checker.UpdateResourceMetrics(10, 0.5, 1024*1024*50)

	// Check health
	status := checker.CheckHealth()

	if status.TotalRequests != 10 {
		t.Errorf("Expected 10 total requests, got %d", status.TotalRequests)
	}

	if status.SuccessRate < 0.99 {
		t.Errorf("Expected success rate ~1.0, got %f", status.SuccessRate)
	}

	if status.FailureRate > 0.01 {
		t.Errorf("Expected failure rate ~0, got %f", status.FailureRate)
	}

	if !status.IsHealthy {
		t.Error("Expected healthy status")
	}

	if status.HealthScore < 90 {
		t.Errorf("Expected high health score, got %d", status.HealthScore)
	}
}

func TestHealthChecker_IsHealthy(t *testing.T) {
	checker := NewHealthChecker()

	// Initially healthy
	checker.CheckHealth()
	if !checker.IsHealthy() {
		t.Error("New checker should be healthy")
	}

	// After successful requests
	for i := 0; i < 10; i++ {
		checker.RecordRequest(true, 100*time.Millisecond, false)
	}
	checker.CheckHealth()
	if !checker.IsHealthy() {
		t.Error("Should be healthy after successful requests")
	}

	// After many failures
	for i := 0; i < 100; i++ {
		checker.RecordRequest(false, 100*time.Millisecond, false)
	}
	checker.CheckHealth()
	if checker.IsHealthy() {
		t.Error("Should be unhealthy after many failures")
	}
}

func TestHealthChecker_GetHealthScore(t *testing.T) {
	checker := NewHealthChecker()

	// Initial score should be 100
	checker.CheckHealth()
	score := checker.GetHealthScore()
	if score != 100 {
		t.Errorf("Expected initial score 100, got %d", score)
	}

	// Score should decrease with failures
	for i := 0; i < 50; i++ {
		checker.RecordRequest(false, 100*time.Millisecond, false)
	}

	checker.CheckHealth()
	newScore := checker.GetHealthScore()
	if newScore >= score {
		t.Errorf("Expected score to decrease, got %d (was %d)", newScore, score)
	}

	// Score should be between 0 and 100
	if newScore < 0 || newScore > 100 {
		t.Errorf("Health score should be between 0-100, got %d", newScore)
	}
}

func TestHealthChecker_Reset(t *testing.T) {
	checker := NewHealthChecker()

	// Record some requests
	for i := 0; i < 50; i++ {
		checker.RecordRequest(false, 100*time.Millisecond, false)
	}

	// Update resource metrics
	checker.UpdateResourceMetrics(100, 0.9, 1024*1024*200)

	// Verify unhealthy state
	checker.CheckHealth()
	if checker.IsHealthy() {
		t.Error("Should be unhealthy before reset")
	}

	// Reset
	checker.Reset()

	// Verify reset state
	if !checker.IsHealthy() {
		t.Error("Should be healthy after reset")
	}

	score := checker.GetHealthScore()
	if score != 100 {
		t.Errorf("Expected score 100 after reset, got %d", score)
	}

	status := checker.CheckHealth()
	if status.TotalRequests != 0 {
		t.Errorf("Expected 0 total requests after reset, got %d", status.TotalRequests)
	}
}

func TestHealthChecker_ConcurrentAccess(t *testing.T) {
	checker := NewHealthChecker()

	var wg sync.WaitGroup
	numGoroutines := 100
	requestsPerGoroutine := 100

	// Concurrent RecordRequest calls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				success := (id+j)%2 == 0
				checker.RecordRequest(success, 100*time.Millisecond, false)
			}
		}(i)
	}

	// Concurrent UpdateResourceMetrics calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				checker.UpdateResourceMetrics(int64(j), float64(j)/100.0, int64(j*1024))
			}
		}()
	}

	// Concurrent CheckHealth calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = checker.CheckHealth()
				_ = checker.IsHealthy()
				_ = checker.GetHealthScore()
			}
		}()
	}

	wg.Wait()

	// Verify final state is consistent
	status := checker.CheckHealth()
	expectedTotal := int64(numGoroutines * requestsPerGoroutine)
	if status.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, status.TotalRequests)
	}

	// Health score should be valid
	score := checker.GetHealthScore()
	if score < 0 || score > 100 {
		t.Errorf("Health score should be between 0-100, got %d", score)
	}
}

func TestHealthChecker_LatencyTracking(t *testing.T) {
	checker := NewHealthChecker()

	// Record requests with varying latencies
	latencies := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		150 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
	}

	for _, latency := range latencies {
		checker.RecordRequest(true, latency, false)
	}

	status := checker.CheckHealth()

	// Verify latency metrics are tracked
	if status.AverageLatency == 0 {
		t.Error("Average latency should be tracked")
	}

	if status.MaxLatency == 0 {
		t.Error("Max latency should be tracked")
	}

	if status.MinLatency == 0 {
		t.Error("Min latency should be tracked")
	}

	// Max should be >= average >= min
	if status.MaxLatency < status.AverageLatency {
		t.Error("Max latency should be >= average latency")
	}

	if status.AverageLatency < status.MinLatency {
		t.Error("Average latency should be >= min latency")
	}
}

func TestHealthChecker_HealthScoreCalculation(t *testing.T) {
	tests := []struct {
		name            string
		successCount    int
		failureCount    int
		timeoutCount    int
		expectedHealthy bool
		minScore        int
		maxScore        int
	}{
		{
			name:            "All successful",
			successCount:    100,
			failureCount:    0,
			timeoutCount:    0,
			expectedHealthy: true,
			minScore:        90,
			maxScore:        100,
		},
		{
			name:            "Mostly successful (96%)",
			successCount:    96,
			failureCount:    4,
			timeoutCount:    0,
			expectedHealthy: true,
			minScore:        90,
			maxScore:        100,
		},
		{
			name:            "Half successful",
			successCount:    50,
			failureCount:    50,
			timeoutCount:    0,
			expectedHealthy: false,
			minScore:        0,
			maxScore:        50,
		},
		{
			name:            "Mostly failures",
			successCount:    10,
			failureCount:    90,
			timeoutCount:    0,
			expectedHealthy: false,
			minScore:        0,
			maxScore:        20,
		},
		{
			name:            "Many timeouts",
			successCount:    50,
			failureCount:    0,
			timeoutCount:    50,
			expectedHealthy: false,
			minScore:        0,
			maxScore:        70,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewHealthChecker()

			// Record successful requests
			for i := 0; i < tt.successCount; i++ {
				checker.RecordRequest(true, 100*time.Millisecond, false)
			}

			// Record failed requests
			for i := 0; i < tt.failureCount; i++ {
				checker.RecordRequest(false, 100*time.Millisecond, false)
			}

			// Record timeout requests
			for i := 0; i < tt.timeoutCount; i++ {
				checker.RecordRequest(false, 5*time.Second, true)
			}

			// Check health
			status := checker.CheckHealth()

			if status.IsHealthy != tt.expectedHealthy {
				t.Errorf("Expected healthy=%v, got %v", tt.expectedHealthy, status.IsHealthy)
			}

			if status.HealthScore < tt.minScore || status.HealthScore > tt.maxScore {
				t.Errorf("Expected score between %d-%d, got %d", tt.minScore, tt.maxScore, status.HealthScore)
			}
		})
	}
}

