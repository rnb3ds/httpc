package httpc

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
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

// Config defines the HTTP client configuration.
// All duration values use time.Duration (e.g., 30 * time.Second).
//
// Configuration values are used as follows:
//   - Timeouts: Request and connection timeout settings
//   - Connections: Connection pool and proxy settings
//   - Security: TLS, SSRF protection, and validation settings
//   - Retry: Retry behavior and custom policies
//   - Middleware: Middleware, headers, and redirect settings
//
// Use DefaultConfig() for production-ready defaults, or use preset
// configurations like SecureConfig(), PerformanceConfig(), or MinimalConfig().
//
// Example:
//
//	cfg := httpc.DefaultConfig()
//	cfg.Timeouts.Request = 60 * time.Second
//	cfg.Retry.MaxRetries = 5
//	client, err := httpc.New(cfg)
type Config struct {
	// Timeouts groups all timeout-related configuration options.
	Timeouts TimeoutConfig

	// Connections groups all connection-related configuration options.
	Connections ConnectionConfig

	// Security groups all security-related configuration options.
	Security SecurityConfig

	// Retry groups all retry-related configuration options.
	Retry RetryConfig

	// Middleware groups all middleware-related configuration options.
	Middleware MiddlewareConfig
}

// RequestOption is a function that modifies a request.
// This is a type alias to the engine's RequestOption for unified type handling.
type RequestOption = engine.RequestOption

// TimeoutConfig groups all timeout-related configuration options.
// This is a type alias to types.TimeoutConfig for convenience.
type TimeoutConfig = types.TimeoutConfig

// ConnectionConfig groups all connection-related configuration options.
// This is a type alias to types.ConnectionConfig for convenience.
type ConnectionConfig = types.ConnectionConfig

// SecurityConfig groups all security-related configuration options.
// This is a type alias to types.SecurityConfig for convenience.
type SecurityConfig = types.SecurityConfig

// RetryConfig groups all retry-related configuration options.
// This is a type alias to types.RetryConfig for convenience.
type RetryConfig = types.RetryConfig

// MiddlewareConfig groups all middleware-related configuration options.
// This is a type alias to types.MiddlewareConfig for convenience.
type MiddlewareConfig = types.MiddlewareConfig

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

func DefaultConfig() *Config {
	return &Config{
		Timeouts:    types.DefaultTimeoutConfig(),
		Connections: types.DefaultConnectionConfig(),
		Security:    types.DefaultSecurityConfig(),
		Retry:       types.DefaultRetryConfig(),
		Middleware:  types.DefaultMiddlewareConfig(),
	}
}

func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
}

func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrNilConfig
	}

	// Validate TimeoutConfig
	if cfg.Timeouts.Request < 0 || cfg.Timeouts.Request > maxTimeout {
		return fmt.Errorf("%w: Request must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeouts.Request)
	}
	if cfg.Timeouts.Dial < 0 || cfg.Timeouts.Dial > maxTimeout {
		return fmt.Errorf("DialTimeout must be 0-%v, got %v", maxTimeout, cfg.Timeouts.Dial)
	}
	if cfg.Timeouts.TLSHandshake < 0 || cfg.Timeouts.TLSHandshake > maxTimeout {
		return fmt.Errorf("TLSHandshakeTimeout must be 0-%v, got %v", maxTimeout, cfg.Timeouts.TLSHandshake)
	}
	if cfg.Timeouts.ResponseHeader < 0 || cfg.Timeouts.ResponseHeader > maxTimeout {
		return fmt.Errorf("ResponseHeaderTimeout must be 0-%v, got %v", maxTimeout, cfg.Timeouts.ResponseHeader)
	}
	if cfg.Timeouts.IdleConn < 0 || cfg.Timeouts.IdleConn > maxTimeout {
		return fmt.Errorf("IdleConnTimeout must be 0-%v, got %v", maxTimeout, cfg.Timeouts.IdleConn)
	}

	// Validate ConnectionConfig
	if cfg.Connections.MaxIdleConns < 0 || cfg.Connections.MaxIdleConns > maxIdleConns {
		return fmt.Errorf("MaxIdleConns must be 0-%d, got %d", maxIdleConns, cfg.Connections.MaxIdleConns)
	}
	if cfg.Connections.MaxConnsPerHost < 0 || cfg.Connections.MaxConnsPerHost > maxConnsPerHost {
		return fmt.Errorf("MaxConnsPerHost must be 0-%d, got %d", maxConnsPerHost, cfg.Connections.MaxConnsPerHost)
	}

	// Validate SecurityConfig
	if cfg.Security.MaxResponseBodySize < 0 || cfg.Security.MaxResponseBodySize > maxResponseBodySize {
		return fmt.Errorf("MaxResponseBodySize must be 0-1GB, got %d", cfg.Security.MaxResponseBodySize)
	}

	// Validate RetryConfig
	if cfg.Retry.MaxRetries < 0 || cfg.Retry.MaxRetries > maxRetries {
		return fmt.Errorf("%w: must be 0-%d, got %d", ErrInvalidRetry, maxRetries, cfg.Retry.MaxRetries)
	}
	if cfg.Retry.Delay < 0 {
		return fmt.Errorf("%w: delay cannot be negative", ErrInvalidRetry)
	}
	if cfg.Retry.BackoffFactor < minBackoffFactor || cfg.Retry.BackoffFactor > maxBackoffFactor {
		return fmt.Errorf("%w: factor must be %.1f-%.1f, got %.1f", ErrInvalidRetry, minBackoffFactor, maxBackoffFactor, cfg.Retry.BackoffFactor)
	}

	// Validate MiddlewareConfig
	if cfg.Middleware.MaxRedirects < 0 || cfg.Middleware.MaxRedirects > 50 {
		return fmt.Errorf("MaxRedirects must be 0-50, got %d", cfg.Middleware.MaxRedirects)
	}

	if len(cfg.Middleware.UserAgent) > maxUserAgentLen || !validation.IsValidHeaderString(cfg.Middleware.UserAgent) {
		return fmt.Errorf("UserAgent invalid: max %d chars, no control characters", maxUserAgentLen)
	}

	for key, value := range cfg.Middleware.Headers {
		if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidHeader, key, err)
		}
	}

	return nil
}

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
