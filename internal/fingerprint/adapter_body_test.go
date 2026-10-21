package fingerprint

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// closeTrackingReader records Close calls on the request body.
type closeTrackingReader struct {
	r    io.Reader
	seen *bool
}

func (c *closeTrackingReader) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *closeTrackingReader) Close() error               { *c.seen = true; return nil }

// RoundTripper contract: the transport owns the request body, and net/http
// does not close it when a custom RoundTripper errors. Every early-error
// arm of Adapter.RoundTrip (here: the SSRF pre-flight) must close it, or a
// caller-supplied io.ReadCloser (an *os.File via WithBody) leaks a handle
// until the GC finalizer runs.
func TestAdapterRoundTrip_EarlyErrorClosesBody(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"}) // AllowPrivateIPs unset

	closed := false
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/x",
		&closeTrackingReader{r: strings.NewReader("body"), seen: &closed})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.RoundTrip(req); err == nil {
		t.Fatal("expected SSRF rejection of the loopback target")
	}
	if !closed {
		t.Error("request body not closed on the early-error path")
	}
}
