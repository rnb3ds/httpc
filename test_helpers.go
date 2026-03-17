package httpc

import (
	"crypto/tls"
	"time"
)

// testConfig returns a configuration suitable for testing with localhost servers.
// SECURITY: This configuration allows private IPs and skips TLS verification.
// DO NOT use in production.
func testConfig() *Config {
	return &Config{
		// Timeouts
		Timeout:               60 * time.Second,
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,

		// Connection
		MaxIdleConns:    200,
		MaxConnsPerHost: 200,
		EnableHTTP2:     false,
		EnableCookies:   true,

		// Security
		InsecureSkipVerify:  true,
		MaxResponseBodySize: 10 * 1024 * 1024,
		AllowPrivateIPs:     true, // Required for localhost testing
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},

		// Retry
		MaxRetries:    0,
		RetryDelay:    100 * time.Millisecond,
		BackoffFactor: 2.0,

		// Middleware
		UserAgent:       "httpc-test/1.0",
		Headers:         make(map[string]string),
		FollowRedirects: true,
	}
}

func newTestClient() (Client, error) {
	return New(testConfig())
}
