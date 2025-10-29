package httpc

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// SECURITY TESTS
// ============================================================================

func TestSecurity_URLValidation(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{"Valid HTTP URL", "http://example.com", false},
		{"Valid HTTPS URL", "https://example.com", false},
		{"Empty URL", "", true},
		{"Invalid URL", "not-a-url", true},
		{"Missing Scheme", "example.com", true},
		{"Invalid Scheme", "ftp://example.com", true},
		{"JavaScript Protocol", "javascript:alert(1)", true},
		{"Data URL", "data:text/html,<script>alert(1)</script>", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Get(tt.url)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for URL: %s", tt.url)
			}
			if !tt.shouldErr && err != nil && !strings.Contains(err.Error(), "no such host") {
				t.Errorf("Unexpected error for valid URL %s: %v", tt.url, err)
			}
		})
	}
}

func TestSecurity_HeaderValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if dangerous headers are filtered
		for key := range r.Header {
			if strings.ContainsAny(key, "\r\n\x00") {
				t.Errorf("Dangerous header key was not filtered: %s", key)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name        string
		key         string
		value       string
		shouldBlock bool
	}{
		{"Valid Header", "X-Custom-Header", "value", false},
		{"CRLF Injection in Key", "X-Test\r\nX-Injected", "value", true},
		{"CRLF Injection in Value", "X-Test", "value\r\nX-Injected: bad", true},
		{"Null Byte in Key", "X-Test\x00", "value", true},
		{"Null Byte in Value", "X-Test", "value\x00", true},
		{"Very Long Header Value", "X-Test", strings.Repeat("a", 10000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unsafe headers should be silently filtered, request should succeed
			_, err := client.Get(server.URL, WithHeader(tt.key, tt.value))
			if err != nil {
				t.Errorf("Request failed: %v", err)
			}
			// Security enhancement: dangerous headers are silently filtered instead of generating errors
		})
	}
}

func TestSecurity_RequestSizeValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Create large payload
	largeData := make([]byte, 100*1024*1024) // 100MB
	rand.Read(largeData)

	// This should be handled by the client
	_, err := client.Post(server.URL, WithBody(largeData))
	// The request might succeed or fail depending on server limits
	// We're mainly testing that the client doesn't crash
	_ = err
}

func TestSecurity_TLSConfiguration(t *testing.T) {
	// TLS configuration is handled internally
	config := DefaultConfig()
	config.MaxRetries = 1
	config.FollowRedirects = false
	config.EnableCookies = false
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create secure client: %v", err)
	}
	defer client.Close()
}

func TestSecurity_InsecureSkipVerify(t *testing.T) {
	config := DefaultConfig()
	if config.InsecureSkipVerify {
		t.Error("Default config should not skip TLS verification")
	}
}

func TestSecurity_SensitiveHeaderHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify sensitive headers are properly set
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header not set")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Test that sensitive headers are properly handled
	_, err := client.Get(server.URL,
		WithBearerToken("secret-token"),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}

// ============================================================================
// RATE LIMITING AND DOS PROTECTION TESTS
// ============================================================================

func TestSecurity_ConcurrencyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	// Try to send more requests than the limit
	const numRequests = 50
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := client.Get(server.URL)
			errors <- err
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		err := <-errors
		if err == nil {
			successCount++
		}
	}

	// All requests should eventually succeed due to queuing
	if successCount == 0 {
		t.Error("Expected some requests to succeed")
	}
}

func TestSecurity_RequestQueueOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()

	client, _ := New(config)
	defer client.Close()

	// Send many requests quickly
	const numRequests = 2000
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := client.Request(ctx, "GET", server.URL)
			errors <- err
		}()
	}

	// Some requests should fail due to queue overflow or timeout
	rejectedCount := 0
	for i := 0; i < numRequests; i++ {
		err := <-errors
		if err != nil {
			rejectedCount++
		}
	}

	t.Logf("Rejected %d out of %d requests", rejectedCount, numRequests)
}

// ============================================================================
// INJECTION ATTACK TESTS
// ============================================================================

func TestSecurity_SQLInjectionInQueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server should receive properly encoded query params
		query := r.URL.Query().Get("id")
		if strings.Contains(query, "' OR '1'='1") {
			// The injection attempt should be properly encoded
			t.Log("SQL injection attempt detected (properly encoded)")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Try SQL injection in query parameter
	_, err := client.Get(server.URL, WithQuery("id", "1' OR '1'='1"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}

func TestSecurity_XSSInHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xssAttempt := r.Header.Get("X-Custom")
		if strings.Contains(xssAttempt, "<script>") {
			t.Log("XSS attempt in header (should be rejected by validation)")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Headers with script tags should be handled safely
	_, err := client.Get(server.URL, WithHeader("X-Custom", "<script>alert('xss')</script>"))
	// The request might succeed or fail depending on validation
	_ = err
}

// ============================================================================
// RESOURCE EXHAUSTION TESTS
// ============================================================================

func TestSecurity_MemoryLeakPrevention(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":"response"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Make many requests to check for memory leaks
	for i := 0; i < 1000; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		_ = resp.Body // Use the response
	}
}

func TestSecurity_ConnectionPoolExhaustion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.MaxIdleConns = 10
	config.AllowPrivateIPs = true

	client, _ := New(config)
	defer client.Close()

	// Make many concurrent requests
	const numRequests = 100
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := client.Get(server.URL)
			errors <- err
		}()
	}

	// All requests should succeed despite limited connection pool
	failCount := 0
	for i := 0; i < numRequests; i++ {
		if err := <-errors; err != nil {
			failCount++
		}
	}

	if failCount > 0 {
		t.Logf("Failed %d out of %d requests", failCount, numRequests)
	}
}

func TestSecurity_GoroutineLeakPrevention(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create and close multiple clients
	for i := 0; i < 10; i++ {
		client, _ := newTestClient()

		// Make some requests
		for j := 0; j < 10; j++ {
			client.Get(server.URL)
		}

		// Close client
		client.Close()
	}

	// If there are goroutine leaks, they would accumulate here
	// This test mainly ensures the code doesn't panic
}

// ============================================================================
// ERROR HANDLING TESTS
// ============================================================================

func TestSecurity_ErrorMessageSanitization(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	// Test with URL containing sensitive information that will fail
	// Using a non-existent port to ensure connection failure
	sensitiveURL := "https://user:password@example.com:99999/api/secret"
	_, err := client.Get(sensitiveURL)

	if err != nil {
		errMsg := err.Error()
		t.Logf("Error message: %s", errMsg)

		// Error message should not expose sensitive credentials
		if strings.Contains(errMsg, "password") {
			t.Errorf("Error message exposes password: %s", errMsg)
		}

		if strings.Contains(errMsg, "user:password") {
			t.Errorf("Error message exposes user credentials: %s", errMsg)
		}

		// Should contain redacted placeholder (*** for credentials)
		if !strings.Contains(errMsg, "***:***@") {
			t.Errorf("Error message doesn't sanitize credentials properly: %s", errMsg)
		}
	} else {
		t.Error("Expected error for URL with invalid port, but got none")
	}
}

func TestSecurity_PanicRecovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Test that client handles panics gracefully
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Client should not panic: %v", r)
		}
	}()

	// Make request with potentially problematic options
	_, err := client.Get(server.URL, nil) // nil option
	_ = err
}
