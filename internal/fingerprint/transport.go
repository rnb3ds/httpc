// The fingerprint transport stack: three fork transports per backend —
// HTTP/1.1 (t1), an HTTP/1.1-ALPN fallback twin (t1fb), and the
// fingerprinted HTTP/2 transport (t2) — under one RoundTripper that prefers
// h2 for https and falls back to HTTP/1.1, restoring the request body via
// GetBody.
//
// Ported from github.com/enetx/surf (ja_roundtripper.go), Copyright enetx/surf
// authors, MIT License, with the pinner hook added: the utls.Config is owned
// here, so VerifyPeerCertificate can carry httpc's certificate pinner on the
// fingerprint path.

package fingerprint

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	ehttp "github.com/enetx/http"
	"github.com/enetx/http2"
	utls "github.com/refraction-networking/utls"
)

// certPinner is the consumer-side certificate-pinning contract (satisfied by
// security.CertificatePinner implementations).
type certPinner interface {
	VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
}

// Transport timeouts and pool sizing (surf defaults; the engine's own
// per-request timeout rides on the request context above this stack).
// NOTE: the pool sizes here are independent of the public
// Connection.MaxIdleConns / MaxConnsPerHost settings, which apply to the
// standard transport only — see the TLSFingerprint docs.
const (
	fpDialTimeout           = 10 * time.Second
	fpTLSHandshakeTimeout   = 10 * time.Second
	fpResponseHeaderTimeout = 10 * time.Second
	fpTCPKeepAlive          = 15 * time.Second
	fpIdleConnTimeout       = 20 * time.Second
	fpMaxIdleConns          = 512
	fpMaxConnsPerHost       = 128
	fpMaxIdleConnsPerHost   = 128
	fpExpectContinueTimeout = 1 * time.Second
	fpH2PingTimeout         = 10 * time.Second
	fpH2ReadIdleTimeout     = 10 * time.Second
	fpH2WriteByteTimeout    = 10 * time.Second
)

// ErrHTTP2Fallback indicates that an HTTPS request attempted HTTP/2 first,
// then tried to fall back to HTTP/1.1, and both attempts failed. Both
// underlying errors are accessible via Unwrap, so errors.Is/errors.As match
// against either leg.
type ErrHTTP2Fallback struct {
	HTTP2 error
	HTTP1 error
}

func (e *ErrHTTP2Fallback) Error() string {
	return fmt.Sprintf("fingerprint: HTTP/2 request failed: %v; HTTP/1.1 fallback failed: %v", e.HTTP2, e.HTTP1)
}

func (e *ErrHTTP2Fallback) Unwrap() []error { return []error{e.HTTP2, e.HTTP1} }

// fpTransport implements ehttp.RoundTripper for one backend (one profile and
// proxy combination).
type fpTransport struct {
	t1                 *ehttp.Transport // HTTP/1.1; also carries the raw dialer (proxy-aware)
	t1fb               *ehttp.Transport // HTTP/1.1 twin with http/1.1-only ALPN, for h2 fallback
	t2                 *http2.Transport // fingerprinted HTTP/2; nil when HTTP/2 is disabled
	profile            *browserProfile
	os                 osKey
	insecureSkipVerify bool
	certPinner         certPinner
	forceHTTP1         bool
}

// newFingerprintTransport builds the transport stack for one backend. A nil
// proxyURL means direct connections; the transports never consult the
// environment for proxies.
func newFingerprintTransport(cfg Config, proxyURL *url.URL, p *browserProfile, os osKey) (*fpTransport, error) {
	dial := (&net.Dialer{Timeout: fpDialTimeout, KeepAlive: fpTCPKeepAlive}).DialContext
	if proxyURL != nil {
		proxyDial, err := newProxyDialer(proxyURL)
		if err != nil {
			return nil, err
		}
		dial = proxyDial
	}

	ft := &fpTransport{
		profile:            p,
		os:                 os,
		insecureSkipVerify: cfg.InsecureSkipVerify,
		certPinner:         cfg.CertPinner,
		forceHTTP1:         !cfg.EnableHTTP2,
	}

	ft.t1 = &ehttp.Transport{
		DialContext:           dial,
		DialTLSContext:        ft.dialTLS,
		DisableCompression:    true,
		ExpectContinueTimeout: fpExpectContinueTimeout,
		ForceAttemptHTTP2:     false, // h2 is t2's job; t1 serves plain http and forced-h1 https
		IdleConnTimeout:       fpIdleConnTimeout,
		MaxConnsPerHost:       fpMaxConnsPerHost,
		MaxIdleConns:          fpMaxIdleConns,
		MaxIdleConnsPerHost:   fpMaxIdleConnsPerHost,
		Proxy:                 nil, // proxying rides on DialContext
		ResponseHeaderTimeout: fpResponseHeaderTimeout,
		TLSHandshakeTimeout:   fpTLSHandshakeTimeout,
	}
	ft.t1fb = ft.t1.Clone()
	ft.t1fb.DialTLSContext = ft.dialTLSHTTP1

	if !ft.forceHTTP1 {
		ft.t2 = ft.buildH2Transport()
	}

	return ft, nil
}

// RoundTrip executes a single request (enetx fork types; the adapter owns
// the stdlib boundary).
func (ft *fpTransport) RoundTrip(req *ehttp.Request) (*ehttp.Response, error) {
	switch strings.ToLower(req.URL.Scheme) {
	case "http":
		// Plain HTTP always uses HTTP/1.1 (HTTP/2 requires TLS).
		return ft.t1.RoundTrip(req)
	case "https":
		return ft.handleHTTPS(req)
	default:
		return nil, fmt.Errorf("fingerprint: invalid URL scheme %q", req.URL.Scheme)
	}
}

// errALPNMismatch marks an h2 dial that failed ALPN negotiation before any
// HTTP bytes were written. handleHTTPS allows the HTTP/1.1 fallback for it
// even on non-idempotent requests: the server never received the request, so
// replaying it cannot duplicate side effects.
var errALPNMismatch = errors.New("fingerprint: negotiated ALPN is not h2")

// isIdempotentMethod mirrors the engine's retry.go definition (RFC 9110
// §9.2.2: safe methods plus PUT and DELETE). fingerprint must not import the
// engine package, so the list is duplicated here on purpose.
func isIdempotentMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS", "TRACE", "PUT", "DELETE":
		return true
	default:
		return false
	}
}

// handleHTTPS prefers the fingerprinted HTTP/2 transport and falls back to
// HTTP/1.1 when it fails, restoring the request body for the retry.
//
// The fallback replays the request, so it is only attempted when the failed
// h2 leg provably never delivered request bytes (ALPN mismatch happens
// before any HTTP bytes are written) or the method is idempotent. Any other
// h2 failure — GOAWAY or RST_STREAM after the server already processed the
// request, a read-idle timeout mid-response — may have committed the
// request's side effects, and re-sending a non-idempotent request would
// duplicate them (e.g. a payment booked twice).
func (ft *fpTransport) handleHTTPS(req *ehttp.Request) (*ehttp.Response, error) {
	if ft.t2 == nil {
		return ft.t1.RoundTrip(req)
	}

	resp, err := ft.t2.RoundTrip(req)
	if err == nil {
		return resp, nil
	}

	h2Err := err

	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	if !errors.Is(h2Err, errALPNMismatch) && !isIdempotentMethod(req.Method) {
		// The h2 attempt may have delivered the request to the server.
		// Surface the h2 failure instead of replaying a non-idempotent
		// request over HTTP/1.1.
		return nil, h2Err
	}

	// Restore the request body for the HTTP/1.1 retry.
	if req.Body != nil && req.Body != ehttp.NoBody {
		if req.GetBody == nil {
			return nil, fmt.Errorf("fingerprint: HTTP/2 failed and cannot retry because req.GetBody is nil: %w", err)
		}

		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, fmt.Errorf("fingerprint: failed to restore body for fallback: %w", bodyErr)
		}
		req.Body = body
	}

	resp, err = ft.t1fb.RoundTrip(req)
	if err != nil {
		return nil, &ErrHTTP2Fallback{HTTP2: h2Err, HTTP1: err}
	}

	return resp, nil
}

// CloseIdleConnections closes idle connections on all three transports.
func (ft *fpTransport) CloseIdleConnections() {
	if ft.t1 != nil {
		ft.t1.CloseIdleConnections()
	}
	if ft.t1fb != nil {
		ft.t1fb.CloseIdleConnections()
	}
	if ft.t2 != nil {
		ft.t2.CloseIdleConnections()
	}
}

// buildH2Transport builds the fingerprinted HTTP/2 transport from the
// profile's SETTINGS data.
func (ft *fpTransport) buildH2Transport() *http2.Transport {
	t := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return ft.dialTLSHTTP2(ctx, network, addr)
		},
		DisableCompression: true,
		IdleConnTimeout:    fpIdleConnTimeout,
		PingTimeout:        fpH2PingTimeout,
		ReadIdleTimeout:    fpH2ReadIdleTimeout,
		WriteByteTimeout:   fpH2WriteByteTimeout,
	}

	h2 := ft.profile.h2
	t.Settings = make([]http2.Setting, 0, len(h2.settings))
	for _, s := range h2.settings {
		t.Settings = append(t.Settings, http2.Setting{ID: http2.SettingID(s.id), Val: s.val})
	}

	if h2.streamID != 0 {
		t.StreamID = h2.streamID
	}
	if h2.connectionFlow != 0 {
		t.ConnectionFlow = h2.connectionFlow
	}
	if h2.hasPriority {
		t.PriorityParam = http2.PriorityParam{
			StreamDep: h2.priority.streamDep,
			Exclusive: h2.priority.exclusive,
			Weight:    h2.priority.weight,
		}
	}

	return t
}

// dialTLS performs the uTLS handshake with the profile's ALPN (h2, http/1.1).
func (ft *fpTransport) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	return ft.tlsHandshake(ctx, network, addr, false)
}

// dialTLSHTTP1 performs the uTLS handshake restricted to HTTP/1.1 ALPN, for
// the fallback transport.
func (ft *fpTransport) dialTLSHTTP1(ctx context.Context, network, addr string) (net.Conn, error) {
	return ft.tlsHandshake(ctx, network, addr, true)
}

// dialTLSHTTP2 performs the uTLS handshake and requires ALPN to have
// selected HTTP/2; anything else fails before HTTP/2 bytes are written.
func (ft *fpTransport) dialTLSHTTP2(ctx context.Context, network, addr string) (net.Conn, error) {
	uconn, err := ft.tlsHandshake(ctx, network, addr, false)
	if err != nil {
		return nil, err
	}

	if negotiated := uconn.ConnectionState().NegotiatedProtocol; negotiated != "h2" {
		uconn.Close()
		return nil, fmt.Errorf("%w: %q, expected h2", errALPNMismatch, negotiated)
	}

	return uconn, nil
}

// tlsHandshake dials (through t1's proxy-aware DialContext), then performs
// the fingerprinted uTLS handshake on the raw connection.
func (ft *fpTransport) tlsHandshake(ctx context.Context, network, addr string, forceHTTP1 bool) (*utls.UConn, error) {
	timeout := ft.t1.TLSHandshakeTimeout
	if timeout > 0 {
		if deadline, ok := ctx.Deadline(); ok {
			if remaining := time.Until(deadline); remaining < timeout {
				timeout = remaining
			}
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	rawConn, err := ft.t1.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	spec, err := specForDial(ft.profile)
	if err != nil {
		rawConn.Close()
		return nil, err
	}

	if forceHTTP1 || ft.forceHTTP1 {
		setAlpnProtocolToHTTP1(spec)
	}

	config := &utls.Config{
		ServerName:             host,
		InsecureSkipVerify:     ft.insecureSkipVerify,
		SessionTicketsDisabled: true,
		OmitEmptyPsk:           true,
	}

	// The pinner runs as crypto/tls's VerifyPeerCertificate does: after
	// normal verification with populated chains when verification is on,
	// and with nil chains under InsecureSkipVerify (leaf-only pinning —
	// the same trust model as the pooled transport path).
	if ft.certPinner != nil {
		pinner := ft.certPinner
		config.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if err := pinner.VerifyPeerCertificate(rawCerts, verifiedChains); err != nil {
				return fmt.Errorf("certificate pinning failed: %w", err)
			}
			return nil
		}
	}

	uconn := utls.UClient(rawConn, config, utls.HelloCustom)
	if err = uconn.ApplyPreset(spec); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("fingerprint: apply ClientHello preset: %w", err)
	}

	if err = uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("fingerprint: uTLS handshake: %+v", err)
	}

	return uconn, nil
}
