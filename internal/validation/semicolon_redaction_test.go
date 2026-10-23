package validation

import "testing"

// TestSanitizeURL_SemicolonSeparatedParams pins the legacy-separator fix:
// ';' is treated as a parameter separator alongside '&' (legacy PHP/servlet
// configurations parse it as one), so sensitive values riding in
// ;-separated parameters are redacted too. Non-sensitive values are
// byte-identical after redaction — separators are preserved as written.
func TestSanitizeURL_SemicolonSeparatedParams(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"semicolon-separated sensitive param",
			"https://h/p?a=1;password=hunter2",
			"https://h/p?a=1;password=[REDACTED]",
		},
		{
			"mixed separators",
			"https://h/p?a=1;token=x&b=2",
			"https://h/p?a=1;token=[REDACTED]&b=2",
		},
		{
			"semicolon param with no equals (keyless sensitive)",
			"https://h/p?a=1;secret",
			"https://h/p?a=1;secret=[REDACTED]",
		},
		{
			"non-sensitive semicolon value stays verbatim",
			"https://h/p?msg=hello;world&x=1",
			"https://h/p?msg=hello;world&x=1",
		},
		{
			"ampersand-only unchanged (regression guard)",
			"https://h/p?a=1&password=hunter2",
			"https://h/p?a=1&password=[REDACTED]",
		},
		{
			// A ';' inside a sensitive VALUE is indistinguishable from a
			// separator: the redaction must extend to the end of the
			// &-segment or the value's tail (";b") leaks readable
			// ("token=[REDACTED];b" — caught in re-review).
			"semicolon inside a sensitive value does not leak the tail",
			"https://h/p?token=a;b&x=1",
			"https://h/p?token=[REDACTED]&x=1",
		},
		{
			// Ambiguous neighbor behind a sensitive key: over-redacted
			// (dropped) rather than risked.
			"possible neighbor after sensitive key is dropped",
			"https://h/p?a=1;token=x;b=2",
			"https://h/p?a=1;token=[REDACTED]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeURL(tt.in); got != tt.want {
				t.Errorf("SanitizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	if !HasSensitiveQueryParams("a=1;password=x") {
		t.Error("HasSensitiveQueryParams must detect ;-separated sensitive keys")
	}
}
