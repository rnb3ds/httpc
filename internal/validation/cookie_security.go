package validation

import (
	"fmt"
	"net/http"
	"strings"
)

// CookieSecurityConfig defines the security requirements for cookies.
// This is used to enforce security attributes like Secure, HttpOnly, and SameSite
// to protect against CSRF, XSS, and session hijacking attacks.
type CookieSecurityConfig struct {
	// RequireSecure requires the Secure attribute to be set.
	// Secure cookies are only sent over HTTPS connections.
	// Recommended: true for production environments.
	RequireSecure bool

	// RequireHttpOnly requires the HttpOnly attribute to be set.
	// HttpOnly cookies cannot be accessed via JavaScript, preventing XSS attacks.
	// Recommended: true for session cookies.
	RequireHttpOnly bool

	// RequireSameSite requires a specific SameSite attribute.
	// Valid values: "Strict", "Lax", "None", "" (empty means no requirement).
	// - Strict: Cookie only sent in first-party context (most secure)
	// - Lax: Cookie sent in first-party context and top-level navigations
	// - None: Cookie sent in all contexts (requires Secure)
	RequireSameSite string

	// AllowSameSiteNone allows SameSite=None attribute.
	// This is useful for cross-site cookies but reduces security.
	// If false and RequireSameSite is empty, cookies with SameSite=None are rejected.
	// Recommended: false for high-security applications.
	AllowSameSiteNone bool
}

// DefaultCookieSecurityConfig returns a recommended security configuration.
// This provides a good balance between security and compatibility.
func DefaultCookieSecurityConfig() *CookieSecurityConfig {
	return &CookieSecurityConfig{
		RequireSecure:     false, // Allow non-HTTPS for development
		RequireHttpOnly:   false, // Allow JavaScript access for flexibility
		RequireSameSite:   "",    // No SameSite requirement
		AllowSameSiteNone: true,  // Allow cross-site cookies
	}
}

// StrictCookieSecurityConfig returns a strict security configuration.
// Use this for high-security applications (financial, medical, government).
func StrictCookieSecurityConfig() *CookieSecurityConfig {
	return &CookieSecurityConfig{
		RequireSecure:     true,
		RequireHttpOnly:   true,
		RequireSameSite:   "Strict",
		AllowSameSiteNone: false,
	}
}

// sameSiteToString converts http.SameSite to a string representation.
func sameSiteToString(sameSite http.SameSite) string {
	switch sameSite {
	case http.SameSiteDefaultMode:
		return "Default"
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteNoneMode:
		return "None"
	default:
		return ""
	}
}

// stringToSameSite converts a string to http.SameSite.
func stringToSameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "strict":
		return http.SameSiteStrictMode
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	case "default":
		return http.SameSiteDefaultMode
	default:
		return http.SameSiteDefaultMode
	}
}

// isSameSiteNone checks if the SameSite value represents None mode.
func isSameSiteNone(sameSite http.SameSite) bool {
	return sameSite == http.SameSiteNoneMode
}

// ValidateCookieSecurity validates a cookie against the security configuration.
// Returns an error if the cookie does not meet the security requirements.
// This is the standard validation for general use.
func ValidateCookieSecurity(cookie *http.Cookie, config *CookieSecurityConfig) error {
	if cookie == nil {
		return fmt.Errorf("cookie is nil")
	}
	if config == nil {
		return nil // No security requirements
	}

	var errors []string

	// Check Secure requirement
	if config.RequireSecure && !cookie.Secure {
		errors = append(errors, "missing Secure attribute")
	}

	// Check HttpOnly requirement
	if config.RequireHttpOnly && !cookie.HttpOnly {
		errors = append(errors, "missing HttpOnly attribute")
	}

	// Check SameSite requirement
	if config.RequireSameSite != "" {
		if err := validateSameSite(cookie, config.RequireSameSite, config.AllowSameSiteNone); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Check if SameSite=None is allowed
	if !config.AllowSameSiteNone && isSameSiteNone(cookie.SameSite) {
		errors = append(errors, "SameSite=None is not allowed")
	}

	if len(errors) > 0 {
		return fmt.Errorf("cookie '%s' failed security validation: %s", cookie.Name, strings.Join(errors, ", "))
	}

	return nil
}

// ValidateCookieStrict validates a cookie with strict security requirements.
// Deprecated: unused in production code — kept for internal utility API surface.
// This delegates Secure/HttpOnly checks to ValidateCookieSecurity, then applies
// additional strict checks for SameSite and Path attributes.
// Accepts SameSite=Strict or SameSite=Lax (but not Default/None).
// Use this for high-security applications.
func ValidateCookieStrict(cookie *http.Cookie, config *CookieSecurityConfig) error {
	if cookie == nil {
		return fmt.Errorf("cookie is nil")
	}

	// Build config for base checks: enforce Secure and HttpOnly, but handle SameSite ourselves
	baseCfg := config
	if baseCfg == nil {
		baseCfg = &CookieSecurityConfig{
			RequireSecure:     true,
			RequireHttpOnly:   true,
			AllowSameSiteNone: false,
		}
	}
	if err := ValidateCookieSecurity(cookie, baseCfg); err != nil {
		return err
	}

	// Strict-specific SameSite check: must be explicitly set to Strict or Lax
	var errs []string
	if cookie.SameSite == http.SameSiteDefaultMode || cookie.SameSite == 0 {
		errs = append(errs, "missing explicit SameSite attribute")
	} else if isSameSiteNone(cookie.SameSite) {
		if !baseCfg.AllowSameSiteNone {
			errs = append(errs, "SameSite=None is not allowed in strict mode")
		}
	}

	// Require Path attribute
	if cookie.Path == "" {
		errs = append(errs, "missing Path attribute")
	}

	if len(errs) > 0 {
		return fmt.Errorf("cookie '%s' failed strict security validation: %s", cookie.Name, strings.Join(errs, ", "))
	}

	return nil
}

// validateSameSite checks if the cookie's SameSite attribute matches the required value.
func validateSameSite(cookie *http.Cookie, required string, allowNone bool) error {
	cookieSameSite := sameSiteToString(cookie.SameSite)
	requiredLower := strings.ToLower(required)

	// Handle empty/unknown SameSite
	if cookieSameSite == "" || cookieSameSite == "Default" {
		// Default mode in modern browsers is Lax
		if requiredLower == "lax" {
			return nil // Default is acceptable for Lax requirement
		}
		return fmt.Errorf("missing SameSite attribute (required: %s)", required)
	}

	// Check if SameSite=None is allowed
	if isSameSiteNone(cookie.SameSite) && !allowNone {
		return fmt.Errorf("SameSite=None is not allowed")
	}

	// Validate match
	if strings.ToLower(cookieSameSite) != requiredLower {
		return fmt.Errorf("invalid SameSite value '%s' (required: %s)", cookieSameSite, required)
	}

	return nil
}

// EnforceCookieSecurity applies security attributes to a cookie based on the configuration.
// Deprecated: unused in production code — kept for internal utility API surface.
// This modifies the cookie in place to meet the security requirements.
func EnforceCookieSecurity(cookie *http.Cookie, config *CookieSecurityConfig) {
	if cookie == nil || config == nil {
		return
	}

	if config.RequireSecure {
		cookie.Secure = true
	}

	if config.RequireHttpOnly {
		cookie.HttpOnly = true
	}

	if config.RequireSameSite != "" {
		cookie.SameSite = stringToSameSite(config.RequireSameSite)
	}

	// Set default path if not set
	if cookie.Path == "" {
		cookie.Path = "/"
	}
}
