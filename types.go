package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
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
	maxHeaderKeyLen     = 256                // Header key length limit
	maxHeaderValueLen   = 8192               // Header value length limit
)

// Config defines the HTTP client configuration.
//
// Thread Safety:
// Config must be treated as immutable after passing it to New().
// Do not modify Config fields after client creation.
// Create a new Config for each client with different settings.
type Config struct {
	Timeout         time.Duration
	MaxIdleConns    int
	MaxConnsPerHost int
	ProxyURL        string

	TLSConfig           *tls.Config
	MinTLSVersion       uint16
	MaxTLSVersion       uint16
	InsecureSkipVerify  bool
	MaxResponseBodySize int64
	AllowPrivateIPs     bool // Allow requests to private/reserved IP ranges (default: true for usability)
	StrictContentLength bool

	MaxRetries    int
	RetryDelay    time.Duration
	BackoffFactor float64

	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	MaxRedirects    int // Maximum number of redirects to follow (default: 10, 0 = no limit)
	EnableHTTP2     bool
	EnableCookies   bool
}

type RequestOption func(*Request) error

type Request struct {
	Method          string
	URL             string
	Headers         map[string]string
	QueryParams     map[string]any
	Body            any
	Timeout         time.Duration
	MaxRetries      int
	Context         context.Context
	Cookies         []http.Cookie
	FollowRedirects *bool // Override client's FollowRedirects setting (nil = use client default)
	MaxRedirects    *int  // Override client's MaxRedirects setting (nil = use client default)
}

type FormData struct {
	Fields map[string]string
	Files  map[string]*FileData
}

type FileData struct {
	Filename    string
	Content     []byte
	ContentType string
}

func DefaultConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second,
		MaxIdleConns:        50,
		MaxConnsPerHost:     10,
		MinTLSVersion:       tls.VersionTLS12,
		MaxTLSVersion:       tls.VersionTLS13,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 10 * 1024 * 1024,
		AllowPrivateIPs:     true, // Default allow private IPs to prevent SSRF
		StrictContentLength: true,
		MaxRetries:          3,
		RetryDelay:          1 * time.Second,
		BackoffFactor:       2.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		MaxRedirects:        10,
		EnableHTTP2:         true,
		EnableCookies:       false,
	}
}

func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
}

// ValidateConfig validates the configuration with reasonable limits.
// ValidateConfig validates the configuration with comprehensive security checks.
// All limits are enforced to prevent resource exhaustion and security issues.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrNilConfig
	}

	// Timeout validation - prevent excessive timeouts
	if cfg.Timeout < 0 || cfg.Timeout > maxTimeout {
		return fmt.Errorf("%w: must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeout)
	}

	// Connection pool validation - prevent resource exhaustion
	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > maxIdleConns {
		return fmt.Errorf("MaxIdleConns must be 0-%d, got %d", maxIdleConns, cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 || cfg.MaxConnsPerHost > maxConnsPerHost {
		return fmt.Errorf("MaxConnsPerHost must be 0-%d, got %d", maxConnsPerHost, cfg.MaxConnsPerHost)
	}

	// Response size validation - prevent memory exhaustion
	if cfg.MaxResponseBodySize < 0 || cfg.MaxResponseBodySize > maxResponseBodySize {
		return fmt.Errorf("MaxResponseBodySize must be 0-1GB, got %d", cfg.MaxResponseBodySize)
	}

	// Retry validation - prevent infinite loops
	if cfg.MaxRetries < 0 || cfg.MaxRetries > maxRetries {
		return fmt.Errorf("%w: must be 0-%d, got %d", ErrInvalidRetry, maxRetries, cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("%w: delay cannot be negative", ErrInvalidRetry)
	}
	if cfg.BackoffFactor < minBackoffFactor || cfg.BackoffFactor > maxBackoffFactor {
		return fmt.Errorf("%w: factor must be %.1f-%.1f, got %.1f", ErrInvalidRetry, minBackoffFactor, maxBackoffFactor, cfg.BackoffFactor)
	}

	// Redirect validation - prevent redirect loops
	if cfg.MaxRedirects < 0 || cfg.MaxRedirects > 50 {
		return fmt.Errorf("MaxRedirects must be 0-50, got %d", cfg.MaxRedirects)
	}

	// User-Agent validation - prevent header injection
	if len(cfg.UserAgent) > maxUserAgentLen || !isValidHeaderString(cfg.UserAgent) {
		return fmt.Errorf("UserAgent invalid: max %d chars, no control characters", maxUserAgentLen)
	}

	// Header validation - prevent header injection attacks
	for key, value := range cfg.Headers {
		if err := validateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidHeader, key, err)
		}
	}

	return nil
}

// validateHeaderKeyValue performs comprehensive header validation to prevent injection attacks.
// Validates both key and value according to HTTP/1.1 and HTTP/2 specifications.
func validateHeaderKeyValue(key, value string) error {
	keyLen := len(key)
	if keyLen == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if keyLen > maxHeaderKeyLen {
		return fmt.Errorf("key too long (max %d)", maxHeaderKeyLen)
	}

	// Prevent HTTP/2 pseudo-header injection
	if key[0] == ':' {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	// Validate key characters - strict validation for security
	for i := range keyLen {
		c := key[i]
		if c < 0x20 || c == 0x7F {
			return fmt.Errorf("invalid characters in key")
		}
		// Additional validation for HTTP header field names (RFC 7230)
		if c == ' ' || c == '\t' || c == '(' || c == ')' || c == '<' || c == '>' ||
			c == '@' || c == ',' || c == ';' || c == ':' || c == '\\' || c == '"' ||
			c == '/' || c == '[' || c == ']' || c == '?' || c == '=' || c == '{' || c == '}' {
			return fmt.Errorf("invalid character in header key: %c", c)
		}
	}

	valueLen := len(value)
	if valueLen > maxHeaderValueLen {
		return fmt.Errorf("value too long (max %d)", maxHeaderValueLen)
	}

	// Validate value characters - allow tab but reject other control chars
	for i := range valueLen {
		c := value[i]
		if (c < 0x20 && c != 0x09) || c == 0x7F {
			return fmt.Errorf("invalid characters in value")
		}
	}

	// Validate common header values to prevent protocol errors
	keyLower := strings.ToLower(key)
	valueLower := strings.ToLower(value)

	switch keyLower {
	case "connection":
		if valueLower != "keep-alive" && valueLower != "close" && valueLower != "upgrade" {
			return fmt.Errorf("invalid Connection header value: %q (expected: keep-alive, close, or upgrade)", value)
		}
	case "content-length":
		// Prevent negative content-length attacks
		if strings.HasPrefix(valueLower, "-") {
			return fmt.Errorf("negative Content-Length not allowed")
		}
	case "host":
		// Prevent host header injection
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("CRLF injection detected in Host header")
		}
	}

	// Check for CRLF injection in any header
	if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("CRLF injection detected")
	}

	return nil
}

// isValidHeaderString validates header string for control characters and CRLF injection.
// More efficient than validateHeaderKeyValue for simple string validation.
func isValidHeaderString(s string) bool {
	for i := range len(s) {
		c := s[i]
		// Reject control characters except tab, and check for CRLF injection
		if (c < 0x20 && c != 0x09) || c == 0x7F || c == '\r' || c == '\n' {
			return false
		}
	}
	return true
}
