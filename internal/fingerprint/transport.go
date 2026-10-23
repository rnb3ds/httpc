// The fingerprint transport: one tls-client backend per (profile, proxy)
// combination, wrapped to look like a plain http.RoundTripper.
//
// The transport is github.com/bogdanfinn/tls-client, which owns the whole
// wire stack — the uTLS handshake, the HTTP/2 (and optionally HTTP/3) frame
// layer, and the HTTP/1.1 writer that preserves header casing. httpc supplies
// its own ClientHello spec and HTTP/2 SETTINGS through a ClientProfile, so the
// fingerprint data stays in this package while the transport machinery does
// not.
//
// Deviations from tls-client's defaults, all deliberate:
//   - HTTP/3 is disabled. A QUIC connection uses a separate TLS stack that
//     bypasses the certificate pinner, and losing the pinner silently is worse
//     than not racing protocols.
//   - Response decompression is disabled so decode.go keeps full ownership of
//     the body (tls-client decodes gzip/br/zstd itself and would leave the
//     engine holding already-decoded bytes with no Content-Encoding).
//   - Proxies are baked in per backend and never swapped. tls-client's
//     SetProxy rebuilds the transport in place without a lock, which races
//     against in-flight requests; the httpc proxy pool rotates by selecting a
//     different backend instead.
//   - Redirects are not followed (the engine's CheckRedirect owns the chain).

package fingerprint

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	ehttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	utls "github.com/bogdanfinn/utls"
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
	fpIdleConnTimeout     = 20 * time.Second
	fpMaxIdleConns        = 512
	fpMaxIdleConnsPerHost = 128
	fpMaxConnsPerHost     = 128
	fpReadIdleTimeout     = 10 * time.Second
	fpH2PingTimeout       = 10 * time.Second
)

// backend is one fingerprint transport stack: the tls-client instance, the
// profile it serves, and the OS its headers are built for.
type backend struct {
	client  tls_client.HttpClient
	profile *browserProfile
	os      osKey
}

// newTransportBackend builds the transport for one profile and proxy
// selection. A nil proxyURL means direct connections; tls-client never
// consults the environment for proxies.
func newTransportBackend(cfg Config, proxyURL *url.URL, p *browserProfile, os osKey) (*backend, error) {
	idle := fpIdleConnTimeout
	transportOpts := &tls_client.TransportOptions{
		IdleConnTimeout:        &idle,
		ReadIdleTimeout:        fpReadIdleTimeout,
		PingTimeout:            fpH2PingTimeout,
		MaxResponseHeaderBytes: 0, // tls-client default
		MaxIdleConns:           fpMaxIdleConns,
		MaxIdleConnsPerHost:    fpMaxIdleConnsPerHost,
		MaxConnsPerHost:        fpMaxConnsPerHost,
		// decode.go owns response decoding; see the package comment.
		DisableCompression: true,
	}

	opts := make([]tls_client.HttpClientOption, 0, 10)
	opts = append(opts,
		tls_client.WithTimeoutSeconds(0), // the engine owns the deadline budget
		tls_client.WithClientProfile(p.clientProfile()),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithTransportOptions(transportOpts),
		// A QUIC connection bypasses the certificate pinner, so it is never
		// attempted on this path.
		tls_client.WithDisableHttp3(),
	)

	// Session resumption is on whenever the profile's ClientHello can carry a
	// pre_shared_key extension (the Chrome profiles): the transport keeps an
	// LRU ticket cache per backend, so repeat connections resume with a PSK
	// the way a real browser does. Profiles without the extension (Firefox,
	// iOS WebKit) full-handshake regardless; the flag forces the full
	// handshake everywhere.
	if cfg.DisableSessionTickets {
		opts = append(opts, tls_client.WithDisableSessionTickets())
	}

	if cfg.InsecureSkipVerify {
		opts = append(opts, tls_client.WithInsecureSkipVerify())
	}

	if !cfg.EnableHTTP2 {
		// Pin the stack to HTTP/1.1, mirroring Connection.EnableHTTP2=false on
		// the standard transport. The ALPN list is rewritten to http/1.1 only,
		// so the ClientHello stays coherent with the protocol actually used.
		opts = append(opts, tls_client.WithForceHttp1())
	}

	if cfg.CertPinner != nil {
		opts = append(opts, tls_client.WithCertificatePinner(&pinnerShim{pinner: cfg.CertPinner}))
	}

	if proxyURL != nil {
		factory, err := newProxyDialerFactory(proxyURL, cfg.InsecureSkipVerify)
		if err != nil {
			return nil, err
		}
		// A custom dialer factory, not WithProxyUrl: the built-in CONNECT path
		// takes its socket deadline from the client timeout, and this package
		// deliberately runs with no client timeout (the engine owns the
		// deadline budget), so the built-in path would set a deadline of "now".
		opts = append(opts, tls_client.WithProxyDialerFactory(factory))
	} else {
		// Direct connections install the validating dialer through the same
		// factory hook (tls-client uses a non-nil factory as THE dialer even
		// without a proxy): resolve once, filter to SSRF-allowed IPs, dial the
		// validated literals. This is what gives the direct fingerprint path
		// the standard pool's dial-layer guarantee — no TOCTOU window, no
		// mixed-record pass. See directdial.go.
		opts = append(opts, tls_client.WithProxyDialerFactory(newValidatingDialerFactory(cfg)))
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: build transport for %s: %w", p.name, err)
	}

	return &backend{client: client, profile: p, os: os}, nil
}

// RoundTrip executes one HTTP round trip through the fingerprinted backend.
// The request is a fork (*ehttp.Request); the adapter owns the stdlib
// boundary.
func (b *backend) RoundTrip(req *ehttp.Request) (*ehttp.Response, error) {
	// RoundTripper contract: on error the request body must be closed. The
	// two early returns below fire before client.Do, so they close directly.
	if req.URL == nil {
		if req.Body != nil {
			_ = req.Body.Close() // best-effort: request is being abandoned
		}
		return nil, fmt.Errorf("fingerprint: request has no URL")
	}

	switch scheme := req.URL.Scheme; scheme {
	case "http", "https":
	default:
		if req.Body != nil {
			_ = req.Body.Close() // best-effort: request is being abandoned
		}
		return nil, fmt.Errorf("fingerprint: invalid URL scheme %q", scheme)
	}

	// The body is wrapped so Close runs exactly once across every closer in
	// the chain: client.Do DOES close the body on errors raised after the
	// inner transport starts running (anything past the body being sent), but
	// it does NOT close on errors raised before that (tls-client's
	// getTransport failures — first-dial connection refused, DNS failure, TLS
	// verification, cert-pin rejection, SSRF dial-layer rejection, proxy
	// CONNECT failure: the common transient class), and nothing above this
	// adapter closes a custom RoundTripper's error returns either. The close
	// below therefore fires unconditionally on error; the once-guard makes it
	// a no-op where tls-client already closed, so pooled body readers return
	// to their pool exactly once and *os.File bodies never double-close.
	if req.Body != nil {
		req.Body = &onceCloser{body: req.Body}
	}

	resp, err := b.client.Do(req)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close() // best-effort: request is aborted
		}
		return nil, err
	}
	return resp, nil
}

// onceCloser makes an io.ReadCloser's Close idempotent. The fingerprint
// backend's error handling closes the request body on every client.Do error
// while tls-client also closes it on its post-send error paths; exactly one
// Close must reach the underlying reader.
type onceCloser struct {
	body io.ReadCloser
	once sync.Once
}

func (o *onceCloser) Read(p []byte) (int, error) { return o.body.Read(p) }

func (o *onceCloser) Close() error {
	var err error
	o.once.Do(func() { err = o.body.Close() })
	return err
}

// CloseIdleConnections releases the backend's idle connections.
func (b *backend) CloseIdleConnections() {
	if b.client != nil {
		b.client.CloseIdleConnections()
	}
}

// pinnerShim adapts httpc's VerifyPeerCertificate-shaped pinner to
// tls-client's Pin(*utls.UConn, host) hook.
//
// The chain passed through mirrors crypto/tls: real chains when the handshake
// verified the peer, nil when InsecureSkipVerify skipped verification (leaf
// pinning then becomes the only gate — the same trust model as the pooled
// transport path).
type pinnerShim struct {
	pinner certPinner
}

// Pin runs the wrapped pinner against the handshake's certificate chain.
func (s *pinnerShim) Pin(conn *utls.UConn, host string) error {
	if s.pinner == nil {
		return nil
	}

	state := conn.ConnectionState()

	rawCerts := make([][]byte, len(state.PeerCertificates))
	for i, cert := range state.PeerCertificates {
		rawCerts[i] = cert.Raw
	}

	if err := s.pinner.VerifyPeerCertificate(rawCerts, state.VerifiedChains); err != nil {
		return fmt.Errorf("certificate pinning failed for %s: %w", host, err)
	}
	return nil
}
