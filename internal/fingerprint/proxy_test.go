package fingerprint

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cybergodev/httpc/internal/connection"
)

// newPoolAdapter builds an adapter wired to a real PoolManager over the given
// proxies, mirroring how the engine constructs the fingerprint path.
func newPoolAdapter(t *testing.T, proxies []string) (*Adapter, *connection.PoolManager) {
	t.Helper()
	pm, err := connection.NewPoolManager(&connection.Config{ProxyPool: proxies})
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	t.Cleanup(func() { _ = pm.Close() })

	a := NewAdapter(Config{
		PresetName:      "chrome",
		AllowPrivateIPs: true, // loopback targets in tests
		Pool:            pm,
	})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}
	return a, pm
}

// closedLoopbackAddr returns a loopback address that is guaranteed not to be
// listening (a released listener port), for deterministic dial failures.
func closedLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return addr
}

// TestSelectProxy_PerAttemptIndex pins that proxy selection rides the owning
// PoolManager: the per-attempt context key selects pool entries
// deterministically (attempt 0 → p0, attempt 1 → p1), and an adapter without
// a pool is always direct (nil, nil).
func TestSelectProxy_PerAttemptIndex(t *testing.T) {
	t.Run("nil pool is direct", func(t *testing.T) {
		a := NewAdapter(Config{PresetName: "chrome"})
		req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
		got, err := a.selectProxy(req)
		if got != nil || err != nil {
			t.Errorf("selectProxy without pool = (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("attempt index selects pool entry", func(t *testing.T) {
		a, _ := newPoolAdapter(t, []string{
			"http://p0.example.com:8080",
			"http://p1.example.com:8080",
		})
		req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)

		for attempt, wantHost := range map[int]string{0: "p0.example.com:8080", 1: "p1.example.com:8080"} {
			ctx := connection.WithProxyAttempt(req.Context(), attempt)
			got, err := a.selectProxy(req.WithContext(ctx))
			if err != nil {
				t.Fatalf("selectProxy(attempt %d): %v", attempt, err)
			}
			if got == nil || got.Host != wantHost {
				t.Errorf("attempt %d selected %v, want %s", attempt, got, wantHost)
			}
		}

		// Without an attempt key the pool rotates (round-robin): still a proxy.
		got, err := a.selectProxy(req)
		if err != nil || got == nil {
			t.Errorf("selectProxy(round-robin) = (%v, %v), want a proxy", got, err)
		}
	})
}

// TestReportProxy_NilArgumentsNoOp pins the guards: no pool or no proxy URL
// makes reporting a no-op (never a panic).
func TestReportProxy_NilArgumentsNoOp(t *testing.T) {
	a, _ := newPoolAdapter(t, []string{"http://p0.example.com:8080"})

	a.reportProxy(nil, true)
	a.reportProxy(nil, false)

	bare := NewAdapter(Config{PresetName: "chrome"})
	u := &url.URL{Scheme: "http", Host: "p.example.com:8080"}
	bare.reportProxy(u, true)
	bare.reportProxy(u, false)
}

// TestReportProxy_FeedsCircuitBreaker pins the observable effect of
// reportProxy(false): after the pool's failure threshold, the proxy's circuit
// opens and subsequent selections skip it (attempt 0 lands on the healthy
// entry instead), and a success report returns it to rotation.
func TestReportProxy_FeedsCircuitBreaker(t *testing.T) {
	const threshold = 3
	pm, err := connection.NewPoolManager(&connection.Config{
		ProxyPool:             []string{"http://p0.example.com:8080", "http://p1.example.com:8080"},
		ProxyFailureThreshold: threshold,
	})
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	a := NewAdapter(Config{PresetName: "chrome", Pool: pm})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	p0 := &url.URL{Scheme: "http", Host: "p0.example.com:8080"}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	ctx0 := connection.WithProxyAttempt(context.Background(), 0)

	// Before the breaker opens, attempt 0 selects p0.
	if got, err := pm.SelectProxy(ctx0, req.URL); err != nil || got.Host != "p0.example.com:8080" {
		t.Fatalf("pre-breaker selection = (%v, %v), want p0", got, err)
	}

	for range threshold {
		a.reportProxy(p0, false)
	}

	// The circuit is open: attempt 0 must now scan forward to p1.
	got, err := pm.SelectProxy(ctx0, req.URL)
	if err != nil {
		t.Fatalf("post-breaker SelectProxy: %v", err)
	}
	if got.Host != "p1.example.com:8080" {
		t.Errorf("open circuit not skipped: attempt 0 selected %s, want p1", got.Host)
	}

	// A success report clears the failure streak; p0 returns to rotation.
	a.reportProxy(p0, true)
	if got, err := pm.SelectProxy(ctx0, req.URL); err != nil || got.Host != "p0.example.com:8080" {
		t.Errorf("after ReportProxySuccess attempt 0 = (%v, %v), want p0 back", got, err)
	}
}

// TestAdapterRoundTrip_ThroughProxy drives the full proxied path offline: a
// loopback CONNECT proxy tunnels to a loopback target, exercising proxy
// selection, per-proxy backend construction, the proxy dialer, and the
// success report to the pool's breaker.
func TestAdapterRoundTrip_ThroughProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxied-ok")
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)

	proxyAddr, _ := startConnectProxy(t, http.StatusOK)

	a, _ := newPoolAdapter(t, []string{"http://" + proxyAddr})

	req, _ := http.NewRequest(http.MethodGet, "http://"+targetURL.Host+"/", nil)
	resp, err := a.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip through loopback proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "proxied-ok" {
		t.Errorf("body = %q, want proxied-ok", body)
	}

	// The proxy backend is cached under its URL key.
	key := (&url.URL{Scheme: "http", Host: proxyAddr}).String()
	if _, ok := a.perProxy[key]; !ok {
		t.Error("per-proxy backend was not cached")
	}
}

// TestAdapterRoundTrip_DeadProxyReportsFailure pins the failure leg: with a
// failure threshold of 1, one round trip through an unreachable proxy both
// fails with a connection-class error and reports the failure — observable
// as every subsequent pool selection skipping the dead entry.
func TestAdapterRoundTrip_DeadProxyReportsFailure(t *testing.T) {
	deadAddr := closedLoopbackAddr(t)

	pm, err := connection.NewPoolManager(&connection.Config{
		ProxyPool: []string{
			"http://" + deadAddr, // round-robin cursor starts here: selected first
			"http://p1.example.com:8080",
		},
		ProxyFailureThreshold: 1,
	})
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	a := NewAdapter(Config{PresetName: "chrome", AllowPrivateIPs: true, Pool: pm})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	_, err = a.RoundTrip(req)
	if err == nil {
		t.Fatal("RoundTrip through a dead proxy should fail")
	}
	if !strings.Contains(err.Error(), deadAddr) {
		t.Errorf("error should mention the dead proxy address %s: %v", deadAddr, err)
	}

	// The failure was reported: the dead proxy's circuit is open, so every
	// subsequent selection (round-robin from any start) lands on p1.
	for range 3 {
		got, err := pm.SelectProxy(context.Background(), req.URL)
		if err != nil {
			t.Fatalf("post-failure SelectProxy: %v", err)
		}
		if got.Host != "p1.example.com:8080" {
			t.Fatalf("dead proxy still selected after reported failure: got %s", got.Host)
		}
	}
}
