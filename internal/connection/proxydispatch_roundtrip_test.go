package connection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// newDispatcherFixture builds a PoolManager whose proxy pool points at two
// local httptest "proxies" with EnableProxyPoolHTTP2 active, so the per-proxy
// dispatcher is installed as the RoundTripper. Each fake proxy answers
// absolute-form proxied GETs with an X-Via header naming itself, letting the
// tests observe which per-proxy transport carried a request.
//
// AllowPrivateIPs sidesteps the proxied-target SSRF guard: the synthetic
// target hostnames used here do not resolve, and the guard fails closed on
// unresolvable hosts (the guard itself is table-tested separately in
// ssrf_target_test.go).
func newDispatcherFixture(t *testing.T) (pm *PoolManager, via1, via2 *httptest.Server) {
	t.Helper()

	newFakeProxy := func(name string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Via", name)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	via1 = newFakeProxy("proxy-one")
	via2 = newFakeProxy("proxy-two")

	pm, err := NewPoolManager(&Config{
		ProxyPool:            []string{via1.URL, via2.URL},
		EnableProxyPoolHTTP2: true,
		EnableHTTP2:          true,
		AllowPrivateIPs:      true,
		DialTimeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	t.Cleanup(func() { _ = pm.Close() })

	if pm.proxyDispatcher == nil {
		t.Fatal("test setup: dispatcher not activated (EnableProxyPoolHTTP2 + 2-proxy pool + h2)")
	}
	return pm, via1, via2
}

// proxiedGet issues one GET through the dispatcher with a pinned proxy-attempt
// index, returning the X-Via marker of the fake proxy that served it.
func proxiedGet(t *testing.T, pm *PoolManager, attempt int, target string) (via string, err error) {
	t.Helper()

	ctx := WithProxyAttempt(context.Background(), attempt)
	ctx = WithProxyRecorder(ctx, &ProxyRecorder{})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	resp, err := pm.GetRoundTripper().RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxied GET status = %d, want 200", resp.StatusCode)
	}
	return resp.Header.Get("X-Via"), nil
}

// TestPerProxyRoundTripper_RoundTrip pins the dispatcher's core contract:
// a request with a pinned attempt index is carried by THAT proxy's dedicated
// transport, and redirect-free repeated attempts through the same proxy reuse
// the same transport (multiplexing), while a different index selects the other
// proxy's transport.
func TestPerProxyRoundTripper_RoundTrip(t *testing.T) {
	pm, _, _ := newDispatcherFixture(t)

	const target = "http://dispatch-target.example/api"

	viaA1, err := proxiedGet(t, pm, 0, target)
	if err != nil {
		t.Fatalf("attempt 0: %v", err)
	}
	viaA2, err := proxiedGet(t, pm, 0, target)
	if err != nil {
		t.Fatalf("attempt 0 (repeat): %v", err)
	}
	viaB, err := proxiedGet(t, pm, 1, target)
	if err != nil {
		t.Fatalf("attempt 1: %v", err)
	}

	if viaA1 != "proxy-one" || viaA2 != "proxy-one" {
		t.Errorf("attempt 0 routed via %q then %q, want proxy-one both times", viaA1, viaA2)
	}
	if viaB != "proxy-two" {
		t.Errorf("attempt 1 routed via %q, want proxy-two", viaB)
	}
}

// TestPerProxyRoundTripper_DefensiveBranches covers the defensive arms of
// RoundTrip: a nil request and a nil URL both delegate to the base transport
// (which rejects them) instead of panicking, and a request the selector maps
// to NO proxy (nil ProxyFunc — a PoolManager built without any proxy config)
// also falls back to the base transport.
func TestPerProxyRoundTripper_DefensiveBranches(t *testing.T) {
	pm, _, _ := newDispatcherFixture(t)
	rt := pm.GetRoundTripper()

	if _, err := rt.RoundTrip(nil); err == nil {
		t.Error("RoundTrip(nil) should delegate to the base transport and error")
	}

	nilURLReq, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	nilURLReq.URL = nil
	if _, err := rt.RoundTrip(nilURLReq); err == nil {
		t.Error("RoundTrip with nil URL should delegate to the base transport and error")
	}
}

// TestPerProxyRoundTripper_NoProxyFallsBackToBase pins the direct-connection
// fallback: a dispatcher whose PoolManager has NO proxy configured (defensive
// — construction does not activate it then) still serves requests through the
// shared base transport.
func TestPerProxyRoundTripper_NoProxyFallsBackToBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Direct construction: no proxy selection is possible, so RoundTrip must
	// use the base transport and reach the origin server directly.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("test setup: http.DefaultTransport is %T, want *http.Transport", http.DefaultTransport)
	}
	d := newPerProxyRoundTripper(nil, base)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := d.RoundTrip(req)
	if err != nil {
		t.Fatalf("direct fallback RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fallback status = %d, want 200", resp.StatusCode)
	}
}

// TestPerProxyRoundTripper_TransportForIsolation pins transportFor's caching
// contract: one transport per proxy URL string, stable identity across calls,
// and distinct transports for distinct proxies. It also drives
// closeIdleConnections over a non-empty dispatcher map (the per-proxy sweep
// branch).
func TestPerProxyRoundTripper_TransportForIsolation(t *testing.T) {
	pm, _, _ := newDispatcherFixture(t)
	d := pm.proxyDispatcher

	u1, _ := url.Parse("http://proxy-one.example:8080")
	u2, _ := url.Parse("http://proxy-two.example:8080")

	t1a := d.transportFor(u1)
	t1b := d.transportFor(u1)
	t2 := d.transportFor(u2)

	if t1a != t1b {
		t.Error("transportFor returned different transports for the same proxy URL")
	}
	if t1a == t2 {
		t.Error("transportFor returned the same transport for two different proxies (HTTP/2 pools would span proxies)")
	}

	// The per-proxy transports must carry a constant Proxy hook pinned to
	// their own proxy — the property that keeps each h2 pool single-proxy.
	got, err := t1a.Proxy(&http.Request{URL: mustParseURLLocal("http://x.example/")})
	if err != nil || got == nil || got.String() != u1.String() {
		t.Errorf("per-proxy transport Proxy hook = (%v, %v), want (%s, nil)", got, err, u1)
	}

	d.closeIdleConnections() // sweeps every per-proxy transport
}

// mustParseURLLocal is the local sibling of the testproxy helper for tests
// that only need a parsed URL and cannot fail.
func mustParseURLLocal(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
