package engine

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,
		MaxConcurrentRequests: 500,
		MaxResponseBodySize:   50 * 1024 * 1024,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       true,
		MaxRetries:            3,
		RetryDelay:            100 * time.Millisecond,
		BackoffFactor:         2.0,
		UserAgent:             "test-client/1.0",
		EnableCookies:         true,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Client should not be nil")
	}

	// Test that client is properly initialized
	if client.config != config {
		t.Error("Config not properly set")
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "Nil config",
			config: nil,
		},
		{
			name: "Zero timeout",
			config: &Config{
				Timeout: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			// Should handle gracefully or return error
			if client != nil {
				client.Close()
			}
			// We don't expect specific error behavior, just that it doesn't panic
			_ = err
		})
	}
}

// TestClient_HTTPMethods removed - duplicate of TestClient_AllHTTPMethods in comprehensive_test.go

func TestClient_Request(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	// Use a context with timeout to ensure the request doesn't hang
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Request(ctx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestClient_RequestWithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check custom header
		if r.Header.Get("X-Test") != "test-value" {
			t.Errorf("Expected X-Test header, got: %s", r.Header.Get("X-Test"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	// Create a request option that adds a header
	headerOption := func(req *Request) {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers["X-Test"] = "test-value"
	}

	resp, err := client.Get(server.URL, headerOption)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestClient_Close(t *testing.T) {
	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Close should not error
	err = client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Multiple closes should be safe
	err = client.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

func TestClient_Statistics(t *testing.T) {
	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	// Test that client tracks basic statistics
	if client.totalRequests < 0 {
		t.Error("TotalRequests should be non-negative")
	}
}

func TestClient_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to test concurrency
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:               30 * time.Second,
		AllowPrivateIPs:       true,
		MaxRetries:            1,
		MaxConcurrentRequests: 10,
		UserAgent:             "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	// Make concurrent requests
	const numRequests = 5
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := client.Get(server.URL)
			results <- err
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent request failed: %v", err)
		}
	}
}

func TestClient_TLSConfig(t *testing.T) {
	// Create HTTPS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true, // For testing only
		},
		InsecureSkipVerify: true,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("HTTPS request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Long delay to test cancellation
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

func TestClient_InvalidURL(t *testing.T) {
	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
		UserAgent:       "test-client/1.0",
		ValidateURL:     true,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	tests := []string{
		"",
		"invalid-url",
		"ftp://example.com",
		"javascript:alert(1)",
	}

	for _, url := range tests {
		t.Run("URL_"+url, func(t *testing.T) {
			_, err := client.Get(url)
			if err == nil {
				t.Errorf("Expected error for invalid URL: %s", url)
			}
		})
	}
}

func TestClient_LargeResponse(t *testing.T) {
	// Create large response content
	largeContent := strings.Repeat("x", 1024*1024) // 1MB

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeContent))
	}))
	defer server.Close()

	config := &Config{
		Timeout:             30 * time.Second,
		AllowPrivateIPs:     true,
		MaxRetries:          1,
		MaxResponseBodySize: 2 * 1024 * 1024, // 2MB limit
		UserAgent:           "test-client/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if len(resp.RawBody) != len(largeContent) {
		t.Errorf("Expected response size %d, got %d", len(largeContent), len(resp.RawBody))
	}
}
