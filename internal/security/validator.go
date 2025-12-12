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

// isLocalhost efficiently detects localhost variations without allocations.
// Optimized for security scanning with comprehensive localhost detection.
func isLocalhost(hostname string) bool {
	hostnameLen := len(hostname)
	if hostnameLen == 0 {
		return false
	}

	// Fast path: IPv4 loopback detection
	if hostnameLen >= 9 && hostname[0] == '1' {
		if hostname == "127.0.0.1" {
			return true
		}
		// Check 127.x.x.x range
		if hostnameLen > 4 && hostname[:4] == "127." {
			return true
		}
	}

	// Exact length matches for common cases (avoid string allocation)
	switch hostnameLen {
	case 9: // "localhost"
		if hostname == "localhost" || hostname == "LOCALHOST" || hostname == "Localhost" {
			return true
		}
	case 3: // "::1"
		if hostname == "::1" {
			return true
		}
	case 7: // "0.0.0.0"
		if hostname == "0.0.0.0" {
			return true
		}
	case 2: // "::"
		if hostname == "::" {
			return true
		}
	}

	// Check for localhost subdomains (only allocate when necessary)
	if hostnameLen > 10 {
		lower := strings.ToLower(hostname)
		if strings.HasPrefix(lower, "localhost.") {
			return true
		}
	}

	return false
}

// isPrivateOrReservedIP comprehensively checks for private and reserved IP ranges.
// Provides defense against SSRF attacks by blocking internal network access.
func isPrivateOrReservedIP(ip net.IP) bool {
	// Use Go's built-in methods for standard checks
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// Additional IPv4 reserved ranges not covered by Go's methods
	if ip4 := ip.To4(); ip4 != nil {
		// Class E (240.0.0.0/4) and 0.0.0.0/8
		if ip4[0] >= 240 || ip4[0] == 0 {
			return true
		}
		// Additional reserved ranges for enhanced security
		// 100.64.0.0/10 (Carrier-grade NAT)
		if ip4[0] == 100 && (ip4[1]&0xC0) == 64 {
			return true
		}
		// 198.18.0.0/15 (Benchmark testing)
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
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

	// Check for whitespace-only key
	hasNonSpace := false
	for i := range keyLen {
		if key[i] != ' ' && key[i] != '\t' {
			hasNonSpace = true
			break
		}
	}
	if !hasNonSpace {
		return fmt.Errorf("header key cannot be empty")
	}

	if keyLen > maxHeaderKeyLen {
		return fmt.Errorf("header key too long (max %d)", maxHeaderKeyLen)
	}
	if key[0] == ':' {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	// Validate key characters - reject control characters and validate allowed chars
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

	// Validate common header values to prevent HTTP/2 errors
	if err := validateCommonHeaderValue(key, value); err != nil {
		return err
	}

	return nil
}

func validateCommonHeaderValue(key, value string) error {
	keyLower := strings.ToLower(key)
	valueLower := strings.ToLower(value)

	switch keyLower {
	case "connection":
		// HTTP/2 does not allow Connection header, but HTTP/1.1 does
		// Valid values: keep-alive, close, upgrade
		if valueLower != "keep-alive" && valueLower != "close" && valueLower != "upgrade" {
			return fmt.Errorf("invalid Connection header value: %q (expected: keep-alive, close, or upgrade)", value)
		}
	case "transfer-encoding":
		// HTTP/2 does not allow Transfer-Encoding header
		// Valid values for HTTP/1.1: chunked, compress, deflate, gzip
		validTE := valueLower == "chunked" || valueLower == "compress" ||
			valueLower == "deflate" || valueLower == "gzip" || valueLower == "identity"
		if !validTE {
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
