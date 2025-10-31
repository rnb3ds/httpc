package httpc

import (
	"crypto/tls"
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

// Removed unused test helper functions:
// - newTestClientWithConfig (not used anywhere)
// - newTestClientWithTimeout (not used anywhere)
// - cleanupClient (not used anywhere)
// - mustCreateClient (not used anywhere)
