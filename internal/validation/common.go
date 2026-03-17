package validation

import (
	"fmt"
	"net/http"
	"strings"
)

// Common validation constants
const (
	MaxCredLen     = 255  // Maximum credential length (username/password)
	MaxTokenLen    = 2048 // Maximum bearer token length
	MaxKeyLen      = 256  // Maximum query parameter key length
	MaxValueLen    = 8192 // Maximum query parameter value length
	MaxFilenameLen = 256  // Maximum filename length for uploads

	MaxCookieNameLen   = 256
	MaxCookieValueLen  = 4096
	MaxCookieDomainLen = 255
	MaxCookiePathLen   = 1024

	MaxHeaderKeyLen   = 256
	MaxHeaderValueLen = 8192
	MaxURLLen         = 2048 // Maximum URL length
)

// dangerousChars contains characters that may be used for injection attacks.
// These characters are commonly used in command injection, SQL injection,
// XSS, and other attack vectors.
const dangerousChars = `"'<>&;` + "`|" + `$\{}[]^~`

// ValidateInputString performs common string validation to prevent injection attacks.
func ValidateInputString(input string, maxLen int, name string, additionalChecks func(rune) error) error {
	inputLen := len(input)
	if inputLen == 0 {
		return fmt.Errorf("%s cannot be empty", name)
	}
	if inputLen > maxLen {
		return fmt.Errorf("%s too long (max %d)", name, maxLen)
	}

	for _, r := range input {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("%s contains invalid characters", name)
		}

		if additionalChecks != nil {
			if err := additionalChecks(r); err != nil {
				return fmt.Errorf("%s validation failed: %w", name, err)
			}
		}
	}
	return nil
}

// ValidateCredential validates credentials for Basic Auth.
func ValidateCredential(cred string, maxLen int, checkColon bool, credType string) error {
	return ValidateInputString(cred, maxLen, credType, func(r rune) error {
		if checkColon && r == ':' {
			return fmt.Errorf("username cannot contain colon")
		}
		return nil
	})
}

// ValidateToken validates bearer tokens according to RFC 6750.
func ValidateToken(token string) error {
	return ValidateInputString(token, MaxTokenLen, "token", func(r rune) error {
		if r == ' ' {
			return fmt.Errorf("token cannot contain spaces")
		}
		return nil
	})
}

// ValidateCredentialStrict validates credentials with additional security checks
// for high-security scenarios (financial, medical, government).
// In addition to standard validation, it blocks dangerous characters commonly
// used in injection attacks.
//
// Parameters:
//   - cred: The credential string to validate
//   - maxLen: Maximum allowed length
//   - checkColon: If true, colons are not allowed (for usernames)
//   - credType: Description of the credential type for error messages
//
// Returns an error if validation fails, nil otherwise.
func ValidateCredentialStrict(cred string, maxLen int, checkColon bool, credType string) error {
	// First perform standard validation
	if err := ValidateCredential(cred, maxLen, checkColon, credType); err != nil {
		return err
	}

	// Additional check for dangerous characters
	if strings.ContainsAny(cred, dangerousChars) {
		return fmt.Errorf("%s contains dangerous characters that may be used for injection attacks", credType)
	}

	return nil
}

// ValidateTokenStrict validates bearer tokens with additional security checks
// for high-security scenarios. It blocks dangerous characters commonly used
// in injection attacks in addition to standard token validation.
//
// This is recommended for financial, medical, and government applications
// where defense-in-depth is required.
func ValidateTokenStrict(token string) error {
	// First perform standard validation
	if err := ValidateToken(token); err != nil {
		return err
	}

	// Additional check for dangerous characters
	if strings.ContainsAny(token, dangerousChars) {
		return fmt.Errorf("token contains dangerous characters that may be used for injection attacks")
	}

	return nil
}

// ValidateQueryKey validates query parameter keys.
func ValidateQueryKey(key string) error {
	return ValidateInputString(key, MaxKeyLen, "query key", func(r rune) error {
		if r == '&' || r == '=' || r == '#' || r == '?' {
			return fmt.Errorf("query key contains reserved characters")
		}
		return nil
	})
}

// ValidateFieldName validates form field names and filenames.
// It checks for dangerous characters and path traversal patterns to prevent
// directory traversal attacks and injection vulnerabilities.
func ValidateFieldName(name string, fieldType string) error {
	// Check for path traversal patterns before character validation
	if strings.Contains(name, "..") || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("field contains path traversal characters")
	}

	return ValidateInputString(name, MaxFilenameLen, fieldType, func(r rune) error {
		if r == '"' || r == '\'' || r == '<' || r == '>' || r == '&' {
			return fmt.Errorf("field contains dangerous characters")
		}
		return nil
	})
}

// ValidateHeaderKeyValue validates HTTP header keys and values.
func ValidateHeaderKeyValue(key, value string) error {
	if err := ValidateInputString(key, MaxHeaderKeyLen, "header key", func(r rune) error {
		if !IsValidHeaderChar(r) {
			return fmt.Errorf("invalid character in header key")
		}
		return nil
	}); err != nil {
		return err
	}

	if strings.HasPrefix(key, ":") {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	if len(value) > MaxHeaderValueLen {
		return fmt.Errorf("header value too long")
	}

	for _, r := range value {
		if (r < 0x20 && r != 0x09) || r == 0x7F {
			return fmt.Errorf("header value contains invalid characters")
		}
	}

	return nil
}

// IsValidHeaderChar checks if a character is valid in HTTP header names.
// Optimized with a lookup table for O(1) character validation.
func IsValidHeaderChar(r rune) bool {
	// Fast path: use lookup table for common ASCII range
	if r >= 0 && r <= 127 {
		return validHeaderCharTable[r]
	}
	return false
}

// validHeaderCharTable is a lookup table for valid HTTP header characters.
// Valid characters are: a-z, A-Z, 0-9, and '-' (hyphen)
var validHeaderCharTable = [128]bool{
	// Digits 0-9 (0x30-0x39)
	0x30: true, true, true, true, true, true, true, true, true, true,
	// Uppercase A-Z (0x41-0x5A)
	0x41: true, true, true, true, true, true, true, true, true, true, // A-J
	0x4B: true, true, true, true, true, true, true, true, true, true, // K-T
	0x55: true, true, true, true, true, true, // U-Z
	// Hyphen (0x2D)
	0x2D: true,
	// Lowercase a-z (0x61-0x7A)
	0x61: true, true, true, true, true, true, true, true, true, true, // a-j
	0x6B: true, true, true, true, true, true, true, true, true, true, // k-t
	0x75: true, true, true, true, true, true, // u-z
}

// IsValidHeaderString checks if a string contains only valid characters for HTTP headers.
// It returns true if the string contains no control characters (except tab), DEL, or CR/LF.
// Optimized to use byte-level checks instead of rune iteration.
func IsValidHeaderString(s string) bool {
	// Fast path: check for common valid characters first
	// Most header values are printable ASCII
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Allow printable ASCII (0x20-0x7E) and tab (0x09)
		// Block control characters (0x00-0x1F except 0x09), DEL (0x7F), CR (0x0D), LF (0x0A)
		if c < 0x20 {
			if c != 0x09 { // tab is allowed
				return false
			}
		} else if c == 0x7F {
			return false
		}
		// CR and LF are in the 0x00-0x1F range, already handled above
	}
	return true
}

// ValidateCookieName validates HTTP cookie names.
func ValidateCookieName(name string) error {
	return ValidateInputString(name, MaxCookieNameLen, "cookie name", func(r rune) error {
		if r == ';' || r == ',' {
			return fmt.Errorf("cookie name contains invalid characters")
		}
		return nil
	})
}

// ValidateCookieValue validates HTTP cookie values.
func ValidateCookieValue(value string) error {
	if len(value) > MaxCookieValueLen {
		return fmt.Errorf("cookie value too long")
	}

	for _, r := range value {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("cookie value contains invalid characters")
		}
	}
	return nil
}

// ValidateCookie performs comprehensive validation of an HTTP cookie including
// name, value, domain, and path attributes.
func ValidateCookie(cookie *http.Cookie) error {
	if err := ValidateCookieName(cookie.Name); err != nil {
		return err
	}

	if err := ValidateCookieValue(cookie.Value); err != nil {
		return err
	}

	// Validate domain if set
	if cookie.Domain != "" {
		domainLen := len(cookie.Domain)
		if domainLen > MaxCookieDomainLen {
			return fmt.Errorf("cookie domain too long (max %d)", MaxCookieDomainLen)
		}
		for i, r := range cookie.Domain {
			if r < 0x20 || r == 0x7F {
				return fmt.Errorf("cookie domain contains invalid characters at position %d", i)
			}
		}
	}

	// Validate path if set
	if cookie.Path != "" {
		pathLen := len(cookie.Path)
		if pathLen > MaxCookiePathLen {
			return fmt.Errorf("cookie path too long (max %d)", MaxCookiePathLen)
		}
		for i, r := range cookie.Path {
			if r < 0x20 || r == 0x7F {
				return fmt.Errorf("cookie path contains invalid characters at position %d", i)
			}
		}
	}

	return nil
}
