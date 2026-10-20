// Package fingerprint implements browser transport fingerprinting (TLS
// ClientHello + HTTP/2 frames + ordered header block) as a standard
// net/http RoundTripper, so the httpc engine (redirects, retries, middleware,
// streaming) stays authoritative.
//
// The transport stack is owned by this package: the TLS layer is upstream
// refraction-networking/utls, the HTTP/1.1 and HTTP/2 layers are the
// enetx/http and enetx/http2 forks (net/http and x/net/http2 extended with
// the wire-level knobs fingerprinting requires — ordered header blocks,
// custom SETTINGS/priority frames, initial stream ids). The orchestration
// (JA assembly, browser profiles, header pipeline, proxy dialing, response
// decoding) was ported from github.com/enetx/surf (MIT) into this package;
// httpc does not depend on that client library.
//
// Design notes:
//   - One transport stack per proxy selection is cached; the httpc proxy
//     pool (rotation, circuit breaking, ProxyRecorder attribution) continues
//     to govern selection via PoolManager.SelectProxy.
//   - Redirects are NOT followed by the backend: the adapter is a plain
//     http.RoundTripper, so the engine's http.Client CheckRedirect policy
//     drives the chain. Redirect hops stay browser-coherent (redirect.go):
//     sec-fetch-site is recomputed against the previous hop's URL,
//     sec-fetch-user is dropped after the first hop, and a 301/302/303
//     downgraded fetch chain keeps fetch-style header tables.
//   - Only HTTP/1.1 and HTTP/2 are used, both fully fingerprinted (h2
//     preferred, h1 fallback with body restore via GetBody). The fallback
//     is replay-safe only: it runs for pre-send failures (ALPN mismatch)
//     or idempotent methods, never for a non-idempotent request whose h2
//     leg may have already reached the server. HTTP/3 is
//     deliberately not attempted: a QUIC fallback to TCP would emit a
//     Go-shaped ClientHello mid-chain.
//   - The browser profile's header set is authoritative: the per-request
//     pipeline replaces same-named request headers (User-Agent, Accept,
//     Accept-Language, ...) so the wire stays coherent with the spoofed
//     ClientHello. Custom headers pass through untouched.
//   - Certificate pinning IS supported on this path: the adapter owns the
//     utls.Config, so the public CertificatePinner runs as
//     VerifyPeerCertificate on every fingerprinted handshake (under
//     InsecureSkipVerify it becomes the only gate, pinning the leaf — the
//     same trust model as the pooled transport path).
//   - Response bodies arrive decoded (gzip/deflate/br/zstd, see decode.go);
//     the engine sees identity bodies with Content-Length already reset.
//   - SSRF protection uses a pre-flight resolve+validate (the backend dials
//     on its own, so the connection-layer validator cannot apply). This
//     leaves a theoretical TOCTOU window between validation and the
//     backend's own DNS resolution — documented in the public
//     SecurityConfig.TLSFingerprint docs.
package fingerprint

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"

	ehttp "github.com/enetx/http"

	"github.com/cybergodev/httpc/internal/connection"
)

// Config wires the adapter to the owning engine client.
type Config struct {
	// PresetName is the resolved preset name (never empty when the adapter
	// is constructed; validated by the public config layer).
	PresetName string

	// InsecureSkipVerify mirrors Security.InsecureSkipVerify: verification
	// runs unless the user explicitly opted out (verify-by-default).
	InsecureSkipVerify bool

	// AllowPrivateIPs mirrors Security.AllowPrivateIPs: when false, the
	// pre-flight SSRF check runs for every request.
	AllowPrivateIPs bool

	// EnableHTTP2 mirrors Connection.EnableHTTP2. When false, the backend is
	// pinned to HTTP/1.1 (ALPN restricted) so the fingerprint path honors
	// the same protocol choice as the standard stack.
	EnableHTTP2 bool

	// ExemptNets mirrors Security.SSRFExemptCIDRs (pre-parsed).
	ExemptNets []*net.IPNet

	// CertPinner, when set, runs as VerifyPeerCertificate on every
	// fingerprinted TLS handshake (nil disables pinning).
	CertPinner certPinner

	// Pool is the owning connection PoolManager (may be nil in tests); it
	// supplies proxy selection and circuit-breaker reporting.
	Pool *connection.PoolManager
}

// backend holds one fingerprint transport stack and the profile it serves.
type backend struct {
	ft      *fpTransport
	profile *browserProfile
	os      osKey
}

// Adapter implements http.RoundTripper over the in-house fingerprint
// transport stack.
type Adapter struct {
	cfg Config

	// base is the direct-connection backend. Construction can fail (the
	// transport validates its proxy configuration); the error is surfaced
	// from RoundTrip instead of NewAdapter so construction stays infallible.
	base    *backend
	baseErr error

	// mu guards perProxy. Backends are keyed by the proxy URL string — one
	// transport stack per proxy so connection pools never mix hops.
	mu        sync.Mutex
	perProxy  map[string]*backend
	closeOnce sync.Once
}

// NewAdapter constructs the adapter. The preset must already be validated
// (ResolvePresetName ok); an invalid name fails the base build and the error
// surfaces on first use — the public config layer is the gate.
func NewAdapter(cfg Config) *Adapter {
	a := &Adapter{
		cfg:      cfg,
		perProxy: make(map[string]*backend),
	}
	a.base, a.baseErr = a.newBackend(nil)
	return a
}

// newBackend builds one fingerprint transport stack. proxyURL nil means a
// direct connection. Timeouts beyond the transport's dial/handshake defaults
// are deliberately not set: the engine owns the deadline budget via the
// request context.
func (a *Adapter) newBackend(proxyURL *url.URL) (*backend, error) {
	spec, ok := presets[a.cfg.PresetName]
	if !ok {
		return nil, fmt.Errorf("fingerprint: unknown preset %q", a.cfg.PresetName)
	}

	p, os := profileFor(spec)
	ft, err := newFingerprintTransport(a.cfg, proxyURL, p, os)
	if err != nil {
		return nil, err
	}
	return &backend{ft: ft, profile: p, os: os}, nil
}

// RoundTrip executes one HTTP round trip through the fingerprinted backend.
func (a *Adapter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, errors.New("fingerprint: request has no URL")
	}
	if a.baseErr != nil {
		return nil, a.baseErr
	}

	// Pre-flight SSRF validation (see package doc for the residual TOCTOU).
	if err := a.preflightSSRF(req); err != nil {
		return nil, err
	}

	// Proxy selection: same logic, recorder, and per-attempt rotation as the
	// standard path. Errors from selection abort the request.
	proxy, err := a.selectProxy(req)
	if err != nil {
		return nil, err
	}

	b, err := a.backendFor(proxy)
	if err != nil {
		// A proxy whose backend cannot even be constructed (e.g. a malformed
		// URL) is unusable — count it against the circuit breaker like a
		// connection failure so rotation phases it out.
		if proxy != nil {
			a.reportProxy(proxy, false)
		}
		return nil, err
	}

	// Per-trip clone first (profile headers and order keys are written to
	// the fork copy, never to the engine's request object), then the
	// stdlib→fork conversion and the profile header pipeline. On redirect
	// hops the pipeline runs under the hop corrections (redirect.go):
	// recomputed sec-fetch-site, dropped sec-fetch-user, and fetch-style
	// tables when a 301/302/303 downgraded a fetch chain to GET.
	wire := prepareWireRequest(req)
	facts := redirectFactsFor(wire)
	forkReq := toForkRequest(wire)
	applyProfileHeaders(&forkReq.Header, tableMethodFor(wire, facts), b.profile, b.os)
	applyRedirectOverrides(&forkReq.Header, facts)

	forkResp, err := b.ft.RoundTrip(forkReq)

	// Circuit-breaker reporting for the selected proxy: connection-class
	// failures count against the proxy; completed exchanges count for it.
	if proxy != nil {
		if isConnectionFailure(err) {
			a.reportProxy(proxy, false)
		} else {
			a.reportProxy(proxy, true)
		}
	}
	if err != nil {
		return nil, err
	}

	resp := toStdResponse(forkResp, req)

	// Decode encoded bodies on the response object the engine reads, so the
	// stale encoded-length hazard cannot exist: ContentEncoding/ContentLength
	// headers are stripped and ContentLength reset together with the wrap.
	if err := decodeResponse(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// defaultUserAgent is httpc's Defaults.UserAgent value. When the engine-side
// request carries exactly this UA, it is dropped so the browser preset's
// version-matched User-Agent is used instead.
const defaultUserAgent = "httpc/1.0"

// engineAcceptEncodings are the values the engine auto-injects when the user
// has not set Accept-Encoding ("gzip, deflate" for buffered requests,
// "identity" for streaming ones). Dropping them lets the preset's full
// browser value (gzip, deflate, br, zstd) onto the wire.
var engineAcceptEncodings = map[string]bool{
	"gzip, deflate": true,
	"identity":      true,
}

// prepareWireRequest clones the engine request for one round trip. The clone
// matters: the profile header pipeline and the fork's ordering magic keys
// mutate the header map in place, and that must not leak back into the
// engine's request across retries and redirect hops (net/http's redirect
// copier propagates every header verbatim). Dropping httpc's own default UA
// and auto-injected Accept-Encoding here is DEFENSE-IN-DEPTH only:
// applyProfileHeaders overwrites both on the wire regardless (headers.go),
// so this matters only if a future profile stops overriding them. In
// particular, an explicitly user-set User-Agent does NOT pass through on
// this path today — the preset always wins.
func prepareWireRequest(req *http.Request) *http.Request {
	out := req.Clone(req.Context())
	if vs := out.Header.Values("User-Agent"); len(vs) == 1 && vs[0] == defaultUserAgent {
		out.Header.Del("User-Agent")
	}
	if vs := out.Header.Values("Accept-Encoding"); len(vs) == 1 && engineAcceptEncodings[vs[0]] {
		out.Header.Del("Accept-Encoding")
	}
	return out
}

// toForkRequest converts a stdlib *http.Request to the enetx/http fork type.
// Headers, body, GetBody and context are preserved; the fork's Response
// back-reference is dropped (unused on outgoing requests). Ported from surf's
// adapter conversions.
func toForkRequest(req *http.Request) *ehttp.Request {
	out := &ehttp.Request{
		Method:           req.Method,
		URL:              req.URL,
		Proto:            req.Proto,
		ProtoMajor:       req.ProtoMajor,
		ProtoMinor:       req.ProtoMinor,
		Header:           ehttp.Header(req.Header),
		Body:             req.Body,
		ContentLength:    req.ContentLength,
		TransferEncoding: req.TransferEncoding,
		Close:            req.Close,
		Host:             req.Host,
		Form:             req.Form,
		PostForm:         req.PostForm,
		MultipartForm:    req.MultipartForm,
		Trailer:          ehttp.Header(req.Trailer),
		RemoteAddr:       req.RemoteAddr,
		RequestURI:       req.RequestURI,
		TLS:              req.TLS,
		Response:         nil,
		GetBody:          req.GetBody,
		Pattern:          req.Pattern,
		Cancel:           req.Cancel, //nolint:staticcheck // deprecated but carried for parity
	}
	return out.WithContext(req.Context())
}

// toStdResponse converts a fork *http.Response back to the stdlib type. The
// engine reads httpResp.Request for request metadata, so the original
// stdlib request is restored into the back-reference field.
func toStdResponse(resp *ehttp.Response, req *http.Request) *http.Response {
	return &http.Response{
		Status:           resp.Status,
		StatusCode:       resp.StatusCode,
		Proto:            resp.Proto,
		ProtoMajor:       resp.ProtoMajor,
		ProtoMinor:       resp.ProtoMinor,
		Header:           http.Header(resp.Header),
		Body:             resp.Body,
		ContentLength:    resp.ContentLength,
		TransferEncoding: resp.TransferEncoding,
		Close:            resp.Close,
		Uncompressed:     resp.Uncompressed,
		Trailer:          http.Header(resp.Trailer),
		Request:          req,
		TLS:              resp.TLS,
	}
}

// backendFor returns the backend for the selected proxy, creating it on
// first use. After Close, perProxy is nil; rather than panic on the write
// below, rebuild lazily — a racing post-Close request is a programming error
// upstream, but must not crash the process.
func (a *Adapter) backendFor(proxy *url.URL) (*backend, error) {
	if proxy == nil {
		return a.base, nil
	}
	key := proxy.String()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.perProxy == nil {
		a.perProxy = make(map[string]*backend)
	}
	if b, ok := a.perProxy[key]; ok {
		return b, nil
	}
	b, err := a.newBackend(proxy)
	if err != nil {
		return nil, err
	}
	a.perProxy[key] = b
	return b, nil
}

// isConnectionFailure reports whether err is a transport-level failure worth
// counting against the proxy circuit breaker. The stack exposes no typed
// sentinels, so classification is structural: net.Error (which url.Error and
// timeouts satisfy), net.OpError, and TLS verification failures count;
// caller cancellations and request-level errors do not. ErrHTTP2Fallback
// traverses via Unwrap, so failures from either of its legs classify.
func isConnectionFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return true
	}
	return false
}

// Close releases backend transports. Safe to call multiple times.
func (a *Adapter) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, b := range a.perProxy {
			b.ft.CloseIdleConnections()
		}
		a.perProxy = nil
		if a.base != nil {
			a.base.ft.CloseIdleConnections()
		}
	})
	return nil
}
