package fingerprint

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ehttp "github.com/bogdanfinn/fhttp"
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

// TestTransport_NegotiationModes table-drives protocol negotiation: an
// h2-capable server negotiates HTTP/2 through the fingerprint stack, an
// http/1.1-only server lands on HTTP/1.1, EnableHTTP2=false pins HTTP/1.1
// against an h2 server (and rewrites ALPN so the ClientHello agrees), and
// verify-by-default rejects self-signed certificates.
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
			name: "h1-only server negotiates h1", h2Server: false, insecure: true, enableH2: true,
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
					_ = resp.Body.Close()
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

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

// TestTransport_NonIdempotentNotReplayed pins the replay-safety invariant that
// survived the transport swap: a POST whose exchange fails after the server
// already received it must surface the error instead of being re-sent. The
// handler aborts mid-response, so a silent retry would show up as a second
// request on the counter.
func TestTransport_NonIdempotentNotReplayed(t *testing.T) {
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
		_ = resp.Body.Close()
		t.Fatal("expected the aborted stream to surface as an error, got nil")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server saw %d requests, want exactly 1 (POST must not be replayed)", got)
	}
}

// spkiPinner pins the SHA-256 of the leaf SPKI (mirrors SPKIHashPinner).
type spkiPinner struct {
	want string
}

func (p *spkiPinner) VerifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return errNoCerts
	}
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	if base64.StdEncoding.EncodeToString(sum[:]) != p.want {
		return errPinMismatch
	}
	return nil
}

var (
	errNoCerts     = &pinTestError{"no certificates presented"}
	errPinMismatch = &pinTestError{"pinned key mismatch"}
)

type pinTestError struct{ msg string }

func (e *pinTestError) Error() string { return e.msg }

// TestTransport_CertificatePinning verifies the pinner adapter: a matching pin
// completes the round trip, and a mismatched pin rejects the handshake through
// the transport's Pin hook.
func TestTransport_CertificatePinning(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "pinned-ok")
	}))
	defer ts.Close()

	leaf := ts.Certificate()
	sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	good := base64.StdEncoding.EncodeToString(sum[:])

	t.Run("matching pin passes", func(t *testing.T) {
		a := newTestAdapter(t, "chrome", true, &spkiPinner{want: good})
		resp, err := a.RoundTrip(httptestReq(t, ts.URL+"/"))
		if err != nil {
			t.Fatalf("RoundTrip with a matching pin: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "pinned-ok" {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("mismatched pin fails the handshake", func(t *testing.T) {
		a := newTestAdapter(t, "chrome", true, &spkiPinner{want: "not-the-pin"})
		resp, err := a.RoundTrip(httptestReq(t, ts.URL+"/"))
		if err == nil {
			_ = resp.Body.Close()
			t.Fatal("mismatched pin accepted")
		}
		if !strings.Contains(err.Error(), "pinning") {
			t.Errorf("error should name pinning, got: %v", err)
		}
	})
}

// TestTransport_CloseIdempotent pins that Close may run twice without error.
func TestTransport_CloseIdempotent(t *testing.T) {
	adapter := NewAdapter(Config{PresetName: "chrome"})
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	// A post-Close round trip rebuilds lazily rather than panicking.
	if _, err := adapter.backendFor(nil); err != nil {
		t.Fatalf("backendFor after Close: %v", err)
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

// TestBackend_InvalidScheme pins the backend's scheme dispatch: only http and
// https are transportable; anything else fails fast with the scheme named.
func TestBackend_InvalidScheme(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	for _, scheme := range []string{"ftp", "ws"} {
		req, err := ehttp.NewRequest(http.MethodGet, scheme+"://example.com/", nil)
		if err != nil {
			t.Fatalf("NewRequest(%q): %v", scheme, err)
		}
		_, err = a.base.RoundTrip(req)
		if err == nil {
			t.Errorf("scheme %q accepted", scheme)
		} else if !strings.Contains(err.Error(), scheme) {
			t.Errorf("error should name the scheme %q: %v", scheme, err)
		}
	}
}

// TestBackend_NilURL pins the missing-URL guard.
func TestBackend_NilURL(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome"})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	req := (&ehttp.Request{Method: http.MethodGet}).WithContext(context.Background())
	if _, err := a.base.RoundTrip(req); err == nil {
		t.Fatal("nil URL accepted")
	}
}

// TestTransport_ContextCanceledReachesDial pins that the engine's request
// context governs the transport: a canceled context aborts the round trip
// instead of waiting on the network.
func TestTransport_ContextCanceledReachesDial(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	req := httptestReq(t, ts.URL+"/").WithContext(ctx)
	start := time.Now()
	_, err := adapter.RoundTrip(req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("canceled request completed without error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("error should be a deadline/cancellation, got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("round trip took %v; the context deadline did not reach the transport", elapsed)
	}
}

// TestTransport_PlainHTTPTarget pins that http:// targets work through the
// fingerprint stack (no TLS, ALPN or pinning involved).
func TestTransport_PlainHTTPTarget(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto)
	}))
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)
	resp, err := adapter.RoundTrip(httptestReq(t, ts.URL+"/"))
	if err != nil {
		t.Fatalf("RoundTrip over plain http: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Proto != "HTTP/1.1" {
		t.Errorf("Proto = %q, want HTTP/1.1 (plain HTTP has no h2)", resp.Proto)
	}
	if resp.TLS != nil {
		t.Error("plain-HTTP response must carry no TLS state")
	}
	_ = url.URL{}
}

// TestTransport_TLSStateTranslated pins the uTLS→crypto/tls conversion: the
// fingerprinted path must expose the same TLS metadata shape as the standard
// transport, not drop it.
func TestTransport_TLSStateTranslated(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)
	resp, err := adapter.RoundTrip(httptestReq(t, ts.URL+"/"))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.TLS == nil {
		t.Fatal("TLS state missing on an https response")
	}
	if resp.TLS.NegotiatedProtocol != "h2" {
		t.Errorf("NegotiatedProtocol = %q, want h2", resp.TLS.NegotiatedProtocol)
	}
	if resp.TLS.Version == 0 || resp.TLS.CipherSuite == 0 {
		t.Errorf("state not populated: version=%#x suite=%#x", resp.TLS.Version, resp.TLS.CipherSuite)
	}
	if len(resp.TLS.PeerCertificates) == 0 {
		t.Error("peer certificates not carried through the conversion")
	}
	if !resp.TLS.HandshakeComplete {
		t.Error("HandshakeComplete not carried")
	}
}
