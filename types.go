package httpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// Response represents an HTTP response with enhanced functionality
type Response struct {
	StatusCode    int
	Status        string
	Headers       http.Header
	Body          string
	RawBody       []byte
	ContentLength int64
	Proto         string
	Duration      time.Duration
	Attempts      int
	Request       interface{} // *http.Request
	Response      interface{} // *http.Response
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

// JSON unmarshals the response body into the provided interface
func (r *Response) JSON(v interface{}) error {
	return json.Unmarshal(r.RawBody, v)
}

// XML unmarshals the response body into the provided interface
func (r *Response) XML(v interface{}) error {
	return xml.Unmarshal(r.RawBody, v)
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

// Config represents the unified client configuration
type Config struct {
	// Network settings
	Timeout               time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	ProxyURL              string

	// Security settings
	TLSConfig             *tls.Config
	MinTLSVersion         uint16
	MaxTLSVersion         uint16
	InsecureSkipVerify    bool
	MaxResponseBodySize   int64
	MaxConcurrentRequests int
	ValidateURL           bool
	ValidateHeaders       bool
	AllowPrivateIPs       bool // Allow requests to private/internal IPs (for testing)

	// Retry settings
	MaxRetries    int
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	BackoffFactor float64
	Jitter        bool

	// Headers and features
	UserAgent       string
	Headers         map[string]string
	FollowRedirects bool
	EnableHTTP2     bool

	// Cookie settings
	CookieJar     http.CookieJar // Custom cookie jar, if nil and EnableCookies is true, a default jar will be created
	EnableCookies bool           // Enable automatic cookie management
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
	ContentType string
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

// DefaultConfig returns a configuration with secure and optimized defaults
func DefaultConfig() *Config {
	return &Config{
		Timeout:               60 * time.Second,
		DialTimeout:           15 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,

		MinTLSVersion:         tls.VersionTLS12,
		MaxTLSVersion:         tls.VersionTLS13,
		InsecureSkipVerify:    false,
		MaxResponseBodySize:   50 * 1024 * 1024,
		MaxConcurrentRequests: 500,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       false,

		MaxRetries:    2,
		RetryDelay:    2 * time.Second,
		MaxRetryDelay: 60 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,

		UserAgent:       "httpc/1.0 (Go HTTP Client)",
		Headers:         make(map[string]string),
		FollowRedirects: true,
		EnableHTTP2:     true,

		EnableCookies: true,
	}
}

// NewCookieJar creates a new cookie jar with default options
func NewCookieJar() (http.CookieJar, error) {
	return cookiejar.New(&cookiejar.Options{
		PublicSuffixList: nil,
	})
}
