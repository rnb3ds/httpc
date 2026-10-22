package httpc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestProxyPool_ConcurrentRequestsNotPinnedByHTTP2Pool pins the regression
// where a proxy pool over HTTPS was silently defeated by net/http's HTTP/2
// connection pool: that pool is keyed by (scheme, target authority) only —
// the proxy carrying a CONNECT tunnel is not part of the key — so once one
// tunnel upgraded to h2, every later request to the same target was served
// from the pool without consulting Transport.Proxy, pinning all traffic to
// whichever proxy connected first (0 further tunnels, no load balancing).
// NewPoolManager now forces HTTP/1 while a pool of more than one proxy is
// active, which this test guards.
//
// The discriminator is phase-based and deterministic:
//
//	phase 1 — one request opens exactly one tunnel; with HTTP/2 enabled that
//	          tunnel upgrades to h2 and registers in the shared h2 pool.
//	phase 2 — a concurrent burst. With the bug, every request hits the h2
//	          pool entry from phase 1 and rides that one tunnel: zero new
//	          tunnels. With HTTP/1, every miss dials through the pool-selected
//	          proxy: several new tunnels across distinct proxies.
//
// ProxyRotatePerRequest is deliberately FALSE here: per-request rotation
// calls CloseIdleConnections at the start of every request, which also
// drains the h2 pool and masks the pinning. The rotate=false path has no
// such drain, so the h2 pool entry from phase 1 survives into phase 2. (The
// real-world report that exposed this — dev_test/main_5.go — hit the same
// root cause through retry attempts during whose backoff sleep other
// requests re-seeded the h2 pool.)
//
// The backend must speak h2 (httptest.Server.EnableHTTP2): the regular
// NewTLSServer negotiates HTTP/1.1 only, which is why the pre-existing
// TestProxyPool_HTTPSRotation never caught this.
func TestProxyPool_ConcurrentRequestsNotPinnedByHTTP2Pool(t *testing.T) {
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok")) // best-effort test response
	}))
	target.EnableHTTP2 = true // the bug requires an h2-capable target
	target.StartTLS()
	defer target.Close()
	targetAddr := strings.TrimPrefix(target.URL, "https://")

	const numProxies = 3
	proxyURLs, tunnelCounts := newCONNECTProxies(t, numProxies, targetAddr)

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = proxyURLs
	// No ProxyRotatePerRequest: see the comment above for why its
	// CloseIdleConnections-per-request would mask the pinning.
	cfg.Security.AllowPrivateIPs = true    // test proxies are on loopback
	cfg.Security.InsecureSkipVerify = true // test TLS cert is self-signed
	cfg.Timeouts.Request = 10 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// --- Phase 1: open exactly one tunnel (and, pre-fix, one h2 pool entry) -

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("phase-1 request failed: %v", err)
	}
	if result.StatusCode() != http.StatusOK {
		t.Fatalf("phase-1 status = %d, want 200", result.StatusCode())
	}
	t.Logf("phase-1 served over %s", result.Proto())
	_ = result.Body()

	baseline := tunnelCounts()
	var phase1Tunnels int64
	for _, c := range baseline {
		phase1Tunnels += c
	}
	if phase1Tunnels != 1 {
		t.Fatalf("phase 1 opened %d tunnels, want exactly 1", phase1Tunnels)
	}

	// --- Phase 2: concurrent burst -------------------------------------------

	const burst = 6
	var wg sync.WaitGroup
	errs := make(chan error, burst)
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := client.Get(target.URL)
			if err != nil {
				errs <- err
				return
			}
			if res.StatusCode() != http.StatusOK {
				errs <- fmt.Errorf("status %d, want 200", res.StatusCode())
				return
			}
			t.Logf("phase-2 served over %s", res.Proto())
			_ = res.Body()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent request failed: %v", err)
	}

	// --- Assert: the burst was not pinned to the phase-1 tunnel --------------

	after := tunnelCounts()
	newTunnels := 0
	proxiesDialed := 0
	for i, c := range after {
		delta := c - baseline[i]
		newTunnels += int(delta)
		if delta > 0 {
			proxiesDialed++
		}
		t.Logf("proxy %d: %d new tunnel(s) during burst", i, delta)
	}
	// With HTTP/1 the burst opens 2 tunnels per pool proxy that was not the
	// phase-1 proxy (round-robin bases {1,2,0,1,2,0}: the phase-1 proxy is
	// served by keep-alive reuse, the other two dial twice each). Assert the
	// conservative lower bound — at least one new tunnel through at least two
	// distinct proxies — which the pinned (h2) build cannot reach: it opens
	// zero new tunnels because every request rides the phase-1 tunnel.
	if newTunnels < 2 || proxiesDialed < 2 {
		t.Errorf("concurrent burst was pinned to the phase-1 tunnel: %d new tunnel(s) across %d proxy(ies), want >=2 across >=2 (HTTP/2 pool bypassed proxy selection)", newTunnels, proxiesDialed)
	}
}
