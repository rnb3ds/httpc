package fingerprint

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	ehttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
)

// closeTrackingReader records Close calls on the request body.
type closeTrackingReader struct {
	r    io.Reader
	seen *bool
}

func (c *closeTrackingReader) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *closeTrackingReader) Close() error               { *c.seen = true; return nil }

// countingCloser records the number of Close calls (not just a bool) so tests
// can assert exactly-once semantics.
type countingCloser struct {
	closes *atomic.Int64
}

func (c *countingCloser) Read(p []byte) (int, error) { return 0, io.EOF }
func (c *countingCloser) Close() error               { c.closes.Add(1); return nil }

// stubDoClient stubs the tls-client interface with a Do func; the embedded
// nil interface panics on any other method, which no test here calls.
type stubDoClient struct {
	tls_client.HttpClient
	do func(req *ehttp.Request) (*ehttp.Response, error)
}

func (s *stubDoClient) Do(req *ehttp.Request) (*ehttp.Response, error) { return s.do(req) }

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

// backendRoundTrip harness: one backend whose tls-client is a stub.
func backendWithStub(do func(*ehttp.Request) (*ehttp.Response, error)) *backend {
	return &backend{client: &stubDoClient{do: do}}
}

func forkReqWithBody(body io.ReadCloser) *ehttp.Request {
	return &ehttp.Request{
		Method: ehttp.MethodPost,
		Header: make(ehttp.Header),
		URL:    &url.URL{Scheme: "https", Host: "host.example.com", Path: "/p"},
		Body:   body,
	}
}

// TestBackendRoundTrip_ErrorClosesBody pins the pre-transport error leak:
// tls-client does NOT close the request body on errors raised before its
// inner transport runs (first-dial failures — connection refused, DNS, TLS
// verification, cert pinning, SSRF dial rejection, proxy CONNECT failure),
// and nothing above the adapter closes a custom RoundTripper's error return
// either. backend.RoundTrip must close it, or an *os.File body leaks a
// descriptor per failed attempt.
func TestBackendRoundTrip_ErrorClosesBody(t *testing.T) {
	var closes atomic.Int64
	b := backendWithStub(func(*ehttp.Request) (*ehttp.Response, error) {
		return nil, errors.New("simulated pre-transport dial failure")
	})

	if _, err := b.RoundTrip(forkReqWithBody(&countingCloser{closes: &closes})); err == nil {
		t.Fatal("stubbed Do should fail")
	}
	if n := closes.Load(); n != 1 {
		t.Errorf("body closed %d times on error, want exactly 1", n)
	}
}

// TestBackendRoundTrip_DoubleCloseIsOnce pins the idempotence that makes the
// unconditional error-path close safe: tls-client DOES close the body on its
// post-send error paths, so backend.RoundTrip's own close must not deliver a
// second Close to the underlying reader (pooled body readers must return to
// their pool exactly once).
func TestBackendRoundTrip_DoubleCloseIsOnce(t *testing.T) {
	var closes atomic.Int64
	b := backendWithStub(func(req *ehttp.Request) (*ehttp.Response, error) {
		// Simulate tls-client's post-send error behavior: it closes the body
		// itself, then errors.
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errors.New("simulated post-send failure")
	})

	if _, err := b.RoundTrip(forkReqWithBody(&countingCloser{closes: &closes})); err == nil {
		t.Fatal("stubbed Do should fail")
	}
	if n := closes.Load(); n != 1 {
		t.Errorf("body closed %d times (transport closed + our close), want exactly 1", n)
	}
}

// TestBackendRoundTrip_SuccessLeavesBodyToTransport pins the flip side: on
// success the transport owns the body lifecycle (it closes it once the
// request is fully sent) — the backend must not close it eagerly, which
// would abort an in-flight upload.
func TestBackendRoundTrip_SuccessLeavesBodyToTransport(t *testing.T) {
	var closes atomic.Int64
	b := backendWithStub(func(req *ehttp.Request) (*ehttp.Response, error) {
		_, _ = io.Copy(io.Discard, req.Body) // transport consumes it
		return &ehttp.Response{StatusCode: 200, Body: ehttp.NoBody, Header: make(ehttp.Header)}, nil
	})

	if _, err := b.RoundTrip(forkReqWithBody(&countingCloser{closes: &closes})); err != nil {
		t.Fatalf("stubbed Do should succeed: %v", err)
	}
	if n := closes.Load(); n != 0 {
		t.Errorf("backend closed the body %d times on success, want 0 (transport owns it)", n)
	}
}
