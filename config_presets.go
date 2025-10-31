package httpc

import (
	"crypto/tls"
	"time"
)

// SecureConfig returns a configuration optimized for security
func SecureConfig() *Config {
	return &Config{
		Timeout:             15 * time.Second, // Shorter timeout for security
		MaxIdleConns:        20,               // Limited connections
		MaxConnsPerHost:     5,                // Conservative per-host limit
		InsecureSkipVerify:  false,            // Always verify TLS
		MaxResponseBodySize: 5 * 1024 * 1024,  // 5MB limit
		AllowPrivateIPs:     false,            // Block private IPs
		MaxRetries:          1,                // Minimal retries
		RetryDelay:          2 * time.Second,
		BackoffFactor:       2.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     false, // Disable redirects for security
		EnableHTTP2:         true,
		EnableCookies:       false, // Disable cookies for security
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		},
	}
}

// PerformanceConfig returns a configuration optimized for performance
func PerformanceConfig() *Config {
	return &Config{
		Timeout:             60 * time.Second,
		MaxIdleConns:        100,              // More connections for performance
		MaxConnsPerHost:     20,               // Higher per-host limit
		InsecureSkipVerify:  false,            // Still secure
		MaxResponseBodySize: 50 * 1024 * 1024, // 50MB for large responses
		AllowPrivateIPs:     false,
		MaxRetries:          3, // More retries for reliability
		RetryDelay:          500 * time.Millisecond,
		BackoffFactor:       1.5, // Gentler backoff
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         true,
		EnableCookies:       true,
	}
}

// TestingConfig returns a configuration suitable for testing
func TestingConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second,
		MaxIdleConns:        10,
		MaxConnsPerHost:     5,
		InsecureSkipVerify:  true,             // Allow for testing
		MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
		AllowPrivateIPs:     true,             // Allow localhost for testing
		MaxRetries:          1,                // Fast failures in tests
		RetryDelay:          100 * time.Millisecond,
		BackoffFactor:       2.0,
		UserAgent:           "httpc-test/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         false, // Simpler for testing
		EnableCookies:       true,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
}

// MinimalConfig returns a minimal configuration with basic settings
func MinimalConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second,
		MaxIdleConns:        10,
		MaxConnsPerHost:     2,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 1 * 1024 * 1024, // 1MB
		AllowPrivateIPs:     false,
		MaxRetries:          0, // No retries
		RetryDelay:          0,
		BackoffFactor:       1.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     false,
		EnableHTTP2:         false,
		EnableCookies:       false,
	}
}
