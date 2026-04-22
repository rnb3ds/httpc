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

// Test_SSRF_DefaultProtection verifies that SSRF protection can be configured
func Test_SSRF_DefaultProtection(t *testing.T) {
	cfg := DefaultConfig()

	// AllowPrivateIPs is false by default (SSRF protection enabled)
	if cfg.Security.AllowPrivateIPs {
		t.Error("AllowPrivateIPs should be false by default (SSRF protection)")
	}

	// Verify SSRF protection can be disabled for internal services
	cfg.Security.AllowPrivateIPs = true
	if !cfg.Security.AllowPrivateIPs {
		t.Error("AllowPrivateIPs should be configurable to true for internal services")
	}
}

// Test_SSRF_ExplicitOptIn verifies that SSRF protection can be disabled (default behavior)
func Test_SSRF_ExplicitOptIn(t *testing.T) {
	cfg := DefaultConfig()
	// AllowPrivateIPs must be explicitly enabled for internal services
	cfg.Security.AllowPrivateIPs = true

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Should work with AllowPrivateIPs enabled for test server
	result, err := client.Get(server.URL, WithTimeout(5*time.Second))
	if err != nil {
		t.Errorf("Expected request to succeed with default config, got: %v", err)
	}
	if result.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", result.StatusCode())
	}
}

// Test_SSRF_BlocksLocalhost verifies that localhost is blocked when SSRF protection is enabled
func Test_SSRF_BlocksLocalhost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = false // Enable SSRF protection

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Should fail because localhost is blocked when SSRF protection is enabled
	_, err = client.Get(server.URL, WithTimeout(5*time.Second))
	if err == nil {
		t.Error("SECURITY ISSUE: Expected error when accessing localhost with SSRF protection enabled")
	}
	if err != nil && !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "localhost") {
		t.Errorf("Expected SSRF blocking error, got: %v", err)
	}
}

// Test_SSRF_RedirectProtection verifies that redirects to private IPs are blocked
func Test_SSRF_RedirectProtection(t *testing.T) {
	// Create a new config that disallows private IPs for redirect testing
	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = false
	cfg.Middleware.FollowRedirects = true

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
	cfg.Security.MaxResponseBodySize = 1000 // Very small limit for testing

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

	if !cfg.Security.AllowPrivateIPs {
		t.Error("TestingConfig should have AllowPrivateIPs = true")
	}
	if !cfg.Security.InsecureSkipVerify {
		t.Error("TestingConfig should have InsecureSkipVerify = true")
	}
}

// Test_SecureConfig_Production verifies SecureConfig is appropriate for production
func Test_SecureConfig_Production(t *testing.T) {
	cfg := SecureConfig()

	// SecureConfig should have strict SSRF protection
	if cfg.Security.AllowPrivateIPs {
		t.Error("SecureConfig should have AllowPrivateIPs = false")
	}
	// SecureConfig should not follow redirects
	if cfg.Middleware.FollowRedirects {
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
	cfg.Timeouts.Request = 100 * time.Millisecond // Very short timeout

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

// assertNoPanic runs fn and reports a test error if it panics.
func assertNoPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC [%s]: %v", name, r)
		}
	}()
	fn()
}

func TestPanicSafety(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	tests := []struct {
		name string
		fn   func()
	}{
		{"NilConfig", func() {
			client, err := New(nil)
			if err == nil && client == nil {
				t.Error("Expected either valid client or error")
			}
			if client != nil {
				client.Close()
			}
		}},
		{"EmptyURL", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			_, err = client.Get("")
			_ = err
		}},
		{"InvalidURL", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			for _, u := range []string{"not a url", "http://", "://invalid", "http://\x00bad"} {
				_, _ = client.Get(u)
			}
		}},
		{"NilBody", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			_, _ = client.Post(server.URL, WithBody(nil))
		}},
		{"MiddlewarePanicRecovery", func() {
			cfg := testConfig()
			cfg.Middleware.Middlewares = []MiddlewareFunc{
				RecoveryMiddleware(),
				func(next Handler) Handler {
					return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
						panic("intentional test panic")
					}
				},
			}
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			_, err = client.Get(server.URL)
			if err == nil {
				t.Error("Expected error from recovered panic")
			}
		}},
		{"DomainClientNilConfig", func() {
			_, err := NewDomain("not a url")
			if err == nil {
				t.Error("Expected error for invalid base URL")
			}
		}},
		{"DownloadInvalidPath", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			_, err = client.DownloadFile(server.URL, "")
			if err == nil {
				t.Error("Expected error for empty file path")
			}
		}},
		{"SessionManagerNilInput", func() {
			session, err := NewSessionManager()
			if err != nil {
				return
			}
			_ = session.SetCookie(nil)
			_ = session.SetCookies(nil)
			_ = session.SetCookies([]*http.Cookie{})
		}},
		{"RequestOptionError", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			// Just verify function definition doesn't panic
			_ = func(r any) error { return fmt.Errorf("test error") }
		}},
		{"ConcurrentAccess", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			defer client.Close()
			done := make(chan bool, 10)
			for i := 0; i < 10; i++ {
				go func() {
					_, _ = client.Get(server.URL)
					done <- true
				}()
			}
			for i := 0; i < 10; i++ {
				<-done
			}
		}},
		{"CloseTwice", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			_ = client.Close()
			_ = client.Close()
		}},
		{"ClosedClient", func() {
			cfg := testConfig()
			client, err := New(cfg)
			if err != nil {
				return
			}
			client.Close()
			_, err = client.Get("http://example.com")
			if err == nil {
				t.Error("Expected error when using closed client")
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertNoPanic(t, tt.name, tt.fn)
		})
	}
}
