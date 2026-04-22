package httpc

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
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

// TimeoutConfig configures timeout behavior for HTTP requests.
// All duration values use time.Duration (e.g., 30 * time.Second).
type TimeoutConfig struct {
	// Request is the overall request timeout including retries.
	// Default: 30s. Set to 0 for no timeout (not recommended for production).
	Request time.Duration

	// Dial is the maximum time to wait for a TCP connection.
	// Default: 10s.
	Dial time.Duration

	// TLSHandshake is the maximum time to wait for TLS handshake.
	// Default: 10s. Only applies to HTTPS connections.
	TLSHandshake time.Duration

	// ResponseHeader is the maximum time to wait for response headers.
	// Default: 30s.
	ResponseHeader time.Duration

	// IdleConn is the maximum time an idle connection remains open.
	// Default: 90s.
	IdleConn time.Duration
}

// ConnectionConfig configures connection pooling and proxy behavior.
type ConnectionConfig struct {
	// MaxIdleConns is the maximum number of idle connections across all hosts.
	// Default: 50.
	MaxIdleConns int

	// MaxConnsPerHost is the maximum connections per host (idle + active).
	// Default: 10.
	MaxConnsPerHost int

	// ProxyURL specifies an explicit proxy server URL (e.g., "http://proxy:8080").
	// Takes precedence over EnableSystemProxy. Default: "" (no proxy).
	ProxyURL string

	// EnableSystemProxy enables automatic detection of system proxy settings.
	// Default: false.
	EnableSystemProxy bool

	// EnableHTTP2 enables HTTP/2 protocol support.
	// Default: true.
	EnableHTTP2 bool

	// EnableCookies enables automatic cookie handling with a cookie jar.
	// Default: false.
	EnableCookies bool

	// EnableDoH enables DNS-over-HTTPS for DNS resolution.
	// Default: false.
	EnableDoH bool

	// DoHCacheTTL is the cache duration for DoH DNS responses.
	// Default: 5 minutes.
	DoHCacheTTL time.Duration
}

// SecurityConfig configures TLS, validation, and SSRF protection.
type SecurityConfig struct {
	// TLSConfig provides custom TLS configuration. If set, MinTLSVersion and
	// MaxTLSVersion are ignored. Default: nil.
	TLSConfig *tls.Config

	// MinTLSVersion is the minimum TLS version. Default: TLS 1.2.
	MinTLSVersion uint16

	// MaxTLSVersion is the maximum TLS version. Default: TLS 1.3.
	MaxTLSVersion uint16

	// InsecureSkipVerify disables TLS certificate verification.
	// WARNING: Only use in testing. Default: false.
	InsecureSkipVerify bool

	// MaxResponseBodySize limits response body size in bytes. Default: 10MB.
	MaxResponseBodySize int64

	// AllowPrivateIPs permits connections to private IP addresses.
	// Default: true. Set to false to enable SSRF protection.
	AllowPrivateIPs bool

	// ValidateURL enables URL validation. Default: true.
	ValidateURL bool

	// ValidateHeaders enables header validation. Default: true.
	ValidateHeaders bool

	// StrictContentLength enables strict Content-Length validation. Default: true.
	StrictContentLength bool

	// CookieSecurity enables cookie security attribute validation.
	// Default: nil (no validation).
	CookieSecurity *validation.CookieSecurityConfig

	// RedirectWhitelist specifies allowed domains for redirects.
	// Default: nil (all redirects allowed).
	RedirectWhitelist []string
}

// RetryConfig configures retry behavior for transient failures.
type RetryConfig struct {
	// MaxRetries is the maximum retry attempts. Default: 3. Set to 0 to disable.
	MaxRetries int

	// Delay is the initial retry delay. Default: 1s.
	Delay time.Duration

	// BackoffFactor multiplies Delay after each failed attempt. Default: 2.0.
	BackoffFactor float64

	// EnableJitter enables jitter in retry delay. Default: true.
	EnableJitter bool

	// CustomPolicy overrides the built-in retry logic. Default: nil.
	CustomPolicy RetryPolicy
}

// MiddlewareConfig configures middleware, default headers, and redirect behavior.
type MiddlewareConfig struct {
	// Middlewares contains middleware functions for request/response interception.
	// Default: nil.
	Middlewares []MiddlewareFunc

	// UserAgent sets the User-Agent header. Default: "httpc/1.0".
	UserAgent string

	// Headers contains default headers added to every request.
	Headers map[string]string

	// FollowRedirects controls automatic redirect following. Default: true.
	FollowRedirects bool

	// MaxRedirects limits automatic redirects. Default: 10.
	MaxRedirects int
}

// Config defines the HTTP client configuration organized into logical groups.
// Use DefaultConfig() for production-ready defaults, or use preset configurations
// like SecureConfig(), PerformanceConfig(), or MinimalConfig().
//
// Example:
//
//	cfg := httpc.DefaultConfig()
//	cfg.Timeouts.Request = 60 * time.Second
//	cfg.Retry.MaxRetries = 5
//	cfg.Connection.ProxyURL = "http://proxy:8080"
//	cfg.Security.AllowPrivateIPs = true
//	client, err := httpc.New(cfg)
type Config struct {
	Timeouts   TimeoutConfig
	Connection ConnectionConfig
	Security   SecurityConfig
	Retry      RetryConfig
	Middleware MiddlewareConfig
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

// BodyKind represents the type of request body for WithBody.
type BodyKind int

const (
	// BodyAuto auto-detects body type based on input data.
	// Auto-detection rules:
	//   - string → text/plain
	//   - []byte → application/octet-stream
	//   - map[string]string → application/x-www-form-urlencoded
	//   - *FormData → multipart/form-data
	//   - io.Reader → passed through (no Content-Type set)
	//   - other types → application/json
	BodyAuto BodyKind = iota

	// BodyJSON forces JSON encoding regardless of input type.
	BodyJSON

	// BodyXML forces XML encoding regardless of input type.
	BodyXML

	// BodyForm forces application/x-www-form-urlencoded encoding.
	// Input must be map[string]string or compatible type.
	BodyForm

	// BodyBinary forces binary/octet-stream encoding.
	// Input must be []byte or string.
	BodyBinary

	// BodyMultipart forces multipart/form-data encoding.
	// Input must be *FormData.
	BodyMultipart
)

// DefaultConfig returns a Config with production-ready defaults.
// The returned config is safe for modification.
//
// SSRF Protection Note:
// By default, AllowPrivateIPs is true for maximum compatibility with VPNs,
// proxies, and corporate networks. If you need SSRF (Server-Side Request Forgery)
// protection, set AllowPrivateIPs = false or use SecureConfig() preset:
//
//	// For SSRF protection:
//	cfg := httpc.DefaultConfig()
//	cfg.Security.AllowPrivateIPs = false
//	// OR use the secure preset:
//	cfg := httpc.SecureConfig()
func DefaultConfig() *Config {
	return &Config{
		Timeouts: TimeoutConfig{
			Request:        30 * time.Second,
			Dial:           10 * time.Second,
			TLSHandshake:   10 * time.Second,
			ResponseHeader: 30 * time.Second,
			IdleConn:       90 * time.Second,
		},
		Connection: ConnectionConfig{
			MaxIdleConns:      50,
			MaxConnsPerHost:   10,
			ProxyURL:          "",
			EnableSystemProxy: false,
			EnableHTTP2:       true,
			EnableCookies:     false,
			EnableDoH:         false,
			DoHCacheTTL:       5 * time.Minute,
		},
		Security: SecurityConfig{
			TLSConfig:           nil,
			MinTLSVersion:       tls.VersionTLS12,
			MaxTLSVersion:       tls.VersionTLS13,
			InsecureSkipVerify:  false,
			MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
			AllowPrivateIPs:     true,
			ValidateURL:         true,
			ValidateHeaders:     true,
			StrictContentLength: true,
		},
		Retry: RetryConfig{
			MaxRetries:    3,
			Delay:         1 * time.Second,
			BackoffFactor: 2.0,
			EnableJitter:  true,
			CustomPolicy:  nil,
		},
		Middleware: MiddlewareConfig{
			Middlewares:     nil,
			UserAgent:       "httpc/1.0",
			Headers:         make(map[string]string),
			FollowRedirects: true,
			MaxRedirects:    10,
		},
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
	if cfg.Timeouts.Request < 0 || cfg.Timeouts.Request > maxTimeout {
		return fmt.Errorf("%w: Timeouts.Request must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeouts.Request)
	}
	if cfg.Timeouts.Dial < 0 || cfg.Timeouts.Dial > maxTimeout {
		return fmt.Errorf("%w: Timeouts.Dial must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeouts.Dial)
	}
	if cfg.Timeouts.TLSHandshake < 0 || cfg.Timeouts.TLSHandshake > maxTimeout {
		return fmt.Errorf("%w: Timeouts.TLSHandshake must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeouts.TLSHandshake)
	}
	if cfg.Timeouts.ResponseHeader < 0 || cfg.Timeouts.ResponseHeader > maxTimeout {
		return fmt.Errorf("%w: Timeouts.ResponseHeader must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeouts.ResponseHeader)
	}
	if cfg.Timeouts.IdleConn < 0 || cfg.Timeouts.IdleConn > maxTimeout {
		return fmt.Errorf("%w: Timeouts.IdleConn must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeouts.IdleConn)
	}

	// Validate connection settings
	if cfg.Connection.MaxIdleConns < 0 || cfg.Connection.MaxIdleConns > maxIdleConns {
		return fmt.Errorf("Connection.MaxIdleConns must be 0-%d, got %d", maxIdleConns, cfg.Connection.MaxIdleConns)
	}
	if cfg.Connection.MaxConnsPerHost < 0 || cfg.Connection.MaxConnsPerHost > maxConnsPerHost {
		return fmt.Errorf("Connection.MaxConnsPerHost must be 0-%d, got %d", maxConnsPerHost, cfg.Connection.MaxConnsPerHost)
	}

	// Validate security settings
	if cfg.Security.MaxResponseBodySize < 0 || cfg.Security.MaxResponseBodySize > maxResponseBodySize {
		return fmt.Errorf("Security.MaxResponseBodySize must be 0-1GB, got %d", cfg.Security.MaxResponseBodySize)
	}

	// Validate retry settings
	if cfg.Retry.MaxRetries < 0 || cfg.Retry.MaxRetries > maxRetries {
		return fmt.Errorf("%w: Retry.MaxRetries must be 0-%d, got %d", ErrInvalidRetry, maxRetries, cfg.Retry.MaxRetries)
	}
	if cfg.Retry.Delay < 0 {
		return fmt.Errorf("%w: Retry.Delay cannot be negative", ErrInvalidRetry)
	}
	if cfg.Retry.BackoffFactor < minBackoffFactor || cfg.Retry.BackoffFactor > maxBackoffFactor {
		return fmt.Errorf("%w: Retry.BackoffFactor must be %.1f-%.1f, got %.1f", ErrInvalidRetry, minBackoffFactor, maxBackoffFactor, cfg.Retry.BackoffFactor)
	}

	// Validate middleware settings
	if cfg.Middleware.MaxRedirects < 0 || cfg.Middleware.MaxRedirects > 50 {
		return fmt.Errorf("Middleware.MaxRedirects must be 0-50, got %d", cfg.Middleware.MaxRedirects)
	}
	if len(cfg.Middleware.UserAgent) > maxUserAgentLen || !validation.IsValidHeaderString(cfg.Middleware.UserAgent) {
		return fmt.Errorf("Middleware.UserAgent invalid: max %d chars, no control characters", maxUserAgentLen)
	}

	for key, value := range cfg.Middleware.Headers {
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
	b.WriteString("Config{Timeouts:{Request: ")
	b.WriteString(c.Timeouts.Request.String())
	b.WriteString(", Dial: ")
	b.WriteString(c.Timeouts.Dial.String())
	b.WriteString(", TLSHandshake: ")
	b.WriteString(c.Timeouts.TLSHandshake.String())

	b.WriteString("}, Connection:{MaxIdleConns: ")
	b.WriteString(strconv.Itoa(c.Connection.MaxIdleConns))
	b.WriteString(", MaxConnsPerHost: ")
	b.WriteString(strconv.Itoa(c.Connection.MaxConnsPerHost))
	b.WriteString(", ProxyURL: ")
	b.WriteString(maskProxyURL(c.Connection.ProxyURL))

	b.WriteString("}, Security:{TLSConfig: ")
	if c.Security.TLSConfig != nil {
		b.WriteString("<configured>")
	} else {
		b.WriteString("<default>")
	}
	b.WriteString(", InsecureSkipVerify: ")
	b.WriteString(strconv.FormatBool(c.Security.InsecureSkipVerify))
	b.WriteString(", AllowPrivateIPs: ")
	b.WriteString(strconv.FormatBool(c.Security.AllowPrivateIPs))

	b.WriteString("}, Retry:{MaxRetries: ")
	b.WriteString(strconv.Itoa(c.Retry.MaxRetries))
	b.WriteString(", BackoffFactor: ")
	b.WriteString(strconv.FormatFloat(c.Retry.BackoffFactor, 'f', 1, 64))

	b.WriteString("}, Middleware:{UserAgent: ")
	b.WriteString(c.Middleware.UserAgent)
	b.WriteString(", FollowRedirects: ")
	b.WriteString(strconv.FormatBool(c.Middleware.FollowRedirects))

	b.WriteString("}}")
	return b.String()
}

// maskProxyURL masks credentials in a proxy URL for safe logging.
// Returns the URL with credentials replaced by "***:***".
func maskProxyURL(proxyURL string) string {
	return validation.SanitizeURL(proxyURL)
}
