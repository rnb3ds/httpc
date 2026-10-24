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
			expected: "https://example.com/api?token=[REDACTED]",
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

// TestSanitizeURL_NilUserCheck was removed: passthrough URLs without
// credentials are rows of TestSanitizeURL ("No credentials*" rows).

// TestSanitizeURL_CredentialRemoval was removed: credential masking is
// asserted with exact expected outputs by TestSanitizeURL
// ("Username and password", "Password with special characters" rows).

func TestSanitizeURL_BoundaryConditions(t *testing.T) {
	t.Run("IPv6 with zone ID", func(t *testing.T) {
		result := SanitizeURL("https://user:pass@[fe80::1%25eth0]:8080/path")
		if !strings.Contains(result, "***:***@") {
			t.Errorf("Credentials not masked: %q", result)
		}
	})

	t.Run("Double URL encoding", func(t *testing.T) {
		result := SanitizeURL("https://example.com/api?q=%25xx")
		if result != "https://example.com/api?q=%25xx" {
			t.Errorf("Expected unchanged, got: %q", result)
		}
	})

	t.Run("Very long hostname", func(t *testing.T) {
		input := "https://" + strings.Repeat("a", 253) + ".com/path"
		result := SanitizeURL(input)
		if len(result) == 0 {
			t.Error("Expected non-empty result for long hostname")
		}
	})

	t.Run("Multiple sensitive params", func(t *testing.T) {
		result := SanitizeURL("https://example.com?token=secretvalue&api_key=mykey123&password=mypass")
		if !strings.Contains(result, "REDACTED") {
			t.Errorf("Expected sensitive params to be redacted, got: %q", result)
		}
		// Verify the sensitive values are not present
		if strings.Contains(result, "secretvalue") || strings.Contains(result, "mykey123") || strings.Contains(result, "mypass") {
			t.Errorf("Sensitive values leaked in URL: %q", result)
		}
	})
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
			result := isSensitiveQueryParam(tt.param)
			if result != tt.expected {
				t.Errorf("isSensitiveQueryParam(%q) = %v, want %v", tt.param, result, tt.expected)
			}
		})
	}
}

// TestSensitiveQueryParamDetection was removed: strict subset of
// TestIsSensitiveQueryParam (same keys plus the case-insensitivity row).

// TestSanitizeURL_FailClosed verifies the fail-closed contract: malformed or
// opaque inputs that url.Parse cannot fully structure must still have their
// credentials redacted instead of being returned verbatim. Returning the raw
// string would leak user:pass@... into logs and error messages — exactly the
// leak SanitizeURL exists to prevent — and malformed URLs are the inputs most
// likely to originate from untrusted sources.
func TestSanitizeURL_FailClosed(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		mustNotContain []string // substrings that must NOT appear (leaks)
		mustContain    []string // substrings that MUST appear (redaction markers)
	}{
		{
			name:           "unparseable URL with credentials",
			input:          "http://user:pass@host/\x01",
			mustNotContain: []string{"user", "pass"},
			mustContain:    []string{"***@"},
		},
		{
			name:           "opaque URL hides userinfo in Opaque",
			input:          "user:pass@host/path",
			mustNotContain: []string{"user:pass", ":pass@"},
			mustContain:    []string{"***@"},
		},
		{
			name:           "scheme-less URL with credentials",
			input:          "//u:p@h/p",
			mustNotContain: []string{"u:p"},
			mustContain:    []string{"***@"},
		},
		{
			name:           "unparseable URL with sensitive query param",
			input:          "http://a:b@h/p?api-key=SK123\x02",
			mustNotContain: []string{"a:b", "SK123"},
			mustContain:    []string{"***@", "api-key=[REDACTED]"},
		},
		// SEC-001 M-2 regressions: an '@' after the first '/' used to make the
		// old LastIndex-based scan return the string verbatim.
		{
			name:           "path '@' must not mask real credentials",
			input:          "http://user:pass@host/p%zz@th?token=SK123",
			mustNotContain: []string{"user", "pass", "SK123"},
			mustContain:    []string{"***@", "token=[REDACTED]"},
		},
		{
			name:           "query '@' must not disable query redaction",
			input:          "http://host/ap%zzi?token=SK123&email=a@b.com",
			mustNotContain: []string{"SK123"},
			mustContain:    []string{"token=[REDACTED]", "email=a@b.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeURL(tt.input)
			for _, leak := range tt.mustNotContain {
				if strings.Contains(got, leak) {
					t.Errorf("SanitizeURL(%q) = %q leaks %q", tt.input, got, leak)
				}
			}
			for _, marker := range tt.mustContain {
				if !strings.Contains(got, marker) {
					t.Errorf("SanitizeURL(%q) = %q missing %q", tt.input, got, marker)
				}
			}
		})
	}
}

// TestSanitizeURL_APiKeyVariant covers the hyphenated spelling added to the
// sensitive-parameter list (previously only api_key/apikey matched, so
// "api-key=..." values reached logs unredacted).
func TestSanitizeURL_APiKeyVariant(t *testing.T) {
	got := SanitizeURL("https://example.com/v1?api-key=SK123")
	if !strings.Contains(got, "api-key=[REDACTED]") {
		t.Errorf("hyphenated api-key not redacted: %q", got)
	}
	if strings.Contains(got, "SK123") {
		t.Errorf("api-key value leaked: %q", got)
	}
}

// TestSanitizeURL_PercentEncodedSensitiveKey pins the fix for a redaction
// bypass: a key obfuscated by percent-encoding ("%74oken" = "token") must
// still be recognized as sensitive, otherwise its value would leak.
func TestSanitizeURL_PercentEncodedSensitiveKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"percent-encoded token key", "https://example.com/a?%74oken=SECRET"},
		{"percent-encoded api_key key", "https://example.com/a?api%5Fkey=SECRET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeURL(tt.input)
			if !strings.Contains(got, "[REDACTED]") {
				t.Errorf("sensitive key not redacted: %q", got)
			}
			if strings.Contains(got, "SECRET") {
				t.Errorf("sensitive value leaked: %q", got)
			}
		})
	}
}

// TestSanitizeURL_UnparseableQueryOnly pins the SEC-001 M-1 fail-closed fix: a
// URL that url.Parse rejects (no userinfo, but a sensitive query parameter)
// must still have the parameter redacted. url.Parse errors quote the raw URL,
// so an intact return would leak the value the parseable path redacts. The
// invalid escape is in the path — url.Parse does not validate query escapes.
func TestSanitizeURL_UnparseableQueryOnly(t *testing.T) {
	got := SanitizeURL("http://host.invalid/ap%zzi?token=SECRETVALUE")
	if !strings.Contains(got, "token=[REDACTED]") {
		t.Errorf("sensitive parameter not redacted on the unparseable path: %q", got)
	}
	if strings.Contains(got, "SECRETVALUE") {
		t.Errorf("sensitive value leaked on the unparseable path: %q", got)
	}
}
