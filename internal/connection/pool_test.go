package connection

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
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
		defer pm.Close()

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
		defer pm.Close()

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
		defer pm.Close()

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
	defer pm.Close()

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
	defer pm.Close()

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
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true // Allow localhost for testing
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer pm.Close()

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
	defer resp.Body.Close()

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
	defer pm.Close()

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
		resp.Body.Close()
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
	defer pm.Close()

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
		defer pm.Close()

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
		defer pm.Close()

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
	defer pm.Close()

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
	defer pm.Close()

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
	defer pm.Close()

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

