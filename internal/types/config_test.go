package types

import (
	"crypto/tls"
	"testing"
	"time"
)

// ============================================================================
// TimeoutConfig Tests
// ============================================================================

func TestDefaultTimeoutConfig(t *testing.T) {
	config := DefaultTimeoutConfig()

	if config.Request <= 0 {
		t.Error("Default Request timeout should be positive")
	}
	if config.Dial <= 0 {
		t.Error("Default Dial timeout should be positive")
	}
	if config.TLSHandshake <= 0 {
		t.Error("Default TLSHandshake timeout should be positive")
	}
	if config.ResponseHeader <= 0 {
		t.Error("Default ResponseHeader timeout should be positive")
	}
	if config.IdleConn <= 0 {
		t.Error("Default IdleConn timeout should be positive")
	}
}

func TestTimeoutConfig_Values(t *testing.T) {
	config := DefaultTimeoutConfig()

	// Verify expected default values
	if config.Request != 30*time.Second {
		t.Errorf("Request timeout = %v, want 30s", config.Request)
	}
	if config.Dial != 10*time.Second {
		t.Errorf("Dial timeout = %v, want 10s", config.Dial)
	}
	if config.TLSHandshake != 10*time.Second {
		t.Errorf("TLSHandshake timeout = %v, want 10s", config.TLSHandshake)
	}
	if config.ResponseHeader != 30*time.Second {
		t.Errorf("ResponseHeader timeout = %v, want 30s", config.ResponseHeader)
	}
	if config.IdleConn != 90*time.Second {
		t.Errorf("IdleConn timeout = %v, want 90s", config.IdleConn)
	}
}

// ============================================================================
// ConnectionConfig Tests
// ============================================================================

func TestDefaultConnectionConfig(t *testing.T) {
	config := DefaultConnectionConfig()

	if config.MaxIdleConns <= 0 {
		t.Error("MaxIdleConns should be positive")
	}
	if config.MaxConnsPerHost <= 0 {
		t.Error("MaxConnsPerHost should be positive")
	}
	if config.ProxyURL != "" {
		t.Error("Default ProxyURL should be empty")
	}
	if config.EnableSystemProxy {
		t.Error("Default EnableSystemProxy should be false")
	}
	if !config.EnableHTTP2 {
		t.Error("Default EnableHTTP2 should be true")
	}
	if config.EnableCookies {
		t.Error("Default EnableCookies should be false")
	}
	if config.EnableDoH {
		t.Error("Default EnableDoH should be false")
	}
}

func TestConnectionConfig_Values(t *testing.T) {
	config := DefaultConnectionConfig()

	if config.MaxIdleConns != 50 {
		t.Errorf("MaxIdleConns = %d, want 50", config.MaxIdleConns)
	}
	if config.MaxConnsPerHost != 10 {
		t.Errorf("MaxConnsPerHost = %d, want 10", config.MaxConnsPerHost)
	}
	if config.DoHCacheTTL != 5*time.Minute {
		t.Errorf("DoHCacheTTL = %v, want 5m", config.DoHCacheTTL)
	}
}

// ============================================================================
// SecurityConfig Tests
// ============================================================================

func TestDefaultSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()

	if config.TLSConfig != nil {
		t.Error("Default TLSConfig should be nil")
	}
	if config.MinTLSVersion < tls.VersionTLS12 {
		t.Error("MinTLSVersion should be at least TLS 1.2")
	}
	if config.MaxTLSVersion < tls.VersionTLS12 {
		t.Error("MaxTLSVersion should be at least TLS 1.2")
	}
	if config.InsecureSkipVerify {
		t.Error("Default InsecureSkipVerify should be false")
	}
	if config.MaxResponseBodySize <= 0 {
		t.Error("MaxResponseBodySize should be positive")
	}
	if config.AllowPrivateIPs {
		t.Error("Default AllowPrivateIPs should be false (SSRF protection)")
	}
	if !config.ValidateURL {
		t.Error("Default ValidateURL should be true")
	}
	if !config.ValidateHeaders {
		t.Error("Default ValidateHeaders should be true")
	}
	if !config.StrictContentLength {
		t.Error("Default StrictContentLength should be true")
	}
}

func TestSecurityConfig_TLSVersions(t *testing.T) {
	config := DefaultSecurityConfig()

	if config.MinTLSVersion != tls.VersionTLS12 {
		t.Errorf("MinTLSVersion = %x, want %x", config.MinTLSVersion, tls.VersionTLS12)
	}
	if config.MaxTLSVersion != tls.VersionTLS13 {
		t.Errorf("MaxTLSVersion = %x, want %x", config.MaxTLSVersion, tls.VersionTLS13)
	}
}

func TestSecurityConfig_MaxResponseBodySize(t *testing.T) {
	config := DefaultSecurityConfig()

	expectedSize := int64(10 * 1024 * 1024) // 10MB
	if config.MaxResponseBodySize != expectedSize {
		t.Errorf("MaxResponseBodySize = %d, want %d", config.MaxResponseBodySize, expectedSize)
	}
}

// ============================================================================
// RetryConfig Tests
// ============================================================================

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries < 0 {
		t.Error("MaxRetries should be non-negative")
	}
	if config.Delay <= 0 {
		t.Error("Delay should be positive")
	}
	if config.BackoffFactor < 1.0 {
		t.Error("BackoffFactor should be at least 1.0")
	}
	if config.CustomRetryPolicy != nil {
		t.Error("Default CustomRetryPolicy should be nil")
	}
}

func TestRetryConfig_Values(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", config.MaxRetries)
	}
	if config.Delay != 1*time.Second {
		t.Errorf("Delay = %v, want 1s", config.Delay)
	}
	if config.BackoffFactor != 2.0 {
		t.Errorf("BackoffFactor = %f, want 2.0", config.BackoffFactor)
	}
	if !config.EnableJitter {
		t.Error("EnableJitter should be true by default")
	}
}

// ============================================================================
// MiddlewareConfig Tests
// ============================================================================

func TestDefaultMiddlewareConfig(t *testing.T) {
	config := DefaultMiddlewareConfig()

	if config.UserAgent == "" {
		t.Error("UserAgent should not be empty")
	}
	if config.Headers == nil {
		t.Error("Headers should be initialized")
	}
	if len(config.Headers) != 0 {
		t.Error("Default Headers should be empty")
	}
	if !config.FollowRedirects {
		t.Error("Default FollowRedirects should be true")
	}
	if config.MaxRedirects <= 0 {
		t.Error("MaxRedirects should be positive")
	}
	if len(config.Middlewares) != 0 {
		t.Error("Default Middlewares should be empty")
	}
}

func TestMiddlewareConfig_Values(t *testing.T) {
	config := DefaultMiddlewareConfig()

	if config.UserAgent != "httpc/1.0" {
		t.Errorf("UserAgent = %s, want 'httpc/1.0'", config.UserAgent)
	}
	if config.MaxRedirects != 10 {
		t.Errorf("MaxRedirects = %d, want 10", config.MaxRedirects)
	}
}

// ============================================================================
// Config Customization Tests
// ============================================================================

func TestTimeoutConfig_Customization(t *testing.T) {
	config := DefaultTimeoutConfig()
	originalRequest := config.Request

	// Modify config
	config.Request = 60 * time.Second

	// Verify modification works
	if config.Request == originalRequest {
		t.Error("Config modification should change value")
	}
	if config.Request != 60*time.Second {
		t.Errorf("Request = %v, want 60s", config.Request)
	}
}

func TestSecurityConfig_Customization(t *testing.T) {
	config := DefaultSecurityConfig()

	// Test allowing private IPs for development
	config.AllowPrivateIPs = true
	config.InsecureSkipVerify = true

	if !config.AllowPrivateIPs {
		t.Error("AllowPrivateIPs should be true after setting")
	}
	if !config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true after setting")
	}
}

func TestConnectionConfig_Customization(t *testing.T) {
	config := DefaultConnectionConfig()

	// Test proxy configuration
	config.ProxyURL = "http://proxy.example.com:8080"
	config.EnableSystemProxy = true
	config.EnableDoH = true

	if config.ProxyURL != "http://proxy.example.com:8080" {
		t.Errorf("ProxyURL = %s, want 'http://proxy.example.com:8080'", config.ProxyURL)
	}
	if !config.EnableSystemProxy {
		t.Error("EnableSystemProxy should be true")
	}
	if !config.EnableDoH {
		t.Error("EnableDoH should be true")
	}
}

func TestRetryConfig_Customization(t *testing.T) {
	config := DefaultRetryConfig()

	// Disable retries
	config.MaxRetries = 0
	config.EnableJitter = false

	if config.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0", config.MaxRetries)
	}
	if config.EnableJitter {
		t.Error("EnableJitter should be false")
	}
}

// ============================================================================
// Edge Cases Tests
// ============================================================================

func TestTimeoutConfig_ZeroValues(t *testing.T) {
	config := TimeoutConfig{}

	// Zero values should be valid (will use defaults elsewhere)
	if config.Request != 0 {
		t.Error("Empty config should have zero Request timeout")
	}
}

func TestConnectionConfig_ZeroMaxIdleConns(t *testing.T) {
	config := ConnectionConfig{}

	// Zero value is valid but not recommended
	if config.MaxIdleConns != 0 {
		t.Error("Empty config should have zero MaxIdleConns")
	}
}

func TestSecurityConfig_NilTLSConfig(t *testing.T) {
	config := SecurityConfig{}

	if config.TLSConfig != nil {
		t.Error("Empty config should have nil TLSConfig")
	}
}
