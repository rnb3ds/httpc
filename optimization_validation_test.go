package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestOptimizations_ConcurrencyImprovements tests the concurrency manager improvements
func TestOptimizations_ConcurrencyImprovements(t *testing.T) {
	requestCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		time.Sleep(50 * time.Millisecond) // Simulate some work
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.MaxRetries = 1
	config.RetryDelay = 10 * time.Millisecond

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numRequests = 200
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)
	successes := int32(0)

	start := time.Now()

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := client.Request(ctx, "GET", server.URL)
			if err != nil {
				errors <- err
			} else {
				atomic.AddInt32(&successes, 1)
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	duration := time.Since(start)

	// Check results
	errorCount := 0
	for err := range errors {
		t.Logf("Request error: %v", err)
		errorCount++
	}

	successCount := atomic.LoadInt32(&successes)
	t.Logf("Completed %d successful requests out of %d total in %v", successCount, numRequests, duration)
	t.Logf("Server processed %d requests", atomic.LoadInt32(&requestCount))
	t.Logf("Error rate: %.2f%%", float64(errorCount)/float64(numRequests)*100)

	// Validate performance improvements
	if successCount < int32(numRequests*0.95) { // Allow 5% failure rate
		t.Errorf("Success rate too low: %d/%d (%.2f%%)", successCount, numRequests, float64(successCount)/float64(numRequests)*100)
	}

	// Should complete reasonably quickly with improved concurrency
	if duration > 10*time.Second {
		t.Errorf("Requests took too long: %v", duration)
	}
}

// TestOptimizations_MemoryManagement tests memory management improvements
func TestOptimizations_MemoryManagement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Return a reasonably sized response
		w.Write([]byte(`{"message":"test response","data":"` + string(make([]byte, 1024)) + `"}`))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Make many requests to test memory management
	const numRequests = 1000
	for i := 0; i < numRequests; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		// Use the response to ensure it's not optimized away
		if len(resp.Body) == 0 {
			t.Errorf("Empty response body at request %d", i)
		}

		// Simulate some processing
		if i%100 == 0 {
			t.Logf("Completed %d requests", i+1)
		}
	}

	t.Log("Memory management test completed successfully")
}

// TestOptimizations_ErrorHandling tests improved error classification
func TestOptimizations_ErrorHandling(t *testing.T) {
	// Test timeout error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.Timeout = 50 * time.Millisecond

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get(server.URL)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
		// Verify error contains timeout information
		if !containsAny(err.Error(), []string{"timeout", "deadline", "context"}) {
			t.Errorf("Error doesn't indicate timeout: %v", err)
		}
	}
}

// TestOptimizations_ContextHandling tests improved context handling
func TestOptimizations_ContextHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	} else {
		t.Logf("Got expected context error: %v", err)
	}
}

// TestOptimizations_ResourceCleanup tests proper resource cleanup
func TestOptimizations_ResourceCleanup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true

	// Create and close multiple clients to test cleanup
	for i := 0; i < 10; i++ {
		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client %d: %v", i, err)
		}

		// Make some requests
		for j := 0; j < 5; j++ {
			_, err := client.Get(server.URL)
			if err != nil {
				t.Errorf("Request failed for client %d, request %d: %v", i, j, err)
			}
		}

		// Close client
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client %d: %v", i, err)
		}
	}

	t.Log("Resource cleanup test completed successfully")
}

// TestOptimizations_ConfigurationValidation tests improved configuration validation
func TestOptimizations_ConfigurationValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		shouldErr bool
	}{
		{
			name: "Valid configuration",
			config: &Config{
				Timeout:         30 * time.Second,
				MaxRetries:      3,
				RetryDelay:      1 * time.Second,
				BackoffFactor:   2.0,
				MaxIdleConns:    100,
				MaxConnsPerHost: 20,
				UserAgent:       "TestAgent/1.0",
			},
			shouldErr: false,
		},
		{
			name: "Invalid timeout",
			config: &Config{
				Timeout: -1 * time.Second,
			},
			shouldErr: true,
		},
		{
			name: "Invalid retry settings",
			config: &Config{
				MaxRetries:    -1,
				RetryDelay:    1 * time.Second,
				BackoffFactor: 2.0,
			},
			shouldErr: true,
		},
		{
			name: "Invalid connection settings",
			config: &Config{
				MaxIdleConns:    -1,
				MaxConnsPerHost: 10,
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if tt.shouldErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Helper function to check if string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if len(substr) > 0 && len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// Helper function removed - using existing newTestClient from test_helpers.go
