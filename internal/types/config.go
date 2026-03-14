// Package types provides shared type definitions used across the httpc library.
package types

import (
	"crypto/tls"
	"time"
)

// TimeoutConfig contains all timeout-related configuration options.
// Use this to group timeout settings for better organization.
type TimeoutConfig struct {
	// Request is the maximum duration for the entire request including retries.
	// Default: 30s. Set to 0 for no timeout (not recommended for production).
	Request time.Duration

	// Dial is the maximum time to wait for a connection to be established.
	// Default: 10s. This is the timeout for the initial TCP connection.
	Dial time.Duration

	// TLSHandshake is the maximum time to wait for a TLS handshake.
	// Default: 10s. Only applies to HTTPS connections.
	TLSHandshake time.Duration

	// ResponseHeader is the maximum time to wait for response headers.
	// Default: 30s. The connection is dropped if headers are not received within this time.
	ResponseHeader time.Duration

	// IdleConn is the maximum time an idle connection remains open.
	// Default: 90s. Connections are closed after this period of inactivity.
	IdleConn time.Duration
}

// ConnectionConfig contains connection-related configuration options.
// Use this to group connection settings for better organization.
type ConnectionConfig struct {
	// MaxIdleConns is the maximum number of idle connections across all hosts.
	// Default: 50. Higher values improve performance for multi-host scenarios.
	MaxIdleConns int

	// MaxConnsPerHost is the maximum number of connections per host (idle + active).
	// Default: 10. Adjust based on server capacity and expected load.
	MaxConnsPerHost int

	// ProxyURL specifies an explicit proxy server URL (e.g., "http://proxy:8080").
	// Takes precedence over EnableSystemProxy. Default: "" (no proxy).
	ProxyURL string

	// EnableSystemProxy enables automatic detection of system proxy settings.
	// Reads from Windows registry, macOS system settings, and environment variables.
	// Ignored if ProxyURL is set. Default: false.
	EnableSystemProxy bool

	// EnableHTTP2 enables HTTP/2 protocol support.
	// Default: true. Disable for HTTP/1.1-only environments.
	EnableHTTP2 bool

	// EnableCookies enables automatic cookie handling with a cookie jar.
	// Default: false. Enable for session-based authentication.
	EnableCookies bool

	// EnableDoH enables DNS-over-HTTPS for DNS resolution.
	// Provides privacy and bypasses DNS-based filtering. Default: false.
	EnableDoH bool

	// DoHCacheTTL is the cache duration for DoH DNS responses.
	// Default: 5 minutes. Ignored if EnableDoH is false.
	DoHCacheTTL time.Duration
}

// SecurityConfig contains security-related configuration options.
// Use this to group security settings for better organization.
type SecurityConfig struct {
	// TLSConfig provides custom TLS configuration. If set, MinTLSVersion and
	// MaxTLSVersion are ignored. Default: nil (uses secure defaults).
	TLSConfig *tls.Config

	// MinTLSVersion is the minimum TLS version. Default: TLS 1.2 (0x0303).
	// Use tls.VersionTLS12 or tls.VersionTLS13.
	MinTLSVersion uint16

	// MaxTLSVersion is the maximum TLS version. Default: TLS 1.3 (0x0304).
	MaxTLSVersion uint16

	// InsecureSkipVerify disables TLS certificate verification.
	// WARNING: This should only be used in testing. Default: false.
	InsecureSkipVerify bool

	// MaxResponseBodySize limits the maximum response body size in bytes.
	// Default: 10MB. Set to 0 for no limit (not recommended).
	MaxResponseBodySize int64

	// AllowPrivateIPs permits connections to private IP addresses (SSRF protection).
	// SECURITY: Set to false by default to prevent Server-Side Request Forgery.
	// Set to true only for development environments or when accessing internal services.
	AllowPrivateIPs bool

	// ValidateURL enables URL validation before sending requests.
	// Default: true. Disable only for compatibility with unusual URL schemes.
	ValidateURL bool

	// ValidateHeaders enables header validation before sending requests.
	// Default: true. Disable only for compatibility with non-standard headers.
	ValidateHeaders bool

	// StrictContentLength enables strict Content-Length validation.
	// Default: true. Disable only for compatibility with broken servers.
	StrictContentLength bool
}

// RetryConfig contains retry-related configuration options.
// Use this to group retry settings for better organization.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts for transient failures.
	// Default: 3. Set to 0 to disable retries.
	MaxRetries int

	// Delay is the initial delay between retry attempts.
	// Default: 1s. Actual delay increases with BackoffFactor.
	Delay time.Duration

	// BackoffFactor multiplies Delay after each failed attempt.
	// Default: 2.0. Must be between 1.0 and 10.0.
	BackoffFactor float64

	// EnableJitter enables jitter in retry delay calculations.
	// Default: true. Helps prevent thundering herd problems in distributed systems.
	EnableJitter bool

	// CustomRetryPolicy allows providing a custom retry policy implementation.
	// If set, it overrides MaxRetries, Delay, BackoffFactor, and EnableJitter.
	// This allows for completely custom retry strategies.
	CustomRetryPolicy RetryPolicy
}

// MiddlewareConfig contains middleware-related configuration options.
// Use this to group middleware settings for better organization.
type MiddlewareConfig struct {
	// Middlewares contains middleware functions for request/response interception.
	// Middlewares are executed in order for requests and reverse order for responses.
	// Default: nil (no middlewares).
	Middlewares []MiddlewareFunc

	// UserAgent sets the User-Agent header for all requests.
	// Default: "httpc/1.0". Max length: 512 characters.
	UserAgent string

	// Headers contains default headers added to every request.
	// Can be overridden per-request using WithHeader option.
	Headers map[string]string

	// FollowRedirects controls automatic redirect following.
	// Default: true. Set to false to handle redirects manually.
	FollowRedirects bool

	// MaxRedirects limits the number of automatic redirects.
	// Default: 10. Set to 0 for unlimited (not recommended).
	MaxRedirects int
}

// DefaultTimeoutConfig returns a TimeoutConfig with production-ready defaults.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Request:        30 * time.Second,
		Dial:           10 * time.Second,
		TLSHandshake:   10 * time.Second,
		ResponseHeader: 30 * time.Second,
		IdleConn:       90 * time.Second,
	}
}

// DefaultConnectionConfig returns a ConnectionConfig with production-ready defaults.
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		MaxIdleConns:    50,
		MaxConnsPerHost: 10,
		EnableHTTP2:     true,
		EnableCookies:   false,
		EnableDoH:       false,
		DoHCacheTTL:     5 * time.Minute,
	}
}

// DefaultSecurityConfig returns a SecurityConfig with production-ready defaults.
// SECURITY: AllowPrivateIPs is set to false by default to prevent SSRF attacks.
// Set to true only for development environments or when you explicitly need
// to access internal services.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		MinTLSVersion:       tls.VersionTLS12,
		MaxTLSVersion:       tls.VersionTLS13,
		MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
		ValidateURL:         true,
		ValidateHeaders:     true,
		StrictContentLength: true,
		AllowPrivateIPs:     false, // SECURITY: Block private IPs by default to prevent SSRF
	}
}

// DefaultRetryConfig returns a RetryConfig with production-ready defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		Delay:         1 * time.Second,
		BackoffFactor: 2.0,
		EnableJitter:  true,
	}
}

// DefaultMiddlewareConfig returns a MiddlewareConfig with production-ready defaults.
func DefaultMiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		UserAgent:       "httpc/1.0",
		Headers:         make(map[string]string),
		FollowRedirects: true,
		MaxRedirects:    10,
	}
}
