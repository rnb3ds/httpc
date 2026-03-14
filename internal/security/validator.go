package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/cybergodev/httpc/internal/validation"
)

type Validator struct {
	config *Config
}

// Compile-time interface check for RequestValidator
var _ RequestValidator = (*Validator)(nil)

// RequestValidator defines the interface for request validation.
type RequestValidator interface {
	ValidateRequest(req *Request) error
}

type Config struct {
	ValidateURL         bool
	ValidateHeaders     bool
	MaxResponseBodySize int64
	AllowPrivateIPs     bool
}

type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]any
	Body        any
}

func NewValidator() *Validator {
	secConfig := &Config{
		ValidateURL:         true,
		ValidateHeaders:     true,
		MaxResponseBodySize: 50 * 1024 * 1024,
		AllowPrivateIPs:     true,
	}

	return &Validator{
		config: secConfig,
	}
}

func NewValidatorWithConfig(config *Config) *Validator {
	if config == nil {
		return NewValidator()
	}

	return &Validator{
		config: config,
	}
}

func (v *Validator) ValidateRequest(req *Request) error {
	if v.config.ValidateURL {
		if err := v.validateURL(req.URL); err != nil {
			return err
		}
	}

	if v.config.ValidateHeaders {
		for key, value := range req.Headers {
			if err := v.validateHeader(key, value); err != nil {
				return fmt.Errorf("invalid header %s: %w", key, err)
			}
		}
	}

	if req.Body != nil {
		if err := v.validateRequestSize(req.Body); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	urlLen := len(urlStr)
	if urlLen > validation.MaxURLLen {
		return fmt.Errorf("URL too long (max %d)", validation.MaxURLLen)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL scheme is required")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("URL host is required")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}

	return v.validateHost(parsedURL.Host)
}

// validateHost performs comprehensive host validation to prevent SSRF attacks.
// Validates both before and after DNS resolution for maximum security.
func (v *Validator) validateHost(host string) error {
	if v.config.AllowPrivateIPs {
		return nil
	}

	// Extract hostname from host:port format
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// Check for localhost variations
	if validation.IsLocalhost(hostname) {
		return fmt.Errorf("localhost access blocked for security")
	}

	// If hostname is an IP address, validate it directly
	if ip := net.ParseIP(hostname); ip != nil {
		if err := validation.ValidateIP(ip); err != nil {
			return fmt.Errorf("private/reserved IP blocked: %s", ip.String())
		}
		return nil
	}

	// For domain names, we rely on the connection pool's DNS resolution validation
	// This provides defense in depth against DNS rebinding attacks
	return nil
}

func (v *Validator) validateHeader(key, value string) error {
	// Use common validation from validation package
	if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
		return err
	}

	// Additional validation for specific header values
	return validateCommonHeaderValue(key, value)
}

func validateCommonHeaderValue(key, value string) error {
	switch strings.ToLower(key) {
	case "connection":
		v := strings.ToLower(value)
		if v != "keep-alive" && v != "close" && v != "upgrade" {
			return fmt.Errorf("invalid Connection header value: %q", value)
		}
	case "transfer-encoding":
		v := strings.ToLower(value)
		if v != "chunked" && v != "compress" && v != "deflate" && v != "gzip" && v != "identity" {
			return fmt.Errorf("invalid Transfer-Encoding header value: %q", value)
		}
	}
	return nil
}

func (v *Validator) validateRequestSize(body any) error {
	if v.config.MaxResponseBodySize <= 0 {
		return nil
	}

	var size int64
	switch v := body.(type) {
	case string:
		size = int64(len(v))
	case []byte:
		size = int64(len(v))
	case url.Values:
		size = int64(len(v.Encode()))
	default:
		// For complex types (FormData, io.Reader, etc.), size validation
		// occurs during request serialization in the request processor
		return nil
	}

	if size > v.config.MaxResponseBodySize {
		return fmt.Errorf("request body exceeds %d bytes", v.config.MaxResponseBodySize)
	}

	return nil
}
