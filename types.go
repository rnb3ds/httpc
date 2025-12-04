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
//
// Thread Safety:
// Response objects are immutable after creation and safe to read from
// multiple goroutines concurrently. All methods are goroutine-safe.
// Do not modify Response fields directly.
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

// JSON unmarshals the response body into the provided interface.
// Returns ErrResponseBodyEmpty if the body is nil or empty.
// Returns ErrResponseBodyTooLarge if the body exceeds 50MB.
func (r *Response) JSON(v any) error {
	if len(r.RawBody) == 0 {
		return ErrResponseBodyEmpty
	}

	if len(r.RawBody) > maxJSONSize {
		return fmt.Errorf("%w: %d bytes exceeds 50MB", ErrResponseBodyTooLarge, len(r.RawBody))
	}

	return json.Unmarshal(r.RawBody, v)
}

func (r *Response) GetCookie(name string) *http.Cookie {
	for _, cookie := range r.Cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func (r *Response) HasCookie(name string) bool {
	return r.GetCookie(name) != nil
}

// String returns a formatted string representation of the response.
// Includes status code, status text, content length, duration, and attempts.
func (r *Response) String() string {
	if r == nil {
		return "<nil Response>"
	}

	var b strings.Builder
	b.Grow(256)

	b.WriteString("Response{")
	b.WriteString("Status: ")
	fmt.Fprintf(&b, "%d %s", r.StatusCode, r.Status)
	b.WriteString(", ContentLength: ")
	fmt.Fprintf(&b, "%d", r.ContentLength)
	b.WriteString(", Duration: ")
	b.WriteString(r.Duration.String())
	b.WriteString(", Attempts: ")
	fmt.Fprintf(&b, "%d", r.Attempts)

	if len(r.Headers) > 0 {
		b.WriteString(", Headers: ")
		fmt.Fprintf(&b, "%d", len(r.Headers))
	}

	if len(r.Cookies) > 0 {
		b.WriteString(", Cookies: ")
		fmt.Fprintf(&b, "%d", len(r.Cookies))
	}

	if len(r.Body) > 0 {
		b.WriteString(", Body: ")
		fmt.Fprintf(&b, "\n%s", r.Body)
	}

	b.WriteString("} ")

	return b.String()
}

// Html is an alias method for the r.Body property
func (r *Response) Html() string {
	if r == nil {
		return ""
	}

	var b strings.Builder
	b.Grow(1024)

	// Body section
	if len(r.Body) > 0 {
		b.WriteString(r.Body) // htmlEscape(r.Body)
	}

	return b.String()
}

// htmlEscape escapes special HTML characters to prevent XSS.
func htmlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := range len(s) {
		c := s[i]
		switch c {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

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
	EnableHTTP2     bool
	EnableCookies   bool
}

type RequestOption func(*Request) error

type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]any
	Body        any
	Timeout     time.Duration
	MaxRetries  int
	Context     context.Context
	Cookies     []*http.Cookie
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
// Returns ErrNilConfig if cfg is nil.
// Returns ErrInvalidTimeout if timeout is negative or exceeds 30 minutes.
// Returns ErrInvalidRetry if retry configuration is invalid.
// Returns ErrInvalidHeader if any header is invalid.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return ErrNilConfig
	}

	if cfg.Timeout < 0 {
		return fmt.Errorf("%w: cannot be negative", ErrInvalidTimeout)
	}
	if cfg.Timeout > maxTimeout {
		return fmt.Errorf("%w: exceeds %v", ErrInvalidTimeout, maxTimeout)
	}

	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > maxIdleConns {
		return fmt.Errorf("MaxIdleConns must be 0-%d, got %d", maxIdleConns, cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 || cfg.MaxConnsPerHost > maxConnsPerHost {
		return fmt.Errorf("MaxConnsPerHost must be 0-%d, got %d", maxConnsPerHost, cfg.MaxConnsPerHost)
	}

	if cfg.MaxResponseBodySize < 0 {
		return fmt.Errorf("MaxResponseBodySize cannot be negative")
	}
	if cfg.MaxResponseBodySize > maxResponseBodySize {
		return fmt.Errorf("MaxResponseBodySize exceeds 1GB")
	}

	if cfg.MaxRetries < 0 || cfg.MaxRetries > maxRetries {
		return fmt.Errorf("%w: must be 0-%d, got %d", ErrInvalidRetry, maxRetries, cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("%w: cannot be negative", ErrInvalidRetry)
	}
	if cfg.BackoffFactor < minBackoffFactor || cfg.BackoffFactor > maxBackoffFactor {
		return fmt.Errorf("%w: must be %.1f-%.1f, got %.1f", ErrInvalidRetry, minBackoffFactor, maxBackoffFactor, cfg.BackoffFactor)
	}

	if len(cfg.UserAgent) > maxUserAgentLen {
		return fmt.Errorf("UserAgent exceeds %d characters", maxUserAgentLen)
	}

	if !isValidHeaderString(cfg.UserAgent) {
		return fmt.Errorf("UserAgent contains invalid characters")
	}

	for key, value := range cfg.Headers {
		if err := validateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidHeader, key, err)
		}
	}

	return nil
}

func validateHeaderKeyValue(key, value string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	keyLen := len(key)
	if keyLen > maxHeaderKeyLen {
		return fmt.Errorf("key too long (max %d)", maxHeaderKeyLen)
	}
	if len(value) > maxHeaderValueLen {
		return fmt.Errorf("value too long (max %d)", maxHeaderValueLen)
	}
	if key[0] == ':' {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	if !isValidHeaderString(key) || !isValidHeaderString(value) {
		return fmt.Errorf("invalid characters")
	}

	// Fast path: check for managed headers without allocation
	if keyLen >= 7 && keyLen <= 17 {
		firstChar := key[0] | 0x20 // Convert to lowercase
		if firstChar == 'c' || firstChar == 't' || firstChar == 'u' {
			// Only allocate if we have a potential match
			lower := strings.ToLower(key)
			switch lower {
			case "connection", "content-length", "transfer-encoding", "upgrade":
				return fmt.Errorf("managed automatically")
			}
		}
	}

	return nil
}

func isValidHeaderString(s string) bool {
	for i := range len(s) {
		c := s[i]
		// Reject control characters (0x00-0x1F) except tab (0x09) and DEL (0x7F)
		// Tab is allowed in header values per RFC 7230
		if (c < 0x20 && c != 0x09) || c == 0x7F {
			return false
		}
	}
	return true
}
