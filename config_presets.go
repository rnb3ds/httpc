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

func SecureConfig() *Config {
	cfg := DefaultConfig()
	cfg.Timeouts.Request = 15 * time.Second
	cfg.Connections.MaxIdleConns = 20
	cfg.Connections.MaxConnsPerHost = 5
	cfg.Security.MaxResponseBodySize = 5 * 1024 * 1024
	cfg.Security.AllowPrivateIPs = false // Strict SSRF protection for security-critical applications
	cfg.Retry.MaxRetries = 1
	cfg.Retry.Delay = 2 * time.Second
	cfg.Middleware.FollowRedirects = false
	cfg.Timeouts.Dial = 5 * time.Second
	cfg.Timeouts.TLSHandshake = 5 * time.Second
	cfg.Timeouts.ResponseHeader = 10 * time.Second
	cfg.Timeouts.IdleConn = 30 * time.Second
	cfg.Security.ValidateURL = true
	cfg.Security.ValidateHeaders = true
	cfg.Retry.EnableJitter = true
	return cfg
}

func PerformanceConfig() *Config {
	cfg := DefaultConfig()
	cfg.Timeouts.Request = 60 * time.Second
	cfg.Connections.MaxIdleConns = 100
	cfg.Connections.MaxConnsPerHost = 20
	cfg.Security.MaxResponseBodySize = 50 * 1024 * 1024
	cfg.Security.StrictContentLength = false
	cfg.Retry.Delay = 500 * time.Millisecond
	cfg.Retry.BackoffFactor = 1.5
	cfg.Connections.EnableCookies = true
	cfg.Timeouts.Dial = 15 * time.Second
	cfg.Timeouts.TLSHandshake = 15 * time.Second
	cfg.Timeouts.ResponseHeader = 60 * time.Second
	cfg.Timeouts.IdleConn = 120 * time.Second
	cfg.Security.ValidateURL = true
	cfg.Security.ValidateHeaders = false
	cfg.Retry.EnableJitter = true
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
	cfg.Security.InsecureSkipVerify = true
	cfg.Security.AllowPrivateIPs = true
	cfg.Connections.MaxIdleConns = 10
	cfg.Connections.MaxConnsPerHost = 5
	cfg.Retry.MaxRetries = 1
	cfg.Retry.Delay = 100 * time.Millisecond
	cfg.Middleware.UserAgent = "httpc-test/1.0"
	cfg.Connections.EnableHTTP2 = false
	cfg.Connections.EnableCookies = true
	cfg.Timeouts.Dial = 5 * time.Second
	cfg.Timeouts.TLSHandshake = 5 * time.Second
	cfg.Timeouts.ResponseHeader = 10 * time.Second
	cfg.Timeouts.IdleConn = 30 * time.Second
	cfg.Security.ValidateURL = false
	cfg.Security.ValidateHeaders = false
	cfg.Retry.EnableJitter = false
	return cfg
}

// MinimalConfig returns a lightweight configuration with minimal features.
// Use this for simple, one-off requests where you don't need retries or advanced features.
func MinimalConfig() *Config {
	cfg := DefaultConfig()
	cfg.Connections.MaxIdleConns = 10
	cfg.Connections.MaxConnsPerHost = 2
	cfg.Security.MaxResponseBodySize = 1 * 1024 * 1024
	cfg.Retry.MaxRetries = 0
	cfg.Retry.Delay = 0
	cfg.Retry.BackoffFactor = 1.0
	cfg.Middleware.FollowRedirects = false
	cfg.Timeouts.Dial = 5 * time.Second
	cfg.Timeouts.TLSHandshake = 5 * time.Second
	cfg.Timeouts.ResponseHeader = 10 * time.Second
	cfg.Timeouts.IdleConn = 30 * time.Second
	cfg.Security.ValidateURL = true
	cfg.Security.ValidateHeaders = true
	cfg.Retry.EnableJitter = false
	return cfg
}
