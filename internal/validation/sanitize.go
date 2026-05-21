package validation

import (
	"net/url"
	"strings"
	"sync"
)

// sensitiveQueryParamNames contains query parameter names whose values should be
// redacted when sanitizing URLs for logging and error messages.
// Shared across packages to avoid duplicate definitions.
var sensitiveQueryParamNames = map[string]bool{
	// OAuth and authentication tokens
	"token": true, "access_token": true, "refresh_token": true,
	"id_token": true, "idtoken": true, "bearer": true,
	// API keys and secrets
	"api_key": true, "apikey": true,
	"secret": true, "secret_key": true, "client_secret": true,
	"private_key": true, "privatekey": true, "private-key": true,
	// Passwords and credentials
	"password": true, "passwd": true, "pass": true, "pwd": true,
	"credential": true, "credentials": true,
	// Session identifiers
	"session_id": true, "sessionid": true,
	// JWT and signatures
	"jwt": true, "signature": true, "sign": true, "sig": true,
}

// asciiToLower converts ASCII uppercase letters to lowercase in-place using a
// stack-allocated buffer. Avoids heap allocation from strings.ToLower for
// typical short query parameter names.
func asciiToLower(s string) string {
	// Fast path: check if any uppercase exists
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	var buf [128]byte
	if len(s) > len(buf) {
		return strings.ToLower(s)
	}
	b := buf[:len(s)]
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// isSensitiveQueryParamCI performs case-insensitive lookup for sensitive query param names.
func isSensitiveQueryParamCI(name string) bool {
	return sensitiveQueryParamNames[asciiToLower(name)]
}

// IsSensitiveQueryParam reports whether the given query parameter name is
// considered sensitive and should be redacted from logs and cache keys.
func IsSensitiveQueryParam(name string) bool {
	return sensitiveQueryParamNames[asciiToLower(name)]
}

// SensitiveQueryParamNames returns the set of sensitive query parameter names.
// Used internally to check URLs for sensitive content without importing the map directly.
func SensitiveQueryParamNames() map[string]bool {
	return sensitiveQueryParamNames
}

// SanitizeURL removes credentials and redacts sensitive query parameters from a URL
// for safe logging. URLs with credentials are transformed from user:pass@host to
// ***:***@host. Sensitive query parameters (token, api_key, password, etc.) have
// their values replaced with [REDACTED].
//
// Returns the original string if the URL cannot be parsed.
//
// This function is used to prevent credential leakage in:
//   - Log messages
//   - Error messages
//   - Audit events
//   - Debug output
//
// Example:
//
//	SanitizeURL("https://user:pass@example.com/path?token=secret")
//	// Returns "https://***:***@example.com/path?token=[REDACTED]"
func SanitizeURL(urlStr string) string {
	if urlStr == "" {
		return ""
	}

	// Fast path: skip parsing if URL has no credentials, query params, fragments,
	// or characters that would be escaped by url.Parse (spaces).
	// Most real URLs pass this check and avoid the expensive url.Parse call.
	// Single scan replaces 4 separate strings.Contains calls.
	// Also skip if URL contains query params but none are sensitive — avoids
	// url.Parse + Query() + Encode() round-trip for the common case.
	if !strings.ContainsAny(urlStr, "@?# ") {
		return urlStr
	}

	// If there are query params, check if any are sensitive before parsing.
	// This avoids the expensive url.Parse + query encode cycle for non-sensitive URLs.
	if idx := strings.IndexByte(urlStr, '?'); idx >= 0 {
		if hasSensitiveQueryParams(urlStr[idx+1:]) {
			// Fall through to full parsing below
		} else if !strings.ContainsAny(urlStr, "@# ") {
			return urlStr
		}
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	// Redact sensitive query parameters
	if parsedURL.RawQuery != "" {
		query := parsedURL.Query()
		redacted := false
		for key := range query {
			if isSensitiveQueryParamCI(key) {
				query.Set(key, "[REDACTED]")
				redacted = true
			}
		}
		if redacted {
			parsedURL.RawQuery = query.Encode()
		}
	}

	// Clear fragment to prevent credential leakage (e.g., OAuth implicit grants)
	parsedURL.Fragment = ""
	parsedURL.RawFragment = ""
	if parsedURL.User == nil {
		return parsedURL.String()
	}

	_, hasPassword := parsedURL.User.Password()
	parsedURL.User = nil

	// Estimate size: scheme (8) + ://***:***@ (10) + host + path + query
	estimatedLen := 18 + len(parsedURL.Scheme) + len(parsedURL.Host) + len(parsedURL.Path) + len(parsedURL.RawQuery)
	b := getSanitizeBuilder()
	b.Grow(estimatedLen)
	b.WriteString(parsedURL.Scheme)
	b.WriteString("://")
	if hasPassword {
		b.WriteString("***:***")
	} else {
		b.WriteString("***")
	}
	b.WriteByte('@')
	b.WriteString(parsedURL.Host)
	b.WriteString(parsedURL.Path)
	if parsedURL.RawQuery != "" {
		b.WriteByte('?')
		b.WriteString(parsedURL.RawQuery)
	}
	result := b.String()
	putSanitizeBuilder(b)
	return result
}

// sanitizeBuilderPool reduces allocations for strings.Builder in SanitizeURL.
var sanitizeBuilderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// hasSensitiveQueryParams checks if any key-value pair in the query string
// has a sensitive parameter name. Uses byte-level ASCII lowercase to avoid
// allocations from strings.ToLower.
func hasSensitiveQueryParams(query string) bool {
	for {
		// Extract key before '='
		eqIdx := strings.IndexByte(query, '=')
		if eqIdx < 0 {
			break
		}
		key := query[:eqIdx]
		if isSensitiveQueryParamCI(key) {
			return true
		}
		// Advance to next parameter
		ampIdx := strings.IndexByte(query, '&')
		if ampIdx < 0 {
			break
		}
		query = query[ampIdx+1:]
	}
	return false
}

func getSanitizeBuilder() *strings.Builder {
	b, ok := sanitizeBuilderPool.Get().(*strings.Builder)
	if !ok || b == nil {
		return &strings.Builder{}
	}
	b.Reset()
	return b
}

func putSanitizeBuilder(b *strings.Builder) {
	if b == nil || b.Cap() > 2048 {
		return
	}
	b.Reset()
	sanitizeBuilderPool.Put(b)
}
