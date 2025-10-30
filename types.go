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

// JSON unmarshals the response body into the provided interface with enhanced security
func (r *Response) JSON(v any) error {
	if r.RawBody == nil {
		return fmt.Errorf("response body is empty")
	}

	// Limit JSON parsing size to prevent memory exhaustion attacks
	if len(r.RawBody) > 100*1024*1024 { // 100MB - increased for better usability
		return fmt.Errorf("response body too large for JSON parsing (%d bytes, max 100MB)", len(r.RawBody))
	}

	// More efficient JSON bomb detection
	bodyStr := string(r.RawBody)
	braceCount := strings.Count(bodyStr, "{") + strings.Count(bodyStr, "}")
	bracketCount := strings.Count(bodyStr, "[") + strings.Count(bodyStr, "]")

	// Check for excessive nesting indicators
	if braceCount > 50000 || bracketCount > 50000 {
		return fmt.Errorf("JSON structure too complex (potential JSON bomb): %d braces, %d brackets",
			braceCount, bracketCount)
	}

	// Check for excessive depth by looking for deeply nested patterns
	maxDepth := 0
	currentDepth := 0
	for _, char := range bodyStr {
		switch char {
		case '{', '[':
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
			if maxDepth > 1000 { // Reasonable depth limit
				return fmt.Errorf("JSON nesting too deep (max 1000 levels)")
			}
		case '}', ']':
			currentDepth--
		}
	}

	// Use standard unmarshaling with the pre-validation
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
	InsecureSkipVerify  bool
	MaxResponseBodySize int64
	AllowPrivateIPs     bool

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
	Filename string
	Content  []byte
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
		Timeout:             60 * time.Second,
		MaxIdleConns:        100,
		MaxConnsPerHost:     20,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 50 * 1024 * 1024, // 50MB
		AllowPrivateIPs:     false,
		MaxRetries:          2,
		RetryDelay:          2 * time.Second,
		BackoffFactor:       2.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true, // Allow redirects but with limits
		EnableHTTP2:         true,
		EnableCookies:       true,
	}
}

// NewCookieJar creates a new cookie jar with default options
func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
}

// ValidateConfig validates the security of the configuration
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate timeout settings with more detailed messages
	if cfg.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative, got %v", cfg.Timeout)
	}
	if cfg.Timeout > 10*time.Minute {
		return fmt.Errorf("timeout too large (max 10 minutes), got %v", cfg.Timeout)
	}

	// Validate connection pool settings with improved limits
	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > 2000 {
		return fmt.Errorf("MaxIdleConns must be between 0 and 2000, got %d", cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 || cfg.MaxConnsPerHost > 200 {
		return fmt.Errorf("MaxConnsPerHost must be between 0 and 200, got %d", cfg.MaxConnsPerHost)
	}

	// Validate logical relationship between connection settings
	if cfg.MaxConnsPerHost > 0 && cfg.MaxIdleConns > 0 {
		// Allow MaxConnsPerHost to be up to MaxIdleConns (more flexible)
		if cfg.MaxConnsPerHost > cfg.MaxIdleConns {
			// Auto-adjust MaxIdleConns to accommodate MaxConnsPerHost
			cfg.MaxIdleConns = cfg.MaxConnsPerHost * 2
			if cfg.MaxIdleConns > 2000 {
				cfg.MaxIdleConns = 2000
			}
		}
	}

	// Validate response body size limits
	if cfg.MaxResponseBodySize < 0 {
		return fmt.Errorf("MaxResponseBodySize cannot be negative, got %d", cfg.MaxResponseBodySize)
	}
	if cfg.MaxResponseBodySize > 2*1024*1024*1024 { // 2GB
		return fmt.Errorf("MaxResponseBodySize too large (max 2GB), got %d", cfg.MaxResponseBodySize)
	}

	// Validate retry settings with improved limits
	if cfg.MaxRetries < 0 || cfg.MaxRetries > 20 {
		return fmt.Errorf("MaxRetries must be between 0 and 20, got %d", cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("RetryDelay cannot be negative, got %v", cfg.RetryDelay)
	}
	if cfg.RetryDelay > 1*time.Minute {
		return fmt.Errorf("RetryDelay too large (max 1 minute), got %v", cfg.RetryDelay)
	}
	if cfg.BackoffFactor < 1.0 || cfg.BackoffFactor > 10.0 {
		return fmt.Errorf("BackoffFactor must be between 1.0 and 10.0, got %f", cfg.BackoffFactor)
	}

	// Validate User-Agent with improved limits
	if len(cfg.UserAgent) > 1024 {
		return fmt.Errorf("UserAgent too long (max 1024 characters), got %d", len(cfg.UserAgent))
	}

	// Check for dangerous characters in User-Agent
	if strings.ContainsAny(cfg.UserAgent, "\r\n\x00") {
		return fmt.Errorf("UserAgent contains invalid control characters")
	}

	// Validate headers map with improved validation
	if cfg.Headers != nil {
		if len(cfg.Headers) > 100 {
			return fmt.Errorf("too many default headers (max 100), got %d", len(cfg.Headers))
		}

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
		return fmt.Errorf("header key too long (max 256 characters)")
	}
	if len(value) > 8192 {
		return fmt.Errorf("header value too long (max 8KB)")
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
