package httpc

import (
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// forwardProxy creates an HTTP forward proxy that forwards requests to their
// target and tags each with an identifying header so tests can detect which
// proxy was actually used.
func forwardProxy(id string) *httptest.Server {
	// Dedicated transport for forwarding — no proxy, no compression surprises.
	fwd := &http.Transport{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For HTTP proxy requests, r.URL is the full target URL.
		outReq := &http.Request{
			Method: r.Method,
			URL:    r.URL,
			Header: r.Header.Clone(),
		}
		outReq.Header.Set("X-Proxy-ID", id)

		resp, err := fwd.RoundTrip(outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		for k, vs := range resp.Header {
			w.Header()[k] = vs
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body) // best-effort test proxying
	}))
}

// TestProxyPool_EndToEndRotation verifies that round-robin actually routes
// sequential requests through different proxies — proving the full pipeline
// (httpc engine → transport.Proxy → pool.Select → proxy dial) rotates IPs.
// TestProxyPool_Rotation consolidates the former EndToEndRotation and
// RotatePerRequest_NoRetries tests (identical echo-target + 3-forwardProxy
// scaffold; only the ProxyRotatePerRequest flag and request count varied).
func TestProxyPool_Rotation(t *testing.T) {
	tests := []struct {
		name             string
		rotatePerRequest bool
		maxRetries       int
		requests         int
	}{
		// Round-robin spreads sequential requests across the pool.
		{"round robin across sequential requests", false, 1, 6},
		// ProxyRotatePerRequest guarantees per-request rotation even on the
		// MaxRetries=0 fast path, which previously had no rotation wiring.
		{"per-request rotation on the no-retry fast path", true, 0, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
			}))
			defer target.Close()

			ids := []string{"proxy-A", "proxy-B", "proxy-C"}
			proxyURLs := make([]string, len(ids))
			for i, id := range ids {
				p := forwardProxy(id)
				defer p.Close()
				proxyURLs[i] = p.URL
			}

			cfg := DefaultConfig()
			cfg.Connection.ProxyPool = proxyURLs
			cfg.Connection.ProxyRotatePerRequest = tt.rotatePerRequest
			cfg.Retry.MaxRetries = tt.maxRetries
			cfg.Security.AllowPrivateIPs = true // test servers are on localhost
			cfg.Timeouts.Request = 5 * time.Second

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			seen := make(map[string]int)
			for i := 0; i < tt.requests; i++ {
				result, err := client.Get(target.URL)
				if err != nil {
					t.Fatalf("Request %d failed: %v", i, err)
				}
				body := strings.TrimSpace(result.Body())
				seen[body]++
				t.Logf("Request %d: proxy=%s, status=%d", i, body, result.StatusCode())
			}

			if len(seen) != 3 {
				t.Errorf("expected 3 distinct proxies visited, got %d: %v", len(seen), seen)
			}
		})
	}
}

// TestProxyPool_RotatePerRequest_SkipsBadProxy was removed: the dead-proxy
// skip scenario is asserted with a stronger contract (dead proxy + Meta
// verification) by TestProxyPool_MetaProxyURL_AfterRetry below.

// TestProxyPool_RotateOn403 verifies that a 403 response triggers a retry
// through a different proxy (status-based rotation via ProxyRotateOnStatus),
// both for a direct target and for a redirect-then-403 target. The redirect
// variant (formerly TestProxyPool_RotateOn403_RedirectSafe) pins that
// redirect-following within a single attempt does NOT consume the rotation
// budget: without deterministic SelectIndex, http.Client's extra Proxy() call
// per redirect would advance the round-robin counter and could land the retry
// on the same proxy.
func TestProxyPool_RotateOn403(t *testing.T) {
	tests := []struct {
		name        string
		redirecting bool
	}{
		{"direct 403 target", false},
		{"redirect-then-403 target stays rotation-safe", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var attempted []string

			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				proxyID := r.Header.Get("X-Proxy-ID")
				mu.Lock()
				attempted = append(attempted, proxyID)
				mu.Unlock()

				// Redirecting variant: first hop redirects (extra Proxy()
				// call within the same attempt), final hop returns 403.
				if tt.redirecting && r.URL.Path != "/challenge" {
					http.Redirect(w, r, "/challenge", http.StatusFound)
					return
				}
				w.WriteHeader(http.StatusForbidden)
			}))
			defer target.Close()

			ids := []string{"proxy-A", "proxy-B", "proxy-C"}
			proxyURLs := make([]string, len(ids))
			for i, id := range ids {
				p := forwardProxy(id)
				defer p.Close()
				proxyURLs[i] = p.URL
			}

			cfg := DefaultConfig()
			cfg.Connection.ProxyPool = proxyURLs
			cfg.Connection.ProxyRotateOnStatus = []int{403}
			cfg.Security.AllowPrivateIPs = true
			cfg.Retry.MaxRetries = 2
			cfg.Retry.Delay = 50 * time.Millisecond
			cfg.Timeouts.Request = 10 * time.Second

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			result, err := client.Get(target.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			if result.StatusCode() != http.StatusForbidden {
				t.Errorf("expected final status 403, got %d", result.StatusCode())
			}

			// Should have attempted at least 2 different proxies
			// (1 original + 1 retry), despite any extra Proxy() calls.
			mu.Lock()
			defer mu.Unlock()
			distinct := make(map[string]bool)
			for _, id := range attempted {
				distinct[id] = true
			}
			if len(distinct) < 2 {
				t.Errorf("expected rotation across >=2 proxies on 403, but only used %d distinct: %v (all attempts: %v)",
					len(distinct), distinct, attempted)
			}
			t.Logf("403 rotation: %d attempts across %d distinct proxies: %v",
				len(attempted), len(distinct), attempted)
		})
	}
}

// newCONNECTProxies spins up n CONNECT-tunnel proxies that each relay to
// targetAddr and count their tunnels. Shared by the HTTPS rotation tests.
func newCONNECTProxies(t *testing.T, n int, targetAddr string) (proxyURLs []string, counts func() []int64) {
	t.Helper()
	raw := make([]int64, n)
	servers := make([]*httptest.Server, n)
	proxyURLs = make([]string, n)

	for i := range proxyURLs {
		idx := i
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				http.Error(w, "only CONNECT supported", http.StatusBadRequest)
				return
			}
			atomic.AddInt64(&raw[idx], 1)

			dst, err := net.Dial("tcp", targetAddr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				_ = dst.Close()
				http.Error(w, "hijack not supported", http.StatusInternalServerError)
				return
			}
			clientConn, bufrw, err := hj.Hijack()
			if err != nil {
				_ = dst.Close()
				return
			}
			defer func() { _ = clientConn.Close() }()
			defer func() { _ = dst.Close() }()

			_, _ = bufrw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n") // best-effort handshake
			_ = bufrw.Flush()                                                       // best-effort handshake completion

			go func() {
				_, _ = io.Copy(dst, bufrw) // tunnel relay
				_ = dst.Close()
			}()
			_, _ = io.Copy(clientConn, dst) // tunnel relay
		}))
		servers[i] = proxy
		t.Cleanup(proxy.Close)
		proxyURLs[i] = proxy.URL
	}

	return proxyURLs, func() []int64 {
		out := make([]int64, len(raw))
		for i := range raw {
			out[i] = atomic.LoadInt64(&raw[i])
		}
		return out
	}
}

// TestProxyPool_HTTPSRotation consolidates the former EndToEndRotation_HTTPS
// and RotatePerRequest_HTTPS tests (identical CONNECT-tunnel scaffold; the
// only differences were the ProxyRotatePerRequest flag and the exactness of
// the distinct-proxy count). Each row proves the full pipeline — engine →
// transport.Proxy → pool.Select → CONNECT tunnel — through real TLS tunnels.
func TestProxyPool_HTTPSRotation(t *testing.T) {
	tests := []struct {
		name              string
		rotatePerRequest  bool
		requests          int
		wantUsedProxies   int    // distinct proxies that opened >=1 tunnel
		wantCountOperator string // "==" or ">="
	}{
		// Baseline: without per-request rotation the pool still spreads
		// sequential requests across all proxies.
		{"sequential requests spread across proxies", false, 6, 3, ">="},
		// ProxyRotatePerRequest: exactly one fresh tunnel per request, no
		// connection reuse through the previous request's tunnel.
		{"per-request rotation uses each proxy exactly once", true, 3, 3, "=="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.RemoteAddr)) // best-effort test response
			}))
			defer target.Close()
			targetAddr := strings.TrimPrefix(target.URL, "https://")

			proxyURLs, tunnelCounts := newCONNECTProxies(t, 3, targetAddr)

			cfg := DefaultConfig()
			cfg.Connection.ProxyPool = proxyURLs
			cfg.Connection.ProxyRotatePerRequest = tt.rotatePerRequest
			cfg.Security.AllowPrivateIPs = true
			cfg.Security.InsecureSkipVerify = true // test TLS cert is self-signed
			cfg.Timeouts.Request = 5 * time.Second

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			for i := 0; i < tt.requests; i++ {
				result, err := client.Get(target.URL)
				if err != nil {
					t.Fatalf("Request %d failed: %v", i, err)
				}
				t.Logf("Request %d: RemoteAddr=%s, status=%d", i,
					strings.TrimSpace(result.Body()), result.StatusCode())
			}

			counts := tunnelCounts()
			used := 0
			for _, c := range counts {
				if c > 0 {
					used++
				}
			}
			switch tt.wantCountOperator {
			case "==":
				if used != tt.wantUsedProxies {
					t.Errorf("distinct CONNECT proxies used = %d, want exactly %d (tunnel counts: %v)",
						used, tt.wantUsedProxies, counts)
				}
			default:
				if used < tt.wantUsedProxies {
					t.Errorf("distinct CONNECT proxies used = %d, want at least %d (tunnel counts: %v)",
						used, tt.wantUsedProxies, counts)
				}
			}
		})
	}
}

// TestProxyPool_RotateOn403_OneProxySucceeds simulates the real-world
// status-based blocking scenario: proxy-A always gets 403 (blocked), proxy-B
// returns 200 (passes). The client should rotate from A to B on 403 and
// return 200.
func TestProxyPool_RotateOn403_OneProxySucceeds(t *testing.T) {
	var mu sync.Mutex
	var attempted []string

	// Target returns 403 if request came from proxy-A, 200 if from proxy-B.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyID := r.Header.Get("X-Proxy-ID")
		mu.Lock()
		attempted = append(attempted, proxyID)
		mu.Unlock()

		if proxyID == "proxy-A" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("blocked")) // best-effort test response
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK from " + proxyID)) // best-effort test response
	}))
	defer target.Close()

	// Two forward proxies: A (returns 403) and B (returns 200).
	proxyA := forwardProxy("proxy-A")
	defer proxyA.Close()
	proxyB := forwardProxy("proxy-B")
	defer proxyB.Close()

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = []string{proxyA.URL, proxyB.URL}
	cfg.Connection.ProxyRotateOnStatus = []int{403}
	cfg.Security.AllowPrivateIPs = true
	cfg.Retry.MaxRetries = 3
	cfg.Retry.Delay = 50 * time.Millisecond
	cfg.Timeouts.Request = 10 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	t.Logf("Final status: %d, body: %s, attempts: %d",
		result.StatusCode(), strings.TrimSpace(result.Body()), result.Meta.Attempts)
	t.Logf("Attempted proxies: %v", attempted)

	if result.StatusCode() != http.StatusOK {
		t.Errorf("expected final status 200 (from proxy-B), got %d", result.StatusCode())
	}
	if len(attempted) < 2 {
		t.Errorf("expected at least 2 attempts (A then B), got %d: %v", len(attempted), attempted)
	}
}

// TestProxyPool_MetaProxyURL verifies Result.Meta.ProxyURL reports the pool
// entry that actually served each request. The target echoes the forwarding
// proxy's ID, so the metadata is cross-checked against ground truth rather
// than against itself.
func TestProxyPool_MetaProxyURL(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
	}))
	defer target.Close()

	ids := []string{"proxy-A", "proxy-B", "proxy-C"}
	proxyURLs := make([]string, len(ids))
	idByURL := make(map[string]string, len(ids))
	for i, id := range ids {
		p := forwardProxy(id)
		defer p.Close()
		proxyURLs[i] = p.URL
		idByURL[p.URL] = id
	}

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = proxyURLs
	cfg.Connection.ProxyRotatePerRequest = true
	cfg.Retry.MaxRetries = 0 // exercise the no-retry fast path
	cfg.Security.AllowPrivateIPs = true
	cfg.Timeouts.Request = 5 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	seen := make(map[string]int)
	for i := 0; i < 3; i++ {
		result, err := client.Get(target.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		if result.Meta.ProxyURL == "" {
			t.Fatalf("Request %d: Meta.ProxyURL is empty, want the serving proxy", i)
		}
		id, ok := idByURL[result.Meta.ProxyURL]
		if !ok {
			t.Fatalf("Request %d: Meta.ProxyURL %q is not a configured pool entry", i, result.Meta.ProxyURL)
		}
		if echoed := strings.TrimSpace(result.Body()); echoed != id {
			t.Errorf("Request %d: Meta.ProxyURL says %s but the request was forwarded by %s",
				i, id, echoed)
		}
		seen[result.Meta.ProxyURL]++
	}
	if len(seen) != 3 {
		t.Errorf("expected Meta.ProxyURL to report 3 distinct proxies, got %d: %v", len(seen), seen)
	}
}

// TestProxyPool_MetaProxyURL_SingleProxy verifies Meta.ProxyURL for a single
// static Connection.ProxyURL (the highest-priority proxy mode).
func TestProxyPool_MetaProxyURL_SingleProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
	}))
	defer target.Close()

	staticProxy := forwardProxy("static-proxy")
	defer staticProxy.Close()

	cfg := DefaultConfig()
	cfg.Connection.ProxyURL = staticProxy.URL
	cfg.Security.AllowPrivateIPs = true
	cfg.Timeouts.Request = 5 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if result.Meta.ProxyURL != staticProxy.URL {
		t.Errorf("Meta.ProxyURL = %q, want %q", result.Meta.ProxyURL, staticProxy.URL)
	}
}

// TestMetaProxyURL_DirectConnection verifies Meta.ProxyURL stays empty when
// no proxy is configured.
func TestMetaProxyURL_DirectConnection(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok")) // best-effort test response
	}))
	defer target.Close()

	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = true // test server is on localhost
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if result.Meta.ProxyURL != "" {
		t.Errorf("Meta.ProxyURL = %q, want empty for direct connection", result.Meta.ProxyURL)
	}
}

// TestProxyPool_MetaProxyURL_AfterRetry verifies Meta.ProxyURL reports the
// proxy that served the FINAL attempt when earlier attempts failed on a dead
// proxy and rotated away — the scenario dev_test/main_6.go debugs.
func TestProxyPool_MetaProxyURL_AfterRetry(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
	}))
	defer target.Close()

	deadProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "should not be reached", http.StatusInternalServerError)
	}))
	deadProxy.Close() // closed immediately so connections are refused

	healthyProxy := forwardProxy("proxy-A")
	defer healthyProxy.Close()

	cfg := DefaultConfig()
	// Dead proxy first: the initial attempt (base index 0) hits it, the
	// retry (base index 1) must rotate to the healthy proxy.
	cfg.Connection.ProxyPool = []string{deadProxy.URL, healthyProxy.URL}
	cfg.Connection.ProxyRotatePerRequest = true
	cfg.Retry.MaxRetries = 1
	cfg.Retry.Delay = 10 * time.Millisecond
	cfg.Security.AllowPrivateIPs = true
	cfg.Timeouts.Request = 10 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("Request failed (should have rotated past dead proxy): %v", err)
	}
	if result.Meta.Attempts < 2 {
		t.Fatalf("expected a retry past the dead proxy, attempts = %d", result.Meta.Attempts)
	}
	if result.Meta.ProxyURL != healthyProxy.URL {
		t.Errorf("Meta.ProxyURL = %q, want %q (the final attempt's proxy)",
			result.Meta.ProxyURL, healthyProxy.URL)
	}
	if echoed := strings.TrimSpace(result.Body()); echoed != "proxy-A" {
		t.Errorf("forwarded by %q, want proxy-A", echoed)
	}
}

// ----------------------------------------------------------------------------
// SOCKS5 pool entries & selection strategies
// ----------------------------------------------------------------------------

// socks5Listener starts a minimal RFC 1928 SOCKS5 server on loopback:
// no-auth greeting + CONNECT, then a blind pipe to the dialed target. It is
// just enough protocol for net/http's built-in SOCKS5 dialer (the transport
// used for socks5:// proxy URLs), so a pool entry can be dialed fully offline.
func socks5Listener(t *testing.T) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("socks5 listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go serveSOCKS5(conn)
		}
	}()
	return ln
}

// serveSOCKS5 handles one no-auth CONNECT request on c and pipes both ways.
func serveSOCKS5(c net.Conn) {
	defer func() { _ = c.Close() }()

	readFull := func(n int) []byte {
		b := make([]byte, n)
		if _, err := io.ReadFull(c, b); err != nil {
			return nil
		}
		return b
	}

	// Client greeting: VER NMETHODS METHODS...
	hdr := readFull(2)
	if hdr == nil || hdr[0] != 0x05 {
		return
	}
	if readFull(int(hdr[1])) == nil {
		return
	}
	// Select no-auth.
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Request: VER CMD(=CONNECT) RSV ATYP ADDR PORT
	req := readFull(4)
	if req == nil || req[1] != 0x01 {
		return
	}
	var host string
	switch req[3] {
	case 0x01: // IPv4
		b := readFull(4)
		if b == nil {
			return
		}
		host = net.IP(b).String()
	case 0x04: // IPv6
		b := readFull(16)
		if b == nil {
			return
		}
		host = net.IP(b).String()
	case 0x03: // domain
		l := readFull(1)
		if l == nil {
			return
		}
		b := readFull(int(l[0]))
		if b == nil {
			return
		}
		host = string(b)
	default:
		return
	}
	pb := readFull(2)
	if pb == nil {
		return
	}
	port := binary.BigEndian.Uint16(pb)

	target, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		// Reply: general failure.
		_, _ = c.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = target.Close() }()

	// Reply: succeeded, zero bound address/port.
	if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(target, c); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, target); done <- struct{}{} }()
	<-done
}

// TestProxyPool_SOCKS5EntryDialed proves a socks5:// pool entry is actually
// dialed end-to-end (pool.Select -> transport SOCKS5 handshake -> target)
// using only loopback listeners.
func TestProxyPool_SOCKS5EntryDialed(t *testing.T) {
	var proxied atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Store(true)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("socks-ok"))
	}))
	defer target.Close()

	ln := socks5Listener(t)

	cfg := DefaultConfig()
	cfg.Connection.ProxyPool = []string{"socks5://" + ln.Addr().String()}
	cfg.Security.AllowPrivateIPs = true // both listeners are on loopback
	cfg.Timeouts.Request = 10 * time.Second

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New(cfg with socks5 pool) failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("request through SOCKS5 pool entry failed: %v", err)
	}
	if strings.TrimSpace(result.Body()) != "socks-ok" {
		t.Errorf("body = %q, want %q", result.Body(), "socks-ok")
	}
	if !proxied.Load() {
		t.Error("target was never reached; SOCKS5 tunnel did not forward")
	}
	if want := "socks5://" + ln.Addr().String(); result.Meta.ProxyURL != want {
		t.Errorf("Meta.ProxyURL = %q, want the socks pool entry %q", result.Meta.ProxyURL, want)
	}
}

// TestProxyPool_StrategySelection verifies both pool strategies through the
// public Config: round-robin alternates deterministically across sequential
// requests, and random always lands inside the pool (order is intentionally
// not asserted). Selection is observed via Result.Meta.ProxyURL.
func TestProxyPool_StrategySelection(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	tests := []struct {
		name     string
		strategy ProxyStrategy
		verify   func(t *testing.T, picked []string, pool map[string]bool)
	}{
		{
			name:     "round robin alternates in order",
			strategy: ProxyStrategyRoundRobin,
			verify: func(t *testing.T, picked []string, pool map[string]bool) {
				if len(picked) != 4 {
					t.Fatalf("collected %d selections, want 4", len(picked))
				}
				// Sequential requests must cycle A,B,A,B across a 2-proxy pool.
				for i, got := range picked {
					want := picked[0]
					if i%2 == 1 {
						want = other(picked[0], pool)
					}
					if got != want {
						t.Errorf("request %d used %q, want %q (round-robin order)", i, got, want)
					}
				}
			},
		},
		{
			name:     "random stays inside the pool",
			strategy: ProxyStrategyRandom,
			verify: func(t *testing.T, picked []string, pool map[string]bool) {
				for i, got := range picked {
					if !pool[got] {
						t.Errorf("request %d selected %q, which is not in the pool", i, got)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyA := forwardProxy("strategy-A")
			defer proxyA.Close()
			proxyB := forwardProxy("strategy-B")
			defer proxyB.Close()

			pool := map[string]bool{proxyA.URL: true, proxyB.URL: true}

			cfg := DefaultConfig()
			cfg.Connection.ProxyPool = []string{proxyA.URL, proxyB.URL}
			cfg.Connection.ProxyPoolStrategy = tt.strategy
			// Close idle conns per request so each Get re-selects a proxy
			// (connection reuse would keep request 2 on request 1's tunnel).
			cfg.Connection.ProxyRotatePerRequest = true
			cfg.Security.AllowPrivateIPs = true
			cfg.Timeouts.Request = 10 * time.Second

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			var picked []string
			for i := 0; i < 4; i++ {
				result, err := client.Get(target.URL)
				if err != nil {
					t.Fatalf("request %d failed: %v", i, err)
				}
				if result.Meta == nil || result.Meta.ProxyURL == "" {
					t.Fatalf("request %d: Meta.ProxyURL not populated", i)
				}
				picked = append(picked, result.Meta.ProxyURL)
			}
			tt.verify(t, picked, pool)
		})
	}
}

// other returns the pool entry that is not want.
func other(want string, pool map[string]bool) string {
	for entry := range pool {
		if entry != want {
			return entry
		}
	}
	return want
}
