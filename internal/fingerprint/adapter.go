// Package fingerprint implements browser transport fingerprinting (TLS
// ClientHello + HTTP/2 frames + ordered, case-exact header block) as a
// standard net/http RoundTripper, so the httpc engine (redirects, retries,
// middleware, streaming) stays authoritative.
//
// The transport is github.com/bogdanfinn/tls-client (BSD-3): it owns the uTLS
// handshake, the HTTP/2 frame layer and the HTTP/1.1 writer that preserves
// header casing. What this package keeps is everything the fingerprint data
// and the httpc contract depend on — the browser profiles and their
// ClientHello specs, the per-method header pipeline, redirect-hop coherence,
// proxy-pool integration, SSRF pre-flight, response decoding, and the
// certificate-pinning adapter. httpc depends on none of enetx/http,
// enetx/http2 or refraction-networking/utls for this path any more.
//
// Design notes:
//   - One backend (one tls-client instance) per proxy selection is cached; the
//     httpc proxy pool (rotation, circuit breaking, ProxyRecorder attribution)
//     continues to govern selection via PoolManager.SelectProxy. Proxies are
//     baked in at construction and never swapped at runtime: tls-client's
//     SetProxy rebuilds its transport in place without a lock, which races
//     with in-flight requests, so rotation happens by picking a different
//     cached backend instead.
//   - Redirects are NOT followed by the backend: the adapter is a plain
//     http.RoundTripper, so the engine's http.Client CheckRedirect policy
//     drives the chain. Redirect hops stay browser-coherent (redirect.go):
//     sec-fetch-site is recomputed against the previous hop's URL,
//     sec-fetch-user is dropped after the first hop, and a 301/302/303
//     downgraded fetch chain keeps fetch-style header tables.
//   - Protocol negotiation is the transport's own (ALPN over the profile's
//     ClientHello). Connection.EnableHTTP2=false rewrites the ALPN list to
//     http/1.1 and pins the stack to HTTP/1.1, mirroring the standard
//     transport's protocol choice. HTTP/3 is deliberately never attempted: a
//     QUIC connection uses a separate TLS stack that bypasses the certificate
//     pinner, and it would also break the single-fingerprint invariant across
//     a connection's lifetime.
//   - The browser profile's header set is authoritative: the per-request
//     pipeline replaces same-named request headers (User-Agent, Accept,
//     Accept-Language, ...) so the wire stays coherent with the spoofed
//     ClientHello. Custom headers pass through untouched.
//   - Certificate pinning IS supported on this path: the public
//     CertificatePinner runs as the transport's Pin hook on every
//     fingerprinted handshake (under InsecureSkipVerify it becomes the only
//     gate, pinning the leaf — the same trust model as the pooled transport
//     path).
//   - Response bodies arrive decoded (gzip/deflate/br/zstd, see decode.go);
//     the engine sees identity bodies with Content-Length already reset. The
//     transport's own decompression is switched off so decoding happens once.
//   - SSRF protection uses a pre-flight resolve+validate (the backend dials
//     on its own, so the connection-layer validator cannot apply). Two gaps
//     remain versus the standard path, documented in the public
//     SecurityConfig.TLSFingerprint docs: (a) a theoretical TOCTOU window
//     between validation and the backend's own DNS resolution, and (b) a
//     host that resolves to a MIX of public and private addresses passes
//     the "at least one allowed IP" pre-flight, and the backend's second
//     resolution may land on the private record — deterministically, with
//     no DNS rebinding required (the standard path dials only validated IP
//     literals and has neither gap).
package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"

	ehttp "github.com/bogdanfinn/fhttp"

	utls "github.com/bogdanfinn/utls"

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

	// DisableSessionTickets mirrors Security.TLSFingerprintDisableSessionTickets:
	// true forces a full TLS handshake on every connection by turning the
	// transport's session-ticket cache off. Without it the transport resumes
	// sessions on profiles whose ClientHello carries a pre_shared_key
	// extension (the Chrome profiles) — per backend, so proxy pools never
	// share ticket state — exactly like a real browser revisiting a host.
	DisableSessionTickets bool

	// CertPinner, when set, runs as the transport's certificate-pin hook on
	// every fingerprinted TLS handshake (nil disables pinning).
	CertPinner certPinner

	// Pool is the owning connection PoolManager (may be nil in tests); it
	// supplies proxy selection and circuit-breaker reporting.
	Pool *connection.PoolManager
}

// Adapter implements http.RoundTripper over the fingerprinted backend.
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

// newBackend resolves the configured preset and builds one transport stack.
// proxyURL nil means a direct connection. Timeouts beyond the transport's
// dial/handshake defaults are deliberately not set: the engine owns the
// deadline budget via the request context.
func (a *Adapter) newBackend(proxyURL *url.URL) (*backend, error) {
	spec, ok := presets[a.cfg.PresetName]
	if !ok {
		return nil, fmt.Errorf("fingerprint: unknown preset %q", a.cfg.PresetName)
	}

	p, os := profileFor(spec)
	b, err := newTransportBackend(a.cfg, proxyURL, p, os)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// RoundTrip executes one HTTP round trip through the fingerprinted backend.
func (a *Adapter) RoundTrip(req *http.Request) (*http.Response, error) {
	// fail closes req.Body before returning: the RoundTripper contract makes
	// the transport own the body, and net/http does not close it when a
	// custom RoundTripper errors — a caller-supplied io.ReadCloser (an
	// *os.File via WithBody) would otherwise leak its handle until the GC
	// finalizer runs.
	fail := func(err error) (*http.Response, error) {
		if req.Body != nil {
			_ = req.Body.Close() // best-effort: the request is aborted anyway
		}
		return nil, err
	}

	if req.URL == nil {
		return fail(errors.New("fingerprint: request has no URL"))
	}
	if a.baseErr != nil {
		return fail(a.baseErr)
	}

	// Pre-flight SSRF validation (see package doc for the residual TOCTOU).
	if err := a.preflightSSRF(req); err != nil {
		return fail(err)
	}

	// Proxy selection: same logic, recorder, and per-attempt rotation as the
	// standard path. Errors from selection abort the request.
	proxy, err := a.selectProxy(req)
	if err != nil {
		return fail(err)
	}

	b, err := a.backendFor(proxy)
	if err != nil {
		// A proxy whose backend cannot even be constructed (e.g. a malformed
		// URL) is unusable — count it against the circuit breaker like a
		// connection failure so rotation phases it out.
		if proxy != nil {
			a.reportProxy(proxy, false)
		}
		return fail(err)
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

	forkResp, err := b.RoundTrip(forkReq)

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

	resp := toStdResponse(forkResp, req, forkReq.Header)

	// Decode encoded bodies on the response object the engine reads, so the
	// stale encoded-length hazard cannot exist: ContentEncoding/ContentLength
	// headers are stripped and ContentLength reset together with the wrap.
	if err := decodeResponse(resp); err != nil {
		if resp.Body != nil {
			_ = resp.Body.Close() // best-effort: the round trip failed
		}
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
// mutate the header map in place — and the transport itself rewrites the
// order key's casing on the map it is handed — so nothing may leak back into
// the engine's request across retries and redirect hops (net/http's redirect
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

// toForkRequest converts a stdlib *http.Request to the fork type the
// transport consumes. Headers, body, GetBody, trailers and context are
// preserved; the magic order keys travel with the header map because both
// libraries spell them identically.
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
		TLS:              nil, // outbound requests carry no TLS state
		Response:         nil, // unused on outgoing requests
		GetBody:          req.GetBody,
		// Pattern has no fork counterpart: it is a server-mux routing key
		// (Go 1.23+) that never reaches the wire, so it is not carried.
		Cancel: req.Cancel, //nolint:staticcheck // deprecated but carried for parity (pinned by TestToForkRequest_FullFieldParity; the engine never sets it)
	}
	return out.WithContext(req.Context())
}

// toStdResponse converts a fork *http.Response back to the stdlib type. The
// engine reads httpResp.Request for request metadata, so the original
// stdlib request is restored into the back-reference field — but its Header
// map is replaced with the fork's wire headers (the browser profile's values
// under the browser's casing, magic order keys dropped) so Result.Request.Headers
// reports what the transport actually sent, not the engine-side default
// httpc/1.0 UA that the profile replaced on the wire. The TLS state is
// translated, not dropped (see state.go).
//
// wire is the fork request's header map after the profile header pipeline;
// the fork's *ehttp.Response.Header holds the RESPONSE headers, so the wire
// request headers cannot be recovered from resp alone.
func toStdResponse(resp *ehttp.Response, req *http.Request, wire ehttp.Header) *http.Response {
	reportReq := req
	if req != nil {
		reportReq = req.Clone(req.Context())
		reportReq.Header = wireHeaders(wire) // nil for empty
	}
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
		Request:          reportReq,
		TLS:              toStdConnState(resp.TLS),
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
// counting against the proxy circuit breaker. The transport exposes no typed
// sentinels, so classification is structural: net.Error (which url.Error and
// timeouts satisfy) and net.OpError count; caller cancellations and
// request-level errors do not.
//
// TLS verification failures need their own arm for a second reason: the
// handshake runs on uTLS, so the error is uTLS's CertificateVerificationError,
// NOT crypto/tls's (the types are distinct even though both print alike). On
// the transport path such an error arrives wrapped in a *url.Error, which
// satisfies net.Error and would already classify — the explicit arm keeps the
// classification correct if it ever surfaces unwrapped.
func isConnectionFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Covers *net.OpError and *url.Error: both implement net.Error, so
		// separate errors.As arms for them were unreachable.
		return true
	}
	var certErr *utls.CertificateVerificationError
	return errors.As(err, &certErr)
}

// CloseIdleConnections closes every backend's idle connections (base and
// per-proxy) while keeping the backends themselves usable — the next request
// dials fresh. Satisfies http.RoundTripper's optional CloseIdleConnections
// contract, and lets callers force fresh connections (for example to
// exercise session resumption rather than idle reuse).
func (a *Adapter) CloseIdleConnections() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, b := range a.perProxy {
		b.CloseIdleConnections()
	}
	if a.base != nil {
		a.base.CloseIdleConnections()
	}
}

// Close closes idle connections on every backend (base and per-proxy) and
// drops the per-proxy backends; the adapter itself remains usable — per-proxy
// backends are rebuilt lazily on the next proxied request, and the base
// backend keeps serving direct connections. Safe to call multiple times
// (subsequent calls are no-ops).
func (a *Adapter) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, b := range a.perProxy {
			b.CloseIdleConnections()
		}
		a.perProxy = nil
		if a.base != nil {
			a.base.CloseIdleConnections()
		}
	})
	return nil
}
