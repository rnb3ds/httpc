package httpc

import (
	"crypto/tls"
	"testing"
	"time"
)

// newTestClient creates a new client for testing with safe defaults
func newTestClient() (Client, error) {
	config := &Config{
		Timeout:             30 * time.Second,
		MaxIdleConns:        10,
		MaxConnsPerHost:     5,
		InsecureSkipVerify:  true,             // For testing only
		MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
		AllowPrivateIPs:     true,             // For testing localhost
		MaxRetries:          2,
		RetryDelay:          100 * time.Millisecond,
		BackoffFactor:       2.0,
		UserAgent:           "httpc-test/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         false, // Disable HTTP/2 for simpler testing
		EnableCookies:       true,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true, // For testing only
		},
	}

	return New(config)
}

// newTestClientWithConfig creates a test client with custom config
func newTestClientWithConfig(config *Config) (Client, error) {
	if config == nil {
		return newTestClient()
	}

	// Ensure test-safe defaults
	if config.TLSConfig == nil {
		config.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	if !config.AllowPrivateIPs {
		config.AllowPrivateIPs = true // Enable for testing
	}

	return New(config)
}

// newTestClientWithTimeout creates a test client with specific timeout
func newTestClientWithTimeout(timeout time.Duration) (Client, error) {
	config := &Config{
		Timeout:             timeout,
		MaxIdleConns:        10,
		MaxConnsPerHost:     5,
		InsecureSkipVerify:  true,
		MaxResponseBodySize: 10 * 1024 * 1024,
		AllowPrivateIPs:     true,
		MaxRetries:          1, // Reduce for faster tests
		RetryDelay:          10 * time.Millisecond,
		BackoffFactor:       2.0,
		UserAgent:           "httpc-test/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         false,
		EnableCookies:       true,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	return New(config)
}

// cleanupClient ensures proper client cleanup in tests
func cleanupClient(client Client) {
	if client != nil {
		if err := client.Close(); err != nil {
			// Log error but don't fail test
		}
	}
}

// mustCreateClient creates a client and fails the test if creation fails
func mustCreateClient(t *testing.T, config *Config) Client {
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	return client
}
