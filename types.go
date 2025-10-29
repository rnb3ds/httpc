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
	if len(r.RawBody) > 50*1024*1024 { // 50MB
		return fmt.Errorf("response body too large for JSON parsing")
	}

	// Check for potential JSON bombs (deeply nested structures)
	if strings.Count(string(r.RawBody), "{") > 10000 ||
		strings.Count(string(r.RawBody), "[") > 10000 {
		return fmt.Errorf("JSON structure too complex (potential JSON bomb)")
	}

	// Use standard unmarshaling but with pre-validation
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

	// Validate timeout settings
	if cfg.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	if cfg.Timeout > 10*time.Minute {
		return fmt.Errorf("timeout too large (max 10 minutes)")
	}

	// Validate connection pool settings
	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > 1000 {
		return fmt.Errorf("MaxIdleConns must be between 0 and 1000")
	}
	if cfg.MaxConnsPerHost < 0 || cfg.MaxConnsPerHost > 100 {
		return fmt.Errorf("MaxConnsPerHost must be between 0 and 100")
	}

	// Validate response body size limits
	if cfg.MaxResponseBodySize < 0 {
		return fmt.Errorf("MaxResponseBodySize cannot be negative")
	}
	if cfg.MaxResponseBodySize > 1024*1024*1024 { // 1GB
		return fmt.Errorf("MaxResponseBodySize too large (max 1GB)")
	}

	// Validate retry settings
	if cfg.MaxRetries < 0 || cfg.MaxRetries > 10 {
		return fmt.Errorf("MaxRetries must be between 0 and 10")
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("RetryDelay cannot be negative")
	}
	if cfg.BackoffFactor < 1.0 || cfg.BackoffFactor > 10.0 {
		return fmt.Errorf("BackoffFactor must be between 1.0 and 10.0")
	}

	// Validate User-Agent
	if len(cfg.UserAgent) > 512 {
		return fmt.Errorf("UserAgent too long (max 512 characters)")
	}

	// Check for dangerous characters in User-Agent
	if strings.ContainsAny(cfg.UserAgent, "\r\n\x00") {
		return fmt.Errorf("UserAgent contains invalid characters")
	}

	// Validate headers map
	if cfg.Headers != nil {
		for key, value := range cfg.Headers {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("header key cannot be empty")
			}
			if len(key) > 256 || len(value) > 8192 {
				return fmt.Errorf("header key or value too long")
			}
			if strings.ContainsAny(key, "\r\n\x00") || strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("header contains invalid characters")
			}
		}
	}

	return nil
}
