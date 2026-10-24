package connection

// ============================================================================
// DIALER, TLS CONFIGURATION, AND SSRF PROTECTION TESTS
//
// Gathers every test that drives createDialer / createTLSConfig /
// resolveAndValidateAddress / trackedConn directly (formerly spread across
// pool_test.go, coverage_test.go, and pool_extra_test.go). Pool-level
// lifecycle, config, and proxy-selection tests live in pool_test.go.
// ============================================================================

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Dialer: closed pool
// ---------------------------------------------------------------------------

// TestPoolManager_CreateDialer_Closed covers the early-return when the pool is
// closed: the dialer must refuse with an error rather than attempt a dial.
func TestPoolManager_CreateDialer_Closed(t *testing.T) {
	pm, err := NewPoolManager(&Config{AllowPrivateIPs: true})
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}

	if err := pm.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !pm.closed.Load() {
		t.Fatal("pool should be closed after Close")
	}

	dialer := pm.createDialer()
	conn, err := dialer(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected error dialing after pool close, got nil")
	}
	if err.Error() != "connection pool is closed" {
		t.Errorf("unexpected dial error after close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dialer: admission control (MaxTotalConns)
// ---------------------------------------------------------------------------

// TestCreateDialer_PoolExhaustion verifies that the second dial is rejected
// with ErrPoolExhausted once the single MaxTotalConns slot is occupied.
func TestCreateDialer_PoolExhaustion(t *testing.T) {
	// TCP listener that accepts and holds a connection so the first dial
	// stays open and occupies a pool slot.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr := listener.Addr().String()

	config := &Config{
		MaxTotalConns:   1,
		AllowPrivateIPs: true,
		DialTimeout:     5 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialer := pm.createDialer()

	// First connection succeeds and occupies the single slot.
	conn1, err := dialer(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("First dial failed: %v", err)
	}
	defer func() { _ = conn1.Close() }()

	// Accept on the server side so the connection completes.
	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	// Second dial should be rejected: MaxTotalConns exceeded.
	_, err = dialer(context.Background(), "tcp", addr)
	if err == nil {
		t.Fatal("Expected ErrPoolExhausted, got nil")
	}
	if !errors.Is(err, ErrPoolExhausted) {
		t.Errorf("Expected ErrPoolExhausted, got: %v", err)
	}

	// Verify rejected-connections counter was incremented.
	metrics := pm.GetMetrics()
	if metrics.RejectedConnections < 1 {
		t.Errorf("Expected RejectedConnections >= 1, got %d", metrics.RejectedConnections)
	}
}

// TestCreateDialer_MaxTotalConns_ConcurrentAdmission exercises the atomic
// slot-reservation under contention: with MaxTotalConns=3 and 10 concurrent
// dials whose connections stay open, exactly 3 must be admitted and exactly 7
// rejected with ErrPoolExhausted — the admission decision must neither
// over-admit (TOCTOU) nor leak slots.
func TestCreateDialer_MaxTotalConns_ConcurrentAdmission(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	addr := listener.Addr().String()

	const (
		maxTotal = 3
		attempts = 10
	)

	config := &Config{
		MaxTotalConns:   maxTotal,
		AllowPrivateIPs: true,
		DialTimeout:     5 * time.Second,
	}
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialer := pm.createDialer()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		conns    []net.Conn
		admitted int
		rejected int
		other    int
	)
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			conn, err := dialer(context.Background(), "tcp", addr)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				admitted++
				conns = append(conns, conn)
			case errors.Is(err, ErrPoolExhausted):
				rejected++
			default:
				other++
			}
		}()
	}
	close(start)
	wg.Wait()

	// Return all slots before asserting so the gauges can be checked too.
	for _, c := range conns {
		_ = c.Close()
	}

	if admitted != maxTotal || rejected != attempts-maxTotal || other != 0 {
		t.Errorf("admitted=%d rejected=%d otherErr=%d, want %d/%d/0",
			admitted, rejected, other, maxTotal, attempts-maxTotal)
	}

	m := pm.GetMetrics()
	if m.RejectedConnections != attempts-maxTotal {
		t.Errorf("RejectedConnections = %d, want %d", m.RejectedConnections, attempts-maxTotal)
	}
	if m.ActiveConnections != 0 || pm.totalConns.Load() != 0 {
		t.Errorf("after closing all conns: ActiveConnections=%d totalConns=%d, want 0/0",
			m.ActiveConnections, pm.totalConns.Load())
	}
}

// ---------------------------------------------------------------------------
// Dialer: success, cancellation, and failure paths
// ---------------------------------------------------------------------------

// TestCreateDialer_AllowPrivateIPs verifies that the dialer permits connections
// to private IPs when AllowPrivateIPs is true.
func TestCreateDialer_AllowPrivateIPs(t *testing.T) {
	// Create a local listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// Accept connections in background
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	config := &Config{
		AllowPrivateIPs: true,
		DialTimeout:     2 * time.Second,
		KeepAlive:       30 * time.Second,
	}
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialFn := pm.createDialer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := dialFn(ctx, "tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("expected successful connection to %s, got error: %v", listener.Addr().String(), err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}

	// Verify the returned connection is a trackedConn
	tc, ok := conn.(*trackedConn)
	if !ok {
		t.Fatal("expected trackedConn wrapper")
	}

	// Verify active connection tracking
	if got := pm.activeConns.Load(); got != 1 {
		t.Errorf("activeConns = %d, want 1", got)
	}

	// Close the tracked connection
	_ = tc.Close()

	if got := pm.activeConns.Load(); got != 0 {
		t.Errorf("activeConns after close = %d, want 0", got)
	}
}

// TestCreateDialer_ContextCancellation verifies that the dialer respects
// context cancellation.
func TestCreateDialer_ContextCancellation(t *testing.T) {
	config := &Config{
		AllowPrivateIPs: true,
		DialTimeout:     5 * time.Second,
	}
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialFn := pm.createDialer()

	// Create an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = dialFn(ctx, "tcp", "192.0.2.1:12345") // RFC 5737 test address
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

// TestPoolManager_ContextCancellation verifies the dial path through a full
// client request: a slow server plus a short request deadline must surface an
// error, not hang.
func TestPoolManager_ContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)

	_, err = client.Do(req)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
}

// TestCreateDialer_ConnectionFailure verifies that connection failures are
// properly tracked in metrics when dialing an unreachable address.
func TestCreateDialer_ConnectionFailure(t *testing.T) {
	config := &Config{
		AllowPrivateIPs: true,
		DialTimeout:     50 * time.Millisecond,
	}
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialFn := pm.createDialer()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Dial a non-existent port on a non-listening address
	// Using 192.0.2.1 (TEST-NET-1, RFC 5737) which should not be routable
	_, err = dialFn(ctx, "tcp", "192.0.2.1:1")
	if err == nil {
		// Connection may unexpectedly succeed in some environments
		t.Log("Connection did not fail — skipping failure metrics check")
		return
	}

	metrics := pm.GetMetrics()
	if metrics.RejectedConnections < 1 {
		t.Errorf("RejectedConnections = %d, want at least 1", metrics.RejectedConnections)
	}
}

// ---------------------------------------------------------------------------
// Dialer: proxy-address bypass paths
// ---------------------------------------------------------------------------

// TestCreateDialer_ProxyAddr_Success covers the proxy-address bypass on the
// success path: a configured proxy host is dialed without SSRF validation and
// tracked as an active connection.
func TestCreateDialer_ProxyAddr_Success(t *testing.T) {
	// Stand up a raw TCP listener to represent a proxy server.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	proxyAddr := listener.Addr().String()

	config := &Config{
		ProxyURL:    "http://" + proxyAddr,
		DialTimeout: 5 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if !pm.isProxyAddr(proxyAddr) {
		t.Fatalf("proxyAddr %q not registered in proxyAddrs", proxyAddr)
	}

	dialer := pm.createDialer()

	// Dial the proxy address — should bypass SSRF and succeed.
	conn, err := dialer(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("Dial to proxy address failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Accept on the listener side to complete the handshake.
	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	// Verify active connection counter increased.
	metrics := pm.GetMetrics()
	if metrics.ActiveConnections < 1 {
		t.Errorf("Expected ActiveConnections >= 1, got %d", metrics.ActiveConnections)
	}
}

// TestCreateDialer_ProxyAddr_Failure covers the proxy-address bypass on the
// failure path: a dead proxy surfaces ErrProxyConnectionFailed.
func TestCreateDialer_ProxyAddr_Failure(t *testing.T) {
	// Use a port that is virtually guaranteed to be closed.
	config := &Config{
		ProxyURL:    "http://127.0.0.1:1",
		DialTimeout: 500 * time.Millisecond,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialer := pm.createDialer()

	_, err = dialer(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("Expected error connecting to dead proxy")
	}
	if !errors.Is(err, ErrProxyConnectionFailed) {
		t.Errorf("Expected ErrProxyConnectionFailed, got: %v", err)
	}
}

// TestCreateDialer_ProxyPool_FailureReporting verifies that a dial failure
// against a proxy-pool host reports the failure to the pool (circuit-breaker
// feed) and counts the attempt as rejected.
func TestCreateDialer_ProxyPool_FailureReporting(t *testing.T) {
	config := &Config{
		ProxyPool: []string{
			"http://127.0.0.1:1", // dead proxy
		},
		ProxyFailureThreshold: 2,
		ProxyCooldown:         30 * time.Second,
		DialTimeout:           500 * time.Millisecond,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	dialer := pm.createDialer()

	_, err = dialer(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("Expected proxy connection failure")
	}
	if !errors.Is(err, ErrProxyConnectionFailed) {
		t.Errorf("Expected ErrProxyConnectionFailed, got: %v", err)
	}

	// Verify the proxy pool recorded the failure.
	// After ProxyFailureThreshold consecutive failures the proxy should
	// be circuit-broken. With threshold=2, one failure is not enough,
	// but the failure count should be > 0.
	metrics := pm.GetMetrics()
	if metrics.RejectedConnections < 1 {
		t.Errorf("Expected RejectedConnections >= 1, got %d", metrics.RejectedConnections)
	}
}

// TestCreateDialer_ProxyPool_SuccessReporting covers the proxy-pool success
// path: the dial succeeds against the pool host and is reported to the pool.
func TestCreateDialer_ProxyPool_SuccessReporting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	proxyAddr := listener.Addr().String()

	config := &Config{
		ProxyPool: []string{
			"http://" + proxyAddr,
		},
		DialTimeout: 5 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if !pm.isProxyAddr(proxyAddr) {
		t.Fatalf("proxyAddr %q not in proxyAddrs", proxyAddr)
	}

	dialer := pm.createDialer()

	conn, err := dialer(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("Dial to proxy pool address failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	metrics := pm.GetMetrics()
	if metrics.ActiveConnections < 1 {
		t.Errorf("Expected ActiveConnections >= 1, got %d", metrics.ActiveConnections)
	}
}

// ---------------------------------------------------------------------------
// Dialer: SSRF protection
// ---------------------------------------------------------------------------

// TestCreateDialer_SSRFRejectsPrivateIP verifies that the dialer function
// rejects connections to private IPs by calling it directly.
func TestCreateDialer_SSRFRejectsPrivateIP(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"loopback", "127.0.0.1:8080"},
		{"private_10", "10.0.0.1:80"},
		{"private_172", "172.16.0.1:80"},
		{"private_192", "192.168.1.1:80"},
		{"link_local", "169.254.1.1:80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				AllowPrivateIPs: false,
				DialTimeout:     1 * time.Second,
			}
			pm, err := NewPoolManager(config)
			if err != nil {
				t.Fatalf("NewPoolManager() error: %v", err)
			}
			defer func() { _ = pm.Close() }()

			dialFn := pm.createDialer()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			_, err = dialFn(ctx, "tcp", tt.address)
			if err == nil {
				t.Errorf("expected SSRF rejection for %s, got nil error", tt.address)
			}

			// Verify rejected connection was tracked
			metrics := pm.GetMetrics()
			if metrics.RejectedConnections < 1 {
				t.Errorf("RejectedConnections = %d, want at least 1", metrics.RejectedConnections)
			}
		})
	}
}

// TestResolveAndValidateAddress verifies address resolution and validation
// including public IPs, private IP rejection, domain resolution, and error cases.
func TestResolveAndValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
		skipOK  bool   // if true, a nil error is acceptable (skip instead of fail)
		want    string // expected result string (exact match, empty means custom validation)
	}{
		// Public IP with port
		{
			name:    "Public IP with port",
			address: "8.8.8.8:443",
			wantErr: false,
			want:    "8.8.8.8:443",
		},
		// Public IP without port: fails closed. The former fallback assumed
		// port 443 and even returned the portless address as the candidate,
		// which the dialer itself would then reject; the shared
		// ValidatedCandidates sequence errors up front instead.
		{
			name:    "Public IP without port",
			address: "8.8.8.8",
			wantErr: true,
		},
		// Private IPs are rejected
		{
			name:    "Private IP 10.x",
			address: "10.0.0.1:443",
			wantErr: true,
		},
		{
			name:    "Private IP 172.16.x",
			address: "172.16.0.1:443",
			wantErr: true,
		},
		{
			name:    "Private IP 192.168.x",
			address: "192.168.1.1:443",
			wantErr: true,
		},
		{
			name:    "Loopback 127.x",
			address: "127.0.0.1:443",
			wantErr: true,
		},
		{
			name:    "Link-local 169.254.x",
			address: "169.254.1.1:443",
			wantErr: true,
		},
		// IPv6 boundaries (IP literals — no DNS/network needed)
		{
			name:    "Public IPv6 with port",
			address: "[2001:4860:4860::8888]:443",
			wantErr: false,
		},
		{
			name:    "IPv6 loopback",
			address: "[::1]:8080",
			wantErr: true,
		},
		{
			name:    "IPv4-mapped IPv6 loopback",
			address: "[::ffff:127.0.0.1]:8080",
			wantErr: true,
		},
		{
			name:    "Private IP without port",
			address: "127.0.0.1",
			wantErr: true,
		},
		// Unresolvable domain
		{
			name:    "Domain resolution failure",
			address: "this-domain-does-not-exist-xyz123.invalid:443",
			wantErr: true,
			skipOK:  true, // DNS resolver may intercept queries
		},
		// Public domain resolution (custom validation in test body)
		{
			name:    "Public domain resolution",
			address: "public-domain",
			wantErr: false,
			want:    "", // custom validation below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPoolManager(nil)
			if err != nil {
				t.Fatalf("NewPoolManager() error: %v", err)
			}
			defer func() { _ = pm.Close() }()

			// Special handling for public domain resolution test
			if tt.address == "public-domain" {
				domains := []string{
					"one.one.one.one:443",
					"dns.google:443",
					"cloudflare.com:443",
				}

				var lastErr error
				for _, domain := range domains {
					results, resolveErr := pm.resolveAndValidateAddress(context.Background(), domain)
					if resolveErr == nil {
						if len(results) == 0 {
							t.Fatalf("expected at least one validated address for %s", domain)
						}
						host, port, splitErr := net.SplitHostPort(results[0])
						if splitErr != nil {
							t.Fatalf("result %q is not a valid host:port: %v", results[0], splitErr)
						}
						if port != "443" {
							t.Errorf("port = %q, want %q", port, "443")
						}
						if net.ParseIP(host) == nil {
							t.Errorf("host %q is not a valid IP address", host)
						}
						return
					}
					lastErr = resolveErr
				}

				t.Logf("No test domain resolved to a public IP (likely network restriction): %v", lastErr)
				return
			}

			result, err := pm.resolveAndValidateAddress(context.Background(), tt.address)

			if tt.wantErr {
				if err == nil {
					if tt.skipOK {
						t.Skip("DNS resolver intercepts queries - unresolvable domain resolved to an IP")
					}
					t.Errorf("expected error for %s, got nil", tt.address)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.want != "" && (len(result) == 0 || result[0] != tt.want) {
				t.Errorf("result = %v, want first candidate %q", result, tt.want)
			}
		})
	}
}

// TestResolveAndValidateAddress_IPLiteralReturnsCandidate verifies the
// multi-candidate return contract: an IP-literal address validates to a
// single-element slice (callers dial candidates in turn for failover).
func TestResolveAndValidateAddress_IPLiteralReturnsCandidate(t *testing.T) {
	pm, err := NewPoolManager(&Config{
		ExemptNets: mustParseCIDRs(t, "8.8.8.0/24"),
	})
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	addrs, err := pm.resolveAndValidateAddress(context.Background(), "8.8.8.8:443")
	if err != nil {
		t.Fatalf("resolveAndValidateAddress: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "8.8.8.8:443" {
		t.Errorf("IP literal should validate to one candidate, got %v", addrs)
	}
}

// TestResolveAndValidateAddress_CoverageGaps covers the DialTimeout-derived
// DNS timeout branch, the nil-context fallback, and the missing-port default.
func TestResolveAndValidateAddress_CoverageGaps(t *testing.T) {
	t.Run("DialTimeout shorter than DNS timeout", func(t *testing.T) {
		// DialTimeout=3s < dnsTimeout=10s → exercises the branch that
		// derives the DNS timeout from DialTimeout.
		config := DefaultConfig()
		config.DialTimeout = 3 * time.Second
		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// "localhost" resolves to 127.0.0.1 (loopback) → FilterAllowedIPs
		// returns empty → exercises the blocked-domain error path.
		_, err = pm.resolveAndValidateAddress(context.Background(), "localhost:443")
		if err == nil {
			t.Error("Expected error for localhost (blocked IP)")
		}
	})

	t.Run("nil context fallback", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// Passing nil context must not panic; it falls back to context.Background().
		_, err = pm.resolveAndValidateAddress(nil, "localhost:443") //nolint:staticcheck // intentionally nil to test the fallback path
		if err == nil {
			t.Error("Expected error for localhost with nil context")
		}
	})

	t.Run("portless address fails closed", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// Address without port → SplitHostPort fails → error. The former
		// fallback silently assumed port 443 (dialing the wrong service for
		// any non-TLS target); the shared ValidatedCandidates sequence fails
		// closed instead.
		_, err = pm.resolveAndValidateAddress(context.Background(), "localhost")
		if err == nil {
			t.Error("Expected error for localhost without port")
		}
	})
}

// ---------------------------------------------------------------------------
// Dialer: DoH paths
// ---------------------------------------------------------------------------

// TestCreateDialer_DoHPath_SSRFBlock verifies that the DoH path blocks
// connections to private IPs when SSRF protection is enabled.
func TestCreateDialer_DoHPath_SSRFBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DoH integration test in short mode")
	}

	config := DefaultConfig()
	config.AllowPrivateIPs = false
	config.EnableDoH = true

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if pm.dohResolver == nil {
		t.Skip("DoH resolver not available")
	}

	dialFn := pm.createDialer()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// This will go through the DoH path and should block private IPs
	_, err = dialFn(ctx, "tcp", "127.0.0.1:8080")
	if err == nil {
		t.Error("expected SSRF block for private IP through DoH path")
	} else {
		t.Logf("DoH SSRF block: %v", err)
	}
}

// TestCreateDialer_DoHResolutionFailure verifies that a failing DoH resolver
// surfaces a "DoH DNS resolution failed" error and releases the pool slot.
func TestCreateDialer_DoHResolutionFailure(t *testing.T) {
	config := &Config{
		EnableDoH:       true,
		AllowPrivateIPs: false,
		DialTimeout:     5 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Close the DoH resolver so LookupIPAddr returns an error immediately.
	if err := pm.dohResolver.Close(); err != nil {
		t.Fatalf("DoH resolver close: %v", err)
	}

	dialer := pm.createDialer()

	_, err = dialer(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("Expected error from closed DoH resolver")
	}
	if !strings.Contains(err.Error(), "DoH DNS resolution failed") {
		t.Errorf("Expected DoH resolution failure, got: %v", err)
	}

	metrics := pm.GetMetrics()
	if metrics.RejectedConnections < 1 {
		t.Errorf("Expected RejectedConnections >= 1, got %d", metrics.RejectedConnections)
	}
}

// ---------------------------------------------------------------------------
// trackedConn
// ---------------------------------------------------------------------------

// TestTrackedConn_Lifecycle verifies that trackedConn properly tracks
// active connection counts on creation, decrement on Close, and handles
// double-close without double-decrementing.
func TestTrackedConn_Lifecycle(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Simulate the state that createDialer sets up: active gauge at zero.
	pm.activeConns.Store(0)

	// Simulate what createDialer does: increment pool active conns
	pm.activeConns.Add(1)

	// Create a trackedConn wrapping a fake connection
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()

	tc := &trackedConn{
		Conn: client,
		pm:   pm,
	}

	// Verify active count before close
	if got := pm.activeConns.Load(); got != 1 {
		t.Errorf("pool activeConns before close = %d, want 1", got)
	}

	// Close the tracked connection
	err = tc.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Verify active count decremented
	if got := pm.activeConns.Load(); got != 0 {
		t.Errorf("pool activeConns after close = %d, want 0", got)
	}

	// Double close should not double-decrement
	err = tc.Close()
	if err != nil {
		t.Errorf("Second Close() error: %v", err)
	}

	if got := pm.activeConns.Load(); got != 0 {
		t.Errorf("pool activeConns after double close = %d, want 0 (no double-decrement)", got)
	}
}

// TestTrackedConn_ConcurrentClose verifies that concurrent Close calls on a
// trackedConn do not cause double-decrement of active connection counters.
func TestTrackedConn_ConcurrentClose(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	pm.activeConns.Add(1)

	server, client := net.Pipe()
	defer func() { _ = server.Close() }()

	tc := &trackedConn{
		Conn: client,
		pm:   pm,
	}

	var wg sync.WaitGroup
	const numClosers = 10
	wg.Add(numClosers)

	for i := 0; i < numClosers; i++ {
		go func() {
			defer wg.Done()
			_ = tc.Close()
		}()
	}

	wg.Wait()

	// Despite 10 Close calls, activeConns should only be decremented once
	if got := pm.activeConns.Load(); got != 0 {
		t.Errorf("pool activeConns after concurrent close = %d, want 0", got)
	}
}

// TestTrackedConn_CloseAfterPoolClosed verifies that closing a tracked
// connection whose pool was closed in between neither panics nor drives the
// aggregate counters negative.
func TestTrackedConn_CloseAfterPoolClosed(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr := listener.Addr().String()

	config := &Config{
		AllowPrivateIPs: true,
		DialTimeout:     5 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}

	dialer := pm.createDialer()

	conn, err := dialer(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	defer func() { _ = serverConn.Close() }()

	// Close the pool while a connection is still active.
	_ = pm.Close()

	// Closing the tracked connection after pool close must not panic
	// and must not drive counters negative.
	if err := conn.Close(); err != nil {
		t.Errorf("Close after pool close returned error: %v", err)
	}

	metrics := pm.GetMetrics()
	// Counters converge to zero on their own; they must never go negative.
	if metrics.ActiveConnections < 0 {
		t.Errorf("ActiveConnections should not be negative, got %d", metrics.ActiveConnections)
	}
	if metrics.TotalConnections < 0 {
		t.Errorf("TotalConnections should not be negative, got %d", metrics.TotalConnections)
	}
}

// ---------------------------------------------------------------------------
// TLS configuration
// ---------------------------------------------------------------------------

// mockCertPinner is a mock implementation of certificate pinner for testing
type mockCertPinner struct {
	shouldFail bool
}

func (m *mockCertPinner) Pin() string {
	return "mock-pinner"
}

func (m *mockCertPinner) VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if m.shouldFail {
		return fmt.Errorf("mock certificate pinning failure")
	}
	return nil
}

// recordingPinner is a CertPinner stub that records invocation.
type recordingPinner struct {
	called bool
}

func (p *recordingPinner) VerifyPeerCertificate(_ [][]byte, _ [][]*x509.Certificate) error {
	p.called = true
	return nil
}

// TestPoolManager_TLSConfig verifies the effective TLS client configuration
// for default and user-supplied configs.
func TestPoolManager_TLSConfig(t *testing.T) {
	t.Run("Default TLS config", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig

		if tlsConfig == nil {
			t.Fatal("TLS config should not be nil")
		}

		if tlsConfig.MinVersion != tls.VersionTLS12 {
			t.Errorf("Expected MinVersion TLS 1.2, got %d", tlsConfig.MinVersion)
		}

		if tlsConfig.MaxVersion != tls.VersionTLS13 {
			t.Errorf("Expected MaxVersion TLS 1.3, got %d", tlsConfig.MaxVersion)
		}

		if tlsConfig.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be false by default")
		}
	})

	t.Run("Custom TLS config", func(t *testing.T) {
		customTLS := &tls.Config{
			MinVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		}

		config := &Config{
			TLSConfig: customTLS,
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		tlsConfig := pm.transport.TLSClientConfig

		if tlsConfig.MinVersion != tls.VersionTLS13 {
			t.Errorf("Expected MinVersion TLS 1.3, got %d", tlsConfig.MinVersion)
		}

		if !tlsConfig.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be true")
		}
	})
}

// TestPoolManager_TLSConfigWithCustomCiphers verifies a user-supplied
// TLSConfig is used verbatim (custom cipher list preserved).
func TestPoolManager_TLSConfigWithCustomCiphers(t *testing.T) {
	config := &Config{
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
			},
		},
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify custom TLS config is used
	if pm.transport.TLSClientConfig == nil {
		t.Fatal("TLS config should not be nil")
	}

	if pm.transport.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected MinVersion TLS 1.3, got %d", pm.transport.TLSClientConfig.MinVersion)
	}
}

// TestPoolManager_TLSConfigWithInsecureSkipVerify verifies the
// InsecureSkipVerify flag propagates into the built TLS config.
func TestPoolManager_TLSConfigWithInsecureSkipVerify(t *testing.T) {
	config := &Config{
		InsecureSkipVerify: true,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	if !pm.transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

// TestCreateTLSConfig_Default verifies that the default TLS configuration has
// correct cipher suites, session cache, and curve preferences.
func TestCreateTLSConfig_Default(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("NewPoolManager() error: %v", err)
	}
	defer func() { _ = pm.Close() }()

	tlsConfig := pm.createTLSConfig()
	if tlsConfig == nil {
		t.Fatal("createTLSConfig() returned nil")
	}

	// Verify session cache is enabled
	if tlsConfig.SessionTicketsDisabled {
		t.Error("SessionTicketsDisabled should be false")
	}
	if tlsConfig.ClientSessionCache == nil {
		t.Error("ClientSessionCache should not be nil")
	}

	// Verify cipher suites
	wantCiphers := []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
	if len(tlsConfig.CipherSuites) != len(wantCiphers) {
		t.Errorf("CipherSuites len = %d, want %d", len(tlsConfig.CipherSuites), len(wantCiphers))
	}
	for i, want := range wantCiphers {
		if i < len(tlsConfig.CipherSuites) && tlsConfig.CipherSuites[i] != want {
			t.Errorf("CipherSuites[%d] = %d, want %d", i, tlsConfig.CipherSuites[i], want)
		}
	}

	// Verify curve preferences
	wantCurves := []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
		tls.CurveP384,
	}
	if len(tlsConfig.CurvePreferences) != len(wantCurves) {
		t.Errorf("CurvePreferences len = %d, want %d", len(tlsConfig.CurvePreferences), len(wantCurves))
	}
	for i, want := range wantCurves {
		if i < len(tlsConfig.CurvePreferences) && tlsConfig.CurvePreferences[i] != want {
			t.Errorf("CurvePreferences[%d] = %d, want %d", i, tlsConfig.CurvePreferences[i], want)
		}
	}

	// Verify renegotiation is disabled
	if tlsConfig.Renegotiation != tls.RenegotiateNever {
		t.Errorf("Renegotiation = %d, want RenegotiateNever", tlsConfig.Renegotiation)
	}
}

// TestCreateTLSConfig_ChainsUserVerifyCallback verifies certificate pinning
// chains onto a user-supplied VerifyPeerCertificate callback instead of
// silently replacing it (custom CA validation / mTLS hooks keep working).
func TestCreateTLSConfig_ChainsUserVerifyCallback(t *testing.T) {
	userCalled := false
	cfg := &Config{
		TLSConfig: &tls.Config{
			VerifyPeerCertificate: func(_ [][]byte, _ [][]*x509.Certificate) error {
				userCalled = true
				return nil
			},
		},
		CertPinner: &recordingPinner{},
	}
	pm := &PoolManager{config: cfg}

	tlsCfg := pm.createTLSConfig()
	if tlsCfg.VerifyPeerCertificate == nil {
		t.Fatal("VerifyPeerCertificate not set")
	}
	if err := tlsCfg.VerifyPeerCertificate(nil, nil); err != nil {
		t.Fatalf("chained callback error: %v", err)
	}
	if !userCalled {
		t.Error("user VerifyPeerCertificate was not invoked")
	}
	if !cfg.CertPinner.(*recordingPinner).called {
		t.Error("cert pinner was not invoked")
	}
}

// TestCreateVerifyPeerCertificate verifies the certificate verification
// callback across pinner success, pinner failure, and InsecureSkipVerify paths.
func TestCreateVerifyPeerCertificate(t *testing.T) {
	tests := []struct {
		name               string
		shouldFail         bool
		insecureSkipVerify bool
		wantErr            bool
	}{
		{"pinner accepts certificate", false, false, false},
		{"pinner rejects certificate", true, false, true},
		{"InsecureSkipVerify with accepting pinner", false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				CertPinner: &mockCertPinner{shouldFail: tt.shouldFail},
			}
			if tt.insecureSkipVerify {
				config.TLSConfig = &tls.Config{InsecureSkipVerify: true}
			}

			pm, err := NewPoolManager(config)
			if err != nil {
				t.Fatalf("NewPoolManager() error: %v", err)
			}
			defer func() { _ = pm.Close() }()

			verifyFn := pm.transport.TLSClientConfig.VerifyPeerCertificate
			if verifyFn == nil {
				t.Fatal("VerifyPeerCertificate should not be nil")
			}

			err = verifyFn(nil, nil)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Proxied-target SSRF guard
// ---------------------------------------------------------------------------

// TestSSRFGuardProxyFunc_DirectWhenNoProxy verifies that a nil proxyFunc
// yields a nil guard (no wrapping for the direct-connection path).
func TestSSRFGuardProxyFunc_DirectWhenNoProxy(t *testing.T) {
	pm := &PoolManager{config: &Config{}}
	if got := pm.ssrfGuardProxyFunc(nil); got != nil {
		t.Fatal("ssrfGuardProxyFunc(nil) = non-nil, want nil")
	}
}

// TestSSRFGuardProxyFunc_BlockedHostname verifies that a proxied request whose
// target hostname resolves to a private/link-local address is rejected before
// the proxy carries it — the deterministic SSRF bypass this guard exists to
// close. "localhost" resolves to 127.0.0.1 (loopback) on every platform, so the
// test is deterministic and needs no network.
func TestSSRFGuardProxyFunc_BlockedHostname(t *testing.T) {
	pm := &PoolManager{config: &Config{AllowPrivateIPs: false}}

	proxy := &url.URL{Scheme: "http", Host: "proxy.example:8080"}
	guard := pm.ssrfGuardProxyFunc(func(*http.Request) (*url.URL, error) {
		return proxy, nil
	})

	req := &http.Request{
		URL:    &url.URL{Scheme: "http", Host: "localhost"},
		Header: make(http.Header),
	}
	req = req.WithContext(context.Background())

	got, err := guard(req)
	if err == nil {
		t.Fatalf("expected SSRF rejection for hostname resolving to loopback, got proxy %v", got)
	}
	if got != nil {
		t.Errorf("guard returned a proxy on rejection: %v", got)
	}
	if !strings.Contains(err.Error(), "SSRF protection") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSSRFGuardProxyFunc_BlockedIPLiteral verifies that a proxied request to a
// literal private IP is rejected without DNS.
func TestSSRFGuardProxyFunc_BlockedIPLiteral(t *testing.T) {
	pm := &PoolManager{config: &Config{AllowPrivateIPs: false}}
	proxy := &url.URL{Scheme: "http", Host: "proxy.example:8080"}
	guard := pm.ssrfGuardProxyFunc(func(*http.Request) (*url.URL, error) {
		return proxy, nil
	})

	req := &http.Request{
		URL:    &url.URL{Scheme: "http", Host: "10.0.0.1"},
		Header: make(http.Header),
	}
	req = req.WithContext(context.Background())

	if _, err := guard(req); err == nil {
		t.Fatal("expected SSRF rejection for literal private IP, got nil")
	}
}

// TestSSRFGuardProxyFunc_AllowPrivateIPs verifies the guard is a pass-through
// when private-IP blocking is disabled (client-level AllowPrivateIPs=true).
func TestSSRFGuardProxyFunc_AllowPrivateIPs(t *testing.T) {
	pm := &PoolManager{config: &Config{AllowPrivateIPs: true}}
	proxy := &url.URL{Scheme: "http", Host: "proxy.example:8080"}
	guard := pm.ssrfGuardProxyFunc(func(*http.Request) (*url.URL, error) {
		return proxy, nil
	})

	req := &http.Request{
		URL:    &url.URL{Scheme: "http", Host: "localhost"},
		Header: make(http.Header),
	}
	req = req.WithContext(context.Background())

	got, err := guard(req)
	if err != nil {
		t.Fatalf("AllowPrivateIPs=true must skip the guard, got error: %v", err)
	}
	if got != proxy {
		t.Errorf("got %v, want the configured proxy", got)
	}
}

// TestSSRFGuardProxyFunc_PerRequestOverride verifies a per-request
// AllowPrivateIPs override (WithAllowPrivateIPs) takes precedence over the
// client-level policy, allowing a proxied request to a blocked host.
func TestSSRFGuardProxyFunc_PerRequestOverride(t *testing.T) {
	pm := &PoolManager{config: &Config{AllowPrivateIPs: false}}
	proxy := &url.URL{Scheme: "http", Host: "proxy.example:8080"}
	guard := pm.ssrfGuardProxyFunc(func(*http.Request) (*url.URL, error) {
		return proxy, nil
	})

	ctx := WithAllowPrivateIPsOverride(context.Background(), true)
	req := &http.Request{
		URL:    &url.URL{Scheme: "http", Host: "localhost"},
		Header: make(http.Header),
	}
	req = req.WithContext(ctx)

	got, err := guard(req)
	if err != nil {
		t.Fatalf("per-request override=true must allow the request, got error: %v", err)
	}
	if got != proxy {
		t.Errorf("got %v, want the configured proxy", got)
	}
}

// TestSSRFGuardProxyFunc_PropagatesProxyError verifies that an error from the
// underlying proxy selector is returned unchanged (no SSRF check on the failure
// path).
func TestSSRFGuardProxyFunc_PropagatesProxyError(t *testing.T) {
	pm := &PoolManager{config: &Config{AllowPrivateIPs: false}}
	wantErr := errors.New("no proxy available")
	guard := pm.ssrfGuardProxyFunc(func(*http.Request) (*url.URL, error) {
		return nil, wantErr
	})

	req := &http.Request{
		URL:    &url.URL{Scheme: "http", Host: "example.com"},
		Header: make(http.Header),
	}
	req = req.WithContext(context.Background())

	if _, err := guard(req); !errors.Is(err, wantErr) {
		t.Errorf("got %v, want the underlying proxy error", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mustParseCIDRs parses CIDR strings for tests.
func mustParseCIDRs(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("ParseCIDR(%q): %v", c, err)
		}
		nets = append(nets, n)
	}
	return nets
}
