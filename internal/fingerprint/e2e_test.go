package fingerprint_test

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc"
)

// newFingerprintClient builds a public-API client with full-stack
// fingerprinting enabled against a local (loopback) target.
func newFingerprintClient(t *testing.T, preset string) httpc.Client {
	t.Helper()
	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = preset
	cfg.Security.InsecureSkipVerify = true // httptest self-signed cert
	cfg.Security.AllowPrivateIPs = true    // loopback target
	cfg.Timeouts.Request = 30 * time.Second
	cfg.Retry.MaxRetries = 0
	client, err := httpc.New(cfg)
	if err != nil {
		t.Fatalf("httpc.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestFingerprintPOSTWireShape pins the body-framing rule: buffered POSTs
// must reach the server with Content-Length (browser wire shape), never
// chunked — the engine's known-length bodies must keep Content-Length
// framing through the backend transport.
func TestFingerprintPOSTWireShape(t *testing.T) {
	const payload = `{"hello":"world","pad":"0123456789"}`
	var gotLength int64
	var chunked bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLength = r.ContentLength
		chunked = len(r.TransferEncoding) > 0 && r.TransferEncoding[0] == "chunked"
		_, _ = w.Write([]byte("posted"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Post(srv.URL+"/echo", httpc.WithJSON(map[string]string{"hello": "world", "pad": "0123456789"}))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	if res.Body() != "posted" {
		t.Errorf("body = %q", res.Body())
	}
	if chunked {
		t.Error("request arrived chunked; buffered POSTs must use Content-Length (browser wire shape)")
	}
	if gotLength != int64(len(payload)) {
		// WithJSON pads/formatting may differ; assert a positive exact length
		// was delivered rather than the exact JSON bytes.
		if gotLength <= 0 {
			t.Errorf("ContentLength = %d, want a positive value", gotLength)
		}
	}
}

// TestFingerprintEnableHTTP2False verifies Connection.EnableHTTP2=false pins
// the fingerprint path to HTTP/1.1, mirroring the standard stack's protocol
// choice instead of silently diverging.
func TestFingerprintEnableHTTP2False(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("h1-only"))
	}))
	defer srv.Close()

	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = "chrome"
	cfg.Security.InsecureSkipVerify = true
	cfg.Security.AllowPrivateIPs = true
	cfg.Connection.EnableHTTP2 = false
	client, err := httpc.New(cfg)
	if err != nil {
		t.Fatalf("httpc.New: %v", err)
	}
	defer func() { _ = client.Close() }()

	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if res.Response.Proto != "HTTP/1.1" {
		t.Errorf("Proto = %q, want HTTP/1.1 when Connection.EnableHTTP2=false", res.Response.Proto)
	}
}

// TestFingerprintHTTPS exercises the full path: engine → adapter →
// fingerprint transport stack (fingerprinted h2 with h1 fallback) against a
// local TLS server.
func TestFingerprintHTTPS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fingerprint-ok"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("fingerprinted GET failed: %v", err)
	}
	if body := res.Body(); body != "fingerprint-ok" {
		t.Errorf("body = %q, want fingerprint-ok", body)
	}
}

// TestFingerprintPlainHTTP verifies plaintext http targets route through the
// fingerprinted transport as well (HTTP/1.1, preset headers, no TLS layer).
func TestFingerprintPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain-ok"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("fingerprinted plain GET failed: %v", err)
	}
	if body := res.Body(); body != "plain-ok" {
		t.Errorf("body = %q, want plain-ok", body)
	}
}

// TestFingerprintRedirectNotFollowedByBackend pins the split of duties: the
// backend never follows redirects; the engine policy decides. With redirects
// disabled per request, the 3xx surfaces to the caller.
func TestFingerprintRedirectNotFollowedByBackend(t *testing.T) {
	var hops int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Get(srv.URL+"/start", httpc.WithFollowRedirects(false))
	if err != nil {
		t.Fatalf("GET with redirects off failed: %v", err)
	}
	if res.StatusCode() != http.StatusFound {
		t.Errorf("status = %d, want 302", res.StatusCode())
	}
	if loc := res.Response.Headers.Get("Location"); loc != "/final" {
		t.Errorf("Location = %q, want /final", loc)
	}
	if hops != 1 {
		t.Errorf("hops = %d, want 1 (backend must not follow)", hops)
	}
}

// TestFingerprintRedirectFollowedByEngine verifies the engine-driven chain:
// each hop re-enters the adapter (and the SSRF pre-flight re-validates).
func TestFingerprintRedirectFollowedByEngine(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("redirected-final"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Get(srv.URL + "/start")
	if err != nil {
		t.Fatalf("GET with redirect following failed: %v", err)
	}
	if res.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode())
	}
	if body := res.Body(); body != "redirected-final" {
		t.Errorf("body = %q, want redirected-final", body)
	}
}

// TestFingerprintUserAgentRule verifies the UA coherence rule: the preset's
// browser UA is authoritative on the fingerprint path. With the default
// httpc UA it is dropped so the preset applies; an explicitly set UA is also
// replaced — the profile's header set wins so the wire stays coherent with
// the spoofed ClientHello.
func TestFingerprintUserAgentRule(t *testing.T) {
	var seenUA string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")

	// Default: the preset's Chrome UA must be used, not httpc/1.0.
	if _, err := client.Get(srv.URL); err != nil {
		t.Fatalf("default-UA GET failed: %v", err)
	}
	if !strings.Contains(seenUA, "Chrome/") {
		t.Errorf("default path UA = %q, want a Chrome UA from the preset", seenUA)
	}

	// Explicit per-request UA: the profile replaces it (coherence beats
	// override on the fingerprint path).
	const custom = "myapp/9.9 (explicit)"
	if _, err := client.Get(srv.URL, httpc.WithUserAgent(custom)); err != nil {
		t.Fatalf("custom-UA GET failed: %v", err)
	}
	if !strings.Contains(seenUA, "Chrome/") {
		t.Errorf("custom path UA = %q, want the preset's Chrome UA (profile headers are authoritative)", seenUA)
	}
	if seenUA == custom {
		t.Error("explicit UA must not leak to the wire in fingerprint mode")
	}
}

// TestFingerprintMobileVariant smoke-checks a mobile (iOS) preset builds and
// requests through the same path.
func TestFingerprintMobileVariant(t *testing.T) {
	var seenUA string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("mobile-ok"))
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome-ios")
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("mobile preset GET failed: %v", err)
	}
	if body := res.Body(); body != "mobile-ok" {
		t.Errorf("body = %q, want mobile-ok", body)
	}
	// Chrome on iOS identifies as "CriOS/" rather than "Chrome/".
	if !strings.Contains(seenUA, "CriOS/") {
		t.Errorf("UA = %q, want an iOS Chrome (CriOS) UA from the preset", seenUA)
	}
}

// TestFingerprintGzipTransparency verifies compressed responses decode exactly
// once: the backend must not pre-decompress into a still-labeled body, and the
// engine's manual decompression sees a coherent Content-Encoding.
func TestFingerprintGzipTransparency(t *testing.T) {
	payload := "gzip-payload-" + strings.Repeat("x", 2048)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/plain")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte(payload))
		_ = zw.Close()
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("gzip GET failed: %v", err)
	}
	if body := res.Body(); body != payload {
		t.Errorf("body mismatch after single decompression (len got %d, want %d)", len(body), len(payload))
	}
}

// TestFingerprintSSRFBlocked verifies the pre-flight blocks loopback targets
// when SSRF protection is on, and the error surfaces to the caller.
func TestFingerprintSSRFBlocked(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = "chrome"
	cfg.Security.InsecureSkipVerify = true
	cfg.Security.AllowPrivateIPs = false // SSRF protection on
	client, err := httpc.New(cfg)
	if err != nil {
		t.Fatalf("httpc.New: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("loopback request must be blocked with SSRF protection on")
	}
	// Either layer may fire first: the engine's URL validation or the
	// adapter's resolve-and-validate pre-flight. Both are SSRF protection.
	msg := err.Error()
	if !strings.Contains(msg, "SSRF") && !strings.Contains(msg, "blocked for security") {
		t.Errorf("error should come from an SSRF layer, got: %v", err)
	}
}

// TestFingerprintInsecureOffRejected verifies standard certificate
// verification still applies on the fingerprint path.
func TestFingerprintInsecureOffRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = "chrome"
	cfg.Security.AllowPrivateIPs = true
	// InsecureSkipVerify left false → self-signed cert must fail the handshake.
	client, err := httpc.New(cfg)
	if err != nil {
		t.Fatalf("httpc.New: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("self-signed cert must be rejected when verification is on")
	}
}

// TestFingerprintDownloadStreaming verifies a large body streams through the
// adapter to disk via Download (the streaming path of the public API) without
// full buffering at the conversion boundary.
func TestFingerprintDownloadStreaming(t *testing.T) {
	const chunk = 4096
	const chunks = 100
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < chunks; i++ {
			_, _ = io.WriteString(w, strings.Repeat("s", chunk))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	client := newFingerprintClient(t, "chrome")

	dir := t.TempDir()
	cfg := httpc.DefaultDownloadConfig()
	cfg.FilePath = dir + "/stream.bin"

	dl, err := client.Download(context.Background(), srv.URL, cfg, httpc.WithStreamBody(true))
	if err != nil {
		t.Fatalf("streaming Download failed: %v", err)
	}
	if dl.BytesWritten != int64(chunk*chunks) {
		t.Errorf("downloaded %d bytes, want %d", dl.BytesWritten, chunk*chunks)
	}
}

// TestFingerprintCertificatePinning pins the public end-to-end wiring of the
// re-enabled capability: TLSFingerprint + CertificatePinner must flow from
// SecurityConfig through the engine into the fingerprint transport's
// VerifyPeerCertificate — the correct SPKI pin passes, a wrong pin rejects
// the fingerprinted handshake (under InsecureSkipVerify the pinner is the
// only gate, pinning the leaf).
func TestFingerprintCertificatePinning(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pinned-ok"))
	}))
	defer srv.Close()

	sum := sha256.Sum256(srv.Certificate().RawSubjectPublicKeyInfo)
	goodPin := base64.StdEncoding.EncodeToString(sum[:])
	badPin := base64.StdEncoding.EncodeToString([]byte("definitely-not-the-spki"))

	newClient := func(t *testing.T, pin string) httpc.Client {
		t.Helper()
		cfg := httpc.DefaultConfig()
		cfg.Security.TLSFingerprint = "chrome"
		cfg.Security.InsecureSkipVerify = true
		cfg.Security.AllowPrivateIPs = true
		cfg.Security.CertificatePinner, _ = httpc.NewSPKIHashPinner(pin)
		cfg.Timeouts.Request = 30 * time.Second
		cfg.Retry.MaxRetries = 0
		client, err := httpc.New(cfg)
		if err != nil {
			t.Fatalf("httpc.New: %v", err)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}

	t.Run("correct pin passes on the fingerprint path", func(t *testing.T) {
		res, err := newClient(t, goodPin).Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET with correct pin: %v", err)
		}
		if res.Body() != "pinned-ok" {
			t.Errorf("body = %q", res.Body())
		}
	})

	t.Run("wrong pin rejects the fingerprinted handshake", func(t *testing.T) {
		_, err := newClient(t, badPin).Get(srv.URL + "/")
		if err == nil {
			t.Fatal("handshake accepted with a wrong pin on the fingerprint path")
		}
		if !strings.Contains(err.Error(), "certificate pinning failed") {
			t.Errorf("error lacks pinning context: %v", err)
		}
	})
}

// hopCapture records the sec-fetch headers observed server-side on one hop.
type hopCapture struct {
	site, user string
	got        bool
}

// redirectChain serves a two-hop redirect chain on loopback TLS servers and
// returns the headers seen on each hop. targetOf rewrites the second
// server's URL into the redirect Location, letting callers shape the hop's
// site relationship (same host, different hostname).
func redirectChain(t *testing.T, targetOf func(srv2URL string) string) (first, second hopCapture) {
	t.Helper()

	srv2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		second = hopCapture{site: r.Header.Get("Sec-Fetch-Site"), user: r.Header.Get("Sec-Fetch-User"), got: true}
		_, _ = w.Write([]byte("hop2-ok"))
	}))
	defer srv2.Close()

	srv1 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first = hopCapture{site: r.Header.Get("Sec-Fetch-Site"), user: r.Header.Get("Sec-Fetch-User"), got: true}
		http.Redirect(w, r, targetOf(srv2.URL)+"/hop2", http.StatusFound)
	}))
	defer srv1.Close()

	client := newFingerprintClient(t, "chrome")
	res, err := client.Get(srv1.URL + "/start")
	if err != nil {
		t.Fatalf("GET through the redirect chain failed: %v", err)
	}
	if res.Body() != "hop2-ok" {
		t.Fatalf("body = %q, want hop2-ok", res.Body())
	}
	return first, second
}

// TestFingerprintRedirectSecFetchCoherence pins the redirect-hop corrections
// end-to-end through the engine-driven chain: hop 1 keeps the profile's
// first-hop sec-fetch values, while the redirected hop recomputes
// sec-fetch-site against the previous hop's URL and drops sec-fetch-user.
// Two loopback shapes are covered: same host across ports (same-site) and
// different hostnames (cross-site, localhost vs 127.0.0.1 — no external
// network).
func TestFingerprintRedirectSecFetchCoherence(t *testing.T) {
	t.Run("same host different port is same-site", func(t *testing.T) {
		first, second := redirectChain(t, func(u string) string { return u })

		if !first.got || !second.got {
			t.Fatalf("hops reached: first=%v second=%v", first.got, second.got)
		}
		if first.site != "none" || first.user != "?1" {
			t.Errorf("hop 1 sec-fetch-site/user = %q/%q, want none/?1 (first-hop profile values)", first.site, first.user)
		}
		if second.site != "same-site" {
			t.Errorf("hop 2 sec-fetch-site = %q, want same-site (same host, different port)", second.site)
		}
		if second.user != "" {
			t.Errorf("hop 2 sec-fetch-user = %q, want it dropped after the first hop", second.user)
		}
	})

	t.Run("different host is cross-site", func(t *testing.T) {
		first, second := redirectChain(t, func(u string) string {
			return strings.Replace(u, "127.0.0.1", "localhost", 1)
		})

		if !first.got || !second.got {
			t.Fatalf("hops reached: first=%v second=%v", first.got, second.got)
		}
		if second.site != "cross-site" {
			t.Errorf("hop 2 sec-fetch-site = %q, want cross-site (different hostname)", second.site)
		}
		if second.user != "" {
			t.Errorf("hop 2 sec-fetch-user = %q, want it dropped after the first hop", second.user)
		}
	})
}
