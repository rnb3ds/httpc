package httpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// isTestEnvironment detects if the code is running in a test environment.
// This is used to warn against using TestingConfig in production.
func isTestEnvironment() bool {
	executable := filepath.Base(os.Args[0])
	// Check for common test executable patterns
	if strings.HasSuffix(executable, ".test") ||
		strings.HasSuffix(executable, ".test.exe") ||
		strings.Contains(executable, ".test.") {
		return true
	}
	// Check for Go test environment
	if os.Getenv("GO_TEST") != "" || strings.Contains(os.Getenv("GOTEST"), "1") {
		return true
	}
	return false
}

// warnTestingConfigInProduction logs a warning if TestingConfig is used outside of a test environment.
// This is a security measure to prevent accidental use of insecure settings in production.
func warnTestingConfigInProduction() {
	if !isTestEnvironment() {
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING] TestingConfig is being used in a non-test environment!\n")
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING] This configuration disables critical security features:\n")
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING]   - TLS certificate verification is DISABLED\n")
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING]   - SSRF protection is DISABLED\n")
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING]   - URL/Header validation is DISABLED\n")
		fmt.Fprintf(os.Stderr, "[SECURITY WARNING] Use SecureConfig() or DefaultConfig() for production!\n")
	}
}

// SecureConfig returns a configuration optimized for security-critical applications.
// This config uses stricter timeouts, disables redirects, and has SSRF protection enabled.
//
// Key security features:
//   - AllowPrivateIPs = false: Blocks connections to private/reserved IP addresses (SSRF protection)
//   - FollowRedirects = false: Prevents redirect-based SSRF attacks
//   - Stricter timeouts: Reduces window for slowloris attacks
//   - Smaller response limits: Prevents memory exhaustion
//
// Use this preset when making requests to user-provided URLs or in security-sensitive contexts.
func SecureConfig() *Config {
	cfg := DefaultConfig()

	// Timeouts - stricter for security
	cfg.Timeout = 15 * time.Second
	cfg.DialTimeout = 5 * time.Second
	cfg.TLSHandshakeTimeout = 5 * time.Second
	cfg.ResponseHeaderTimeout = 10 * time.Second
	cfg.IdleConnTimeout = 30 * time.Second

	// Connection - conservative limits
	cfg.MaxIdleConns = 20
	cfg.MaxConnsPerHost = 5

	// Security - strict settings
	cfg.AllowPrivateIPs = false               // Strict SSRF protection
	cfg.MaxResponseBodySize = 5 * 1024 * 1024 // 5MB limit
	cfg.ValidateURL = true
	cfg.ValidateHeaders = true

	// Retry - minimal retries
	cfg.MaxRetries = 1
	cfg.RetryDelay = 2 * time.Second
	cfg.EnableJitter = true

	// Middleware - no redirects for security
	cfg.FollowRedirects = false

	return cfg
}

// PerformanceConfig returns a configuration optimized for high-throughput scenarios.
// This config uses larger connection pools, longer timeouts, while maintaining
// essential security validations.
//
// SECURITY NOTE: This configuration maintains URL and header validation for security.
// If you need to disable these for maximum performance in a trusted environment,
// manually set cfg.ValidateURL = false and cfg.ValidateHeaders = false, but be
// aware of the security implications (injection attacks, SSRF).
func PerformanceConfig() *Config {
	cfg := DefaultConfig()

	// Timeouts - longer for throughput
	cfg.Timeout = 60 * time.Second
	cfg.DialTimeout = 15 * time.Second
	cfg.TLSHandshakeTimeout = 15 * time.Second
	cfg.ResponseHeaderTimeout = 60 * time.Second
	cfg.IdleConnTimeout = 120 * time.Second

	// Connection - larger pools for throughput
	cfg.MaxIdleConns = 100
	cfg.MaxConnsPerHost = 20
	cfg.EnableCookies = true

	// Security - maintain essential validation for security
	cfg.MaxResponseBodySize = 50 * 1024 * 1024 // 50MB
	cfg.StrictContentLength = false            // Can be relaxed for performance
	cfg.ValidateURL = true                     // SECURITY: Keep URL validation
	cfg.ValidateHeaders = true                 // SECURITY: Keep header validation (O(1) lookup table)

	// Retry - faster retries
	cfg.RetryDelay = 500 * time.Millisecond
	cfg.BackoffFactor = 1.5
	cfg.EnableJitter = true

	return cfg
}

// TestingConfig returns a configuration optimized for testing environments.
// WARNING: This config disables security features and should NEVER be used in production.
// Use this ONLY for local development and testing with localhost/private networks.
//
// SECURITY: This function will log a warning if called outside of a test environment.
// For production, use SecureConfig() or DefaultConfig() instead.
func TestingConfig() *Config {
	// Security warning for non-test environments
	warnTestingConfigInProduction()

	cfg := DefaultConfig()

	// Timeouts - shorter for faster tests
	cfg.DialTimeout = 5 * time.Second
	cfg.TLSHandshakeTimeout = 5 * time.Second
	cfg.ResponseHeaderTimeout = 10 * time.Second
	cfg.IdleConnTimeout = 30 * time.Second

	// Connection - minimal for testing
	cfg.MaxIdleConns = 10
	cfg.MaxConnsPerHost = 5
	cfg.EnableHTTP2 = false
	cfg.EnableCookies = true

	// Security - DISABLED for testing only
	cfg.InsecureSkipVerify = true
	cfg.AllowPrivateIPs = true // Allow localhost/private IPs
	cfg.ValidateURL = false
	cfg.ValidateHeaders = false

	// Retry - minimal for faster tests
	cfg.MaxRetries = 1
	cfg.RetryDelay = 100 * time.Millisecond
	cfg.EnableJitter = false

	// Middleware - test user agent
	cfg.UserAgent = "httpc-test/1.0"

	return cfg
}

// MinimalConfig returns a lightweight configuration with minimal features.
// Use this for simple, one-off requests where you don't need retries or advanced features.
func MinimalConfig() *Config {
	cfg := DefaultConfig()

	// Timeouts - reasonable defaults
	cfg.DialTimeout = 5 * time.Second
	cfg.TLSHandshakeTimeout = 5 * time.Second
	cfg.ResponseHeaderTimeout = 10 * time.Second
	cfg.IdleConnTimeout = 30 * time.Second

	// Connection - minimal
	cfg.MaxIdleConns = 10
	cfg.MaxConnsPerHost = 2

	// Security - standard validation
	cfg.MaxResponseBodySize = 1 * 1024 * 1024 // 1MB
	cfg.ValidateURL = true
	cfg.ValidateHeaders = true

	// Retry - disabled
	cfg.MaxRetries = 0
	cfg.RetryDelay = 0
	cfg.BackoffFactor = 1.0
	cfg.EnableJitter = false

	// Middleware - no redirects
	cfg.FollowRedirects = false

	return cfg
}
