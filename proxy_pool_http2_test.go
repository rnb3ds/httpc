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

// newH2TLSTarget starts an h2-capable TLS test backend. The regular
// httptest.NewTLSServer negotiates HTTP/1.1 only (NextProtos ["http/1.1"]),
// which hides all HTTP/2 behavior.
func newH2TLSTarget(t *testing.T) *httptest.Server {
	t.Helper()
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok")) // best-effort test response
	}))
	target.EnableHTTP2 = true
	target.StartTLS()
	t.Cleanup(target.Close)
	return target
}

// TestProxyPool_HTTP2PerProxyTransport verifies the opt-in
// EnableProxyPoolHTTP2 mode: each proxy gets its own transport with its own
// HTTP/2 pool, so (a) HTTP/2 is actually negotiated through the tunnels,
// (b) proxy rotation still distributes requests across the pool — the h2
// pool can no longer pin traffic to the first proxy because it never spans
// proxies — and (c) Meta.ProxyURL still reports the serving proxy.
func TestProxyPool_HTTP2PerProxyTransport(t *testing.T) {
	target := newH2TLSTarget(t)
	targetAddr := strings.TrimPrefix(target.URL, "https://")

	const numProxies = 3
	proxyURLs, tunnelCounts := newCONNECTProxies(t, numProxies, targetAddr)

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = proxyURLs
	cfg.Connection.EnableProxyPoolHTTP2 = true
	cfg.Security.AllowPrivateIPs = true    // test proxies are on loopback
	cfg.Security.InsecureSkipVerify = true // test TLS cert is self-signed
	cfg.Timeouts.Request = 10 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Phase 1: one request must negotiate HTTP/2 through the selected proxy
	// and report that proxy in Meta.
	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("phase-1 request failed: %v", err)
	}
	if proto := result.Proto(); proto != "HTTP/2.0" {
		t.Fatalf("phase-1 proto = %q, want HTTP/2.0 (per-proxy transport should keep h2 enabled)", proto)
	}
	if result.Meta.ProxyURL == "" {
		t.Fatal("phase-1 Meta.ProxyURL is empty; dispatcher must record the selected proxy")
	}
	_ = result.Body()

	baseline := tunnelCounts()

	// Phase 2: concurrent burst must still rotate — at least two proxies that
	// had no tunnel must be dialed. (The phase-1 proxy's requests ride its
	// existing h2 tunnel, which is correct: the selected proxy is that one.)
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
			_ = res.Body()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent request failed: %v", err)
	}

	after := tunnelCounts()
	proxiesDialed := 0
	for i, c := range after {
		delta := c - baseline[i]
		if delta > 0 {
			proxiesDialed++
		}
		t.Logf("proxy %d: %d new tunnel(s) during burst", i, delta)
	}
	if proxiesDialed < 2 {
		t.Errorf("burst rotated through %d newly-dialed proxy(ies), want >=2 — dispatcher selection is not distributing", proxiesDialed)
	}
}

// TestProxyPool_HTTP2PerProxyRetryRotation covers the dev_test/main_5.go
// scenario under the dispatcher: a request whose first attempt lands on a
// dead proxy must retry through the SELECTED next proxy (SelectIndex(base+1))
// — deterministically, because each proxy's connections are isolated.
func TestProxyPool_HTTP2PerProxyRetryRotation(t *testing.T) {
	target := newH2TLSTarget(t)
	targetAddr := strings.TrimPrefix(target.URL, "https://")

	// Pool layout pins the retry landing spot: warmup takes base 0 (live P0),
	// the 4-request burst takes bases {1,2,3,0}; the base-1 request hits the
	// dead proxy and retries with SelectIndex(1+1=2) → P2, always.
	liveProxies, tunnelCounts := newCONNECTProxies(t, 3, targetAddr)
	dead := "http://127.0.0.1:1" // nothing listens; dial refused instantly
	proxyURLs := []string{liveProxies[0], dead, liveProxies[1], liveProxies[2]}

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = proxyURLs
	cfg.Connection.EnableProxyPoolHTTP2 = true
	cfg.Connection.ProxyRotatePerRequest = true
	cfg.Security.AllowPrivateIPs = true
	cfg.Security.InsecureSkipVerify = true
	cfg.Timeouts.Request = 10 * time.Second
	cfg.Retry.MaxRetries = 0 // rotation raises the budget to 3
	cfg.Retry.Delay = 50 * time.Millisecond

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Warmup: consumes base index 0 (P0).
	if result, err := client.Get(target.URL); err != nil {
		t.Fatalf("warmup failed: %v", err)
	} else {
		_ = result.Body()
	}

	const burst = 4
	type outcome struct {
		attempts int
		proxyURL string
		err      error
	}
	results := make([]outcome, burst)
	var wg sync.WaitGroup
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res, err := client.Get(target.URL)
			if err != nil {
				results[idx] = outcome{err: err}
				return
			}
			results[idx] = outcome{attempts: res.Meta.Attempts, proxyURL: res.Meta.ProxyURL}
			_ = res.Body()
		}(i)
	}
	wg.Wait()

	retried := 0
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("request %d failed: %v", i, r.err)
		}
		if r.attempts < 1 {
			t.Fatalf("request %d attempts = %d, want >=1", i, r.attempts)
		}
		if r.attempts > 1 {
			retried++
			if r.proxyURL != liveProxies[1] {
				t.Errorf("retried request (%d attempts) served by %q, want the rotated proxy %q", r.attempts, r.proxyURL, liveProxies[1])
			}
		} else if r.proxyURL == dead {
			t.Errorf("request %d reports the dead proxy as its server", i)
		}
	}
	if retried != 1 {
		t.Errorf("expected exactly 1 request to retry past the dead proxy, got %d", retried)
	}
	_ = tunnelCounts() // counters available for debugging failures
}

// TestProxyPool_HTTP2PerProxyOptIn pins the default: without
// EnableProxyPoolHTTP2 the shared transport stays on forced HTTP/1 (the
// step-1 fix), so the h2 mode is genuinely opt-in.
func TestProxyPool_HTTP2PerProxyOptIn(t *testing.T) {
	target := newH2TLSTarget(t)
	targetAddr := strings.TrimPrefix(target.URL, "https://")

	proxyURLs, _ := newCONNECTProxies(t, 2, targetAddr)

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = proxyURLs // EnableProxyPoolHTTP2 left false
	cfg.Security.AllowPrivateIPs = true
	cfg.Security.InsecureSkipVerify = true
	cfg.Timeouts.Request = 10 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if proto := result.Proto(); proto != "HTTP/1.1" {
		t.Fatalf("proto = %q, want HTTP/1.1 without EnableProxyPoolHTTP2", proto)
	}
	_ = result.Body()
}

// TestEnableProxyPoolHTTP2Validation pins the fail-fast rule: requesting the
// per-proxy HTTP/2 mode while HTTP/2 is disabled is rejected by ValidateConfig
// instead of being silently ignored.
func TestEnableProxyPoolHTTP2Validation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = []string{"http://p1.example.com:8080", "http://p2.example.com:8080"}
	cfg.Connection.EnableProxyPoolHTTP2 = true
	cfg.Connection.EnableHTTP2 = false

	if _, err := New(cfg); err == nil {
		t.Fatal("New() succeeded with EnableProxyPoolHTTP2 and EnableHTTP2=false, want validation error")
	} else if !strings.Contains(err.Error(), "EnableProxyPoolHTTP2 requires") {
		t.Fatalf("New() error = %v, want an EnableProxyPoolHTTP2 requires EnableHTTP2 message", err)
	}
}
