package engine

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/cybergodev/httpc/internal/validation"
)

// TestClientErrorQuotedURLRedaction pins the engine-side %q redaction path
// (SEC-001 M-1): net/url renders *url.Error with %q, so a URL whose sensitive
// query value contains a character %q escapes (", \, control chars) reaches
// the cause text in ESCAPED form. Before the quoted form was matched,
// redactedCauseMessage's substring check missed it and ClientError.Error()
// printed the raw cause with the sensitive value fully readable (escaped, but
// readable).
func TestClientErrorQuotedURLRedaction(t *testing.T) {
	const rawURL = `https://host/p?token=a"b`
	// Exactly how net/http surfaces a transport failure: *url.Error quotes the
	// URL with %q — strconv.Quote semantics.
	cause := &url.Error{Op: "Get", URL: rawURL, Err: errors.New("dial tcp: connection refused")}

	clientErr := classifyErrorWithSanitizedURL(cause, validation.SanitizeURL(rawURL), "GET", 1)
	msg := clientErr.Error()

	for _, leak := range []string{`a"b`, `a\"b`} {
		if strings.Contains(msg, leak) {
			t.Errorf("ClientError.Error() leaks the sensitive value: %q", msg)
		}
	}
	if !strings.Contains(msg, "[REDACTED]") {
		t.Errorf("ClientError.Error() should carry the redacted form, got: %q", msg)
	}
}

// TestClientErrorQuotedURLRedactionUserinfo extends the same guarantee to
// userinfo credentials: the sanitized URL field and the redacted cause must
// agree — neither may carry the password.
func TestClientErrorQuotedURLRedactionUserinfo(t *testing.T) {
	const rawURL = `https://user:se"cret@host/p`
	cause := &url.Error{Op: "Get", URL: rawURL, Err: errors.New("connection refused")}

	clientErr := classifyErrorWithSanitizedURL(cause, validation.SanitizeURL(rawURL), "GET", 1)
	msg := clientErr.Error()

	for _, leak := range []string{`se"cret`, `se\"cret`} {
		if strings.Contains(msg, leak) {
			t.Errorf("ClientError.Error() leaks the password: %q", msg)
		}
	}
}
