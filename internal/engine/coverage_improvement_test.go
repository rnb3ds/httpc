package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ============================================================================
// ENGINE COVERAGE IMPROVEMENT TESTS
// ============================================================================

// ----------------------------------------------------------------------------
// Uncovered HTTP Method Shortcuts (0% coverage)
// ----------------------------------------------------------------------------

func TestClient_HTTPMethodShortcuts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	cfg := &Config{
		Timeout:         30 * time.Second,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		AllowPrivateIPs: true,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("Post", func(t *testing.T) {
		resp, err := client.Post(server.URL)
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Put", func(t *testing.T) {
		resp, err := client.Put(server.URL)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Patch", func(t *testing.T) {
		resp, err := client.Patch(server.URL)
		if err != nil {
			t.Fatalf("Patch failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		resp, err := client.Delete(server.URL)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Head", func(t *testing.T) {
		resp, err := client.Head(server.URL)
		if err != nil {
			t.Fatalf("Head failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Options", func(t *testing.T) {
		resp, err := client.Options(server.URL)
		if err != nil {
			t.Fatalf("Options failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

// ----------------------------------------------------------------------------
// Health Status (0% coverage)
// ----------------------------------------------------------------------------

func TestClient_HealthStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		Timeout:         30 * time.Second,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		AllowPrivateIPs: true,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("GetHealthStatus", func(t *testing.T) {
		status := client.GetHealthStatus()
		if status.TotalRequests < 0 {
			t.Error("TotalRequests should not be negative")
		}
		if status.FailedRequests < 0 {
			t.Error("FailedRequests should not be negative")
		}
	})

	t.Run("IsHealthy", func(t *testing.T) {
		healthy := client.IsHealthy()
		if !healthy {
			t.Error("New client should be healthy")
		}
	})

	// Make some requests to update health metrics
	_, _ = client.Get(server.URL)

	t.Run("HealthStatusAfterRequests", func(t *testing.T) {
		status := client.GetHealthStatus()
		if status.TotalRequests == 0 {
			t.Error("TotalRequests should be > 0 after making requests")
		}
	})
}

// ----------------------------------------------------------------------------
// Error Code Classification (0% coverage)
// ----------------------------------------------------------------------------

func TestClientError_Code(t *testing.T) {
	tests := []struct {
		name     string
		errType  ErrorType
		wantCode string
	}{
		{"Network", ErrorTypeNetwork, "NETWORK_ERROR"},
		{"Timeout", ErrorTypeTimeout, "TIMEOUT"},
		{"ContextCanceled", ErrorTypeContextCanceled, "CONTEXT_CANCELED"},
		{"ResponseRead", ErrorTypeResponseRead, "RESPONSE_READ_ERROR"},
		{"Transport", ErrorTypeTransport, "TRANSPORT_ERROR"},
		{"RetryExhausted", ErrorTypeRetryExhausted, "RETRY_EXHAUSTED"},
		{"TLS", ErrorTypeTLS, "TLS_ERROR"},
		{"Certificate", ErrorTypeCertificate, "CERTIFICATE_ERROR"},
		{"DNS", ErrorTypeDNS, "DNS_ERROR"},
		{"Validation", ErrorTypeValidation, "VALIDATION_ERROR"},
		{"CircuitBreaker", ErrorTypeCircuitBreaker, "CIRCUIT_BREAKER_OPEN"},
		{"HTTP", ErrorTypeHTTP, "HTTP_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ClientError{
				Type:    tt.errType,
				Message: "test error",
			}
			code := err.Code()
			if code != tt.wantCode {
				t.Errorf("Expected code %v, got %v", tt.wantCode, code)
			}
		})
	}
}



// ----------------------------------------------------------------------------
// Context Timeout in Sleep
// ----------------------------------------------------------------------------

func TestClient_SleepWithContext(t *testing.T) {
	cfg := &Config{
		Timeout:         30 * time.Second,
		MaxIdleConns:    10,
		MaxConnsPerHost: 5,
		AllowPrivateIPs: true,
	}
	client, _ := NewClient(cfg)
	defer client.Close()

	t.Run("ContextCanceledDuringSleep", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := client.sleepWithContext(ctx, 200*time.Millisecond)
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected context deadline exceeded error")
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("Sleep should have been interrupted, took %v", elapsed)
		}
	})
}



// ----------------------------------------------------------------------------
// Retry Engine Edge Cases
// ----------------------------------------------------------------------------

func TestRetryEngine_EdgeCases(t *testing.T) {
	t.Run("ZeroMaxRetries", func(t *testing.T) {
		cfg := &Config{
			MaxRetries:    0,
			RetryDelay:    1 * time.Second,
			BackoffFactor: 2.0,
		}
		engine := NewRetryEngine(cfg)
		
		shouldRetry := engine.ShouldRetry(nil, nil, 1)
		if shouldRetry {
			t.Error("Should not retry when MaxRetries is 0")
		}
	})

	t.Run("DelayWithRetryAfterHeader", func(t *testing.T) {
		cfg := &Config{
			MaxRetries:    3,
			RetryDelay:    1 * time.Second,
			BackoffFactor: 2.0,
		}
		engine := NewRetryEngine(cfg)
		resp := &Response{
			StatusCode: 429,
			Headers: map[string][]string{
				"Retry-After": {"5"},
			},
		}
		delay := engine.GetDelayWithResponse(1, resp)
		if delay < 5*time.Second {
			t.Errorf("Expected delay >= 5s, got %v", delay)
		}
	})
}


