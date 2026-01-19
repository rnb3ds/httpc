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

const (
	maxURLLen         = 2048
	maxHeaderKeyLen   = 256
	maxHeaderValueLen = 8192
)

func (v *Validator) validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	urlLen := len(urlStr)
	if urlLen > maxURLLen {
		return fmt.Errorf("URL too long (max %d)", maxURLLen)
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
	if isLocalhost(hostname) {
		return fmt.Errorf("localhost access blocked for security")
	}

	// If hostname is an IP address, validate it directly
	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateOrReservedIP(ip) {
			return fmt.Errorf("private/reserved IP blocked: %s", ip.String())
		}
		return nil
	}

	// For domain names, we rely on the connection pool's DNS resolution validation
	// This provides defense in depth against DNS rebinding attacks
	return nil
}

// isLocalhost detects localhost variations.
func isLocalhost(hostname string) bool {
	switch hostname {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	}

	if len(hostname) > 4 && hostname[:4] == "127." {
		return true
	}

	if len(hostname) > 10 && strings.HasPrefix(strings.ToLower(hostname), "localhost.") {
		return true
	}

	return false
}

// isPrivateOrReservedIP checks for private and reserved IP ranges.
func isPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] >= 240 || ip4[0] == 0 || (ip4[0] == 100 && (ip4[1]&0xC0) == 64) ||
			(ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)) {
			return true
		}
	}

	return false
}

func (v *Validator) validateHeader(key, value string) error {
	keyLen := len(key)
	if keyLen == 0 {
		return fmt.Errorf("header key cannot be empty")
	}

	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("header key cannot be empty")
	}

	if keyLen > maxHeaderKeyLen {
		return fmt.Errorf("invalid header key")
	}

	if key[0] == ':' {
		return fmt.Errorf("invalid header key")
	}

	for i := range keyLen {
		c := key[i]
		if c < 0x20 || c == 0x7F || !validation.IsValidHeaderChar(rune(c)) {
			return fmt.Errorf("header contains invalid characters")
		}
	}

	valueLen := len(value)
	if valueLen > maxHeaderValueLen {
		return fmt.Errorf("header value too long (max %d)", maxHeaderValueLen)
	}

	for i := range valueLen {
		c := value[i]
		if (c < 0x20 && c != 0x09) || c == 0x7F {
			return fmt.Errorf("header contains invalid characters")
		}
	}

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

