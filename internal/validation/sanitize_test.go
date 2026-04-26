package validation

import (
	"strings"
	"testing"
)

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "No credentials",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "No credentials with query",
			input:    "https://example.com/path?q=1",
			expected: "https://example.com/path?q=1",
		},
		{
			name:     "No credentials with fragment",
			input:    "https://example.com/path#section",
			expected: "https://example.com/path",
		},

		// Username only
		{
			name:     "Username only",
			input:    "https://user@example.com/path",
			expected: "https://***@example.com/path",
		},
		{
			name:     "Username with port",
			input:    "https://user@example.com:8080/path",
			expected: "https://***@example.com:8080/path",
		},

		// Username and password
		{
			name:     "Username and password",
			input:    "https://user:pass@example.com/path",
			expected: "https://***:***@example.com/path",
		},
		{
			name:     "Password with special characters",
			input:    "https://user:p@ss:word@example.com/path",
			expected: "https://***:***@example.com/path",
		},
		{
			name:     "URL encoded password",
			input:    "https://user:pass%40word@example.com/path",
			expected: "https://***:***@example.com/path",
		},
		{
			name:     "Credentials with query and fragment",
			input:    "https://user:pass@example.com/path?q=1#section",
			expected: "https://***:***@example.com/path?q=1",
		},
		{
			name:     "Credentials with port",
			input:    "https://user:pass@example.com:8443/api",
			expected: "https://***:***@example.com:8443/api",
		},

		// Different schemes
		{
			name:     "HTTP scheme with credentials",
			input:    "http://user:pass@example.com/path",
			expected: "http://***:***@example.com/path",
		},
		{
			name:     "FTP scheme with credentials",
			input:    "ftp://user:pass@ftp.example.com/file",
			expected: "ftp://***:***@ftp.example.com/file",
		},

		// Edge cases
		{
			name:     "Empty password",
			input:    "https://user:@example.com/path",
			expected: "https://***:***@example.com/path",
		},
		{
			name:     "Invalid URL",
			input:    "://invalid",
			expected: "://invalid",
		},
		{
			name:     "Relative URL without scheme",
			input:    "/path/to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "Domain only",
			input:    "example.com",
			expected: "example.com",
		},
		{
			name:     "IP address with credentials",
			input:    "https://user:pass@192.168.1.1/admin",
			expected: "https://***:***@192.168.1.1/admin",
		},
		{
			name:     "IPv6 address with credentials",
			input:    "https://user:pass@[::1]:8080/path",
			expected: "https://***:***@[::1]:8080/path",
		},
		{
			name:     "Long path",
			input:    "https://user:pass@example.com/a/b/c/d/e/f",
			expected: "https://***:***@example.com/a/b/c/d/e/f",
		},

		// Security edge cases
		{
			name:     "Sensitive query params are redacted",
			input:    "https://example.com/api?token=secret123",
			expected: "https://example.com/api?token=%5BREDACTED%5D",
		},
		{
			name:     "Special characters in username",
			input:    "https://user%40domain:pass@example.com/path",
			expected: "https://***:***@example.com/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeURL_NilUserCheck(t *testing.T) {
	// Verify that URLs without user info pass through unchanged
	urls := []string{
		"https://example.com",
		"http://localhost:8080",
		"https://api.example.com/v1/resource?id=123",
	}

	for _, url := range urls {
		t.Run(url, func(t *testing.T) {
			result := SanitizeURL(url)
			if result != url {
				t.Errorf("URL without credentials was modified: %q -> %q", url, result)
			}
		})
	}
}

func TestSanitizeURL_CredentialRemoval(t *testing.T) {
	// Verify that credentials are always replaced with asterisks
	urls := []string{
		"https://admin:supersecret@example.com",
		"https://root:password123@example.com",
		"https://test:test@example.com",
	}

	for _, url := range urls {
		t.Run(url, func(t *testing.T) {
			result := SanitizeURL(url)
			if result == url {
				t.Errorf("Credentials were not removed from URL")
			}
			if len(result) > 0 && result[0] != ':' {
				if !strings.Contains(result, "***:***@") && !strings.Contains(result, "***@") {
					t.Errorf("Expected masked credentials in result: %q", result)
				}
			}
		})
	}
}

func TestSanitizeURL_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"IPv6 with zone ID", "https://user:pass@[fe80::1%25eth0]:8080/path", "https://***:***@[fe80::1%25eth0]:8080/path"},
		{"Double URL encoding", "https://example.com/api?q=%25xx", "https://example.com/api?q=%25xx"},
		{"Very long hostname", "https://" + strings.Repeat("a", 253) + ".com/path", ""},
		{"Multiple sensitive params", "https://example.com?token=secret&api_key=key123&password=pass", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeURL(tt.input)
			// For URLs with credentials, verify they are masked
			if strings.Contains(tt.input, "user:pass@") && !strings.Contains(result, "***:***@") {
				t.Errorf("Credentials not masked in %q -> %q", tt.input, result)
			}
			// For long URLs, just verify no panic
			t.Logf("SanitizeURL(%q) = %q", tt.input[:min(50, len(tt.input))], result[:min(50, len(result))])
		})
	}
}

func TestIsSensitiveQueryParam(t *testing.T) {
	tests := []struct {
		name     string
		param    string
		expected bool
	}{
		// Sensitive params
		{"token", "token", true},
		{"access_token", "access_token", true},
		{"api_key", "api_key", true},
		{"password", "password", true},
		{"secret", "secret", true},
		{"jwt", "jwt", true},
		{"session_id", "session_id", true},

		// Case insensitive
		{"TOKEN uppercase", "TOKEN", true},
		{"Token mixed", "Token", true},
		{"API_KEY upper", "API_KEY", true},

		// Non-sensitive params
		{"page not sensitive", "page", false},
		{"limit not sensitive", "limit", false},
		{"id not sensitive", "id", false},
		{"q not sensitive", "q", false},
		{"empty not sensitive", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSensitiveQueryParam(tt.param)
			if result != tt.expected {
				t.Errorf("IsSensitiveQueryParam(%q) = %v, want %v", tt.param, result, tt.expected)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
