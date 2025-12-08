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

// Response represents an HTTP response.
// Deprecated: Use Result instead for new code.
// This type is maintained for backward compatibility only.
type Response struct {
	StatusCode     int
	Status         string
	Headers        http.Header
	Body           string
	RawBody        []byte
	ContentLength  int64
	Duration       time.Duration
	Attempts       int
	Cookies        []*http.Cookie
	RedirectChain  []string
	RedirectCount  int
	RequestHeaders http.Header
}

// IsSuccess returns true if the response status code indicates success (2xx).
func (r *Response) IsSuccess() bool {
	code := r.StatusCode
	return code >= 200 && code < 300
}

// IsRedirect returns true if the response status code indicates a redirect (3xx).
func (r *Response) IsRedirect() bool {
	code := r.StatusCode
	return code >= 300 && code < 400
}

// IsClientError returns true if the response status code indicates a client error (4xx).
func (r *Response) IsClientError() bool {
	code := r.StatusCode
	return code >= 400 && code < 500
}

// IsServerError returns true if the response status code indicates a server error (5xx).
func (r *Response) IsServerError() bool {
	code := r.StatusCode
	return code >= 500 && code < 600
}

// JSON unmarshals the response body into the provided interface.
func (r *Response) JSON(v any) error {
	bodyLen := len(r.RawBody)
	if bodyLen == 0 {
		return ErrResponseBodyEmpty
	}
	if bodyLen > maxJSONSize {
		return fmt.Errorf("%w: %d bytes exceeds 50MB", ErrResponseBodyTooLarge, bodyLen)
	}
	return json.Unmarshal(r.RawBody, v)
}

// GetCookie returns a specific cookie from the response by name.
func (r *Response) GetCookie(name string) *http.Cookie {
	for _, cookie := range r.Cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// HasCookie checks if a specific cookie exists in the response.
func (r *Response) HasCookie(name string) bool {
	return r.GetCookie(name) != nil
}

// GetRequestCookies extracts cookies from the request Cookie header.
func (r *Response) GetRequestCookies() []*http.Cookie {
	if r.RequestHeaders == nil {
		return nil
	}
	cookieHeader := r.RequestHeaders.Get("Cookie")
	if cookieHeader == "" {
		return nil
	}
	return parseCookieHeader(cookieHeader)
}

// GetRequestCookie returns a specific cookie from the request by name.
func (r *Response) GetRequestCookie(name string) *http.Cookie {
	cookies := r.GetRequestCookies()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// HasRequestCookie checks if a specific cookie was sent in the request.
func (r *Response) HasRequestCookie(name string) bool {
	return r.GetRequestCookie(name) != nil
}

// String returns a formatted string representation of the response.
func (r *Response) String() string {
	if r == nil {
		return "<nil Response>"
	}

	var b strings.Builder
	b.Grow(256)
	b.WriteString("Response{Status: ")
	b.WriteString(itoa(r.StatusCode))
	b.WriteByte(' ')
	b.WriteString(r.Status)
	b.WriteString(", ContentLength: ")
	b.WriteString(itoa64(r.ContentLength))
	b.WriteString(", Duration: ")
	b.WriteString(r.Duration.String())
	b.WriteString(", Attempts: ")
	b.WriteString(itoa(r.Attempts))

	if len(r.Headers) > 0 {
		b.WriteString(", Headers: ")
		b.WriteString(itoa(len(r.Headers)))
	}
	if len(r.Cookies) > 0 {
		b.WriteString(", Cookies: ")
		b.WriteString(itoa(len(r.Cookies)))
	}
	if len(r.Body) > 0 {
		b.WriteString(", Body: \n")
		b.WriteString(r.Body)
	}
	b.WriteByte('}')
	return b.String()
}

// Html returns the response body as HTML content.
func (r *Response) Html() string {
	if r == nil {
		return ""
	}
	return r.Body
}

const (
	maxJSONSize         = 50 * 1024 * 1024
	maxTimeout          = 30 * time.Minute
	maxIdleConns        = 1000
	maxConnsPerHost     = 1000
	maxResponseBodySize = 1024 * 1024 * 1024
	maxRetries          = 10
	minBackoffFactor    = 1.0
	maxBackoffFactor    = 10.0
	maxUserAgentLen     = 512
	maxHeaderKeyLen     = 256
	maxHeaderValueLen   = 8192
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
	AllowPrivateIPs     bool
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
		AllowPrivateIPs:     false,
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
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrNilConfig
	}

	if cfg.Timeout < 0 || cfg.Timeout > maxTimeout {
		return fmt.Errorf("%w: must be 0-%v, got %v", ErrInvalidTimeout, maxTimeout, cfg.Timeout)
	}

	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > maxIdleConns {
		return fmt.Errorf("MaxIdleConns must be 0-%d, got %d", maxIdleConns, cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 || cfg.MaxConnsPerHost > maxConnsPerHost {
		return fmt.Errorf("MaxConnsPerHost must be 0-%d, got %d", maxConnsPerHost, cfg.MaxConnsPerHost)
	}

	if cfg.MaxResponseBodySize < 0 || cfg.MaxResponseBodySize > maxResponseBodySize {
		return fmt.Errorf("MaxResponseBodySize must be 0-1GB, got %d", cfg.MaxResponseBodySize)
	}

	if cfg.MaxRetries < 0 || cfg.MaxRetries > maxRetries {
		return fmt.Errorf("%w: must be 0-%d, got %d", ErrInvalidRetry, maxRetries, cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("%w: delay cannot be negative", ErrInvalidRetry)
	}
	if cfg.BackoffFactor < minBackoffFactor || cfg.BackoffFactor > maxBackoffFactor {
		return fmt.Errorf("%w: factor must be %.1f-%.1f, got %.1f", ErrInvalidRetry, minBackoffFactor, maxBackoffFactor, cfg.BackoffFactor)
	}

	if cfg.MaxRedirects < 0 || cfg.MaxRedirects > 50 {
		return fmt.Errorf("MaxRedirects must be 0-50, got %d", cfg.MaxRedirects)
	}

	if len(cfg.UserAgent) > maxUserAgentLen || !isValidHeaderString(cfg.UserAgent) {
		return fmt.Errorf("UserAgent invalid: max %d chars, no control characters", maxUserAgentLen)
	}

	for key, value := range cfg.Headers {
		if err := validateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidHeader, key, err)
		}
	}

	return nil
}

func validateHeaderKeyValue(key, value string) error {
	keyLen := len(key)
	if keyLen == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if keyLen > maxHeaderKeyLen {
		return fmt.Errorf("key too long (max %d)", maxHeaderKeyLen)
	}
	if key[0] == ':' {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	// Validate key characters (no tab allowed in keys)
	for i := range keyLen {
		c := key[i]
		if c < 0x20 || c == 0x7F {
			return fmt.Errorf("invalid characters in key")
		}
	}

	valueLen := len(value)
	if valueLen > maxHeaderValueLen {
		return fmt.Errorf("value too long (max %d)", maxHeaderValueLen)
	}

	// Validate value characters (tab allowed in values)
	for i := range valueLen {
		c := value[i]
		if (c < 0x20 && c != 0x09) || c == 0x7F {
			return fmt.Errorf("invalid characters in value")
		}
	}

	return nil
}

func isValidHeaderString(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < 0x20 && c != 0x09) || c == 0x7F {
			return false
		}
	}
	return true
}
