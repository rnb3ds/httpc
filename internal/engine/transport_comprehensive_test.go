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
// TRANSPORT LAYER COMPREHENSIVE TESTS
// ============================================================================

func TestTransport_BasicFunctionality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
	}

	poolManager, err := connection.NewPoolManager(connection.DefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer poolManager.Close()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTransport_TLSConfigurationComprehensive(t *testing.T) {
	// Create HTTPS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Secure OK"))
	}))
	defer server.Close()

	tests := []struct {
		name        string
		tlsConfig   *tls.Config
		expectError bool
	}{
		{
			name: "InsecureSkipVerify true",
			tlsConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			expectError: false,
		},
		{
			name: "InsecureSkipVerify false",
			tlsConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
			expectError: true, // Because test server uses self-signed certificate
		},
		{
			name: "Custom TLS version",
			tlsConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
				MaxVersion:         tls.VersionTLS13,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Timeout:         30 * time.Second,
				AllowPrivateIPs: true,
				MaxRetries:      1,
				TLSConfig:       tt.tlsConfig,
			}

			connConfig := connection.DefaultConfig()
			connConfig.TLSConfig = tt.tlsConfig

			poolManager, err := connection.NewPoolManager(connConfig)
			if err != nil {
				t.Fatalf("Failed to create pool manager: %v", err)
			}
			defer poolManager.Close()

			transport, err := NewTransport(config, poolManager)
			if err != nil {
				t.Fatalf("Failed to create transport: %v", err)
			}
			defer transport.Close()

			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			resp, err := transport.RoundTrip(req)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
					if resp != nil {
						resp.Body.Close()
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestTransport_ConnectionPooling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
	}

	connConfig := connection.DefaultConfig()
	connConfig.MaxIdleConns = 10
	connConfig.MaxIdleConnsPerHost = 5

	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer poolManager.Close()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Send multiple requests to the same server
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("GET", server.URL, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip %d failed: %v", i, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i, resp.StatusCode)
		}
	}
}

func TestTransport_Timeouts(t *testing.T) {
	// Create a slow response server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // 2 second delay
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Slow OK"))
	}))
	defer server.Close()

	tests := []struct {
		name        string
		timeout     time.Duration
		expectError bool
	}{
		{
			name:        "Short timeout (should timeout)",
			timeout:     500 * time.Millisecond,
			expectError: true,
		},
		{
			name:        "Long timeout (should succeed)",
			timeout:     5 * time.Second,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Timeout:         tt.timeout,
				AllowPrivateIPs: true,
				MaxRetries:      0, // Disable retry to test pure timeout
			}

			connConfig := connection.DefaultConfig()

			poolManager, err := connection.NewPoolManager(connConfig)
			if err != nil {
				t.Fatalf("Failed to create pool manager: %v", err)
			}
			defer poolManager.Close()

			transport, err := NewTransport(config, poolManager)
			if err != nil {
				t.Fatalf("Failed to create transport: %v", err)
			}
			defer transport.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			start := time.Now()
			resp, err := transport.RoundTrip(req)
			duration := time.Since(start)

			if tt.expectError {
				if err == nil {
					t.Error("Expected timeout error, got nil")
					if resp != nil {
						resp.Body.Close()
					}
				}
				// Check if timeout occurred within expected time
				if duration > tt.timeout+500*time.Millisecond {
					t.Errorf("Timeout took too long: %v (expected around %v)", duration, tt.timeout)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestTransport_ErrorHandling(t *testing.T) {
	config := &Config{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      0,
	}

	connConfig := connection.DefaultConfig()

	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer poolManager.Close()

	transport, err := NewTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{
			name:        "Invalid URL",
			url:         "://invalid-url",
			expectError: true,
		},
		{
			name:        "Non-existent host",
			url:         "http://this-host-does-not-exist-12345.com",
			expectError: true,
		},
		{
			name:        "Connection refused",
			url:         "http://127.0.0.1:99999", // Unlikely to be used port
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil && !tt.expectError {
				t.Fatalf("Failed to create request: %v", err)
			}
			if err != nil && tt.expectError {
				return // Expected error
			}

			resp, err := transport.RoundTrip(req)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
					if resp != nil {
						resp.Body.Close()
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if resp != nil {
				resp.Body.Close()
			}
		})
	}
}
