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
func ValidateFieldName(name string, fieldType string) error {
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
		if !isValidHeaderChar(r) {
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

// isValidHeaderChar checks if a character is valid in HTTP header names.
func isValidHeaderChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '-'
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
