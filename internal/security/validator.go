package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
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
		AllowPrivateIPs:     false,
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

func (v *Validator) validateHost(host string) error {
	if v.config.AllowPrivateIPs {
		return nil
	}

	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	if isLocalhost(hostname) {
		return fmt.Errorf("localhost not allowed")
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateOrReservedIP(ip) {
			return fmt.Errorf("private IP not allowed: %s", ip.String())
		}
		return nil
	}

	return nil
}

func isLocalhost(hostname string) bool {
	hostnameLen := len(hostname)
	if hostnameLen == 0 {
		return false
	}

	// Fast path: check common cases without allocation
	if hostname[0] == '1' && hostnameLen >= 9 {
		if hostname == "127.0.0.1" {
			return true
		}
		if hostnameLen > 4 && hostname[:4] == "127." {
			return true
		}
	}

	// Check exact matches case-insensitively without allocation
	if hostnameLen == 9 && (hostname == "localhost" || hostname == "LOCALHOST" || hostname == "Localhost") {
		return true
	}
	if hostname == "::1" || hostname == "0.0.0.0" || hostname == "::" {
		return true
	}

	// Only allocate for prefix check
	if hostnameLen > 10 {
		lower := strings.ToLower(hostname)
		return strings.HasPrefix(lower, "localhost.")
	}

	return false
}

func isPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] >= 240 || ip4[0] == 0 {
			return true
		}
	}
	return false
}

func (v *Validator) validateHeader(key, value string) error {
	if key == "" || strings.TrimSpace(key) == "" {
		return fmt.Errorf("header key cannot be empty")
	}

	keyLen := len(key)
	if keyLen > maxHeaderKeyLen {
		return fmt.Errorf("header key too long (max %d)", maxHeaderKeyLen)
	}
	if key[0] == ':' {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	// Validate key characters - reject control characters
	for i := range keyLen {
		c := key[i]
		if c < 0x20 || c == 0x7F {
			return fmt.Errorf("header contains invalid characters")
		}
		if !isValidHeaderChar(rune(c)) {
			return fmt.Errorf("invalid character in header key")
		}
	}

	valueLen := len(value)
	if valueLen > maxHeaderValueLen {
		return fmt.Errorf("header value too long (max %d)", maxHeaderValueLen)
	}

	// Validate value characters - reject control characters except tab (0x09)
	for i := range valueLen {
		c := value[i]
		if (c < 0x20 && c != 0x09) || c == 0x7F {
			return fmt.Errorf("header contains invalid characters")
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
	default:
		return nil
	}

	if size > v.config.MaxResponseBodySize {
		return fmt.Errorf("request body exceeds %d bytes", v.config.MaxResponseBodySize)
	}

	return nil
}

func isValidHeaderChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '-'
}
