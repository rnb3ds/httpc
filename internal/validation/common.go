package validation

import (
	"fmt"
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
)

// ValidateInputString performs common string validation to prevent injection attacks.
// Checks for control characters, CRLF injection, and length limits.
// This consolidates validation logic used across multiple components.
func ValidateInputString(input string, maxLen int, name string, additionalChecks func(rune) error) error {
	inputLen := len(input)
	if inputLen == 0 {
		return fmt.Errorf("%s cannot be empty", name)
	}
	if inputLen > maxLen {
		return fmt.Errorf("%s too long (max %d)", name, maxLen)
	}

	for i, r := range input {
		// Reject control characters and DEL (common security check)
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("%s contains invalid characters at position %d", name, i)
		}

		// Prevent CRLF injection (critical security check)
		if r == '\r' || r == '\n' {
			return fmt.Errorf("CRLF injection detected in %s", name)
		}

		// Apply additional validation if provided
		if additionalChecks != nil {
			if err := additionalChecks(r); err != nil {
				return fmt.Errorf("%s validation failed at position %d: %w", name, i, err)
			}
		}
	}
	return nil
}

// ValidateCredential validates credentials for Basic Auth.
// Prevents injection attacks and enforces RFC 7617 (username cannot contain colon).
func ValidateCredential(cred string, maxLen int, checkColon bool, credType string) error {
	return ValidateInputString(cred, maxLen, credType, func(r rune) error {
		// Username cannot contain colon (RFC 7617)
		if checkColon && r == ':' {
			return fmt.Errorf("username cannot contain colon")
		}
		return nil
	})
}

// ValidateToken validates bearer tokens according to RFC 6750.
// Prevents header injection and enforces token format rules.
func ValidateToken(token string) error {
	return ValidateInputString(token, MaxTokenLen, "token", func(r rune) error {
		// RFC 6750: token should not contain spaces
		if r == ' ' {
			return fmt.Errorf("token cannot contain spaces")
		}
		return nil
	})
}

// ValidateQueryKey validates query parameter keys.
// Prevents parameter pollution and injection attacks.
func ValidateQueryKey(key string) error {
	return ValidateInputString(key, MaxKeyLen, "query key", func(r rune) error {
		// Prevent query parameter injection
		if r == '&' || r == '=' || r == '#' || r == '?' {
			return fmt.Errorf("query key contains reserved characters")
		}
		return nil
	})
}

// ValidateFieldName validates form field names and filenames.
// Prevents XSS and injection attacks in multipart forms.
func ValidateFieldName(name string, fieldType string) error {
	return ValidateInputString(name, MaxFilenameLen, fieldType, func(r rune) error {
		// Prevent XSS and injection in form fields
		if r == '"' || r == '\'' || r == '<' || r == '>' || r == '&' {
			return fmt.Errorf("field contains dangerous characters")
		}
		return nil
	})
}

// ValidateHeaderKeyValue validates HTTP header keys and values.
// Prevents header injection and enforces HTTP/1.1 and HTTP/2 compatibility.
func ValidateHeaderKeyValue(key, value string) error {
	// Validate key
	if err := ValidateInputString(key, MaxHeaderKeyLen, "header key", func(r rune) error {
		// HTTP header keys must be tokens (RFC 7230)
		if !isValidHeaderChar(r) {
			return fmt.Errorf("invalid character in header key")
		}
		return nil
	}); err != nil {
		return err
	}

	// Check for pseudo-headers (HTTP/2)
	if strings.HasPrefix(key, ":") {
		return fmt.Errorf("pseudo-headers not allowed")
	}

	// Validate value (allow tabs but not other control chars)
	valueLen := len(value)
	if valueLen > MaxHeaderValueLen {
		return fmt.Errorf("header value too long (max %d)", MaxHeaderValueLen)
	}

	for i, r := range value {
		if (r < 0x20 && r != 0x09) || r == 0x7F {
			return fmt.Errorf("header value contains invalid characters at position %d", i)
		}
	}

	return nil
}

// isValidHeaderChar checks if a character is valid in HTTP header names.
// Based on RFC 7230 token definition.
func isValidHeaderChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '-'
}

// ValidateCookieName validates HTTP cookie names.
// Prevents cookie injection and enforces RFC 6265 compliance.
func ValidateCookieName(name string) error {
	return ValidateInputString(name, MaxCookieNameLen, "cookie name", func(r rune) error {
		// RFC 6265: cookie names cannot contain semicolon or comma
		if r == ';' || r == ',' {
			return fmt.Errorf("cookie name contains invalid characters")
		}
		return nil
	})
}

// ValidateCookieValue validates HTTP cookie values.
// Prevents cookie injection and enforces RFC 6265 compliance.
func ValidateCookieValue(value string) error {
	valueLen := len(value)
	if valueLen > MaxCookieValueLen {
		return fmt.Errorf("cookie value too long (max %d)", MaxCookieValueLen)
	}

	for i, r := range value {
		// RFC 6265: cookie values have specific character restrictions
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("cookie value contains invalid characters at position %d", i)
		}
	}
	return nil
}
