package engine

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
)

// ============================================================================
// TRANSPORT LAYER TESTS
// ============================================================================

func TestTransport_Creation(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
		AllowPrivateIPs: true, // Allow test server access
		EnableHTTP2:     true,
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	if transport == nil {
		t.Error("Transport should not be nil")
	}
}

func TestTransport_HTTPRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success"}`))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTransport_TLSConfiguration(t *testing.T) {
	// Create HTTPS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:        true,
		ValidateHeaders:    true,
		MinTLSVersion:      tls.VersionTLS12,
		MaxTLSVersion:      tls.VersionTLS13,
		InsecureSkipVerify: true, // For testing
	}

	connConfig := testConnectionConfig()
	connConfig.InsecureSkipVerify = true // For testing
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("HTTPS request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTransport_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Exceed timeout duration
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 500 * time.Millisecond, // Short timeout

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	connConfig := testConnectionConfig()
	connConfig.DialTimeout = 500 * time.Millisecond
	connConfig.ResponseHeaderTimeout = 500 * time.Millisecond
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	start := time.Now()
	_, err = transport.RoundTrip(req)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if duration > 1*time.Second {
		t.Errorf("Request took too long to timeout: %v", duration)
	}
}

func TestTransport_ConnectionReuse(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	connConfig := testConnectionConfig()
	connConfig.MaxIdleConns = 10
	connConfig.MaxIdleConnsPerHost = 5
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	// Send multiple requests to the same server
	for i := range 5 {
		req, err := http.NewRequest("GET", server.URL, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Request %d: expected status 200, got %d", i, resp.StatusCode)
		}
	}

	if requestCount != 5 {
		t.Errorf("Expected 5 requests, server received %d", requestCount)
	}
}

func TestTransport_Close(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Test close
	err = transport.Close()
	if err != nil {
		t.Errorf("Failed to close transport: %v", err)
	}

	// Test repeated close
	err = transport.Close()
	if err != nil {
		t.Errorf("Second close should not error: %v", err)
	}
}

func TestTransport_UserAgent(t *testing.T) {
	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
		UserAgent:       "TestClient/1.0",
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set User-Agent
	if config.UserAgent != "" {
		req.Header.Set("User-Agent", config.UserAgent)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if receivedUserAgent != "TestClient/1.0" {
		t.Errorf("Expected User-Agent 'TestClient/1.0', got '%s'", receivedUserAgent)
	}
}

func TestTransport_Headers(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set custom headers
	req.Header.Set("X-Custom-Header", "test-value")
	req.Header.Set("Accept", "application/json")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if receivedHeaders.Get("X-Custom-Header") != "test-value" {
		t.Errorf("Expected X-Custom-Header 'test-value', got '%s'", receivedHeaders.Get("X-Custom-Header"))
	}

	if receivedHeaders.Get("Accept") != "application/json" {
		t.Errorf("Expected Accept 'application/json', got '%s'", receivedHeaders.Get("Accept"))
	}
}
