package httpc

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CONFIGURATION TESTS - Config validation, presets, TLS versions
// ============================================================================

// ----------------------------------------------------------------------------
// Config Creation and Defaults
// ----------------------------------------------------------------------------

// TestConfig_Defaults pins the exact value of every DefaultConfig field: a
// silently-shifted default is a behavior change for every caller starting
// from DefaultConfig(), so regressions must fail by name, not merely "be
// positive". (Values mirrored from DefaultConfig in types.go.)
func TestConfig_Defaults(t *testing.T) {
	config := DefaultConfig()

	// Timeouts
	if config.Timeouts.Request != 180*time.Second {
		t.Errorf("Timeouts.Request = %v, want 180s", config.Timeouts.Request)
	}
	if config.Timeouts.Dial != 10*time.Second {
		t.Errorf("Timeouts.Dial = %v, want 10s", config.Timeouts.Dial)
	}
	if config.Timeouts.TLSHandshake != 10*time.Second {
		t.Errorf("Timeouts.TLSHandshake = %v, want 10s", config.Timeouts.TLSHandshake)
	}
	if config.Timeouts.ResponseHeader != 0 {
		t.Errorf("Timeouts.ResponseHeader = %v, want 0 (disabled)", config.Timeouts.ResponseHeader)
	}
	if config.Timeouts.IdleConn != 90*time.Second {
		t.Errorf("Timeouts.IdleConn = %v, want 90s", config.Timeouts.IdleConn)
	}

	// Connection
	if config.Connection.MaxIdleConns != 50 {
		t.Errorf("Connection.MaxIdleConns = %d, want 50", config.Connection.MaxIdleConns)
	}
	if config.Connection.MaxConnsPerHost != 10 {
		t.Errorf("Connection.MaxConnsPerHost = %d, want 10", config.Connection.MaxConnsPerHost)
	}
	if config.Connection.ProxyURL != "" {
		t.Errorf("Connection.ProxyURL = %q, want empty", config.Connection.ProxyURL)
	}
	if config.Connection.EnableSystemProxy {
		t.Error("Connection.EnableSystemProxy = true, want false")
	}
	if !config.Connection.EnableHTTP2 {
		t.Error("Connection.EnableHTTP2 = false, want true")
	}
	if config.Connection.EnableCookies {
		t.Error("Connection.EnableCookies = true, want false")
	}
	if config.Connection.EnableDoH {
		t.Error("Connection.EnableDoH = true, want false")
	}
	if config.Connection.DoHCacheTTL != 5*time.Minute {
		t.Errorf("Connection.DoHCacheTTL = %v, want 5m", config.Connection.DoHCacheTTL)
	}

	// Security
	if config.Security.TLSConfig != nil {
		t.Error("Security.TLSConfig = non-nil, want nil")
	}
	if config.Security.MinTLSVersion != tls.VersionTLS12 {
		t.Errorf("Security.MinTLSVersion = %#x, want TLS 1.2", config.Security.MinTLSVersion)
	}
	if config.Security.MaxTLSVersion != tls.VersionTLS13 {
		t.Errorf("Security.MaxTLSVersion = %#x, want TLS 1.3", config.Security.MaxTLSVersion)
	}
	if config.Security.InsecureSkipVerify {
		t.Error("Security.InsecureSkipVerify = true, want false")
	}
	if config.Security.MaxResponseBodySize != 10*1024*1024 {
		t.Errorf("Security.MaxResponseBodySize = %d, want 10MB", config.Security.MaxResponseBodySize)
	}
	if config.Security.MaxDecompressedBodySize != 100*1024*1024 {
		t.Errorf("Security.MaxDecompressedBodySize = %d, want 100MB", config.Security.MaxDecompressedBodySize)
	}
	if config.Security.AllowPrivateIPs {
		t.Error("Security.AllowPrivateIPs = true, want false")
	}
	if !config.Security.ValidateURL {
		t.Error("Security.ValidateURL = false, want true")
	}
	if !config.Security.ValidateHeaders {
		t.Error("Security.ValidateHeaders = false, want true")
	}
	if !config.Security.StrictContentLength {
		t.Error("Security.StrictContentLength = false, want true")
	}
	if !config.Security.FollowRedirects {
		t.Error("Security.FollowRedirects = false, want true")
	}
	if config.Security.MaxRedirects != 10 {
		t.Errorf("Security.MaxRedirects = %d, want 10", config.Security.MaxRedirects)
	}

	// Retry
	if config.Retry.MaxRetries != 3 {
		t.Errorf("Retry.MaxRetries = %d, want 3", config.Retry.MaxRetries)
	}
	if config.Retry.Delay != 1*time.Second {
		t.Errorf("Retry.Delay = %v, want 1s", config.Retry.Delay)
	}
	if config.Retry.BackoffFactor != 2.0 {
		t.Errorf("Retry.BackoffFactor = %v, want 2.0", config.Retry.BackoffFactor)
	}
	if !config.Retry.EnableJitter {
		t.Error("Retry.EnableJitter = false, want true")
	}
	if config.Retry.MaxRetryDelay != 30*time.Second {
		t.Errorf("Retry.MaxRetryDelay = %v, want 30s", config.Retry.MaxRetryDelay)
	}
	if config.Retry.RetryNonIdempotent {
		t.Error("Retry.RetryNonIdempotent = true, want false")
	}
	if config.Retry.CustomPolicy != nil {
		t.Error("Retry.CustomPolicy = non-nil, want nil")
	}

	// Middleware / Defaults
	if config.Middleware.Middlewares != nil {
		t.Error("Middleware.Middlewares = non-nil, want nil")
	}
	if config.Defaults.UserAgent != "httpc/1.0" {
		t.Errorf("Defaults.UserAgent = %q, want %q", config.Defaults.UserAgent, "httpc/1.0")
	}
	if config.Defaults.Headers == nil || len(config.Defaults.Headers) != 0 {
		t.Errorf("Defaults.Headers = %#v, want empty non-nil map", config.Defaults.Headers)
	}
}

// ----------------------------------------------------------------------------
// Config Presets - Creation and Field Verification
// ----------------------------------------------------------------------------

func TestConfig_Presets(t *testing.T) {
	t.Run("SecureConfig", func(t *testing.T) {
		config := SecureConfig()
		client, err := New(config)
		if err != nil {
			t.Fatalf("New(SecureConfig()) failed: %v", err)
		}
		defer func() { _ = client.Close() }()

		// Verify security-focused settings
		if config.Security.MinTLSVersion < tls.VersionTLS12 {
			t.Error("Secure config should enforce TLS 1.2+")
		}
		if config.Security.InsecureSkipVerify {
			t.Error("Secure config should not skip TLS verification")
		}
		if config.Security.AllowPrivateIPs {
			t.Error("Secure config should not allow private IPs")
		}
		if config.Timeouts.Request != 15*time.Second {
			t.Errorf("Expected Timeout=15s, got %v", config.Timeouts.Request)
		}
		if config.Retry.MaxRetries != 1 {
			t.Errorf("Expected MaxRetries=1, got %d", config.Retry.MaxRetries)
		}
		if config.Security.FollowRedirects {
			t.Error("Expected FollowRedirects=false")
		}
	})

	t.Run("PerformanceConfig", func(t *testing.T) {
		config := PerformanceConfig()
		client, err := New(config)
		if err != nil {
			t.Fatalf("New(PerformanceConfig()) failed: %v", err)
		}
		defer func() { _ = client.Close() }()

		// Verify performance-focused settings
		if config.Connection.MaxIdleConns <= 0 {
			t.Error("Performance config should have connection pooling")
		}
		if !config.Connection.EnableHTTP2 {
			t.Error("Performance config should enable HTTP/2")
		}
		if config.Timeouts.Request != 60*time.Second {
			t.Errorf("Expected Timeout=60s, got %v", config.Timeouts.Request)
		}
	})

	t.Run("MinimalConfig", func(t *testing.T) {
		config := MinimalConfig()
		client, err := New(config)
		if err != nil {
			t.Fatalf("New(MinimalConfig()) failed: %v", err)
		}
		defer func() { _ = client.Close() }()

		// Verify minimal settings
		if config.Retry.MaxRetries != 0 {
			t.Error("Minimal config should have no retries")
		}
		if config.Security.FollowRedirects {
			t.Error("Minimal config should not follow redirects")
		}
	})

	t.Run("TestingConfig", func(t *testing.T) {
		config := TestingConfig()

		// Verify testing-focused settings
		if config.Timeouts.Request != 180*time.Second {
			t.Errorf("Expected timeout 180s, got %v", config.Timeouts.Request)
		}
		if !config.Security.AllowPrivateIPs {
			t.Error("Expected AllowPrivateIPs to be true")
		}
		if !config.Security.InsecureSkipVerify {
			t.Error("Expected InsecureSkipVerify to be true for testing")
		}
		if config.Retry.MaxRetries != 1 {
			t.Errorf("Expected MaxRetries=1, got %d", config.Retry.MaxRetries)
		}
		if config.Defaults.UserAgent != "httpc-test/1.0" {
			t.Errorf("Expected UserAgent='httpc-test/1.0', got %q", config.Defaults.UserAgent)
		}
	})
}

// ----------------------------------------------------------------------------
// Config Validation - Field-Specific Tests
// ----------------------------------------------------------------------------

func TestConfig_Validation(t *testing.T) {
	t.Run("Timeout", func(t *testing.T) {
		tests := []struct {
			name    string
			timeout time.Duration
			wantErr bool
		}{
			{"Positive", 30 * time.Second, false},
			{"Zero", 0, false},
			{"Negative", -1 * time.Second, true},
			{"TooLarge", 24 * time.Hour, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.Timeouts.Request = tt.timeout
				_, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("MaxRetries", func(t *testing.T) {
		tests := []struct {
			name       string
			maxRetries int
			wantErr    bool
		}{
			{"Zero", 0, false},
			{"Positive", 3, false},
			{"Negative", -1, true},
			{"TooLarge", 100, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.Retry.MaxRetries = tt.maxRetries
				_, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("ConnectionPool", func(t *testing.T) {
		tests := []struct {
			name         string
			maxIdleConns int
			maxConns     int
			wantErr      bool
		}{
			{"Valid", 100, 10, false},
			{"NegativeIdle", -1, 10, true},
			{"NegativePerHost", 100, -1, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.Connection.MaxIdleConns = tt.maxIdleConns
				config.Connection.MaxConnsPerHost = tt.maxConns
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					_ = client.Close()
				}
			})
		}
	})

	t.Run("UserAgent", func(t *testing.T) {
		tests := []struct {
			name      string
			userAgent string
			wantErr   bool
		}{
			{"Valid", "MyApp/1.0", false},
			{"Empty", "", false},
			{"WithCRLF", "MyApp\r\n/1.0", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.Defaults.UserAgent = tt.userAgent
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					_ = client.Close()
				}
			})
		}
	})
}

// ----------------------------------------------------------------------------
// TLS Configuration
// ----------------------------------------------------------------------------

func TestConfig_TLSVersions(t *testing.T) {
	// (The former "MinTLSVersion" and "MaxTLSVersion" subtests were removed:
	// both tables contained only wantErr=false rows setting the versions to
	// their DefaultConfig values — the identical configuration every other
	// test constructs — while the invalid-value rows live in
	// TestValidateConfig_AdditionalBoundaries. The subtests below carry real
	// coverage: the Min>Max rejection and the TLSConfig merge.)

	t.Run("TLSVersionRange", func(t *testing.T) {
		tests := []struct {
			name       string
			minVersion uint16
			maxVersion uint16
			wantErr    bool
		}{
			{"Valid: 1.2-1.3", tls.VersionTLS12, tls.VersionTLS13, false},
			{"Valid: 1.2-1.2", tls.VersionTLS12, tls.VersionTLS12, false},
			{"Invalid: Min>Max", tls.VersionTLS13, tls.VersionTLS12, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.Security.MinTLSVersion = tt.minVersion
				config.Security.MaxTLSVersion = tt.maxVersion
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					_ = client.Close()
				}
			})
		}
	})

	t.Run("WithTLSConfig", func(t *testing.T) {
		config := DefaultConfig()
		config.Security.MinTLSVersion = tls.VersionTLS12
		config.Security.MaxTLSVersion = tls.VersionTLS13
		config.Security.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS13,
		}

		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer func() { _ = client.Close() }()
	})
}

// ----------------------------------------------------------------------------
// Config Immutability
// ----------------------------------------------------------------------------

func TestConfig_Modification(t *testing.T) {
	config := DefaultConfig()
	config.Security.AllowPrivateIPs = true
	originalTimeout := config.Timeouts.Request

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Modify config after client creation
	config.Timeouts.Request = 1 * time.Nanosecond
	config.Retry.MaxRetries = 10

	// Sanity check: config was actually modified
	if config.Timeouts.Request == originalTimeout {
		t.Fatal("sanity check failed: config should have been modified")
	}

	// Verify client is unaffected: make a request that would time out
	// if the client used the modified 1ns timeout.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Small delay that 1ns timeout can't survive
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err = client.Get(server.URL)
	if err != nil {
		t.Errorf("client should use original timeout, not modified 1ns: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Internal Helper Functions
// ----------------------------------------------------------------------------

// TestConfig_InternalHelpers and TestConfig_AdvancedFields were removed: the former
// asserted isTestEnvironment()==true inside a test binary (tautology; real
// coverage in TestIsTestEnvironment_BoundaryConditions and client_test.go's
// ISV-warning table), the latter assigned a kitchen-sink of fields and only
// asserted New() succeeds — every field's accept/reject behavior is already
// table-tested in TestConfig_Validation and TestValidateConfig_AdditionalBoundaries.

func TestConfig_String(t *testing.T) {
	t.Run("Nil config", func(t *testing.T) {
		var config *Config = nil
		result := config.String()
		if result != "Config{<nil>}" {
			t.Errorf("Expected 'Config{<nil>}', got %q", result)
		}
	})

	t.Run("Default config", func(t *testing.T) {
		config := DefaultConfig()
		result := config.String()

		// Verify key parts are present
		if !strings.Contains(result, "Config{") {
			t.Error("String should start with 'Config{'")
		}
		if !strings.Contains(result, "Request:") {
			t.Error("String should contain 'Request:'")
		}
		if !strings.Contains(result, "ProxyURL:") {
			t.Error("String should contain 'ProxyURL:'")
		}
		if !strings.Contains(result, "TLSConfig:") {
			t.Error("String should contain 'TLSConfig:'")
		}
		if !strings.Contains(result, "<default>") {
			t.Error("String should contain '<default>' for nil TLSConfig")
		}
	})

	t.Run("Config with TLS", func(t *testing.T) {
		config := DefaultConfig()
		config.Security.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		result := config.String()
		if !strings.Contains(result, "<configured>") {
			t.Error("String should contain '<configured>' for non-nil TLSConfig")
		}
	})

	t.Run("Config with all fields", func(t *testing.T) {
		config := &Config{
			Timeouts: TimeoutConfig{
				Request:      30 * time.Second,
				Dial:         5 * time.Second,
				TLSHandshake: 5 * time.Second,
			},
			Connection: ConnectionConfig{
				MaxIdleConns:    100,
				MaxConnsPerHost: 20,
				ProxyURL:        "http://proxy:8080",
			},
			Security: SecurityConfig{
				InsecureSkipVerify: true,
				AllowPrivateIPs:    true,
				FollowRedirects:    false,
			},
			Retry: RetryConfig{
				MaxRetries:    3,
				BackoffFactor: 1.5,
			},
			Defaults: RequestDefaults{
				UserAgent: "test-agent",
			},
		}

		result := config.String()

		// Verify all key fields are present
		expectedParts := []string{
			"Request:",
			"Dial:",
			"TLSHandshake:",
			"MaxIdleConns:",
			"MaxConnsPerHost:",
			"ProxyURL:",
			"InsecureSkipVerify:",
			"AllowPrivateIPs:",
			"MaxRetries:",
			"BackoffFactor:",
			"UserAgent:",
			"FollowRedirects:",
		}

		for _, part := range expectedParts {
			if !strings.Contains(result, part) {
				t.Errorf("String should contain %q", part)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// maskProxyURL Tests (via Config.String())
// ----------------------------------------------------------------------------

func TestMaskProxyURL(t *testing.T) {
	tests := []struct {
		name     string
		proxyURL string
		// We test this via Config.String() since maskProxyURL is not exported
		contains    string
		notContains string
	}{
		{
			name:     "Empty URL",
			proxyURL: "",
			contains: "ProxyURL:",
		},
		{
			name:     "URL without credentials",
			proxyURL: "http://proxy.example.com:8080",
			contains: "proxy.example.com",
		},
		{
			name:        "URL with credentials",
			proxyURL:    "http://user:secret@proxy.example.com:8080",
			notContains: "secret",
		},
		{
			name:     "HTTPS proxy URL",
			proxyURL: "https://proxy.example.com:8443",
			contains: "proxy.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.Connection.ProxyURL = tt.proxyURL
			result := config.String()

			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got: %s", tt.contains, result)
			}
			if tt.notContains != "" && strings.Contains(result, tt.notContains) {
				t.Errorf("Expected result NOT to contain %q, got: %s", tt.notContains, result)
			}
		})
	}
}

func TestConfig_String_UserAgentTruncation(t *testing.T) {
	config := DefaultConfig()
	config.Defaults.UserAgent = strings.Repeat("x", 60)
	result := config.String()
	if !strings.Contains(result, "x...") {
		t.Error("Long UserAgent should be truncated with '...'")
	}
	config.Defaults.UserAgent = "short-agent"
	result = config.String()
	if !strings.Contains(result, "short-agent") {
		t.Error("Short UserAgent should appear in full")
	}
}

func TestDefaultCookieSecurityConfig(t *testing.T) {
	cfg := DefaultCookieSecurityConfig()
	if cfg == nil {
		t.Fatal("DefaultCookieSecurityConfig returned nil")
	}
	if cfg.RequireSecure {
		t.Error("Default should not require Secure")
	}
	if cfg.RequireHttpOnly {
		t.Error("Default should not require HttpOnly")
	}
}

func TestStrictCookieSecurityConfig(t *testing.T) {
	cfg := StrictCookieSecurityConfig()
	if cfg == nil {
		t.Fatal("StrictCookieSecurityConfig returned nil")
	}
	if !cfg.RequireSecure {
		t.Error("Strict should require Secure")
	}
	if !cfg.RequireHttpOnly {
		t.Error("Strict should require HttpOnly")
	}
	if cfg.RequireSameSite != "Strict" {
		t.Error("Strict should require SameSite=Strict")
	}
}

// ----------------------------------------------------------------------------
// ValidateConfig additional boundary cases
// ----------------------------------------------------------------------------

func TestValidateConfig_AdditionalBoundaries(t *testing.T) {
	// Built before the table so a failure fails here (on the test's own
	// goroutine) instead of inside a table closure, where FailNow would hit
	// the parent t across a subtest boundary.
	pinner, err := NewSPKIHashPinner(testSPKIHashSecondary)
	if err != nil {
		t.Fatalf("NewSPKIHashPinner: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"nil config", func(c *Config) {}, true},
		{"negative dial timeout", func(c *Config) { c.Timeouts.Dial = -1 * time.Second }, true},
		{"negative TLS handshake timeout", func(c *Config) { c.Timeouts.TLSHandshake = -1 * time.Second }, true},
		{"negative response header timeout", func(c *Config) { c.Timeouts.ResponseHeader = -1 * time.Second }, true},
		{"negative idle conn timeout", func(c *Config) { c.Timeouts.IdleConn = -1 * time.Second }, true},
		{"negative max idle conns", func(c *Config) { c.Connection.MaxIdleConns = -1 }, true},
		{"negative max conns per host", func(c *Config) { c.Connection.MaxConnsPerHost = -1 }, true},
		{"negative max response body size", func(c *Config) { c.Security.MaxResponseBodySize = -1 }, true},
		{"negative retry delay", func(c *Config) { c.Retry.Delay = -1 * time.Second }, true},
		{"invalid middleware headers", func(c *Config) { c.Defaults.Headers = map[string]string{"X-Bad": "value\r\nevil"} }, true},
		{"retry delay zero", func(c *Config) { c.Retry.Delay = 0 }, false},
		{"max retries at limit (10)", func(c *Config) { c.Retry.MaxRetries = 10 }, false},
		{"max retries over limit (11)", func(c *Config) { c.Retry.MaxRetries = 11 }, true},
		{"request timeout at limit (30m)", func(c *Config) { c.Timeouts.Request = 30 * time.Minute }, false},
		{"request timeout over limit (30m+1ms)", func(c *Config) { c.Timeouts.Request = 30*time.Minute + time.Millisecond }, true},
		{"backoff factor zero", func(c *Config) { c.Retry.BackoffFactor = 0 }, true},
		{"negative backoff factor", func(c *Config) { c.Retry.BackoffFactor = -1 }, true},
		{"max response body size zero", func(c *Config) { c.Security.MaxResponseBodySize = 0 }, false},
		{"backoff factor at minimum", func(c *Config) { c.Retry.BackoffFactor = 1.0 }, false},
		{"backoff factor at maximum", func(c *Config) { c.Retry.BackoffFactor = 10.0 }, false},
		{"backoff factor over maximum", func(c *Config) { c.Retry.BackoffFactor = 11.0 }, true},
		{"invalid proxy pool entry", func(c *Config) { c.Connection.ProxyPool = []string{"ftp://bad.example.com:8080"} }, true},
		{"negative proxy failure threshold", func(c *Config) { c.Connection.ProxyFailureThreshold = -1 }, true},
		{"negative proxy cooldown", func(c *Config) { c.Connection.ProxyCooldown = -1 * time.Second }, true},
		{"invalid rotate status code low", func(c *Config) { c.Connection.ProxyRotateOnStatus = []int{99} }, true},
		{"invalid rotate status code high", func(c *Config) { c.Connection.ProxyRotateOnStatus = []int{600} }, true},
		{"valid proxy pool", func(c *Config) { c.Connection.ProxyPool = []string{"http://proxy:8080", "socks5://proxy2:1080"} }, false},
		{"valid rotate status code", func(c *Config) { c.Connection.ProxyRotateOnStatus = []int{403, 429} }, false},
		{"zero proxy failure threshold", func(c *Config) { c.Connection.ProxyFailureThreshold = 0 }, false},
		{"zero proxy cooldown", func(c *Config) { c.Connection.ProxyCooldown = 0 }, false},
		{"TLS min version below TLS 1.0", func(c *Config) { c.Security.MinTLSVersion = tls.VersionTLS10 - 1 }, true},
		{"TLS min version above TLS 1.3", func(c *Config) { c.Security.MinTLSVersion = tls.VersionTLS13 + 1 }, true},
		{"TLS max version above TLS 1.3", func(c *Config) { c.Security.MaxTLSVersion = 0xFFFF }, true},
		{"TLS min version zero (unset)", func(c *Config) { c.Security.MinTLSVersion = 0 }, false},
		{"valid TLS fingerprint chrome", func(c *Config) { c.Security.TLSFingerprint = "chrome" }, false},
		{"valid TLS fingerprint preset name", func(c *Config) { c.Security.TLSFingerprint = "chrome-windows" }, false},
		{"valid TLS fingerprint mobile variant", func(c *Config) { c.Security.TLSFingerprint = "chrome-ios" }, false},
		{"valid TLS fingerprint mixed case", func(c *Config) { c.Security.TLSFingerprint = "  FireFox " }, false},
		{"invalid TLS fingerprint", func(c *Config) { c.Security.TLSFingerprint = "opera" }, true},
		{"invalid TLS fingerprint safari (no profile)", func(c *Config) { c.Security.TLSFingerprint = "safari" }, true},
		{"invalid TLS fingerprint unknown version", func(c *Config) { c.Security.TLSFingerprint = "chrome-999" }, true},
		{"invalid TLS fingerprint stale version pin", func(c *Config) { c.Security.TLSFingerprint = "chrome-155" }, true},
		{"TLS fingerprint with custom TLSConfig rejected", func(c *Config) {
			c.Security.TLSFingerprint = "chrome"
			c.Security.TLSConfig = &tls.Config{}
		}, true},
		{"TLS fingerprint with certificate pinner accepted", func(c *Config) {
			c.Security.TLSFingerprint = "chrome"
			c.Security.CertificatePinner = pinner
		}, false},
		{"empty TLS fingerprint (disabled)", func(c *Config) { c.Security.TLSFingerprint = "" }, false},
		{"TLS versions valid explicit range", func(c *Config) {
			c.Security.MinTLSVersion = tls.VersionTLS11
			c.Security.MaxTLSVersion = tls.VersionTLS12
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nil config" {
				if err := ValidateConfig(nil); err == nil {
					t.Error("expected error for nil config")
				}
				return
			}
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			if err := ValidateConfig(&cfg); (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// (*Config).Validate — method form of validation
// ----------------------------------------------------------------------------

// TestConfigValidate pins the method-vs-function equivalence: (*Config).Validate
// is a pure delegate of ValidateConfig, so the two must return identical errors
// for the same invalid config. (The nil-receiver and DefaultConfig-valid rows
// formerly here are covered by TestValidateConfig_AdditionalBoundaries/"nil
// config" and by every New(DefaultConfig()) construction respectively.)
func TestConfigValidate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeouts.Request = -1 * time.Second
	methodErr := cfg.Validate()
	funcErr := ValidateConfig(&cfg)
	if methodErr == nil || funcErr == nil {
		t.Fatalf("expected errors, got Validate()=%v, ValidateConfig()=%v", methodErr, funcErr)
	}
	if methodErr.Error() != funcErr.Error() {
		t.Errorf("Validate() and ValidateConfig() disagree: %q vs %q", methodErr, funcErr)
	}
}

// The "Boundary condition tests for config_convert helpers" section moved to
// config_convert_test.go, alongside the other config_convert.go coverage.

func TestIsTestEnvironment_BoundaryConditions(t *testing.T) {
	t.Parallel()

	// Exercise the pure detection logic directly. These tests MUST NOT mutate
	// os.Args / environment variables: they are process-global and other
	// parallel tests read them via New() -> isTestEnvironment(), so writing
	// them here would data-race under -race.
	cases := []struct {
		name       string
		executable string
		goTest     string
		gotest     string
		want       bool
	}{
		{name: "GO_TEST env var", executable: "/usr/bin/myapp", goTest: "1", gotest: "", want: true},
		{name: "GOTEST env var", executable: "/usr/bin/myapp", goTest: "", gotest: "1", want: true},
		{name: "non-test binary returns false", executable: "/usr/bin/myapp", goTest: "", gotest: "", want: false},
		{name: "test infix pattern", executable: "/tmp/my.test.custom", goTest: "", gotest: "", want: true},
		{name: "dot-test suffix", executable: "runner.test", goTest: "", gotest: "", want: true},
		{name: "dot-test-exe suffix", executable: "runner.test.exe", goTest: "", gotest: "", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isTestEnvironmentFrom(tc.executable, tc.goTest, tc.gotest); got != tc.want {
				t.Errorf("isTestEnvironmentFrom(%q, %q, %q) = %v, want %v",
					tc.executable, tc.goTest, tc.gotest, got, tc.want)
			}
		})
	}
}

func TestWarnTestingConfigInProduction(t *testing.T) {
	origArgs := os.Args[0]
	origGoTest := os.Getenv("GO_TEST")
	origGotest := os.Getenv("GOTEST")
	defer func() {
		os.Args[0] = origArgs
		_ = os.Setenv("GO_TEST", origGoTest)
		_ = os.Setenv("GOTEST", origGotest)
		// Restore warning state
		testingConfigWarnOnce = sync.Once{}
		insecureSkipVerifyWarnOnce = sync.Once{}
		securityWarnOutput = os.Stderr
	}()

	// Simulate non-test environment
	os.Args[0] = "/usr/bin/myapp"
	_ = os.Setenv("GO_TEST", "")
	_ = os.Setenv("GOTEST", "")

	// Reset once so the warning fires in this test
	testingConfigWarnOnce = sync.Once{}

	// Capture warning output via SetSecurityWarnOutput
	var buf bytes.Buffer
	SetSecurityWarnOutput(&buf)

	warnTestingConfigInProduction()

	output := buf.String()

	if !strings.Contains(output, "SECURITY WARNING") {
		t.Error("expected security warning in stderr")
	}
	if !strings.Contains(output, "TLS") {
		t.Error("expected TLS warning")
	}
	if !strings.Contains(output, "SSRF") {
		t.Error("expected SSRF warning")
	}
}
