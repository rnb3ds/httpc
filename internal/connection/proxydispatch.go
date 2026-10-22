package connection

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
)

// perProxyRoundTripper routes every request through a dedicated *http.Transport
// bound to the proxy that PoolManager.SelectProxy picks for it (honoring
// per-attempt indexes from WithProxyAttempt, round-robin/random strategy,
// circuit breaking, and the ProxyRecorder carried on the request context).
//
// It exists to make HTTP/2 usable with a rotating proxy pool. net/http keys its
// shared HTTP/2 pool by (scheme, target authority) — the proxy carrying a
// CONNECT tunnel is not part of the key — so a single shared transport with h2
// enabled pools tunnels from DIFFERENT proxies under the same key and later
// requests ride whichever tunnel registered first, bypassing proxy selection
// entirely (see the force-HTTP/1 comment in NewPoolManager). Giving each proxy
// its own transport confines every h2 pool to a single proxy: rotation is
// honored per request while concurrent requests through the SAME proxy
// multiplex over one tunnel.
//
// Requests for which no proxy is selected (no pool configured — dispatcher is
// not activated then, but defensive) fall back to the shared base transport.
type perProxyRoundTripper struct {
	pm   *PoolManager
	base *http.Transport // fallback when no proxy is selected

	mu    sync.Mutex
	byKey map[string]*http.Transport // proxy URL string → transport
}

// newPerProxyRoundTripper builds a dispatcher over pm. base is the shared
// transport constructed by NewPoolManager; it serves direct-connection
// fallbacks and CloseIdleConnections compatibility.
func newPerProxyRoundTripper(pm *PoolManager, base *http.Transport) *perProxyRoundTripper {
	return &perProxyRoundTripper{
		pm:    pm,
		base:  base,
		byKey: make(map[string]*http.Transport),
	}
}

// RoundTrip selects the proxy for the request and sends it through that
// proxy's transport. Redirect hops re-enter here (the http.Client calls its
// Transport once per hop); the attempt index on the context is unchanged
// within one logical attempt, so every hop of an attempt selects the same
// proxy — the redirect-safe property the shared transport provides.
func (d *perProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		// A nil request must not be delegated: http.Transport.RoundTrip
		// dereferences req (req.Context()) before its own nil validation and
		// would panic — the opposite of the no-panic contract this branch
		// exists for (found by TestPerProxyRoundTripper_DefensiveBranches).
		// Wording mirrors the stdlib's own message for the same condition.
		return nil, errors.New("http: nil Request passed to Transport.RoundTrip")
	}
	if req.URL == nil {
		// Defensive: net/http validates this before reaching a Transport, and
		// the base transport rejects a nil-URL request with an error (unlike a
		// nil request, above); delegate rather than duplicate the message.
		return d.base.RoundTrip(req)
	}
	proxyURL, err := d.pm.SelectProxy(req.Context(), req.URL)
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return d.base.RoundTrip(req)
	}
	return d.transportFor(proxyURL).RoundTrip(req)
}

// transportFor returns the transport pinned to proxyURL, creating it lazily on
// first use. Transports are never removed while the manager is open — the pool
// is bounded by its configured size.
func (d *perProxyRoundTripper) transportFor(proxyURL *url.URL) *http.Transport {
	key := proxyURL.String()

	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.byKey[key]; ok {
		return t
	}

	// HTTP/2 is the point of the dispatcher (it is only activated when
	// EnableHTTP2 is on), and per-proxy isolation replaces the force-HTTP/1
	// the shared transport applies for multi-proxy pools.
	t := d.pm.buildTransport(d.pm.config.ForceAttemptHTTP2 && d.pm.config.EnableHTTP2)
	// The dispatcher has already selected this proxy for the request — and
	// recorded it into the request's ProxyRecorder via SelectProxy — so this
	// transport's Proxy hook is a constant. The SSRF guard still wraps it: it
	// validates the TARGET hostname on every proxied round trip exactly as the
	// shared transport does.
	u := proxyURL
	t.Proxy = d.pm.ssrfGuardProxyFunc(func(*http.Request) (*url.URL, error) { return u, nil })

	d.byKey[key] = t
	return t
}

// closeIdleConnections closes idle connections in every per-proxy transport.
// Called from PoolManager.CloseIdleConnections (which also sweeps the shared
// transport), so proxy rotation's idle-drain and shutdown cover the dispatcher.
func (d *perProxyRoundTripper) closeIdleConnections() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.byKey {
		t.CloseIdleConnections()
	}
}
