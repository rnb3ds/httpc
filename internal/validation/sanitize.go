package validation

import (
	"fmt"
	"net/url"
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

	path := parsedURL.Path
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}
	if parsedURL.Fragment != "" {
		path += "#" + parsedURL.Fragment
	}

	if hasPassword {
		return fmt.Sprintf("%s://***:***@%s%s", parsedURL.Scheme, parsedURL.Host, path)
	}
	return fmt.Sprintf("%s://***@%s%s", parsedURL.Scheme, parsedURL.Host, path)
}
