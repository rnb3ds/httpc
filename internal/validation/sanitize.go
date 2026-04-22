package validation

import (
	"net/url"
	"strings"
)

// SanitizeURL removes credentials from a URL for safe logging.
// URLs with credentials are transformed from user:pass@host to ***:***@host.
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
//	SanitizeURL("https://user:pass@example.com/path") // Returns "https://***:***@example.com/path"
//	SanitizeURL("https://example.com/path")           // Returns "https://example.com/path"
func SanitizeURL(urlStr string) string {
	if urlStr == "" {
		return ""
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	if parsedURL.User == nil {
		return parsedURL.String()
	}

	_, hasPassword := parsedURL.User.Password()
	parsedURL.User = nil

	// Estimate size: scheme (8) + ://***:***@ (10) + host + path + query + fragment
	estimatedLen := 18 + len(parsedURL.Scheme) + len(parsedURL.Host) + len(parsedURL.Path) + len(parsedURL.RawQuery) + len(parsedURL.Fragment)
	var b strings.Builder
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
	if parsedURL.Fragment != "" {
		b.WriteByte('#')
		b.WriteString(parsedURL.Fragment)
	}
	return b.String()
}
