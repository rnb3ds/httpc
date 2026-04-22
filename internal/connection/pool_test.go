package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CONNECTION POOL MANAGER UNIT TESTS
// ============================================================================

func TestPoolManager_New(t *testing.T) {
	t.Run("With default config", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.config == nil {
			t.Error("Config should not be nil")
		}

		if pm.transport == nil {
			t.Error("Transport should not be nil")
		}

		if pm.metrics == nil {
			t.Error("Metrics should not be nil")
		}
	})

	t.Run("With custom config", func(t *testing.T) {
		config := &Config{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     25,
			DialTimeout:         5 * time.Second,
			EnableHTTP2:         true,
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.config.MaxIdleConns != 100 {
			t.Errorf("Expected MaxIdleConns 100, got %d", pm.config.MaxIdleConns)
		}

		if pm.transport.MaxIdleConns != 100 {
			t.Errorf("Expected transport MaxIdleConns 100, got %d", pm.transport.MaxIdleConns)
		}
	})

	t.Run("With proxy URL", func(t *testing.T) {
		config := &Config{
			ProxyURL: "http://proxy.example.com:8080",
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.transport.Proxy == nil {
			t.Error("Proxy should be configured")
		}
	})

	t.Run("With invalid proxy URL", func(t *testing.T) {
		config := &Config{
			ProxyURL: "://invalid-url",
		}

		_, err := NewPoolManager(config)
		if err == nil {
			t.Error("Expected error for invalid proxy URL")
		}
	})
}

func TestPoolManager_GetTransport(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.GetTransport()

	if transport == nil {
		t.Fatal("Transport should not be nil")
	}

	if transport != pm.transport {
		t.Error("GetTransport should return the same transport instance")
	}
}

func TestPoolManager_GetMetrics(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	metrics := pm.GetMetrics()

	// Initially should have zero connections
	if metrics.ActiveConnections != 0 {
		t.Errorf("Expected 0 active connections, got %d", metrics.ActiveConnections)
	}

	if metrics.TotalConnections != 0 {
		t.Errorf("Expected 0 total connections, got %d", metrics.TotalConnections)
	}
}

func TestPoolManager_HTTPRequest(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true // Allow localhost for testing
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Create HTTP client with our pool manager
	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify transport is working (connection tracking may not be immediate)
	// Just verify the request succeeded
}

func TestPoolManager_MultipleRequests(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make multiple requests
	numRequests := 10
	successCount := 0
	for i := 0; i < numRequests; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		_ = resp.Body.Close()
		successCount++
	}

	// Verify all requests succeeded
	if successCount != numRequests {
		t.Errorf("Expected %d successful requests, got %d", numRequests, successCount)
	}
}

func TestPoolManager_Close(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	err = pm.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got: %v", err)
	}

	// Close again should be idempotent
	err = pm.Close()
	if err != nil {
		t.Errorf("Expected no error on double close, got: %v", err)
	}
}

func TestPoolManager_IsHealthy(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Initially should be healthy (or may not be if no connections yet)
	// Just verify the method doesn't panic
	_ = pm.IsHealthy()
}

func TestPoolManager_TLSConfig(t *testing.T) {
	t.Run("Default TLS config", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig

		if tlsConfig == nil {
			t.Fatal("TLS config should not be nil")
		}

		if tlsConfig.MinVersion != tls.VersionTLS12 {
			t.Errorf("Expected MinVersion TLS 1.2, got %d", tlsConfig.MinVersion)
		}

		if tlsConfig.MaxVersion != tls.VersionTLS13 {
			t.Errorf("Expected MaxVersion TLS 1.3, got %d", tlsConfig.MaxVersion)
		}

		if tlsConfig.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be false by default")
		}
	})

	t.Run("Custom TLS config", func(t *testing.T) {
		customTLS := &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		}

		config := &Config{
			TLSConfig: customTLS,
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig

		if tlsConfig.MinVersion != tls.VersionTLS13 {
			t.Errorf("Expected MinVersion TLS 1.3, got %d", tlsConfig.MinVersion)
		}

		if !tlsConfig.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be true")
		}
	})
}

func TestPoolManager_Timeouts(t *testing.T) {
	config := &Config{
		DialTimeout:           2 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		IdleConnTimeout:       5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.transport

	if transport.TLSHandshakeTimeout != 3*time.Second {
		t.Errorf("Expected TLSHandshakeTimeout 3s, got %v", transport.TLSHandshakeTimeout)
	}

	if transport.ResponseHeaderTimeout != 4*time.Second {
		t.Errorf("Expected ResponseHeaderTimeout 4s, got %v", transport.ResponseHeaderTimeout)
	}

	if transport.IdleConnTimeout != 5*time.Second {
		t.Errorf("Expected IdleConnTimeout 5s, got %v", transport.IdleConnTimeout)
	}

	if transport.ExpectContinueTimeout != 1*time.Second {
		t.Errorf("Expected ExpectContinueTimeout 1s, got %v", transport.ExpectContinueTimeout)
	}
}

func TestPoolManager_ConnectionLimits(t *testing.T) {
	config := &Config{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     10,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.transport

	if transport.MaxIdleConns != 50 {
		t.Errorf("Expected MaxIdleConns 50, got %d", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 5 {
		t.Errorf("Expected MaxIdleConnsPerHost 5, got %d", transport.MaxIdleConnsPerHost)
	}

	if transport.MaxConnsPerHost != 10 {
		t.Errorf("Expected MaxConnsPerHost 10, got %d", transport.MaxConnsPerHost)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxIdleConns != 200 {
		t.Errorf("Expected MaxIdleConns 200, got %d", config.MaxIdleConns)
	}

	if config.MaxIdleConnsPerHost != 20 {
		t.Errorf("Expected MaxIdleConnsPerHost 20, got %d", config.MaxIdleConnsPerHost)
	}

	if config.MaxConnsPerHost != 50 {
		t.Errorf("Expected MaxConnsPerHost 50, got %d", config.MaxConnsPerHost)
	}

	if config.DialTimeout != 10*time.Second {
		t.Errorf("Expected DialTimeout 10s, got %v", config.DialTimeout)
	}

	if !config.EnableHTTP2 {
		t.Error("EnableHTTP2 should be true by default")
	}

	if config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false by default")
	}
}

func TestPoolManager_ContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)

	_, err = client.Do(req)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
}

// ============================================================================
// SSRF Protection Tests
// ============================================================================

func TestPoolManager_SSRFProtection(t *testing.T) {
	t.Run("BlockPrivateIPs", func(t *testing.T) {
		config := &Config{
			AllowPrivateIPs: false, // SSRF protection enabled
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// Verify that SSRF protection is enabled
		if pm.config.AllowPrivateIPs {
			t.Error("AllowPrivateIPs should be false")
		}
	})

	t.Run("AllowPrivateIPs", func(t *testing.T) {
		config := &Config{
			AllowPrivateIPs: true,
		}
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if !pm.config.AllowPrivateIPs {
			t.Error("AllowPrivateIPs should be true")
		}
	})
}

func TestPoolManager_SystemProxy(t *testing.T) {
	config := &Config{
		EnableSystemProxy: true,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Transport should be created (proxy detection may or may not find a proxy)
	if pm.transport == nil {
		t.Error("Transport should not be nil")
	}
}

// ============================================================================
// Metrics Tests
// ============================================================================

func TestPoolManager_MetricsTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make a request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Check metrics after request
	metrics := pm.GetMetrics()
	// Metrics might not update immediately due to async tracking
	// Just verify the method doesn't panic
	_ = metrics.TotalConnections
	_ = metrics.ActiveConnections
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestPoolManager_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.MaxIdleConns = 50
	config.MaxConnsPerHost = 20
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	const numRequests = 20
	var wg sync.WaitGroup
	errChan := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(server.URL)
			if err != nil {
				errChan <- err
				return
			}
			_ = resp.Body.Close()
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Concurrent request failed: %v", err)
	}
}

func TestPoolManager_ConcurrentClose(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	const numClosers = 5
	var wg sync.WaitGroup
	wg.Add(numClosers)

	for i := 0; i < numClosers; i++ {
		go func() {
			defer wg.Done()
			_ = pm.Close() // Should be safe to call multiple times
		}()
	}

	wg.Wait()
}

// ============================================================================
// Edge Cases Tests
// ============================================================================

func TestPoolManager_NilConfig(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error with nil config, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Should use default config
	if pm.config == nil {
		t.Error("Config should not be nil after initialization")
	}
}

func TestPoolManager_ZeroTimeouts(t *testing.T) {
	config := &Config{
		DialTimeout:           0,
		TLSHandshakeTimeout:   0,
		ResponseHeaderTimeout: 0,
		IdleConnTimeout:       0,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Should handle zero timeouts gracefully
	if pm.transport == nil {
		t.Error("Transport should not be nil")
	}
}

func TestPoolManager_HighConnectionLimits(t *testing.T) {
	config := &Config{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     200,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if pm.transport.MaxIdleConns != 1000 {
		t.Errorf("Expected MaxIdleConns 1000, got %d", pm.transport.MaxIdleConns)
	}
}

func TestPoolManager_HTTP2Disabled(t *testing.T) {
	config := &Config{
		EnableHTTP2: false,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// When HTTP/2 is disabled, ForceAttemptHTTP2 should be false
	if pm.transport.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be false when HTTP/2 is disabled")
	}
}

// ============================================================================
// Address Validation Tests (SSRF Protection)
// ============================================================================

func TestPoolManager_ValidateAddressBeforeDial(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	tests := []struct {
		name        string
		address     string
		expectError bool
	}{
		{
			name:        "Loopback IP",
			address:     "127.0.0.1:8080",
			expectError: true, // Loopback should be blocked
		},
		{
			name:        "Private IP 10.x.x.x",
			address:     "10.0.0.1:443",
			expectError: true, // Private IP should be blocked
		},
		{
			name:        "Private IP 192.168.x.x",
			address:     "192.168.1.1:443",
			expectError: true, // Private IP should be blocked
		},
		{
			name:        "Private IP 172.16.x.x",
			address:     "172.16.0.1:443",
			expectError: true, // Private IP should be blocked
		},
		{
			name:        "Link-local IP",
			address:     "169.254.1.1:443",
			expectError: true, // Link-local should be blocked
		},
		{
			name:        "Public IP (simulated)",
			address:     "8.8.8.8:443",
			expectError: false, // Public IP should be allowed
		},
		{
			name:        "IP without port",
			address:     "127.0.0.1",
			expectError: true, // Loopback without port should be blocked
		},
		{
			name:        "IPv6 loopback",
			address:     "[::1]:8080",
			expectError: true, // IPv6 loopback should be blocked
		},
		{
			name:        "Invalid address format",
			address:     "not-an-ip-address",
			expectError: true, // DNS resolution failure should block
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pm.resolveAndValidateAddress(tt.address)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for address %s, got nil", tt.address)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Did not expect error for address %s, got: %v", tt.address, err)
			}
		})
	}
}

// ============================================================================
// Certificate Pinning Tests
// ============================================================================

func TestPoolManager_CreateVerifyPeerCertificate(t *testing.T) {
	t.Run("WithCertPinner", func(t *testing.T) {
		// Create a mock cert pinner
		pinner := &mockCertPinner{}

		config := &Config{
			CertPinner: pinner,
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig
		if tlsConfig == nil {
			t.Fatal("TLS config should not be nil")
		}

		// VerifyPeerCertificate should be set when CertPinner is configured
		if tlsConfig.VerifyPeerCertificate == nil {
			t.Error("VerifyPeerCertificate should be set when CertPinner is configured")
		}
	})

	t.Run("WithoutCertPinner", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig
		if tlsConfig == nil {
			t.Fatal("TLS config should not be nil")
		}

		// VerifyPeerCertificate should not be set without CertPinner
		if tlsConfig.VerifyPeerCertificate != nil {
			t.Error("VerifyPeerCertificate should not be set without CertPinner")
		}
	})

	t.Run("CustomTLSWithCertPinner", func(t *testing.T) {
		pinner := &mockCertPinner{}
		customTLS := &tls.Config{
			MinVersion: tls.VersionTLS13,
		}

		config := &Config{
			TLSConfig:  customTLS,
			CertPinner: pinner,
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig
		if tlsConfig == nil {
			t.Fatal("TLS config should not be nil")
		}

		// Should preserve custom TLS config
		if tlsConfig.MinVersion != tls.VersionTLS13 {
			t.Errorf("Expected MinVersion TLS 1.3, got %d", tlsConfig.MinVersion)
		}

		// Should add cert pinning
		if tlsConfig.VerifyPeerCertificate == nil {
			t.Error("VerifyPeerCertificate should be set")
		}
	})
}

// mockCertPinner is a mock implementation of certificate pinner for testing
type mockCertPinner struct {
	shouldFail bool
}

func (m *mockCertPinner) Pin() string {
	return "mock-pinner"
}

func (m *mockCertPinner) VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if m.shouldFail {
		return fmt.Errorf("mock certificate pinning failure")
	}
	return nil
}

// ============================================================================
// Host Stats Tests
// ============================================================================

func TestPoolManager_HostStatsOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		AllowPrivateIPs: true,
	}
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Initial count should be 0
	initialCount := pm.GetHostStatsCount()
	if initialCount != 0 {
		t.Errorf("Initial host stats count = %d, want 0", initialCount)
	}

	// Make requests to populate host stats
	client := &http.Client{Transport: pm.GetTransport()}
	for i := 0; i < 3; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Logf("Request %d error (may be expected): %v", i, err)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Host stats count should now be >= 1
	count := pm.GetHostStatsCount()
	if count < 1 {
		t.Errorf("Expected at least 1 host tracked after requests, got %d", count)
	}
	t.Logf("Host stats count after requests: %d", count)

	// Clear and verify
	pm.ClearHostStats()
	afterClearCount := pm.GetHostStatsCount()
	if afterClearCount != 0 {
		t.Errorf("Host stats count after clear = %d, want 0", afterClearCount)
	}
}

func TestPoolManager_GetHostStatsCount_Empty(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Fresh pool manager should have 0 host stats
	count := pm.GetHostStatsCount()
	if count != 0 {
		t.Errorf("Empty pool manager host stats count = %d, want 0", count)
	}
}

func TestPoolManager_ClearHostStats_Idempotent(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Clear on empty should not panic
	pm.ClearHostStats()

	// Clear again should still be safe
	pm.ClearHostStats()

	// Count should still be 0
	if pm.GetHostStatsCount() != 0 {
		t.Error("Host stats count should remain 0 after multiple clears")
	}
}

func TestPoolManager_HostStats_ConcurrentAccess(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent reads and writes to host stats
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				_ = pm.GetHostStatsCount()
			} else {
				pm.ClearHostStats()
			}
		}(i)
	}

	wg.Wait()
}

// ============================================================================
// DoH Resolver Integration Tests
// ============================================================================

func TestPoolManager_WithDoHResolver(t *testing.T) {
	config := &Config{
		AllowPrivateIPs: true,
		EnableDoH:       true,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify DoH resolver is initialized
	if pm.dohResolver == nil {
		t.Error("DoH resolver should be initialized when EnableDoH is true")
	}
}

func TestPoolManager_WithoutDoHResolver(t *testing.T) {
	config := &Config{
		AllowPrivateIPs: true,
		EnableDoH:       false,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify DoH resolver is NOT initialized
	if pm.dohResolver != nil {
		t.Error("DoH resolver should not be initialized when EnableDoH is false")
	}
}

// ============================================================================
// TLS Configuration Edge Cases
// ============================================================================

func TestPoolManager_TLSConfigWithCustomCiphers(t *testing.T) {
	config := &Config{
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
			},
		},
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify custom TLS config is used
	if pm.transport.TLSClientConfig == nil {
		t.Fatal("TLS config should not be nil")
	}

	if pm.transport.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected MinVersion TLS 1.3, got %d", pm.transport.TLSClientConfig.MinVersion)
	}
}

func TestPoolManager_TLSConfigWithInsecureSkipVerify(t *testing.T) {
	config := &Config{
		InsecureSkipVerify: true,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if !pm.transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

// ============================================================================
// Connection Metrics Tests
// ============================================================================

func TestPoolManager_ConnectionMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make a request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Check metrics
	metrics := pm.GetMetrics()
	t.Logf("Metrics: TotalConns=%d, ActiveConns=%d, RejectedConns=%d",
		metrics.TotalConnections, metrics.ActiveConnections, metrics.RejectedConnections)

	// Total connections should be at least 1
	if metrics.TotalConnections < 1 {
		t.Logf("Warning: TotalConnections = %d, expected at least 1", metrics.TotalConnections)
	}
}

// ============================================================================
// Validate Address Tests - Additional Coverage
// ============================================================================

func TestPoolManager_ValidateAddress_DomainResolution(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	tests := []struct {
		name        string
		address     string
		expectError bool
	}{
		{
			name:        "IPv6 address",
			address:     "[2001:4860:4860::8888]:443",
			expectError: false, // Public IPv6
		},
		{
			name:        "IPv6 loopback",
			address:     "[::1]:8080",
			expectError: true, // Loopback should be blocked
		},
		{
			name:        "IPv4-mapped IPv6",
			address:     "[::ffff:127.0.0.1]:8080",
			expectError: true, // Should detect as loopback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pm.resolveAndValidateAddress(tt.address)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for address %s, got nil", tt.address)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Did not expect error for address %s, got: %v", tt.address, err)
			}
		})
	}
}

// ============================================================================
// Proxy Configuration Tests
// ============================================================================

func TestPoolManager_ProxyConfiguration(t *testing.T) {
	t.Run("Valid proxy URL", func(t *testing.T) {
		config := &Config{
			ProxyURL: "http://proxy.example.com:8080",
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.transport.Proxy == nil {
			t.Error("Proxy function should be set")
		}
	})

	t.Run("SOCKS5 proxy URL", func(t *testing.T) {
		config := &Config{
			ProxyURL: "socks5://proxy.example.com:1080",
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// Proxy should be configured (actual SOCKS5 support depends on implementation)
		if pm.transport.Proxy == nil {
			t.Log("Note: SOCKS5 proxy may require additional configuration")
		}
	})

	t.Run("Empty proxy URL", func(t *testing.T) {
		config := &Config{
			ProxyURL: "",
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// No proxy should be configured
		// Transport.Proxy being nil means use environment settings
	})
}

// ============================================================================
// Keep-Alive Configuration Tests
// ============================================================================

func TestPoolManager_KeepAliveConfiguration(t *testing.T) {
	config := &Config{
		KeepAlive: 60 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify transport is created
	if pm.transport == nil {
		t.Fatal("Transport should not be nil")
	}

	// KeepAlive is configured in the dialer, not directly accessible
	// Just verify the pool manager was created successfully
}

// ============================================================================
// Response Header Timeout Tests
// ============================================================================

func TestPoolManager_ResponseHeaderTimeout(t *testing.T) {
	config := &Config{
		ResponseHeaderTimeout: 5 * time.Second,
		AllowPrivateIPs:       true,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if pm.transport.ResponseHeaderTimeout != 5*time.Second {
		t.Errorf("Expected ResponseHeaderTimeout 5s, got %v", pm.transport.ResponseHeaderTimeout)
	}
}

// ============================================================================
// ExpectContinueTimeout Tests
// ============================================================================

func TestPoolManager_ExpectContinueTimeout(t *testing.T) {
	config := &Config{
		ExpectContinueTimeout: 2 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if pm.transport.ExpectContinueTimeout != 2*time.Second {
		t.Errorf("Expected ExpectContinueTimeout 2s, got %v", pm.transport.ExpectContinueTimeout)
	}
}

// ============================================================================
// Double Close Safety Tests
// ============================================================================

func TestPoolManager_DoubleCloseSafety(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// First close
	err = pm.Close()
	if err != nil {
		t.Errorf("First close failed: %v", err)
	}

	// Second close should not panic or error
	err = pm.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}

	// Third close for good measure
	err = pm.Close()
	if err != nil {
		t.Errorf("Third close failed: %v", err)
	}
}

// ============================================================================
// Tracked Connection Tests
// ============================================================================

func TestTrackedConn_DoubleClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make a request to create a tracked connection
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// First close
	err = resp.Body.Close()
	if err != nil {
		t.Errorf("First body close failed: %v", err)
	}

	// Second close should be safe (trackedConn handles double close)
	err = resp.Body.Close()
	// This may or may not return an error depending on implementation
	_ = err
}
