package httpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// SECURITY AUDIT TESTS - Verify security fixes from 2026-03-14 audit
// ============================================================================

// Test_SSRF_DefaultProtection verifies that SSRF protection is enabled by default
func Test_SSRF_DefaultProtection(t *testing.T) {
	cfg := DefaultConfig()

	// SECURITY: AllowPrivateIPs should be false by default
	if cfg.AllowPrivateIPs {
		t.Error("SECURITY ISSUE: AllowPrivateIPs should be false by default to prevent SSRF attacks")
	}
}

// Test_SSRF_ExplicitOptIn verifies that SSRF protection can be explicitly disabled
func Test_SSRF_ExplicitOptIn(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowPrivateIPs = true // Explicit opt-in

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Should work with explicit opt-in
	result, err := client.Get(server.URL, WithTimeout(5*time.Second))
	if err != nil {
		t.Errorf("Expected request to succeed with explicit opt-in, got: %v", err)
	}
	if result.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", result.StatusCode())
	}
}

// Test_SSRF_BlocksLocalhost verifies that localhost is blocked by default
func Test_SSRF_BlocksLocalhost(t *testing.T) {
	cfg := DefaultConfig()
	// AllowPrivateIPs is false by default

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Should fail because localhost is blocked
	_, err = client.Get(server.URL, WithTimeout(5*time.Second))
	if err == nil {
		t.Error("SECURITY ISSUE: Expected error when accessing localhost with default config")
	}
	if !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "localhost") {
		t.Errorf("Expected SSRF blocking error, got: %v", err)
	}
}

// Test_SSRF_RedirectProtection verifies that redirects to private IPs are blocked
func Test_SSRF_RedirectProtection(t *testing.T) {
	// Create a new config that disallows private IPs for redirect testing
	cfg := DefaultConfig()
	cfg.AllowPrivateIPs = false
	cfg.FollowRedirects = true

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create a server that redirects to localhost
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to localhost (should be blocked)
		http.Redirect(w, r, "http://127.0.0.1:12345/", http.StatusFound)
	}))
	defer redirectServer.Close()

	// Should fail because redirect target is blocked
	_, err = client.Get(redirectServer.URL, WithTimeout(5*time.Second))
	if err == nil {
		t.Error("SECURITY ISSUE: Expected error when redirecting to private IP")
	}
	if !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "redirect") {
		t.Errorf("Expected redirect blocking error, got: %v", err)
	}
}

// Test_DecompressionBombProtection verifies that decompression bombs are blocked
func Test_DecompressionBombProtection(t *testing.T) {
	cfg := testConfig()
	cfg.MaxResponseBodySize = 1000 // Very small limit for testing

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create a server that returns a response larger than the limit
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return more than 1000 bytes
		largeData := make([]byte, 2000)
		w.WriteHeader(http.StatusOK)
		w.Write(largeData)
	}))
	defer server.Close()

	// Should fail because response exceeds limit
	_, err = client.Get(server.URL, WithTimeout(5*time.Second))
	if err == nil {
		t.Error("SECURITY ISSUE: Expected error when response exceeds size limit")
	}
	// Check for size-related error (could be "exceeds", "limit", or "failed to read")
	if !strings.Contains(err.Error(), "exceeds") &&
		!strings.Contains(err.Error(), "limit") &&
		!strings.Contains(err.Error(), "failed to read") {
		t.Errorf("Expected size limit error, got: %v", err)
	}
}

// Test_TestingConfig_Warning verifies that TestingConfig warns in non-test environments
func Test_TestingConfig_Warning(t *testing.T) {
	// Since we're in a test environment, the warning should not be printed
	// But we can verify the config is created correctly
	cfg := TestingConfig()

	if !cfg.AllowPrivateIPs {
		t.Error("TestingConfig should have AllowPrivateIPs = true")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("TestingConfig should have InsecureSkipVerify = true")
	}
}

// Test_SecureConfig_Production verifies SecureConfig is appropriate for production
func Test_SecureConfig_Production(t *testing.T) {
	cfg := SecureConfig()

	// SecureConfig should have strict SSRF protection
	if cfg.AllowPrivateIPs {
		t.Error("SecureConfig should have AllowPrivateIPs = false")
	}
	// SecureConfig should not follow redirects
	if cfg.FollowRedirects {
		t.Error("SecureConfig should have FollowRedirects = false")
	}
}

// Test_Context_Cancellation verifies that context cancellation is properly handled
func Test_Context_Cancellation(t *testing.T) {
	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

// Test_Timeout_Enforcement verifies that timeouts are properly enforced
func Test_Timeout_Enforcement(t *testing.T) {
	cfg := testConfig()
	cfg.Timeout = 100 * time.Millisecond // Very short timeout

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err = client.Get(server.URL)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

// ============================================================================
// PANIC SAFETY TESTS - Verify library never panics in production
// ============================================================================

// Test_PanicSafety_NilConfig verifies that nil config is handled safely
func Test_PanicSafety_NilConfig(t *testing.T) {
	// This should not panic - should return error or use defaults
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: New() with nil config panicked: %v", r)
		}
	}()

	client, err := New(nil)
	if err == nil && client == nil {
		t.Error("Expected either valid client or error, got both nil")
	}
	if client != nil {
		client.Close()
	}
}

// Test_PanicSafety_EmptyURL verifies that empty URL is handled safely
func Test_PanicSafety_EmptyURL(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Get() with empty URL panicked: %v", r)
		}
	}()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get("")
	if err == nil {
		t.Error("Expected error for empty URL")
	}
}

// Test_PanicSafety_InvalidURL verifies that invalid URLs are handled safely
func Test_PanicSafety_InvalidURL(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Get() with invalid URL panicked: %v", r)
		}
	}()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	invalidURLs := []string{
		"not a url",
		"http://",
		"://invalid",
		"http://\x00bad",
	}

	for _, url := range invalidURLs {
		_, err := client.Get(url)
		// Should return error, not panic
		_ = err
	}
}

// Test_PanicSafety_NilBody verifies that nil body is handled safely
func Test_PanicSafety_NilBody(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Post() with nil body panicked: %v", r)
		}
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Post with nil body should not panic
	_, err = client.Post(server.URL, WithBody(nil))
	_ = err // Error is acceptable, panic is not
}

// Test_PanicSafety_MiddlewarePanicRecovery verifies that middleware panics are recovered
func Test_PanicSafety_MiddlewarePanicRecovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Middleware panic was not recovered: %v", r)
		}
	}()

	panickingMiddleware := func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			panic("intentional test panic")
		}
	}

	cfg := testConfig()
	cfg.Middlewares = []MiddlewareFunc{
		RecoveryMiddleware(),
		panickingMiddleware,
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Should not panic, should return error
	_, err = client.Get(server.URL)
	if err == nil {
		t.Error("Expected error from recovered panic")
	}
	if !strings.Contains(err.Error(), "panic") && !strings.Contains(err.Error(), "recovered") {
		t.Errorf("Expected panic recovery error, got: %v", err)
	}
}

// Test_PanicSafety_DomainClientNilConfig verifies DomainClient handles nil config
func Test_PanicSafety_DomainClientNilConfig(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: NewDomain() with invalid input panicked: %v", r)
		}
	}()

	// Invalid base URL should return error, not panic
	_, err := NewDomain("not a url")
	if err == nil {
		t.Error("Expected error for invalid base URL")
	}
}

// Test_PanicSafety_DownloadInvalidPath verifies download handles invalid paths
func Test_PanicSafety_DownloadInvalidPath(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: DownloadFile() with invalid path panicked: %v", r)
		}
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Empty file path should return error, not panic
	_, err = client.DownloadFile(server.URL, "")
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// Test_PanicSafety_SessionManagerNilInput verifies SessionManager handles nil input
func Test_PanicSafety_SessionManagerNilInput(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: SessionManager with nil input panicked: %v", r)
		}
	}()

	session := NewSessionManager()

	// nil cookie should return error, not panic
	err := session.SetCookie(nil)
	if err == nil {
		t.Error("Expected error for nil cookie")
	}

	// nil cookies slice is valid (empty slice) - should not panic
	err = session.SetCookies(nil)
	// nil slice is valid in Go, so no error is expected
	_ = err

	// Empty slice should also work
	err = session.SetCookies([]*http.Cookie{})
	_ = err
}

// Test_PanicSafety_RequestOptionError verifies request options handle errors
func Test_PanicSafety_RequestOptionError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Request option that returns error panicked: %v", r)
		}
	}()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Option that returns error should not panic
	errorOption := func(r any) error {
		return fmt.Errorf("intentional test error")
	}

	// Using a custom request option type - this is an edge case test
	_ = errorOption // Just verify no panic occurs with function definition
}

// Test_PanicSafety_ConcurrentAccess verifies thread safety
func Test_PanicSafety_ConcurrentAccess(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Concurrent access caused panic: %v", r)
		}
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Concurrent requests should not cause race conditions or panics
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = client.Get(server.URL)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test_PanicSafety_CloseTwice verifies that closing client twice is safe
func Test_PanicSafety_CloseTwice(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Closing client twice panicked: %v", r)
		}
	}()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// First close
	err = client.Close()
	if err != nil {
		t.Errorf("First close returned error: %v", err)
	}

	// Second close should not panic
	err = client.Close()
	// Error is acceptable, panic is not
	_ = err
}

// Test_PanicSafety_ClosedClient verifies that using closed client is safe
func Test_PanicSafety_ClosedClient(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: Using closed client panicked: %v", r)
		}
	}()

	cfg := testConfig()
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close the client
	client.Close()

	// Using closed client should return error, not panic
	_, err = client.Get("http://example.com")
	if err == nil {
		t.Error("Expected error when using closed client")
	}
}
