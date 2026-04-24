package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/cybergodev/httpc/internal/types"
	"github.com/cybergodev/httpc/internal/validation"
)

type Validator struct {
	config *Config
}

// Compile-time interface check for requestValidator
var _ requestValidator = (*Validator)(nil)

// requestValidator defines the interface for request validation.
type requestValidator interface {
	ValidateRequest(req *Request) error
}

type Config struct {
	ValidateURL         bool
	ValidateHeaders     bool
	MaxResponseBodySize int64
	AllowPrivateIPs     bool
	ExemptNets          []*net.IPNet
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

	cfg := *config
	if config.ExemptNets != nil {
		cfg.ExemptNets = make([]*net.IPNet, len(config.ExemptNets))
		copy(cfg.ExemptNets, config.ExemptNets)
	}

	return &Validator{
		config: &cfg,
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
		if err := v.validateRequestBodySize(req.Body); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateURL(urlStr string) error {
	// Use centralized URL validation from validation package
	if err := validation.ValidateURL(urlStr); err != nil {
		return err
	}

	// Parse URL for host validation (already validated above)
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("URL parse failed: %w", err)
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
		if err := validation.ValidateIPWithExemptions(ip, v.config.ExemptNets); err != nil {
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

// validateRequestBodySize checks the request body against the configured size limit.
// MaxResponseBodySize serves as a general body-size cap for both request and response payloads.
func (v *Validator) validateRequestBodySize(body any) error {
	if v.config.MaxResponseBodySize <= 0 {
		return nil
	}

	var size int64
	switch b := body.(type) {
	case string:
		size = int64(len(b))
	case []byte:
		size = int64(len(b))
	case url.Values:
		size = int64(len(b.Encode()))
	case *types.FormData:
		for _, v := range b.Fields {
			size += int64(len(v))
		}
		for _, f := range b.Files {
			size += int64(len(f.Content))
		}
	default:
		// For io.Reader and other types, caller is responsible for size control.
		// Consider wrapping with io.LimitReader for untrusted sources.
		return nil
	}

	if size > v.config.MaxResponseBodySize {
		return fmt.Errorf("request body size %d exceeds limit %d bytes", size, v.config.MaxResponseBodySize)
	}

	return nil
}
