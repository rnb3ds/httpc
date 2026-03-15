package httpc

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
	"github.com/cybergodev/httpc/internal/types"
	"github.com/cybergodev/httpc/internal/validation"
)

const (
	maxJSONSize         = 50 * 1024 * 1024   // 50MB
	maxTimeout          = 30 * time.Minute   // 30 minutes
	maxIdleConns        = 1000               // Connection pool limit
	maxConnsPerHost     = 1000               // Per-host connection limit
	maxResponseBodySize = 1024 * 1024 * 1024 // 1GB
	maxRetries          = 10                 // Maximum retry attempts
	minBackoffFactor    = 1.0                // Minimum backoff multiplier
	maxBackoffFactor    = 10.0               // Maximum backoff multiplier
	maxUserAgentLen     = 512                // User-Agent header limit
)

// Config defines the HTTP client configuration with a flat structure.
// All configuration options are directly accessible at the top level.
// All duration values use time.Duration (e.g., 30 * time.Second).
//
// Use DefaultConfig() for production-ready defaults, or use preset
// configurations like SecureConfig(), PerformanceConfig(), or MinimalConfig().
//
// Example:
//
//	cfg := httpc.DefaultConfig()
//	cfg.Timeout = 60 * time.Second
//	cfg.MaxRetries = 5
//	cfg.ProxyURL = "http://proxy:8080"
//	cfg.AllowPrivateIPs = true
//	client, err := httpc.New(cfg)
type Config struct {
	// === Timeouts (5 fields) ===

	// Timeout is the overall request timeout including retries.
	// Default: 30s. Set to 0 for no timeout (not recommended for production).
	Timeout time.Duration

	// DialTimeout is the maximum time to wait for a TCP connection.
	// Default: 10s. This is the timeout for the initial connection.
	DialTimeout time.Duration

	// TLSHandshakeTimeout is the maximum time to wait for TLS handshake.
	// Default: 10s. Only applies to HTTPS connections.
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout is the maximum time to wait for response headers.
	// Default: 30s. The connection is dropped if headers are not received in time.
	ResponseHeaderTimeout time.Duration

	// IdleConnTimeout is the maximum time an idle connection remains open.
	// Default: 90s. Connections are closed after this period of inactivity.
	IdleConnTimeout time.Duration

	// === Connection (8 fields) ===

	// MaxIdleConns is the maximum number of idle connections across all hosts.
	// Default: 50. Higher values improve performance for multi-host scenarios.
	MaxIdleConns int

	// MaxConnsPerHost is the maximum connections per host (idle + active).
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

	// === Security (9 fields) ===

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
	// SECURITY: Default is false to prevent Server-Side Request Forgery.
	// Set to true only for development or when accessing internal services.
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

	// === Retry (5 fields) ===

	// MaxRetries is the maximum number of retry attempts for transient failures.
	// Default: 3. Set to 0 to disable retries.
	MaxRetries int

	// RetryDelay is the initial delay between retry attempts.
	// Default: 1s. Actual delay increases with BackoffFactor.
	RetryDelay time.Duration

	// BackoffFactor multiplies RetryDelay after each failed attempt.
	// Default: 2.0. Must be between 1.0 and 10.0.
	BackoffFactor float64

	// EnableJitter enables jitter in retry delay calculations.
	// Default: true. Helps prevent thundering herd problems.
	EnableJitter bool

	// CustomRetryPolicy allows providing a custom retry policy implementation.
	// If set, it overrides MaxRetries, RetryDelay, BackoffFactor, and EnableJitter.
	CustomRetryPolicy RetryPolicy

	// === Middleware (5 fields) ===

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

// RequestOption is a function that modifies a request.
// This is a type alias to the engine's RequestOption for unified type handling.
type RequestOption = engine.RequestOption

// RetryPolicy defines the interface for custom retry behavior.
// This is a type alias to types.RetryPolicy for convenience.
type RetryPolicy = types.RetryPolicy

// FormData represents multipart form data for HTTP requests.
// This is a type alias to types.FormData for backward compatibility.
//
// Example:
//
//	form := &httpc.FormData{
//	    Fields: map[string]string{"key": "value"},
//	    Files: map[string]*httpc.FileData{
//	        "file": {Filename: "test.txt", Content: []byte("hello")},
//	    },
//	}
type FormData = types.FormData

// FileData represents a file to be uploaded in a multipart form.
// This is a type alias to types.FileData for backward compatibility.
type FileData = types.FileData

// RequestMutator provides read-write access to request data for middleware.
// Middleware can inspect and modify request properties before the request is sent.
//
// This is a type alias to the shared types package, ensuring compile-time
// type compatibility between public and internal layers.
type RequestMutator = types.RequestMutator

// ResponseMutator provides read-write access to response data.
// Middleware can inspect and modify response properties after the request completes.
// This is useful for:
//   - Response caching middleware
//   - Response transformation (e.g., JSON pretty-printing)
//   - Content encoding/decoding
//   - Response filtering
//
// This is a type alias to the shared types package, ensuring compile-time
// type compatibility between public and internal layers.
type ResponseMutator = types.ResponseMutator

// Handler processes an HTTP request and returns a response.
// This is the core function signature for request processing in the middleware chain.
//
// This is a type alias to the shared types package, ensuring compile-time
// type compatibility between public and internal layers.
type Handler = types.Handler

// MiddlewareFunc wraps a Handler with additional functionality.
// Middleware can inspect/modify requests, handle responses, add logging, etc.
//
// This is a type alias to the shared types package, ensuring compile-time
// type compatibility between public and internal layers.
type MiddlewareFunc = types.MiddlewareFunc

// DefaultConfig returns a Config with production-ready defaults.
// The returned config is safe for modification.
func DefaultConfig() *Config {
	return &Config{
		// Timeouts
		Timeout:               30 * time.Second,
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,

		// Connection
		MaxIdleConns:      50,
		MaxConnsPerHost:   10,
		ProxyURL:          "",
		EnableSystemProxy: false,
		EnableHTTP2:       true,
		EnableCookies:     false,
		EnableDoH:         false,
		DoHCacheTTL:       5 * time.Minute,

		// Security
		TLSConfig:           nil,
		MinTLSVersion:       tls.VersionTLS12,
		MaxTLSVersion:       tls.VersionTLS13,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
		AllowPrivateIPs:     false,            // SECURITY: Block private IPs by default
		ValidateURL:         true,
		ValidateHeaders:     true,
		StrictContentLength: true,

		// Retry
		MaxRetries:        3,
		RetryDelay:        1 * time.Second,
		BackoffFactor:     2.0,
		EnableJitter:      true,
		CustomRetryPolicy: nil,

		// Middleware
		Middlewares:     nil,
		UserAgent:       "httpc/1.0",
		Headers:         make(map[string]string),
		FollowRedirects: true,
		MaxRedirects:    10,
	}
}

// NewCookieJar creates a new cookie jar for cookie management.
func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
}

// ValidateConfig validates the configuration and returns an error if invalid.
// This is called internally by New() but can also be called explicitly.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrNilConfig
	}

	// Validate timeouts
	if cfg.Timeout < 0 || cfg.Timeout > maxTimeout {
		return fmt.Errorf("%w: Timeout must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeout)
	}
	if cfg.DialTimeout < 0 || cfg.DialTimeout > maxTimeout {
		return fmt.Errorf("DialTimeout must be 0-%v, got %v", maxTimeout, cfg.DialTimeout)
	}
	if cfg.TLSHandshakeTimeout < 0 || cfg.TLSHandshakeTimeout > maxTimeout {
		return fmt.Errorf("TLSHandshakeTimeout must be 0-%v, got %v", maxTimeout, cfg.TLSHandshakeTimeout)
	}
	if cfg.ResponseHeaderTimeout < 0 || cfg.ResponseHeaderTimeout > maxTimeout {
		return fmt.Errorf("ResponseHeaderTimeout must be 0-%v, got %v", maxTimeout, cfg.ResponseHeaderTimeout)
	}
	if cfg.IdleConnTimeout < 0 || cfg.IdleConnTimeout > maxTimeout {
		return fmt.Errorf("IdleConnTimeout must be 0-%v, got %v", maxTimeout, cfg.IdleConnTimeout)
	}

	// Validate connection settings
	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > maxIdleConns {
		return fmt.Errorf("MaxIdleConns must be 0-%d, got %d", maxIdleConns, cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 || cfg.MaxConnsPerHost > maxConnsPerHost {
		return fmt.Errorf("MaxConnsPerHost must be 0-%d, got %d", maxConnsPerHost, cfg.MaxConnsPerHost)
	}

	// Validate security settings
	if cfg.MaxResponseBodySize < 0 || cfg.MaxResponseBodySize > maxResponseBodySize {
		return fmt.Errorf("MaxResponseBodySize must be 0-1GB, got %d", cfg.MaxResponseBodySize)
	}

	// Validate retry settings
	if cfg.MaxRetries < 0 || cfg.MaxRetries > maxRetries {
		return fmt.Errorf("%w: MaxRetries must be 0-%d, got %d", ErrInvalidRetry, maxRetries, cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("%w: RetryDelay cannot be negative", ErrInvalidRetry)
	}
	if cfg.BackoffFactor < minBackoffFactor || cfg.BackoffFactor > maxBackoffFactor {
		return fmt.Errorf("%w: BackoffFactor must be %.1f-%.1f, got %.1f", ErrInvalidRetry, minBackoffFactor, maxBackoffFactor, cfg.BackoffFactor)
	}

	// Validate middleware settings
	if cfg.MaxRedirects < 0 || cfg.MaxRedirects > 50 {
		return fmt.Errorf("MaxRedirects must be 0-50, got %d", cfg.MaxRedirects)
	}
	if len(cfg.UserAgent) > maxUserAgentLen || !validation.IsValidHeaderString(cfg.UserAgent) {
		return fmt.Errorf("UserAgent invalid: max %d chars, no control characters", maxUserAgentLen)
	}

	for key, value := range cfg.Headers {
		if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidHeader, key, err)
		}
	}

	return nil
}

// String returns a safe string representation of the Config.
// Sensitive values are masked:
//   - ProxyURL credentials (user:pass@host -> ***:***@host)
//   - TLSConfig displays as <configured> or <default>
//   - Headers are not displayed (may contain sensitive data)
func (c *Config) String() string {
	if c == nil {
		return "Config{<nil>}"
	}

	var b strings.Builder
	b.WriteString("Config{")

	// Timeouts
	b.WriteString("Timeout: ")
	b.WriteString(c.Timeout.String())
	b.WriteString(", DialTimeout: ")
	b.WriteString(c.DialTimeout.String())
	b.WriteString(", TLSHandshakeTimeout: ")
	b.WriteString(c.TLSHandshakeTimeout.String())

	// Connection
	b.WriteString(", MaxIdleConns: ")
	b.WriteString(strconv.Itoa(c.MaxIdleConns))
	b.WriteString(", MaxConnsPerHost: ")
	b.WriteString(strconv.Itoa(c.MaxConnsPerHost))
	b.WriteString(", ProxyURL: ")
	b.WriteString(maskProxyURL(c.ProxyURL))

	// Security
	b.WriteString(", TLSConfig: ")
	if c.TLSConfig != nil {
		b.WriteString("<configured>")
	} else {
		b.WriteString("<default>")
	}
	b.WriteString(", InsecureSkipVerify: ")
	b.WriteString(strconv.FormatBool(c.InsecureSkipVerify))
	b.WriteString(", AllowPrivateIPs: ")
	b.WriteString(strconv.FormatBool(c.AllowPrivateIPs))

	// Retry
	b.WriteString(", MaxRetries: ")
	b.WriteString(strconv.Itoa(c.MaxRetries))
	b.WriteString(", BackoffFactor: ")
	b.WriteString(strconv.FormatFloat(c.BackoffFactor, 'f', 1, 64))

	// Middleware
	b.WriteString(", UserAgent: ")
	b.WriteString(c.UserAgent)
	b.WriteString(", FollowRedirects: ")
	b.WriteString(strconv.FormatBool(c.FollowRedirects))

	b.WriteByte('}')
	return b.String()
}

// maskProxyURL masks credentials in a proxy URL for safe logging.
// Returns the URL with credentials replaced by "***:***".
func maskProxyURL(proxyURL string) string {
	if proxyURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return "<invalid>"
	}

	if parsedURL.User == nil {
		return parsedURL.String()
	}

	_, hasPassword := parsedURL.User.Password()
	parsedURL.User = nil

	path := parsedURL.Path
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}
	if parsedURL.Fragment != "" {
		path += "#" + parsedURL.Fragment
	}

	if hasPassword {
		return fmt.Sprintf("%s://***:***@%s%s", parsedURL.Scheme, parsedURL.Host, path)
	}
	return fmt.Sprintf("%s://***@%s%s", parsedURL.Scheme, parsedURL.Host, path)
}
