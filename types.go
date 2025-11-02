package httpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// Response represents an HTTP response
type Response struct {
	StatusCode    int
	Status        string
	Headers       http.Header
	Body          string
	RawBody       []byte
	ContentLength int64
	Duration      time.Duration
	Attempts      int
	Cookies       []*http.Cookie
}

// IsSuccess returns true if the response status code indicates success (2xx)
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsRedirect returns true if the response status code indicates a redirect (3xx)
func (r *Response) IsRedirect() bool {
	return r.StatusCode >= 300 && r.StatusCode < 400
}

// IsClientError returns true if the response status code indicates a client error (4xx)
func (r *Response) IsClientError() bool {
	return r.StatusCode >= 400 && r.StatusCode < 500
}

// IsServerError returns true if the response status code indicates a server error (5xx)
func (r *Response) IsServerError() bool {
	return r.StatusCode >= 500 && r.StatusCode < 600
}

// JSON unmarshals the response body into the provided interface with security validation
func (r *Response) JSON(v any) error {
	if r.RawBody == nil {
		return fmt.Errorf("response body is empty")
	}

	// Reasonable size limit for JSON parsing
	if len(r.RawBody) > 50*1024*1024 { // 50MB limit
		return fmt.Errorf("response body too large for JSON parsing (%d bytes, maxInt 50MB)", len(r.RawBody))
	}

	// Simple JSON bomb protection - check for excessive repetition
	bodyStr := string(r.RawBody)

	// Quick check for obvious JSON bombs
	if strings.Count(bodyStr, "{") > 10000 || strings.Count(bodyStr, "[") > 10000 {
		return fmt.Errorf("JSON structure too complex (potential JSON bomb)")
	}

	// Simple depth check by counting nested brackets
	maxDepth := 0
	currentDepth := 0
	for _, char := range bodyStr {
		switch char {
		case '{', '[':
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
			if maxDepth > 500 { // Reasonable depth limit
				return fmt.Errorf("JSON nesting too deep (maxInt 500 levels)")
			}
		case '}', ']':
			currentDepth--
		}
	}

	// Use standard JSON unmarshaling
	return json.Unmarshal(r.RawBody, v)
}

// GetCookie returns the named cookie from the response or nil if not found
func (r *Response) GetCookie(name string) *http.Cookie {
	for _, cookie := range r.Cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// HasCookie returns true if the response contains a cookie with the given name
func (r *Response) HasCookie(name string) bool {
	return r.GetCookie(name) != nil
}

// Config represents the client configuration
type Config struct {
	// Network settings
	Timeout         time.Duration
	MaxIdleConns    int
	MaxConnsPerHost int
	ProxyURL        string

	// Security settings
	TLSConfig           *tls.Config
	MinTLSVersion       uint16 // Minimum TLS version (e.g., tls.VersionTLS12)
	MaxTLSVersion       uint16 // Maximum TLS version (e.g., tls.VersionTLS13)
	InsecureSkipVerify  bool
	MaxResponseBodySize int64
	AllowPrivateIPs     bool
	StrictContentLength bool

	// Retry settings
	MaxRetries    int
	RetryDelay    time.Duration
	BackoffFactor float64

	// Headers and features
	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	EnableHTTP2     bool
	EnableCookies   bool
}

// RequestOption defines a function that modifies a request
type RequestOption func(*Request)

// Request represents an HTTP request configuration
type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]any
	Body        any
	Timeout     time.Duration
	MaxRetries  int
	Context     context.Context
	Cookies     []*http.Cookie // Cookies to send with this request
}

// FormData represents form data for multipart/form-data requests
type FormData struct {
	Fields map[string]string
	Files  map[string]*FileData
}

// FileData represents a file to be uploaded
type FileData struct {
	Filename    string
	Content     []byte
	ContentType string // Optional: MIME type of the file (e.g., "application/pdf", "image/jpeg")
}

// HTTPError represents an HTTP error response (public API)
type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
	Method     string
}

// Error returns the error message
func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s %s", e.StatusCode, e.Method, e.URL)
}

// DefaultConfig returns a configuration with secure defaults
func DefaultConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second, // Reduced for better responsiveness
		MaxIdleConns:        50,               // Reduced for better resource management
		MaxConnsPerHost:     10,               // Reduced to prevent overwhelming servers
		MinTLSVersion:       tls.VersionTLS12, // Minimum TLS 1.2
		MaxTLSVersion:       tls.VersionTLS13, // Maximum TLS 1.3
		InsecureSkipVerify:  false,            // Always secure by default
		MaxResponseBodySize: 10 * 1024 * 1024, // 10MB - more reasonable default
		AllowPrivateIPs:     false,            // Secure by default
		StrictContentLength: true,             // Strict by default for security
		MaxRetries:          3,                // Reasonable retry count
		RetryDelay:          1 * time.Second,  // Faster initial retry
		BackoffFactor:       2.0,              // Standard exponential backoff
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         true,
		EnableCookies:       false, // Disabled by default for security
	}
}

// NewCookieJar creates a new cookie jar with default options
func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
}

// ValidateConfig validates the configuration with reasonable limits
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate timeout settings - be more permissive
	if cfg.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative, got %v", cfg.Timeout)
	}
	if cfg.Timeout > 30*time.Minute { // Increased limit for long-running operations
		return fmt.Errorf("timeout too large (maxInt 30 minutes), got %v", cfg.Timeout)
	}

	// Validate connection pool settings - more reasonable limits
	if cfg.MaxIdleConns < 0 {
		return fmt.Errorf("MaxIdleConns cannot be negative, got %d", cfg.MaxIdleConns)
	}
	if cfg.MaxIdleConns > 1000 {
		return fmt.Errorf("MaxIdleConns too large (maxInt 1000), got %d", cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 {
		return fmt.Errorf("MaxConnsPerHost cannot be negative, got %d", cfg.MaxConnsPerHost)
	}
	if cfg.MaxConnsPerHost > 1000 {
		return fmt.Errorf("MaxConnsPerHost too large (maxInt 1000), got %d", cfg.MaxConnsPerHost)
	}

	// Validate response body size limits
	if cfg.MaxResponseBodySize < 0 {
		return fmt.Errorf("MaxResponseBodySize cannot be negative, got %d", cfg.MaxResponseBodySize)
	}

	// Validate retry settings - more permissive
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("MaxRetries cannot be negative, got %d", cfg.MaxRetries)
	}
	if cfg.MaxRetries > 10 {
		return fmt.Errorf("MaxRetries too large (maxInt 10), got %d", cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("RetryDelay cannot be negative, got %v", cfg.RetryDelay)
	}
	if cfg.BackoffFactor < 1.0 {
		return fmt.Errorf("BackoffFactor must be at least 1.0, got %f", cfg.BackoffFactor)
	}

	// Validate User-Agent - basic validation only
	if strings.ContainsAny(cfg.UserAgent, "\r\n\x00") {
		return fmt.Errorf("UserAgent contains invalid control characters")
	}

	// Validate headers map - basic validation
	if cfg.Headers != nil {
		for key, value := range cfg.Headers {
			if err := validateHeaderKeyValue(key, value); err != nil {
				return fmt.Errorf("invalid header %s: %w", key, err)
			}
		}
	}

	return nil
}

// validateHeaderKeyValue validates a single header key-value pair
func validateHeaderKeyValue(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("header key cannot be empty or whitespace-only")
	}
	if len(key) > 256 {
		return fmt.Errorf("header key too long (maxInt 256 characters)")
	}
	if len(value) > 8192 {
		return fmt.Errorf("header value too long (maxInt 8KB)")
	}
	if strings.ContainsAny(key, "\r\n\x00") {
		return fmt.Errorf("header key contains invalid characters")
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("header value contains invalid characters")
	}

	// Check for HTTP/2 pseudo-headers (should not be set manually)
	if strings.HasPrefix(key, ":") {
		return fmt.Errorf("pseudo-headers (starting with ':') are not allowed")
	}

	// Check for headers that should be managed automatically
	keyLower := strings.ToLower(key)
	switch keyLower {
	case "content-length", "transfer-encoding", "connection", "upgrade":
		return fmt.Errorf("header '%s' is managed automatically", key)
	}

	return nil
}

// FormatBytes formats bytes in human-readable format (e.g., "1.50 KB", "2.30 MB")
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed formats speed in human-readable format (e.g., "1.50 KB/s", "2.30 MB/s")
func FormatSpeed(bytesPerSecond float64) string {
	const unit = 1024.0
	if bytesPerSecond < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}

	units := []string{"KB/s", "MB/s", "GB/s", "TB/s", "PB/s", "EB/s"}
	div := unit
	exp := 0

	for bytesPerSecond >= div*unit && exp < len(units)-1 {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[exp])
}
