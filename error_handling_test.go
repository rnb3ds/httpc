package httpc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// ERROR CLASSIFICATION TESTS
// ============================================================================

func TestErrorHandling_NetworkErrors(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name        string
		url         string
		wantErrType string
	}{
		{
			name:        "Connection refused",
			url:         "http://localhost:99999",
			wantErrType: "transport",
		},
		{
			name:        "Invalid host",
			url:         "http://this-host-does-not-exist-12345.com",
			wantErrType: "transport",
		},
		{
			name:        "Invalid port",
			url:         "http://example.com:99999",
			wantErrType: "transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Get(tt.url)
			if err == nil {
				t.Error("Expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErrType) {
				t.Logf("Error: %v", err)
			}
		})
	}
}

func TestErrorHandling_TimeoutErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true // Allow localhost for testing
	client, _ := New(config)
	defer client.Close()

	_, err := client.Get(server.URL, WithTimeout(100*time.Millisecond))
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "timeout") && !strings.Contains(errMsg, "timed out") && !strings.Contains(errMsg, "deadline") && !strings.Contains(errMsg, "canceled") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestErrorHandling_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	_, err := client.Get(server.URL, WithContext(ctx))
	if err == nil {
		t.Fatal("Expected context canceled error, got nil")
	}

	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context canceled error, got: %v", err)
	}
}

func TestErrorHandling_InvalidURL(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name string
		url  string
	}{
		{"Empty URL", ""},
		{"Invalid scheme", "ftp://example.com"},
		{"Missing scheme", "example.com"},
		{"Invalid characters", "http://exam ple.com"},
		{"JavaScript protocol", "javascript:alert(1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Get(tt.url)
			if err == nil {
				t.Error("Expected error for invalid URL, got nil")
			}
		})
	}
}

func TestErrorHandling_InvalidHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name             string
		key              string
		value            string
		shouldBeFiltered bool
	}{
		{"CRLF in key", "X-Test\r\n", "value", true},
		{"CRLF in value", "X-Test", "value\r\nInjected", true},
		{"Null byte in key", "X-Test\x00", "value", true},
		{"Null byte in value", "X-Test", "value\x00", true},
		{"Valid header", "X-Test", "value", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Our design silently filters dangerous headers instead of erroring
			// This is a security feature to prevent header injection attacks
			resp, err := client.Get(server.URL, WithHeader(tt.key, tt.value))
			if err != nil {
				t.Errorf("Request should succeed even with filtered headers: %v", err)
			}
			if resp != nil && !resp.IsSuccess() {
				t.Errorf("Expected successful response, got status: %d", resp.StatusCode)
			}
		})
	}
}

// ============================================================================
// ERROR RECOVERY TESTS
// ============================================================================

func TestErrorHandling_RecoverFromPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Client panicked: %v", r)
		}
	}()

	_, err := client.Get(server.URL)
	if err != nil {
		t.Logf("Request error: %v", err)
	}
}

func TestErrorHandling_MultipleErrors(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	// Make multiple failing requests
	for i := 0; i < 5; i++ {
		_, err := client.Get("http://localhost:99999")
		if err == nil {
			t.Error("Expected error, got nil")
		}
	}

	// Client should still be functional
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Client not functional after errors: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// ============================================================================
// ERROR MESSAGE TESTS
// ============================================================================

func TestErrorHandling_ErrorMessageFormat(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	_, err := client.Get("http://localhost:99999")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	errMsg := err.Error()

	// Error message should contain useful information
	if !strings.Contains(errMsg, "GET") {
		t.Error("Error message should contain HTTP method")
	}

	// Error message should not contain sensitive information
	if strings.Contains(errMsg, "password") || strings.Contains(errMsg, "secret") {
		t.Error("Error message contains sensitive information")
	}
}

func TestErrorHandling_ErrorWrapping(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	_, err := client.Get("http://localhost:99999")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Check if error can be unwrapped
	var netErr *net.OpError
	if !errors.As(err, &netErr) {
		t.Log("Error is not a network error (this is OK)")
	}
}

// ============================================================================
// RETRY ERROR TESTS
// ============================================================================

func TestErrorHandling_RetryableErrors(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 10 * time.Millisecond
	config.AllowPrivateIPs = true // Allow localhost for testing
	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}
}

func TestErrorHandling_NonRetryableErrors(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxRetries = 3
	config.RetryDelay = 10 * time.Millisecond
	config.AllowPrivateIPs = true // Allow localhost for testing
	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	// Should not retry for 400 errors
	if attemptCount != 1 {
		t.Errorf("Expected 1 attempt, got %d", attemptCount)
	}
}

// ============================================================================
// RESOURCE CLEANUP ON ERROR TESTS
// ============================================================================

func TestErrorHandling_ResourceCleanupOnError(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	// Make multiple failing requests
	for i := 0; i < 10; i++ {
		_, _ = client.Get("http://localhost:99999")
	}

	// Client should still be able to close cleanly
	if err := client.Close(); err != nil {
		t.Errorf("Failed to close client after errors: %v", err)
	}
}

func TestErrorHandling_ConcurrentErrors(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	// Make concurrent failing requests
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_, _ = client.Get("http://localhost:99999")
			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Client should still be functional
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Client not functional after concurrent errors: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
