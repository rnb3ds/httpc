package fingerprint

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	ehttp "github.com/bogdanfinn/fhttp"
	utls "github.com/bogdanfinn/utls"

	"github.com/cybergodev/httpc/internal/connection"
)

// TestBackendForAfterClose verifies the post-Close guard: a request racing
// Close must not panic on the nilled per-proxy map.
func TestBackendForAfterClose(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome-latest"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	u := &url.URL{Scheme: "http", Host: "proxy.example.com:8080"}
	backend, err := a.backendFor(u) // must not panic
	if err != nil {
		t.Fatalf("backendFor after Close: %v", err)
	}
	if backend == nil {
		t.Fatal("backendFor should rebuild lazily after Close")
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close should be idempotent: %v", err)
	}
}

// TestNewBackendWithProxy verifies a per-proxy backend builds with the proxy
// wired in and is cached under its URL key.
func TestNewBackendWithProxy(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome-windows"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	u := &url.URL{Scheme: "http", Host: "proxy.example.com:8080"}
	b1, err := a.backendFor(u)
	if err != nil {
		t.Fatalf("backendFor(proxy): %v", err)
	}
	b2, err := a.backendFor(u)
	if err != nil {
		t.Fatalf("backendFor(proxy) second call: %v", err)
	}
	if b1 != b2 {
		t.Error("per-proxy backends should be cached, not rebuilt per request")
	}
}

// TestNewBackendUnknownPreset verifies an unvalidated preset name surfaces
// as a RoundTrip error rather than a panic (the public config layer is the
// real gate; this is defense in depth).
func TestNewBackendUnknownPreset(t *testing.T) {
	a := NewAdapter(Config{PresetName: "netscape"})
	if a.baseErr == nil {
		t.Fatal("unknown preset should fail the base build")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if _, err := a.RoundTrip(req); err == nil {
		t.Error("RoundTrip should surface the construction error")
	}
}

// TestAdapterCloseWithProxies exercises Close with populated per-proxy map.
func TestAdapterCloseWithProxies(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome-latest"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	for _, host := range []string{"p1.example.com:8080", "p2.example.com:8080"} {
		if _, err := a.backendFor(&url.URL{Scheme: "http", Host: host}); err != nil {
			t.Fatalf("backendFor(%s): %v", host, err)
		}
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestIsProxyLegFailure pins the proxy circuit-breaker error classifier:
// ONLY tunnel-establishment failures (proxyLegError, however deeply the
// transport wraps them) count against a proxy; nil, caller cancellations,
// and — the regression this guards — target-leg failures (origin timeouts
// and TLS verification errors through an established tunnel, which the old
// wholesale net.Error classification counted) must not.
func TestIsProxyLegFailure(t *testing.T) {
	dialErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not a failure", nil, false},
		{"proxy-leg dial failure", &proxyLegError{err: dialErr}, true},
		{"wrapped proxy-leg failure", fmt.Errorf("RoundTrip: %w", &proxyLegError{err: dialErr}), true},
		{
			"url-wrapped proxy-leg failure",
			&url.Error{Op: "Post", URL: "https://example.com/", Err: &proxyLegError{err: dialErr}},
			true,
		},
		{
			"proxy-leg dns failure",
			&proxyLegError{err: &net.DNSError{Err: "no such host", Name: "proxy.example.com"}},
			true,
		},
		// Target-leg failures through a healthy proxy: the proxy did its
		// job, so these report SUCCESS — previously both rows opened the
		// proxy's circuit after the failure threshold.
		{
			"target read timeout (net.Error, no proxy-leg mark)",
			&net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded},
			false,
		},
		{
			"url-wrapped target timeout",
			&url.Error{Op: "Get", URL: "https://example.com/", Err: context.DeadlineExceeded},
			false,
		},
		// uTLS's type, not crypto/tls's: the fingerprint handshake runs on
		// uTLS, so a broken TARGET certificate surfaces this way.
		{"utls target verification error", &utls.CertificateVerificationError{Err: errors.New("x509: unknown authority")}, false},
		{
			"url-wrapped utls target verification error",
			&url.Error{Op: "Get", URL: "https://example.com/", Err: &utls.CertificateVerificationError{Err: errors.New("x509: unknown authority")}},
			false,
		},
		{"context canceled by caller", context.Canceled, false},
		{"context canceled under proxy-leg wrap", &proxyLegError{err: context.Canceled}, false},
		{"arbitrary request error", errors.New("bad request"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProxyLegFailure(tt.err); got != tt.want {
				t.Errorf("isProxyLegFailure(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestPrepareWireRequest pins the wire-preparation rules: the request is
// cloned (the profile header pipeline mutates headers in place and must not
// leak into the engine's request), httpc's default UA and the engine-auto
// Accept-Encoding are dropped, and everything else — explicit headers, body,
// GetBody, context — is preserved.
func TestPrepareWireRequest(t *testing.T) {
	t.Run("drops default httpc user-agent", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.Header.Set("User-Agent", defaultUserAgent)
		if out := prepareWireRequest(req); out.Header.Get("User-Agent") != "" {
			t.Error("default httpc UA should be dropped so the preset UA applies")
		}
	})

	t.Run("forwards explicit user-agent", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.Header.Set("User-Agent", "myapp/2.0")
		if out := prepareWireRequest(req); out.Header.Get("User-Agent") != "myapp/2.0" {
			t.Errorf("User-Agent = %q, want myapp/2.0", out.Header.Get("User-Agent"))
		}
	})

	t.Run("drops engine-auto accept-encoding so preset value survives", func(t *testing.T) {
		for _, auto := range []string{"gzip, deflate", "identity"} {
			req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			req.Header.Set("Accept-Encoding", auto)
			if out := prepareWireRequest(req); out.Header.Get("Accept-Encoding") != "" {
				t.Errorf("engine auto Accept-Encoding %q should be dropped", auto)
			}
		}
	})

	t.Run("forwards explicit accept-encoding", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.Header.Set("Accept-Encoding", "br")
		if out := prepareWireRequest(req); out.Header.Get("Accept-Encoding") != "br" {
			t.Errorf("Accept-Encoding = %q, want br", out.Header.Get("Accept-Encoding"))
		}
	})

	t.Run("clone isolation and field preservation", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "https://example.com/echo", strings.NewReader("payload"))
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("X-Custom", "keep")
		if req.GetBody == nil {
			t.Fatal("test setup: NewRequest should have set GetBody for *strings.Reader")
		}

		out := prepareWireRequest(req)

		if out == req {
			t.Error("prepareWireRequest must return a clone, not the same request")
		}
		if out.Header.Get("User-Agent") != "" {
			t.Error("default UA dropped on the clone")
		}
		if out.Header.Get("X-Custom") != "keep" {
			t.Error("custom header must pass through")
		}
		// The engine's original request must be untouched.
		if req.Header.Get("User-Agent") != defaultUserAgent {
			t.Error("original request headers must not be mutated by wire preparation")
		}
		if out.ContentLength != req.ContentLength {
			t.Errorf("ContentLength = %d, want %d (Content-Length framing relies on it)", out.ContentLength, req.ContentLength)
		}
		if out.Body != req.Body {
			t.Error("body reader should be shared with the clone for this single attempt")
		}
		if out.GetBody == nil {
			t.Error("GetBody must be preserved for protocol-fallback replays")
		}
		if out.Context() != req.Context() {
			t.Error("context must be preserved (engine owns deadlines)")
		}
	})
}

// ctxKey is a private context key for parity tests.
type ctxKey string

// TestToForkRequest_FullFieldParity pins the stdlib→fork conversion: every
// field a request can carry survives verbatim (the fork's wire magic rides on
// Header, GetBody and Context), and the fork's Response back-reference stays
// nil on outgoing requests.
func TestToForkRequest_FullFieldParity(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey("marker"), "engine-ctx")

	cancelCh := make(chan struct{})
	req := &http.Request{
		Method:     http.MethodPost,
		URL:        &url.URL{Scheme: "https", Host: "example.com:8443", Path: "/submit"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"User-Agent":         {"test/1.0"},
			"X-Multi":            {"a", "b"},
			ehttp.HeaderOrderKey: {"user-agent", "x-multi"},
		},
		Body:             io.NopCloser(strings.NewReader("body")),
		ContentLength:    42,
		TransferEncoding: []string{"chunked"},
		Host:             "override.example.com",
		Form:             url.Values{"q": {"fork"}},
		PostForm:         url.Values{"token": {"post"}},
		MultipartForm:    &multipart.Form{Value: map[string][]string{"f": {"v"}}},
		Trailer:          http.Header{"X-Trailer": {"end"}},
		RemoteAddr:       "10.0.0.1:1234",
		RequestURI:       "/original?uri=1",
		TLS:              &tls.ConnectionState{NegotiatedProtocol: "h2"},
		Cancel:           cancelCh, //nolint:staticcheck // parity of the deprecated field is the point; the engine never sets it
		Pattern:          "/submit",
	}
	req = req.WithContext(ctx)

	fork := toForkRequest(req)

	if fork.Method != req.Method {
		t.Errorf("Method not carried: %q != %q", fork.Method, req.Method)
	}
	// URL is deliberately not pointer-compared: the fork's WithContext clones
	// it (legacy net/http behaviour, Issue 23544), so identity is expected to
	// differ while the value must not.
	if fork.URL == nil || fork.URL.String() != req.URL.String() {
		t.Errorf("URL not carried: %v != %v", fork.URL, req.URL)
	}
	if fork.Proto != "HTTP/1.1" || fork.ProtoMajor != 1 || fork.ProtoMinor != 1 {
		t.Errorf("proto fields = %q %d.%d", fork.Proto, fork.ProtoMajor, fork.ProtoMinor)
	}
	if fork.Header.Get("User-Agent") != "test/1.0" {
		t.Errorf("Header not carried: %v", fork.Header)
	}
	if len(fork.Header["X-Multi"]) != 2 {
		t.Errorf("multi-value header collapsed: %v", fork.Header["X-Multi"])
	}
	if len(fork.Header[ehttp.HeaderOrderKey]) != 2 {
		t.Errorf("HeaderOrderKey must pass through the conversion, got %v", fork.Header[ehttp.HeaderOrderKey])
	}
	if fork.Body != req.Body {
		t.Error("Body reader not shared")
	}
	if fork.ContentLength != 42 {
		t.Errorf("ContentLength = %d, want 42", fork.ContentLength)
	}
	if len(fork.TransferEncoding) != 1 || fork.TransferEncoding[0] != "chunked" {
		t.Errorf("TransferEncoding = %v", fork.TransferEncoding)
	}
	if fork.Host != "override.example.com" {
		t.Errorf("Host = %q", fork.Host)
	}
	if fork.Form.Get("q") != "fork" || fork.PostForm.Get("token") != "post" {
		t.Errorf("Form/PostForm not carried: %v %v", fork.Form, fork.PostForm)
	}
	if fork.MultipartForm == nil || len(fork.MultipartForm.Value["f"]) != 1 {
		t.Errorf("MultipartForm not carried: %v", fork.MultipartForm)
	}
	if fork.Trailer.Get("X-Trailer") != "end" {
		t.Errorf("Trailer not carried: %v", fork.Trailer)
	}
	if fork.RemoteAddr != "10.0.0.1:1234" || fork.RequestURI != "/original?uri=1" {
		t.Errorf("RemoteAddr/RequestURI = %q %q", fork.RemoteAddr, fork.RequestURI)
	}
	if fork.TLS != nil {
		t.Error("outgoing fork requests must carry no TLS state")
	}
	if fork.Response != nil {
		t.Error("fork Response back-reference must be nil on outgoing requests")
	}
	if fork.Cancel != cancelCh { //nolint:staticcheck // parity of the deprecated field is the point
		t.Error("Cancel channel not carried for parity")
	}
	if fork.Context() != ctx {
		t.Error("WithContext must preserve the engine's context (deadlines ride on it)")
	}
	if v, ok := fork.Context().Value(ctxKey("marker")).(string); !ok || v != "engine-ctx" {
		t.Error("context values lost in conversion")
	}
}

// TestToForkRequest_MinimalFields pins the pass-through branches for a bare
// request: nothing is invented, nothing panics.
func TestToForkRequest_MinimalFields(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	fork := toForkRequest(req)

	if fork.Method != http.MethodGet || fork.Host != "example.com" {
		t.Errorf("minimal conversion lost basics: %v %v", fork.Method, fork.Host)
	}
	if fork.Form != nil || fork.PostForm != nil || fork.MultipartForm != nil {
		t.Error("form fields invented for a bodyless GET")
	}
	if fork.Trailer != nil || fork.TransferEncoding != nil {
		t.Error("trailer/transfer-encoding invented")
	}
	if fork.Context() != req.Context() {
		t.Error("default context not preserved")
	}
}

// TestToStdResponse_FullFieldParity pins the fork→stdlib response conversion,
// including the Request back-reference contract: the engine reads
// resp.Request for request metadata, so the original stdlib request's fields
// are restored — except Header, which is replaced with the fork's wire
// headers (browser casing, magic order keys dropped) so Result.Request.Headers
// reports what actually went out.
func TestToStdResponse_FullFieldParity(t *testing.T) {
	origReq, _ := http.NewRequest(http.MethodGet, "https://example.com/page", nil)
	origReq.Header.Set("User-Agent", "httpc/1.0")

	forkResp := &ehttp.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		Header: ehttp.Header{
			"Content-Type":   {"text/html"},
			"Content-Length": {"7"},
		},
		Body:             io.NopCloser(strings.NewReader("<html/>")),
		ContentLength:    7,
		TransferEncoding: []string{"identity"},
		Close:            true,
		Uncompressed:     true,
		Trailer:          ehttp.Header{"X-Trailer": {"done"}},
		TLS:              &utls.ConnectionState{NegotiatedProtocol: "h2"},
	}

	// The fork REQUEST headers are a separate map: the profile pipeline wrote
	// the browser values here; the fork's Response.Header is the server's.
	forkReqHeaders := ehttp.Header{
		"User-Agent":          {"Mozilla/5.0 ... Chrome/155.0"},
		"sec-ch-ua":           {`"Chromium";v="155"`},
		ehttp.HeaderOrderKey:  {"user-agent", "sec-ch-ua"},
		ehttp.PHeaderOrderKey: {":method", ":authority", ":scheme", ":path"},
	}

	resp := toStdResponse(forkResp, origReq, forkReqHeaders)

	if resp.Status != "200 OK" || resp.StatusCode != 200 {
		t.Errorf("status = %q %d", resp.Status, resp.StatusCode)
	}
	if resp.Proto != "HTTP/2.0" || resp.ProtoMajor != 2 || resp.ProtoMinor != 0 {
		t.Errorf("proto = %q %d.%d", resp.Proto, resp.ProtoMajor, resp.ProtoMinor)
	}
	if resp.Header.Get("Content-Type") != "text/html" {
		t.Errorf("response Header not carried: %v", resp.Header)
	}
	if resp.Body != forkResp.Body {
		t.Error("Body reader not shared (streaming ownership would break)")
	}
	if resp.ContentLength != 7 {
		t.Errorf("ContentLength = %d", resp.ContentLength)
	}
	if len(resp.TransferEncoding) != 1 || resp.TransferEncoding[0] != "identity" {
		t.Errorf("TransferEncoding = %v", resp.TransferEncoding)
	}
	if !resp.Close {
		t.Error("Close flag not carried")
	}
	if !resp.Uncompressed {
		t.Error("Uncompressed flag not carried")
	}
	if resp.Trailer.Get("X-Trailer") != "done" {
		t.Errorf("Trailer not converted: %v", resp.Trailer)
	}
	if resp.TLS == nil || resp.TLS.NegotiatedProtocol != "h2" {
		t.Errorf("TLS state not carried: %v", resp.TLS)
	}
	// Request back-reference: a clone (not the original), with the wire header
	// map in place of the engine-side header map.
	if resp.Request == nil || resp.Request == origReq {
		t.Fatalf("Request back-reference = %v, want a clone of the original", resp.Request)
	}
	if resp.Request.URL.String() != origReq.URL.String() {
		t.Errorf("report request URL = %q, want %q", resp.Request.URL, origReq.URL)
	}
	if got := resp.Request.Header.Get("User-Agent"); got != "Mozilla/5.0 ... Chrome/155.0" {
		t.Errorf("report request User-Agent = %q, want the wire (browser) UA", got)
	}
	// The browser stores sec-ch-ua under its lower-case key; Header.Get
	// canonicalises the lookup, so assert the raw key directly.
	if got := resp.Request.Header["sec-ch-ua"]; len(got) != 1 || got[0] != `"Chromium";v="155"` { //nolint:staticcheck // SA1008: deliberate — the fingerprint wire set keeps sec-ch-ua lower-case
		t.Errorf("report request sec-ch-ua = %v, want the wire value under browser casing", got)
	}
	if _, ok := resp.Request.Header[ehttp.HeaderOrderKey]; ok {
		t.Error("report request Header must drop the Header-Order magic key")
	}
	if _, ok := resp.Request.Header[ehttp.PHeaderOrderKey]; ok {
		t.Error("report request Header must drop the PHeader-Order magic key")
	}
	// The original request's header map must be untouched.
	if origReq.Header.Get("User-Agent") != "httpc/1.0" {
		t.Errorf("original request headers mutated: %v", origReq.Header)
	}
}

// TestToStdResponse_MinimalFields pins the zero-value conversion.
func TestToStdResponse_MinimalFields(t *testing.T) {
	req, _ := http.NewRequest(http.MethodHead, "https://example.com/", nil)
	resp := toStdResponse(&ehttp.Response{}, req, nil)

	if resp.StatusCode != 0 || resp.Body != nil || resp.Header != nil {
		t.Errorf("minimal response not passthrough: %+v", resp)
	}
	// Request back-reference is a clone carrying the (empty) wire header map.
	if resp.Request == nil || resp.Request == req {
		t.Error("Request back-reference must be a clone on the fingerprint path")
	}
	if resp.Request.URL.String() != req.URL.String() {
		t.Errorf("report request URL = %q, want %q", resp.Request.URL, req.URL)
	}
}

// TestAdapterRoundTrip_NilURL pins the guard arm: a request without a URL
// fails fast instead of panicking downstream.
func TestAdapterRoundTrip_NilURL(t *testing.T) {
	if _, err := (&Adapter{}).RoundTrip(&http.Request{}); err == nil {
		t.Error("nil URL should be rejected")
	} else if !strings.Contains(err.Error(), "no URL") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestBackendFor_UnsupportedProxyScheme pins the per-proxy construction
// error chain: backendFor → newBackend → newTransportBackend →
// newProxyDialerFactory rejects an unsupported scheme and the error surfaces to
// the caller (not a panic, no cache entry). http, https, socks4/4a and
// socks5/5h are all supported, so the probe uses a scheme no dialer knows.
func TestBackendFor_UnsupportedProxyScheme(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	for _, scheme := range []string{"ftp", "quic", ""} {
		_, err := a.backendFor(&url.URL{Scheme: scheme, Host: "127.0.0.1:1"})
		if err == nil {
			t.Fatalf("proxy scheme %q accepted", scheme)
		}
		if !strings.Contains(err.Error(), "proxy scheme") {
			t.Errorf("error should name the proxy scheme, got: %v", err)
		}
	}
	if len(a.perProxy) != 0 {
		t.Errorf("failed backend must not be cached, got %d entries", len(a.perProxy))
	}

	// The supported schemes must construct cleanly (no dial happens here).
	// socks4/socks4a are absent on purpose: x/net/proxy.FromURL implements
	// socks5/socks5h only, so accepting them would defer a guaranteed
	// "proxy: unknown scheme" failure to every dial.
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		if _, err := a.backendFor(&url.URL{Scheme: scheme, Host: "127.0.0.1:1"}); err != nil {
			t.Errorf("proxy scheme %q rejected: %v", scheme, err)
		}
	}
}

// TestAdapterRoundTrip_SSRFBlocksLoopback pins the pre-flight abort arm: with
// SSRF protection on, a loopback target fails the round trip before any dial.
func TestAdapterRoundTrip_SSRFBlocksLoopback(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"}) // AllowPrivateIPs unset
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	_, err := a.RoundTrip(req)
	if err == nil {
		t.Fatal("loopback target accepted with SSRF protection on")
	}
	if !strings.Contains(err.Error(), "SSRF") {
		t.Errorf("error lacks SSRF context: %v", err)
	}
}

// TestAdapterRoundTrip_UndecodableBodyFailsRoundTrip pins that a body whose
// declared encoding is unsupported fails the round trip instead of reaching
// the engine as compressed bytes with the headers stripped.
//
// (A gzip-branded garbage body cannot be used here: the fhttp fork does
// NOT auto-decompress on this path — its addedGzip gate only fires when the
// fork itself injected Accept-Encoding (i.e. the request carried no
// Accept-Encoding header), and DisableCompression is set on every fingerprint
// transport, so the fork never injects one. The profile pipeline always writes
// a browser Accept-Encoding, so decodeResponse, not the fork, owns
// decompression. The unsupported-token path is untouched by the fork.)
func TestAdapterRoundTrip_UndecodableBodyFailsRoundTrip(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "x-exotic")
		_, _ = w.Write([]byte("opaque-bytes"))
	}))
	defer ts.Close()

	a := newTestAdapter(t, "chrome", true, nil) // AllowPrivateIPs: loopback target

	resp, err := a.RoundTrip(httptestReq(t, ts.URL+"/"))
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("unsupported Content-Encoding accepted")
	}
	if !strings.Contains(err.Error(), "unsupported Content-Encoding") {
		t.Errorf("error lacks encoding context: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SSRF pre-flight (merged from ssrf_test.go — same concern as the adapter's
// RoundTrip, which invokes the pre-flight before every backend exchange).
// ---------------------------------------------------------------------------

func TestPreflightSSRF(t *testing.T) {
	newAdapter := func(allow bool, exempts ...string) *Adapter {
		a := &Adapter{cfg: Config{AllowPrivateIPs: allow}}
		for _, c := range exempts {
			_, network, err := net.ParseCIDR(c)
			if err != nil {
				t.Fatalf("ParseCIDR(%q): %v", c, err)
			}
			a.cfg.ExemptNets = append(a.cfg.ExemptNets, network)
		}
		return a
	}
	newReq := func(url string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		return req
	}

	t.Run("loopback literal blocked by default", func(t *testing.T) {
		a := newAdapter(false)
		if err := a.preflightSSRF(newReq("http://127.0.0.1/x")); err == nil {
			t.Error("loopback IP must be blocked when SSRF protection is on")
		}
	})

	t.Run("private literal blocked", func(t *testing.T) {
		a := newAdapter(false)
		if err := a.preflightSSRF(newReq("https://10.0.0.5/x")); err == nil {
			t.Error("private IP must be blocked when SSRF protection is on")
		}
	})

	t.Run("exempt CIDR allows matching literal", func(t *testing.T) {
		a := newAdapter(false, "10.0.0.0/8")
		if err := a.preflightSSRF(newReq("https://10.0.0.5/x")); err != nil {
			t.Errorf("exempted CIDR should pass: %v", err)
		}
	})

	t.Run("allow private bypasses everything", func(t *testing.T) {
		a := newAdapter(true)
		if err := a.preflightSSRF(newReq("http://127.0.0.1/x")); err != nil {
			t.Errorf("AllowPrivateIPs should skip the pre-flight: %v", err)
		}
	})

	t.Run("per-request override enables private access", func(t *testing.T) {
		a := newAdapter(false)
		req := newReq("http://127.0.0.1/x")
		ctx := connection.WithAllowPrivateIPsOverride(req.Context(), true)
		if err := a.preflightSSRF(req.WithContext(ctx)); err != nil {
			t.Errorf("context override should allow loopback: %v", err)
		}
	})

	t.Run("local hostname resolving to loopback is blocked", func(t *testing.T) {
		a := newAdapter(false)
		req := newReq("http://localhost/x")
		if err := a.preflightSSRF(req); err == nil {
			t.Error("localhost should resolve to a blocked address and fail")
		}
	})

	t.Run("local hostname exempted by CIDR passes", func(t *testing.T) {
		// The exemption filter runs on the RESOLVED addresses: localhost
		// resolves to loopback, the exempt CIDR admits it.
		a := newAdapter(false, "127.0.0.0/8")
		req := newReq("http://localhost/x")
		if err := a.preflightSSRF(req); err != nil {
			t.Errorf("exempted resolution should pass: %v", err)
		}
	})

	t.Run("unresolvable hostname fails fast", func(t *testing.T) {
		// The .invalid TLD is reserved to never resolve (RFC 6761): the
		// pre-flight surfaces the resolver failure instead of passing it on.
		a := newAdapter(false)
		req := newReq("http://no-such-host.invalid/x")
		if err := a.preflightSSRF(req); err == nil {
			t.Error("unresolvable hostname should fail the pre-flight")
		}
	})

	t.Run("empty host rejected", func(t *testing.T) {
		a := newAdapter(false)
		req := newReq("http:///x")
		if err := a.preflightSSRF(req); err == nil {
			t.Error("empty host must be rejected")
		}
	})
}

// TestPreflightSSRF_PublicServer exercises the pass path against a real local
// listener (allowed via AllowPrivateIPs) — an httptest server doubles as the
// backend target in the adapter e2e tests below.
func TestPreflightSSRF_PublicServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	a := &Adapter{cfg: Config{AllowPrivateIPs: true}}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err := a.preflightSSRF(req); err != nil {
		t.Fatalf("allowed target should pass pre-flight: %v", err)
	}
}
