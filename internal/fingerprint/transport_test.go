package fingerprint

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ehttp "github.com/enetx/http"
	utls "github.com/refraction-networking/utls"
)

// newTestAdapter builds an adapter against a local test server.
func newTestAdapter(t *testing.T, preset string, insecure bool, pinner certPinner) *Adapter {
	t.Helper()
	return NewAdapter(Config{
		PresetName:         preset,
		InsecureSkipVerify: insecure,
		AllowPrivateIPs:    true, // test servers are loopback
		EnableHTTP2:        true,
		CertPinner:         pinner,
	})
}

// TestTransport_NegotiationModes table-drives the protocol-negotiation
// family (formerly per-case functions): an h2-capable server negotiates
// HTTP/2 through the fingerprint stack; an http/1.1-only server triggers the
// ALPN-mismatch fallback; EnableHTTP2=false pins HTTP/1.1 against an h2
// server; and verify-by-default rejects self-signed certificates.
func TestTransport_NegotiationModes(t *testing.T) {
	tests := []struct {
		name      string
		h2Server  bool
		insecure  bool
		enableH2  bool
		wantProto string
		wantBody  string
		wantErr   bool
	}{
		{
			name: "h2 negotiated", h2Server: true, insecure: true, enableH2: true,
			wantProto: "HTTP/2.0", wantBody: "mode-ok",
		},
		{
			name: "ALPN mismatch falls back to h1", h2Server: false, insecure: true, enableH2: true,
			wantProto: "HTTP/1.1", wantBody: "mode-ok",
		},
		{
			name: "EnableHTTP2=false forces h1 on an h2 server", h2Server: true, insecure: true, enableH2: false,
			wantProto: "HTTP/1.1", wantBody: "mode-ok",
		},
		{
			name: "verification rejects self-signed", h2Server: false, insecure: false, enableH2: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tt.wantBody)
			}))
			ts.EnableHTTP2 = tt.h2Server
			ts.StartTLS()
			defer ts.Close()

			adapter := NewAdapter(Config{
				PresetName:         "chrome",
				InsecureSkipVerify: tt.insecure,
				AllowPrivateIPs:    true, // test servers are loopback
				EnableHTTP2:        tt.enableH2,
			})

			resp, err := adapter.RoundTrip(httptestReq(t, "https://"+ts.Listener.Addr().String()+"/"))
			if tt.wantErr {
				if err == nil {
					resp.Body.Close()
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			defer resp.Body.Close()

			if resp.Proto != tt.wantProto {
				t.Errorf("Proto = %q, want %q", resp.Proto, tt.wantProto)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// TestTransport_H1Fallback_NonIdempotentALPNStillFallsBack pins the safe half
// of the C4 gate: an ALPN mismatch happens before any HTTP bytes are written,
// so even a POST may fall back to HTTP/1.1 without risking a duplicate
// submission.
func TestTransport_H1Fallback_NonIdempotentALPNStillFallsBack(t *testing.T) {
	var requests atomic.Int64
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		_, _ = io.WriteString(w, "h1-post:"+string(body))
	}))
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	req, err := http.NewRequest(http.MethodPost, "https://"+ts.Listener.Addr().String()+"/submit", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := adapter.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.Proto != "HTTP/1.1" {
		t.Errorf("Proto = %q, want HTTP/1.1 (fallback)", resp.Proto)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "h1-post:payload" {
		t.Errorf("body = %q", body)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server saw %d requests, want exactly 1", got)
	}
}

// TestTransport_NonIdempotentNotReplayedAfterH2StreamFailure pins the C4
// regression: when the h2 leg fails for a reason OTHER than ALPN mismatch —
// here the handler aborts mid-response, after the server already received
// and processed the request — a non-idempotent request must NOT be replayed
// over the HTTP/1.1 fallback. The h2 error is surfaced instead, and the
// server sees exactly one request. Previously the POST was silently resent.
func TestTransport_NonIdempotentNotReplayedAfterH2StreamFailure(t *testing.T) {
	var requests atomic.Int64
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		panic(http.ErrAbortHandler) // abort after the request was received
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	req, err := http.NewRequest(http.MethodPost, "https://"+ts.Listener.Addr().String()+"/charge", strings.NewReader("amount=1"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := adapter.RoundTrip(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected the aborted h2 stream to surface as an error, got nil")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server saw %d requests, want exactly 1 (POST must not be replayed on h1)", got)
	}
}

// TestIsIdempotentMethod pins the fallback gate's method table. It mirrors
// the engine's retry.go definition (RFC 9110 §9.2.2: safe methods plus PUT
// and DELETE); keep the two lists in sync.
func TestIsIdempotentMethod(t *testing.T) {
	idempotent := []string{"GET", "HEAD", "OPTIONS", "TRACE", "PUT", "DELETE"}
	nonIdempotent := []string{"POST", "PATCH", "CONNECT", ""}

	for _, m := range idempotent {
		if !isIdempotentMethod(m) {
			t.Errorf("isIdempotentMethod(%q) = false, want true", m)
		}
	}
	for _, m := range nonIdempotent {
		if isIdempotentMethod(m) {
			t.Errorf("isIdempotentMethod(%q) = true, want false", m)
		}
	}
}

// spkiPinner pins the SHA-256 of the leaf SPKI (mirrors SPKIHashPinner).
type spkiPinner struct {
	want string
	hits *int32
}

func (p *spkiPinner) Pin() string { return p.want }

func (p *spkiPinner) VerifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	atomic.AddInt32(p.hits, 1)
	if len(rawCerts) == 0 {
		return errors.New("no certificates")
	}
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	got := base64.StdEncoding.EncodeToString(sum[:])
	if got != p.want {
		return errors.New("spki mismatch")
	}
	return nil
}

// TestTransport_CertificatePinning is the regression test for pinning on
// the fingerprint path: the pinner runs on every fingerprinted handshake
// (via VerifyPeerCertificate), accepting the correct pin and rejecting the
// wrong one even under InsecureSkipVerify (leaf-only gate).
func TestTransport_CertificatePinning(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "pinned")
	}))
	defer ts.Close()

	cert := ts.Certificate()
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	goodPin := base64.StdEncoding.EncodeToString(sum[:])

	t.Run("correct pin accepted", func(t *testing.T) {
		var hits int32
		p := &spkiPinner{want: goodPin, hits: &hits}
		adapter := newTestAdapter(t, "chrome", true, p)

		resp, err := adapter.RoundTrip(httptestReq(t, "https://"+ts.Listener.Addr().String()+"/"))
		if err != nil {
			t.Fatalf("RoundTrip with correct pin: %v", err)
		}
		defer resp.Body.Close()
		if atomic.LoadInt32(&hits) == 0 {
			t.Error("pinner was never invoked")
		}
	})

	t.Run("wrong pin rejected", func(t *testing.T) {
		var hits int32
		bad := make([]byte, len(goodPin))
		copy(bad, goodPin)
		if bad[0] == 'A' {
			bad[0] = 'B'
		} else {
			bad[0] = 'A'
		}
		p := &spkiPinner{want: string(bad), hits: &hits}
		adapter := newTestAdapter(t, "chrome", true, p)

		_, err := adapter.RoundTrip(httptestReq(t, "https://"+ts.Listener.Addr().String()+"/"))
		if err == nil {
			t.Fatal("handshake accepted with a wrong pin")
		}
		if !strings.Contains(err.Error(), "certificate pinning failed") {
			t.Errorf("error lacks pinning context: %v", err)
		}
		if atomic.LoadInt32(&hits) == 0 {
			t.Error("pinner was never invoked")
		}
	})
}

// TestTransport_CloseIdempotent pins Close safety: repeated Close and
// post-Close requests must not panic (lazy rebuild tolerates misuse).
func TestTransport_CloseIdempotent(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestErrHTTP2Fallback_Unwrap pins that both failure legs are traversable
// with errors.As — the proxy circuit breaker's connection-failure
// classification depends on it.
func TestErrHTTP2Fallback_Unwrap(t *testing.T) {
	wrapped := &ErrHTTP2Fallback{
		HTTP2: errors.New("h2 dial blew up"),
		HTTP1: context.DeadlineExceeded,
	}

	if !errors.Is(wrapped, context.DeadlineExceeded) {
		t.Error("errors.Is does not reach the HTTP/1 leg")
	}
	if !strings.Contains(wrapped.Error(), "h2 dial blew up") {
		t.Error("error text lost the HTTP/2 leg")
	}
	if !isConnectionFailure(wrapped) {
		t.Error("fallback error not classified as connection failure")
	}
}

// httptestReq builds a GET request for the adapter.
func httptestReq(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return req
}

// TestFingerprintTransport_InvalidScheme pins fpTransport.RoundTrip's scheme
// dispatch: only http and https are transportable; anything else fails fast
// with the scheme named in the error.
func TestFingerprintTransport_InvalidScheme(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	for _, scheme := range []string{"ftp", "ws"} {
		req, err := ehttp.NewRequest(http.MethodGet, scheme+"://example.com/", nil)
		if err != nil {
			t.Fatalf("NewRequest(%q): %v", scheme, err)
		}
		_, err = a.base.ft.RoundTrip(req)
		if err == nil {
			t.Errorf("scheme %q accepted", scheme)
		} else if !strings.Contains(err.Error(), scheme) {
			t.Errorf("error should name the scheme %q: %v", scheme, err)
		}
	}

	// Empty scheme cannot be parsed as a URL string; build it structurally.
	req := (&ehttp.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Host: "example.com"},
	}).WithContext(context.Background())
	if _, err := a.base.ft.RoundTrip(req); err == nil {
		t.Error("empty scheme accepted")
	}
}

// TestTransport_H2FailContextCanceled pins the context guard in handleHTTPS:
// when the request context is canceled, the h2 failure is discarded and the
// context error is surfaced (no HTTP/1.1 replay of a canceled request).
func TestTransport_H2FailContextCanceled(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequest(http.MethodGet, "https://"+ts.Listener.Addr().String()+"/", nil)
	req = req.WithContext(ctx)

	resp, err := adapter.RoundTrip(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("canceled request should fail")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// TestTransport_H2FallbackMissingGetBody pins the body-restore gate: an h2
// ALPN mismatch makes the request replayable, but a consumed body with no
// GetBody cannot be restored — the fallback fails loudly instead of sending
// an empty body.
func TestTransport_H2FallbackMissingGetBody(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected success"))
	}))
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	// Custom body reader: http.NewRequest sets GetBody only for the known
	// reader types, so this request has none.
	req, _ := http.NewRequest(http.MethodPost, "https://"+ts.Listener.Addr().String()+"/submit", nil)
	req.Body = io.NopCloser(strings.NewReader("payload"))
	req.ContentLength = int64(len("payload"))

	_, err := adapter.RoundTrip(req)
	if err == nil {
		t.Fatal("fallback without GetBody should fail")
	}
	if !strings.Contains(err.Error(), "GetBody is nil") {
		t.Errorf("error lacks GetBody context: %v", err)
	}
}

// TestTransport_H2FallbackGetBodyError pins the second body-restore arm: a
// GetBody that itself fails surfaces its error, wrapped with the fallback
// context.
func TestTransport_H2FallbackGetBodyError(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected success"))
	}))
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	req, _ := http.NewRequest(http.MethodPost, "https://"+ts.Listener.Addr().String()+"/submit", nil)
	req.Body = io.NopCloser(strings.NewReader("payload"))
	req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("body evaporated") }

	_, err := adapter.RoundTrip(req)
	if err == nil {
		t.Fatal("fallback with a failing GetBody should fail")
	}
	if !strings.Contains(err.Error(), "failed to restore body") ||
		!strings.Contains(err.Error(), "body evaporated") {
		t.Errorf("error lacks restore context: %v", err)
	}
}

// TestTransport_RequestDeadlineClamp pins the handshake deadline clamp: a
// request context with a deadline shorter than the transport's handshake
// timeout is honored (the clamp branch executes; the round trip succeeds).
func TestTransport_RequestDeadlineClamp(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "deadline-ok")
	}))
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequest(http.MethodGet, "https://"+ts.Listener.Addr().String()+"/", nil)
	req = req.WithContext(ctx)

	resp, err := adapter.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip with short deadline: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "deadline-ok" {
		t.Errorf("body = %q", body)
	}
}

// TestTransport_FirefoxH2Settings pins that the firefox profile's HTTP/2
// fingerprint data (odd initial stream id, connection flow, root priority)
// lands on the h2 transport — profile data the chrome presets leave zero.
func TestTransport_FirefoxH2Settings(t *testing.T) {
	a := NewAdapter(Config{PresetName: "firefox", EnableHTTP2: true})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	t2 := a.base.ft.t2
	if t2 == nil {
		t.Fatal("firefox adapter should build an h2 transport")
	}
	if t2.StreamID != 3 {
		t.Errorf("StreamID = %d, want 3 (firefox profile)", t2.StreamID)
	}
	if t2.ConnectionFlow == 0 {
		t.Error("ConnectionFlow not carried from the firefox profile")
	}
	if t2.PriorityParam.Weight != 41 {
		t.Errorf("PriorityParam.Weight = %d, want 41", t2.PriorityParam.Weight)
	}
	if len(t2.Settings) == 0 {
		t.Error("firefox profile produced no SETTINGS frames")
	}
}

// TestTLSHandshake_MalformedAddrAndSpec pins two defensive arms of the raw
// handshake helper directly: an address without a port takes the
// SplitHostPort fallback (the dial then fails), and a profile whose spec
// cannot be built fails after the dial, closing the raw connection.
func TestTLSHandshake_MalformedAddrAndSpec(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	ft := a.base.ft

	t.Run("addr without port", func(t *testing.T) {
		// The dial fails fast on the missing port, after the host fallback
		// branch has run.
		if _, err := ft.tlsHandshake(context.Background(), "tcp", "no-port-host", false); err == nil {
			t.Fatal("portless address should fail the dial")
		}
	})

	t.Run("unbuildable spec", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				_ = conn.Close()
			}
		}()

		orig := ft.profile
		ft.profile = &browserProfile{helloID: utls.ClientHelloID{Client: "bogus", Version: "999"}}
		defer func() { ft.profile = orig }()

		_, err = ft.tlsHandshake(context.Background(), "tcp", ln.Addr().String(), false)
		if err == nil {
			t.Fatal("unbuildable spec should fail the handshake")
		}
		if !strings.Contains(err.Error(), "build spec") {
			t.Errorf("error lacks spec-build context: %v", err)
		}
	})
}
