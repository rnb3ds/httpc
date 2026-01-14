package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"

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
	maxHeaderKeyLen     = 256                // Header key length limit
	maxHeaderValueLen   = 8192               // Header value length limit
)

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
	MaxRedirects    int
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
		AllowPrivateIPs:     true,
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
		if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalidHeader, key, err)
		}
	}

	return nil
}

func isValidHeaderString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 0x20 && c != 0x09) || c == 0x7F || c == '\r' || c == '\n' {
			return false
		}
	}
	return true
}
