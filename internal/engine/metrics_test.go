package engine

import (
	"sync"
	"testing"
)

func TestMetrics_RecordRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		success bool
		latency int64
	}{
		{"SuccessZeroLatency", true, 0},
		{"SuccessNonZeroLatency", true, 1000000}, // 1ms in ns
		{"FailureNonZeroLatency", false, 500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Metrics{}
			m.RecordRequest(tt.latency, tt.success)

			snap := m.Snapshot()
			if snap.TotalRequests != 1 {
				t.Errorf("TotalRequests = %d, want 1", snap.TotalRequests)
			}
			if tt.success && snap.SuccessfulRequests != 1 {
				t.Errorf("SuccessfulRequests = %d, want 1", snap.SuccessfulRequests)
			}
			if !tt.success && snap.FailedRequests != 1 {
				t.Errorf("FailedRequests = %d, want 1", snap.FailedRequests)
			}
		})
	}
}

func TestMetrics_Snapshot(t *testing.T) {
	m := &Metrics{}
	m.RecordRequest(100, true)
	m.RecordRequest(200, true)
	m.RecordRequest(300, false)

	snap := m.Snapshot()
	if snap.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", snap.TotalRequests)
	}
	if snap.SuccessfulRequests != 2 {
		t.Errorf("SuccessfulRequests = %d, want 2", snap.SuccessfulRequests)
	}
	if snap.FailedRequests != 1 {
		t.Errorf("FailedRequests = %d, want 1", snap.FailedRequests)
	}
	if snap.AverageLatency == 0 {
		t.Error("AverageLatency should be non-zero after recording requests")
	}
}

func TestMetrics_Reset(t *testing.T) {
	m := &Metrics{}
	m.RecordRequest(100, true)
	m.RecordRequest(200, false)

	m.Reset()

	snap := m.Snapshot()
	if snap.TotalRequests != 0 {
		t.Errorf("TotalRequests = %d, want 0 after reset", snap.TotalRequests)
	}
	if snap.SuccessfulRequests != 0 {
		t.Errorf("SuccessfulRequests = %d, want 0 after reset", snap.SuccessfulRequests)
	}
	if snap.FailedRequests != 0 {
		t.Errorf("FailedRequests = %d, want 0 after reset", snap.FailedRequests)
	}
	if snap.AverageLatency != 0 {
		t.Errorf("AverageLatency = %v, want 0 after reset", snap.AverageLatency)
	}
}

func TestMetrics_GetHealthStatus(t *testing.T) {
	tests := []struct {
		name        string
		successes   int64
		failures    int64
		wantHealthy bool
		wantRate    float64
	}{
		{"NoRequests_Healthy", 0, 0, true, 0.0},
		{"AllSuccess_Healthy", 10, 0, true, 0.0},
		{"LowErrorRate_Healthy", 19, 1, true, 0.05},
		{"HighErrorRate_Unhealthy", 5, 50, false, 0.9090909090909091},
		{"AllFailures_Unhealthy", 0, 10, false, 1.0},
		{"Boundary10Pct_Unhealthy", 9, 1, false, 0.1}, // error rate == 0.1 means NOT healthy (< 0.1)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Metrics{}
			for i := int64(0); i < tt.successes; i++ {
				m.RecordRequest(100, true)
			}
			for i := int64(0); i < tt.failures; i++ {
				m.RecordRequest(100, false)
			}

			status := m.GetHealthStatus()
			if status.Healthy != tt.wantHealthy {
				t.Errorf("Healthy = %v, want %v (error rate = %f)", status.Healthy, tt.wantHealthy, status.ErrorRate)
			}
			total := tt.successes + tt.failures
			if status.TotalRequests != total {
				t.Errorf("TotalRequests = %d, want %d", status.TotalRequests, total)
			}
			if status.SuccessfulRequests != tt.successes {
				t.Errorf("SuccessfulRequests = %d, want %d", status.SuccessfulRequests, tt.successes)
			}
			if status.FailedRequests != tt.failures {
				t.Errorf("FailedRequests = %d, want %d", status.FailedRequests, tt.failures)
			}
		})
	}
}

func TestMetrics_IsHealthy(t *testing.T) {
	t.Run("Healthy", func(t *testing.T) {
		m := &Metrics{}
		m.RecordRequest(100, true)
		if !m.IsHealthy() {
			t.Error("Client with only successful requests should be healthy")
		}
	})

	t.Run("Unhealthy", func(t *testing.T) {
		m := &Metrics{}
		for i := 0; i < 20; i++ {
			m.RecordRequest(100, false)
		}
		if m.IsHealthy() {
			t.Error("Client with all failures should be unhealthy")
		}
	})
}

func TestMetrics_Concurrent(t *testing.T) {
	m := &Metrics{}
	var wg sync.WaitGroup
	const goroutines = 100
	const opsPerGoroutine = 100

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(success bool) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				m.RecordRequest(int64(i*100), success)
			}
		}(g%2 == 0)
	}
	wg.Wait()

	snap := m.Snapshot()
	expected := int64(goroutines * opsPerGoroutine)
	if snap.TotalRequests != expected {
		t.Errorf("TotalRequests = %d, want %d", snap.TotalRequests, expected)
	}
}
