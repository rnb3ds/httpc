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

// JSON unmarshals the response body into the provided interface
func (r *Response) JSON(v any) error {
	if r.RawBody == nil {
		return fmt.Errorf("response body is empty")
	}

	const maxJSONSize = 50 * 1024 * 1024 // 50MB
	if len(r.RawBody) > maxJSONSize {
		return fmt.Errorf("response body too large for JSON parsing (%d bytes, max 50MB)", len(r.RawBody))
	}

	if err := validateJSONComplexity(r.RawBody); err != nil {
		return err
	}

	return json.Unmarshal(r.RawBody, v)
}

func validateJSONComplexity(data []byte) error {
	const maxDepth = 100
	const maxBrackets = 10000

	depth := 0
	maxDepthSeen := 0
	bracketCount := 0

	for _, ch := range data {
		switch ch {
		case '{', '[':
			depth++
			bracketCount++
			if depth > maxDepthSeen {
				maxDepthSeen = depth
			}
			if depth > maxDepth {
				return fmt.Errorf("JSON nesting too deep (max depth %d)", maxDepth)
			}
			if bracketCount > maxBrackets {
				return fmt.Errorf("JSON structure too complex (too many nested structures)")
			}
		case '}', ']':
			depth--
		}
	}

	return nil
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

type RequestOption func(*Request)

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

type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
	Method     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s %s", e.StatusCode, e.Method, e.URL)
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

// ValidateConfig validates the configuration with reasonable limits
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if cfg.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative, got %v", cfg.Timeout)
	}
	if cfg.Timeout > 30*time.Minute {
		return fmt.Errorf("timeout too large (max 30 minutes), got %v", cfg.Timeout)
	}

	if cfg.MaxIdleConns < 0 {
		return fmt.Errorf("MaxIdleConns cannot be negative, got %d", cfg.MaxIdleConns)
	}
	if cfg.MaxIdleConns > 1000 {
		return fmt.Errorf("MaxIdleConns too large (max 1000), got %d", cfg.MaxIdleConns)
	}
	if cfg.MaxConnsPerHost < 0 {
		return fmt.Errorf("MaxConnsPerHost cannot be negative, got %d", cfg.MaxConnsPerHost)
	}
	if cfg.MaxConnsPerHost > 1000 {
		return fmt.Errorf("MaxConnsPerHost too large (max 1000), got %d", cfg.MaxConnsPerHost)
	}

	if cfg.MaxResponseBodySize < 0 {
		return fmt.Errorf("MaxResponseBodySize cannot be negative, got %d", cfg.MaxResponseBodySize)
	}

	if cfg.MaxRetries < 0 {
		return fmt.Errorf("MaxRetries cannot be negative, got %d", cfg.MaxRetries)
	}
	if cfg.MaxRetries > 10 {
		return fmt.Errorf("MaxRetries too large (max 10), got %d", cfg.MaxRetries)
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("RetryDelay cannot be negative, got %v", cfg.RetryDelay)
	}
	if cfg.BackoffFactor < 1.0 {
		return fmt.Errorf("BackoffFactor must be at least 1.0, got %f", cfg.BackoffFactor)
	}

	if strings.ContainsAny(cfg.UserAgent, "\r\n\x00") {
		return fmt.Errorf("UserAgent contains invalid control characters")
	}

	if cfg.Headers != nil {
		for key, value := range cfg.Headers {
			if err := validateHeaderKeyValue(key, value); err != nil {
				return fmt.Errorf("invalid header %s: %w", key, err)
			}
		}
	}

	return nil
}

func validateHeaderKeyValue(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("header key cannot be empty")
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

	if strings.HasPrefix(key, ":") {
		return fmt.Errorf("pseudo-headers are not allowed")
	}

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
