package validation

import (
	"strings"
	"testing"
)

// TestRedactURLInMessage locks the escaped-form redaction: net/url renders
// *url.Error and parse errors with %q, so a URL containing a character %q
// escapes (", \, control characters) reaches the message in ESCAPED form —
// a plain substring match misses it and the credential/sensitive value leaks
// (this exact shape silently defeated the SEC-001 M-1 redaction before the
// quoted form was matched too).
func TestRedactURLInMessage(t *testing.T) {
	const sensitive = `https://host/p?token=a"b` // quote in the value: %q-escaped on render
	quoted := `"https://host/p?token=a\"b": dial tcp: connection refused`

	tests := []struct {
		name      string
		msg       string
		rawURL    string
		want      string
		wantFound bool
	}{
		{
			name:      "quoted form (url.Error rendering) is redacted",
			msg:       `Get ` + quoted,
			rawURL:    sensitive,
			want:      `Get "https://host/p?token=[REDACTED]": dial tcp: connection refused`,
			wantFound: true,
		},
		{
			name:      "raw form is redacted",
			msg:       `failed to fetch https://host/p?token=xyz (attempt 1)`,
			rawURL:    `https://host/p?token=xyz`,
			want:      `failed to fetch https://host/p?token=[REDACTED] (attempt 1)`,
			wantFound: true,
		},
		{
			name:      "raw and quoted forms in one message",
			msg:       `get ` + quoted + ` for https://host/p?token=a"b`,
			rawURL:    sensitive,
			want:      `get "https://host/p?token=[REDACTED]": dial tcp: connection refused for https://host/p?token=[REDACTED]`,
			wantFound: true,
		},
		{
			name:      "nothing sensitive in the URL: unchanged, not found",
			msg:       `Get "https://host/p?x=1": fine`,
			rawURL:    `https://host/p?x=1`,
			want:      `Get "https://host/p?x=1": fine`,
			wantFound: false,
		},
		{
			name:      "URL absent from the message: unchanged, not found",
			msg:       `dial tcp: connection refused`,
			rawURL:    sensitive,
			want:      `dial tcp: connection refused`,
			wantFound: false,
		},
		{
			name:      "backslash in value is redacted through the quoted form",
			msg:       `Get "https://host/p?token=a\\b": refused`,
			rawURL:    `https://host/p?token=a\b`,
			want:      `Get "https://host/p?token=[REDACTED]": refused`,
			wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := RedactURLInMessage(tt.msg, tt.rawURL)
			if got != tt.want || found != tt.wantFound {
				t.Errorf("RedactURLInMessage(%q, %q) = (%q, %v), want (%q, %v)",
					tt.msg, tt.rawURL, got, found, tt.want, tt.wantFound)
			}
			if found && strings.Contains(got, "a\"b") {
				t.Errorf("redacted message still leaks the raw sensitive value: %q", got)
			}
		})
	}
}

// TestValidateProxyURLQuotedPasswordNotLeaked pins the M-1 end-to-end path: a
// proxy URL whose password contains a quote is rejected by url.Parse with the
// URL embedded %q-escaped in the parse error; the redaction must scrub the
// password from that escaped rendering too (it leaked verbatim before).
func TestValidateProxyURLQuotedPasswordNotLeaked(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		leakSub string // must NOT appear in the returned error text
	}{
		{"quote in password", `http://user:pa"ss@host:8080`, `pa"ss`},
		{"backslash in password", `http://user:pa\ss@host:8080`, `pa\ss`},
		{"control char in password", "http://user:pa\x01ss@host:8080", "pa\x01ss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateProxyURL(tt.rawURL)
			if err == nil {
				t.Fatalf("URL %q should fail validation", tt.rawURL)
			}
			msg := err.Error()
			if strings.Contains(msg, tt.leakSub) {
				t.Errorf("error leaks the password verbatim: %q", msg)
			}
			// The escaped rendering (%q) must not leak it either.
			if idx := strings.Index(msg, "pa"); idx >= 0 && strings.Contains(msg[idx:], "ss") && !strings.Contains(msg, "***") {
				t.Errorf("error appears to carry unredacted password material: %q", msg)
			}
		})
	}
}
