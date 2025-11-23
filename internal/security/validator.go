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
			return fmt.Errorf("URL validation failed: %w", err)
		}
	}

	if v.config.ValidateHeaders {
		for key, value := range req.Headers {
			if err := v.validateHeader(key, value); err != nil {
				return fmt.Errorf("header validation failed for %s: %w", key, err)
			}
		}
	}

	if req.Body != nil {
		if err := v.validateRequestSize(req.Body); err != nil {
			return fmt.Errorf("request size validation failed: %w", err)
		}
	}

	return nil
}

func (v *Validator) validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	if len(urlStr) > 2048 {
		return fmt.Errorf("URL too long (max 2048 characters)")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL scheme is required")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL host is required")
	}

	// Only allow HTTP schemes
	validSchemes := []string{"http", "https"}
	schemeValid := false
	for _, scheme := range validSchemes {
		if parsedURL.Scheme == scheme {
			schemeValid = true
			break
		}
	}

	if !schemeValid {
		return fmt.Errorf("unsupported URL scheme: %s (only http/https allowed)", parsedURL.Scheme)
	}

	if err := v.validateHost(parsedURL.Host); err != nil {
		return fmt.Errorf("host validation failed: %w", err)
	}

	return nil
}

func (v *Validator) validateHost(host string) error {
	if v.config.AllowPrivateIPs {
		return nil
	}

	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// Check for localhost patterns first
	if isLocalhost(hostname) {
		return fmt.Errorf("localhost and loopback addresses are not allowed")
	}

	// Check if hostname is already an IP address
	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateOrReservedIP(ip) {
			return fmt.Errorf("private or reserved IP addresses are not allowed: %s", ip.String())
		}
		return nil
	}

	// For domain names, DNS resolution happens at connection time.
	// IMPORTANT: The primary SSRF protection is in internal/connection/pool.go
	// where we validate resolved IPs AFTER DNS lookup but BEFORE connection.
	// This pre-validation only catches obvious cases (localhost, direct IPs).
	// The post-resolution validation is the critical security boundary.

	return nil
}

func isLocalhost(hostname string) bool {
	hostname = strings.ToLower(hostname)
	return hostname == "localhost" ||
		hostname == "127.0.0.1" ||
		hostname == "::1" ||
		hostname == "0.0.0.0" ||
		hostname == "::" ||
		strings.HasPrefix(hostname, "127.") ||
		strings.HasPrefix(hostname, "localhost.")
}

func isPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if ip.IsPrivate() {
		return true
	}

	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	if ip.IsMulticast() {
		return true
	}

	if ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] >= 240 {
			return true
		}
		if ip4[0] == 0 {
			return true
		}
	}

	return false
}

func (v *Validator) validateHeader(key, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("header key cannot be empty")
	}

	if len(key) > 256 {
		return fmt.Errorf("header key too long (max 256 characters)")
	}

	if strings.ContainsAny(key, "\r\n\x00") || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("header contains invalid characters")
	}

	if len(value) > 8192 {
		return fmt.Errorf("header value too long (max 8KB)")
	}

	for _, r := range key {
		if !isValidHeaderChar(r) {
			return fmt.Errorf("invalid character in header key")
		}
	}

	if strings.HasPrefix(key, ":") {
		return fmt.Errorf("pseudo-headers are not allowed")
	}

	keyLower := strings.ToLower(key)
	switch keyLower {
	case "content-length", "transfer-encoding":
		return fmt.Errorf("header is managed automatically")
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
	case nil:
		return nil
	default:
		return nil
	}

	if size > v.config.MaxResponseBodySize {
		return fmt.Errorf("request body size %d exceeds maximum %d", size, v.config.MaxResponseBodySize)
	}

	return nil
}

func isValidHeaderChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '-'
}
