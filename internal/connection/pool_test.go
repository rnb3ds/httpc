package connection

// ============================================================================
// CONNECTION POOL MANAGER UNIT TESTS
//
// PoolManager lifecycle, configuration defaults, and proxy selection
// (single URL / proxy pool / system proxy). Dialer, TLS, and SSRF tests live
// in dialer_test.go; context-helper tests in context_helpers_test.go.
// ============================================================================

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/proxypool"
)

// countingListener wraps a listener and counts accepted connections so tests
// can assert how many TCP connections were actually dialed (idle-connection
// reuse, per-host admission caps).
type countingListener struct {
	net.Listener
	accepted atomic.Int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted.Add(1)
	}
	return c, err
}

// newCountingServer starts an HTTP server on a fresh loopback listener whose
// accepted-connection count is tracked by the returned countingListener. The
// server is closed automatically when the test ends.
func newCountingServer(t *testing.T, handler http.Handler) *countingListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	cl := &countingListener{Listener: ln}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(cl) }()
	t.Cleanup(func() { _ = srv.Close() })
	return cl
}

// ============================================================================
// Construction & Lifecycle
// ============================================================================

func TestPoolManager_New(t *testing.T) {
	t.Run("With default config", func(t *testing.T) {
		pm, err := NewPoolManager(nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.config == nil {
			t.Error("Config should not be nil")
		}

		if pm.transport == nil {
			t.Error("Transport should not be nil")
		}

	})

	t.Run("With custom config", func(t *testing.T) {
		config := &Config{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     25,
			DialTimeout:         5 * time.Second,
			EnableHTTP2:         true,
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.config.MaxIdleConns != 100 {
			t.Errorf("Expected MaxIdleConns 100, got %d", pm.config.MaxIdleConns)
		}

		if pm.transport.MaxIdleConns != 100 {
			t.Errorf("Expected transport MaxIdleConns 100, got %d", pm.transport.MaxIdleConns)
		}
	})

	t.Run("With proxy URL", func(t *testing.T) {
		config := &Config{
			ProxyURL: "http://proxy.example.com:8080",
		}

		pm, err := NewPoolManager(config)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.transport.Proxy == nil {
			t.Error("Proxy should be configured")
		}
	})

	// Invalid ProxyURL rejection is table-driven in TestNewPoolManager_ProxyURLValidation.
}

func TestPoolManager_GetTransport(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.GetTransport()

	if transport == nil {
		t.Fatal("Transport should not be nil")
	}

	if transport != pm.transport {
		t.Error("GetTransport should return the same transport instance")
	}
}

func TestPoolManager_Close(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	err = pm.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got: %v", err)
	}

	// Close again should be idempotent
	err = pm.Close()
	if err != nil {
		t.Errorf("Expected no error on double close, got: %v", err)
	}
}

// TestPoolManager_CloseWithDoHResolver verifies that Close also releases the
// DoH resolver's HTTP client resources without error.
func TestPoolManager_CloseWithDoHResolver(t *testing.T) {
	config := &Config{
		EnableDoH:   true,
		DoHCacheTTL: 1 * time.Minute,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}

	if pm.dohResolver == nil {
		t.Fatal("DoH resolver should be initialized")
	}

	// Close should exercise the DoH resolver close path without error.
	if err := pm.Close(); err != nil {
		t.Errorf("Close with DoH resolver returned error: %v", err)
	}
}

// ============================================================================
// Configuration defaults (single table pinning every DefaultConfig field)
// ============================================================================

// TestDefaultConfig pins every field of DefaultConfig, folding in the former
// TestDefaultConfigProxySettings (no proxy by default) and
// TestConfig_CertPinnerDefaultNil (no pinner by default).
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		// Pool sizing
		{"MaxIdleConns", config.MaxIdleConns, 200},
		{"MaxIdleConnsPerHost", config.MaxIdleConnsPerHost, 20},
		{"MaxConnsPerHost", config.MaxConnsPerHost, 50},
		{"MaxTotalConns", config.MaxTotalConns, 1000},

		// Timeouts
		{"DialTimeout", config.DialTimeout, 10 * time.Second},
		{"KeepAlive", config.KeepAlive, 30 * time.Second},
		{"TLSHandshakeTimeout", config.TLSHandshakeTimeout, 10 * time.Second},
		{"ResponseHeaderTimeout", config.ResponseHeaderTimeout, 30 * time.Second},
		{"IdleConnTimeout", config.IdleConnTimeout, 90 * time.Second},
		{"ExpectContinueTimeout", config.ExpectContinueTimeout, 1 * time.Second},
		{"MaxResponseHeaderBytes", config.MaxResponseHeaderBytes, int64(0)},

		// TLS
		{"MinTLSVersion", config.MinTLSVersion, uint16(tls.VersionTLS12)},
		{"MaxTLSVersion", config.MaxTLSVersion, uint16(tls.VersionTLS13)},
		{"InsecureSkipVerify", config.InsecureSkipVerify, false},
		{"TLSConfig", config.TLSConfig, (*tls.Config)(nil)},

		// Protocol
		{"EnableHTTP2", config.EnableHTTP2, true},
		{"ForceAttemptHTTP2", config.ForceAttemptHTTP2, true},
		{"DisableKeepAlives", config.DisableKeepAlives, false},

		// Proxy: no proxy of any kind by default (backward compatibility).
		{"ProxyURL", config.ProxyURL, ""},
		{"EnableSystemProxy", config.EnableSystemProxy, false},
		{"ProxyPool", config.ProxyPool, []string(nil)},
		{"ProxyPoolStrategy", config.ProxyPoolStrategy, proxypool.StrategyRoundRobin},
		{"ProxyFailureThreshold", config.ProxyFailureThreshold, 0},
		{"ProxyCooldown", config.ProxyCooldown, time.Duration(0)},
		{"EnableProxyPoolHTTP2", config.EnableProxyPoolHTTP2, false},

		// Security
		{"AllowPrivateIPs", config.AllowPrivateIPs, false},
		{"ExemptNets", config.ExemptNets, []*net.IPNet(nil)},
		{"CertPinner", config.CertPinner, CertPinner(nil)},

		// Cookies / DNS
		{"CookieJar", config.CookieJar, http.CookieJar(nil)},
		{"EnableDoH", config.EnableDoH, false},
		{"DoHCacheTTL", config.DoHCacheTTL, time.Duration(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("DefaultConfig().%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ============================================================================
// Config propagation into the transport
// ============================================================================

func TestPoolManager_Timeouts(t *testing.T) {
	config := &Config{
		DialTimeout:           2 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		IdleConnTimeout:       5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.transport

	if transport.TLSHandshakeTimeout != 3*time.Second {
		t.Errorf("Expected TLSHandshakeTimeout 3s, got %v", transport.TLSHandshakeTimeout)
	}

	if transport.ResponseHeaderTimeout != 4*time.Second {
		t.Errorf("Expected ResponseHeaderTimeout 4s, got %v", transport.ResponseHeaderTimeout)
	}

	if transport.IdleConnTimeout != 5*time.Second {
		t.Errorf("Expected IdleConnTimeout 5s, got %v", transport.IdleConnTimeout)
	}

	if transport.ExpectContinueTimeout != 1*time.Second {
		t.Errorf("Expected ExpectContinueTimeout 1s, got %v", transport.ExpectContinueTimeout)
	}
}

func TestPoolManager_ConnectionLimits(t *testing.T) {
	tests := []struct {
		name                string
		maxIdleConns        int
		maxIdleConnsPerHost int
		maxConnsPerHost     int
	}{
		{
			name:                "Standard limits",
			maxIdleConns:        50,
			maxIdleConnsPerHost: 5,
			maxConnsPerHost:     10,
		},
		{
			name:                "High limits",
			maxIdleConns:        1000,
			maxIdleConnsPerHost: 100,
			maxConnsPerHost:     200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				MaxIdleConns:        tt.maxIdleConns,
				MaxIdleConnsPerHost: tt.maxIdleConnsPerHost,
				MaxConnsPerHost:     tt.maxConnsPerHost,
			}

			pm, err := NewPoolManager(config)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			defer func() { _ = pm.Close() }()

			transport := pm.transport

			if transport.MaxIdleConns != tt.maxIdleConns {
				t.Errorf("Expected MaxIdleConns %d, got %d", tt.maxIdleConns, transport.MaxIdleConns)
			}

			if transport.MaxIdleConnsPerHost != tt.maxIdleConnsPerHost {
				t.Errorf("Expected MaxIdleConnsPerHost %d, got %d", tt.maxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
			}

			if transport.MaxConnsPerHost != tt.maxConnsPerHost {
				t.Errorf("Expected MaxConnsPerHost %d, got %d", tt.maxConnsPerHost, transport.MaxConnsPerHost)
			}
		})
	}
}

func TestPoolManager_HTTP2Disabled(t *testing.T) {
	config := &Config{
		EnableHTTP2: false,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// When HTTP/2 is disabled, ForceAttemptHTTP2 should be false
	if pm.transport.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be false when HTTP/2 is disabled")
	}
}

func TestPoolManager_WithDoHResolver(t *testing.T) {
	config := &Config{
		AllowPrivateIPs: true,
		EnableDoH:       true,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify DoH resolver is initialized
	if pm.dohResolver == nil {
		t.Error("DoH resolver should be initialized when EnableDoH is true")
	}
}

func TestPoolManager_WithoutDoHResolver(t *testing.T) {
	config := &Config{
		AllowPrivateIPs: true,
		EnableDoH:       false,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Verify DoH resolver is NOT initialized
	if pm.dohResolver != nil {
		t.Error("DoH resolver should not be initialized when EnableDoH is false")
	}
}

// ============================================================================
// Proxy URL validation (single-URL and pool entries, one table)
// ============================================================================

// TestNewPoolManager_ProxyURLValidation merges the former
// TestNewPoolManager_InvalidProxyURL, TestValidProxyURLs, and
// TestPoolManager_ProxyPool_InvalidURL: both configuration surfaces validate
// proxy URLs with the same rules (http/https and socks5/socks5h, host
// required). One invalid pool entry rejects the whole construction.
func TestNewPoolManager_ProxyURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		pool    bool   // true: configure as a proxy-pool entry behind a valid lead entry; false: as ProxyURL
		proxy   string // ProxyURL mode: the URL under test
		entry   string // pool mode: the entry appended after the valid lead entry
		wantErr string // empty means construction must succeed
	}{
		// Single ProxyURL — accepted schemes and forms.
		{"accepts http", false, "http://proxy.example.com:8080", "", ""},
		{"accepts https", false, "https://proxy.example.com:8443", "", ""},
		{"accepts nonstandard port", false, "http://proxy.example.com:7890", "", ""},
		{"accepts other http host", false, "http://proxy2.example.com:8080", "", ""},
		{"accepts socks5", false, "socks5://proxy.example.com:1080", "", ""},
		{"accepts socks5h", false, "socks5h://proxy.example.com:1080", "", ""},
		// Single ProxyURL — rejected forms.
		{"rejects missing host", false, "http://", "", "missing host"},
		{"rejects ftp scheme", false, "ftp://proxy.com", "", "unsupported proxy URL scheme"},
		// socks5 is valid; socks4 is not (regression guard for the scheme set).
		{"rejects socks4", false, "socks4://proxy.com", "", "unsupported proxy URL scheme"},
		{"rejects unparseable URL", false, "://", "", "invalid proxy URL"},
		// Proxy pool entries are validated with the same rules: one invalid
		// entry (after a valid one, proving the rejection is entry-specific)
		// rejects the whole construction.
		{"pool rejects ftp entry", true, "", "ftp://invalid.example.com:8080", "unsupported proxy URL scheme"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var config *Config
			if tc.pool {
				config = &Config{
					ProxyPool: []string{
						"http://valid.example.com:8080",
						tc.entry,
					},
					DialTimeout: 10 * time.Second,
				}
			} else {
				config = &Config{
					ProxyURL:          tc.proxy,
					EnableSystemProxy: false,
					DialTimeout:       10 * time.Second,
				}
			}

			pm, err := NewPoolManager(config)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid proxy URL rejected: %v", err)
				}
				_ = pm.Close()
				return
			}

			if err == nil {
				_ = pm.Close()
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// ============================================================================
// Proxy configuration priority and selection
// ============================================================================

// TestProxyConfigurationPriority tests that proxy configuration follows the correct priority:
// 1. Manual ProxyURL (highest priority)
// 2. EnableSystemProxy (auto-detect)
// 3. Direct connection (no proxy)
func TestProxyConfigurationPriority(t *testing.T) {
	// Note: the pure "EnableSystemProxy with no manual URL" case is intentionally
	// omitted — its outcome depends on the host's OS-level proxy config (Windows
	// registry / macOS system settings), which cannot be asserted deterministically.
	// (The former TestPoolManager_SystemProxy only asserted transport creation,
	// true of every construction, so it was removed.) The cases below are all
	// deterministic.
	tests := []struct {
		name              string
		proxyURL          string
		enableSystemProxy bool
		expectProxySet    bool
		description       string
	}{
		{
			name:              "Manual proxy only",
			proxyURL:          "http://proxy.example.com:8080",
			enableSystemProxy: false,
			expectProxySet:    true,
			description:       "Manual proxy URL should be used",
		},
		{
			name:              "Direct connection (default)",
			enableSystemProxy: false,
			expectProxySet:    false,
			description:       "No proxy should be configured",
		},
		{
			name:              "Manual proxy overrides system proxy",
			proxyURL:          "http://proxy.example.com:8888",
			enableSystemProxy: true,
			expectProxySet:    true,
			description:       "Manual proxy should take priority over system proxy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				ProxyURL:          tt.proxyURL,
				EnableSystemProxy: tt.enableSystemProxy,
				DialTimeout:       10 * time.Second,
				KeepAlive:         30 * time.Second,
			}

			pm, err := NewPoolManager(config)
			if err != nil {
				t.Fatalf("NewPoolManager() failed: %v", err)
			}
			defer func() { _ = pm.Close() }()

			transport := pm.GetTransport()
			if transport == nil {
				t.Fatal("GetTransport() returned nil")
			}

			proxyFunc := transport.Proxy
			hasProxy := proxyFunc != nil

			// Assert in both directions: a proxy must be set exactly when expected.
			if tt.expectProxySet && !hasProxy {
				t.Errorf("%s: expected proxy to be set, but it was nil", tt.description)
			}
			if !tt.expectProxySet && hasProxy {
				t.Errorf("%s: expected no proxy, but one was configured", tt.description)
			}

			// When a manual proxy is configured, verify it resolves to that URL.
			if tt.proxyURL != "" && hasProxy {
				testURL, _ := url.Parse("https://www.example.com")
				testReq := &http.Request{URL: testURL}
				resolved, err := proxyFunc(testReq)
				if err != nil {
					t.Errorf("Proxy function returned error: %v", err)
					return
				}
				expectedURL, _ := url.Parse(tt.proxyURL)
				if resolved == nil || resolved.String() != expectedURL.String() {
					t.Errorf("Expected proxy URL %s, got %v", expectedURL.String(), resolved)
				}
			}
		})
	}
}

// TestPoolManager_ProxyPool verifies that a PoolManager with a proxy pool
// configures transport.Proxy and rotates through the pool's proxies.
func TestPoolManager_ProxyPool(t *testing.T) {
	config := &Config{
		ProxyPool: []string{
			"http://proxy1.example.com:8080",
			"http://proxy2.example.com:8080",
			"http://proxy3.example.com:8080",
		},
		ProxyPoolStrategy: proxypool.StrategyRoundRobin,
		DialTimeout:       10 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() failed: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.GetTransport()
	if transport.Proxy == nil {
		t.Fatal("transport.Proxy should be set for a proxy pool")
	}

	if pm.proxyPool == nil {
		t.Fatal("pm.proxyPool should be initialized")
	}

	// Round-robin should cycle through all three proxies
	testReq := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		proxyURL, err := transport.Proxy(testReq)
		if err != nil {
			t.Fatalf("Proxy() error: %v", err)
		}
		seen[proxyURL.Host] = true
	}
	if len(seen) != 3 {
		t.Errorf("round-robin visited %d distinct proxies, want 3 (got: %v)", len(seen), seen)
	}
}

// TestPoolManager_ProxyPool_AllHostsExempted verifies that every proxy host in
// the pool is seeded into proxyAddrs so SSRF validation bypasses them.
func TestPoolManager_ProxyPool_AllHostsExempted(t *testing.T) {
	config := &Config{
		ProxyPool: []string{
			"http://proxy1.example.com:8080",
			"http://proxy2.example.com:8080",
			"socks5://proxy3.example.com:1080",
		},
		DialTimeout: 10 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() failed: %v", err)
	}
	defer func() { _ = pm.Close() }()

	for _, want := range []string{
		"proxy1.example.com:8080",
		"proxy2.example.com:8080",
		"proxy3.example.com:1080",
	} {
		if !pm.isProxyAddr(want) {
			t.Errorf("proxy host %q not in proxyAddrs (SSRF would block it)", want)
		}
	}
}

// TestPoolManager_ProxyPool_PriorityOverSystemProxy verifies that ProxyPool
// takes priority over EnableSystemProxy.
func TestPoolManager_ProxyPool_PriorityOverSystemProxy(t *testing.T) {
	config := &Config{
		ProxyPool: []string{
			"http://pool-proxy.example.com:8080",
		},
		EnableSystemProxy: true,
		DialTimeout:       10 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() failed: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Proxy pool should be active, not system proxy
	if pm.proxyPool == nil {
		t.Error("proxy pool should be configured (takes priority over system proxy)")
	}

	testReq := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	got, err := pm.GetTransport().Proxy(testReq)
	if err != nil {
		t.Fatalf("Proxy() error: %v", err)
	}
	if got.Host != "pool-proxy.example.com:8080" {
		t.Errorf("Proxy() returned %s, want pool-proxy.example.com:8080", got.Host)
	}
}

// TestPoolManager_ProxyURL_OverridesProxyPool verifies that a single ProxyURL
// wins over ProxyPool.
func TestPoolManager_ProxyURL_OverridesProxyPool(t *testing.T) {
	config := &Config{
		ProxyURL: "http://single-proxy.example.com:8080",
		ProxyPool: []string{
			"http://pool-proxy.example.com:8080",
			"http://pool-proxy2.example.com:8080",
		},
		DialTimeout: 10 * time.Second,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager() failed: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Single ProxyURL should win — proxy pool not created
	if pm.proxyPool != nil {
		t.Error("proxy pool should NOT be created when ProxyURL is set (ProxyURL wins)")
	}

	testReq := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com"}}
	got, err := pm.GetTransport().Proxy(testReq)
	if err != nil {
		t.Fatalf("Proxy() error: %v", err)
	}
	if got.Host != "single-proxy.example.com:8080" {
		t.Errorf("Proxy() returned %s, want single-proxy.example.com:8080", got.Host)
	}
}

// TestProxyPool_TransportProxy_SelectIndex covers the per-attempt index path
// of transport.Proxy: WithProxyAttempt on the request context routes selection
// through deterministic SelectIndex instead of round-robin.
func TestProxyPool_TransportProxy_SelectIndex(t *testing.T) {
	config := &Config{
		ProxyPool: []string{
			"http://proxy0.example.com:8080",
			"http://proxy1.example.com:8080",
			"http://proxy2.example.com:8080",
		},
		ProxyPoolStrategy: proxypool.StrategyRoundRobin,
	}

	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	transport := pm.GetTransport()
	if transport.Proxy == nil {
		t.Fatal("transport.Proxy should be set for proxy pool")
	}

	// Without ProxyAttempt on context → normal round-robin Select.
	reqNormal := &http.Request{
		URL: &url.URL{Scheme: "https", Host: "example.com"},
	}
	got, err := transport.Proxy(reqNormal.WithContext(context.Background()))
	if err != nil {
		t.Fatalf("Proxy() error: %v", err)
	}
	if got == nil {
		t.Fatal("Proxy() returned nil for normal request")
	}

	// With ProxyAttempt=1 on context → SelectIndex(1) should return proxy at index 1.
	reqWithAttempt := &http.Request{
		URL: &url.URL{Scheme: "https", Host: "example.com"},
	}
	ctx := WithProxyAttempt(context.Background(), 1)
	got, err = transport.Proxy(reqWithAttempt.WithContext(ctx))
	if err != nil {
		t.Fatalf("Proxy() with attempt error: %v", err)
	}
	if got.Host != "proxy1.example.com:8080" {
		t.Errorf("SelectIndex(1) returned %s, want proxy1.example.com:8080", got.Host)
	}

	// With ProxyAttempt=2 → SelectIndex(2).
	reqWithAttempt2 := &http.Request{
		URL: &url.URL{Scheme: "https", Host: "example.com"},
	}
	ctx2 := WithProxyAttempt(context.Background(), 2)
	got, err = transport.Proxy(reqWithAttempt2.WithContext(ctx2))
	if err != nil {
		t.Fatalf("Proxy() with attempt=2 error: %v", err)
	}
	if got.Host != "proxy2.example.com:8080" {
		t.Errorf("SelectIndex(2) returned %s, want proxy2.example.com:8080", got.Host)
	}
}

// TestPoolManager_ProxyCallbackRecords verifies the transport's Proxy callback
// records its selection into the per-request ProxyRecorder for all configured
// proxy modes (pool deterministic, pool round-robin, single static URL), and
// that it tolerates a missing recorder (direct-connection wiring).
func TestPoolManager_ProxyCallbackRecords(t *testing.T) {
	newReq := func(ctx context.Context) *http.Request {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext: %v", err)
		}
		return req
	}

	poolCfg := &Config{
		ProxyPool: []string{
			"http://proxy1.example.com:8080",
			"http://proxy2.example.com:8080",
			"http://proxy3.example.com:8080",
		},
	}

	t.Run("pool deterministic path", func(t *testing.T) {
		pm, err := NewPoolManager(poolCfg)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		rec := &ProxyRecorder{}
		ctx := WithProxyRecorder(WithProxyAttempt(context.Background(), 0), rec)
		u, err := pm.transport.Proxy(newReq(ctx))
		if err != nil {
			t.Fatalf("Proxy: %v", err)
		}
		// SelectIndex(0) with all circuits closed returns the first entry.
		if want := "http://proxy1.example.com:8080"; u.String() != want || rec.Last() != want {
			t.Errorf("proxy = %q, recorded = %q, want %q", u.String(), rec.Last(), want)
		}
	})

	t.Run("pool round-robin path", func(t *testing.T) {
		pm, err := NewPoolManager(poolCfg)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		rec := &ProxyRecorder{}
		// No attempt index on ctx — falls back to round-robin Select.
		ctx := WithProxyRecorder(context.Background(), rec)
		u, err := pm.transport.Proxy(newReq(ctx))
		if err != nil {
			t.Fatalf("Proxy: %v", err)
		}
		if rec.Last() != u.String() {
			t.Errorf("recorded %q, want the returned proxy %q", rec.Last(), u.String())
		}
	})

	t.Run("single ProxyURL path", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{ProxyURL: "http://static.example.com:3128"})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		rec := &ProxyRecorder{}
		ctx := WithProxyRecorder(context.Background(), rec)
		u, err := pm.transport.Proxy(newReq(ctx))
		if err != nil {
			t.Fatalf("Proxy: %v", err)
		}
		if want := "http://static.example.com:3128"; u.String() != want || rec.Last() != want {
			t.Errorf("proxy = %q, recorded = %q, want %q", u.String(), rec.Last(), want)
		}
	})

	t.Run("missing recorder does not panic", func(t *testing.T) {
		pm, err := NewPoolManager(poolCfg)
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if _, err := pm.transport.Proxy(newReq(context.Background())); err != nil {
			t.Fatalf("Proxy without recorder: %v", err)
		}
	})
}

// TestPoolManager_HasProxy verifies HasProxy reflects the configured proxy
// mode so the engine attaches a recorder only when a proxy may be selected.
func TestPoolManager_HasProxy(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"default (direct)", nil, false},
		{"system proxy only", &Config{EnableSystemProxy: true}, false},
		{"single ProxyURL", &Config{ProxyURL: "http://static.example.com:3128"}, true},
		{"proxy pool", &Config{ProxyPool: []string{"http://proxy1.example.com:8080"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPoolManager(tt.cfg)
			if err != nil {
				t.Fatalf("NewPoolManager: %v", err)
			}
			defer func() { _ = pm.Close() }()
			if got := pm.HasProxy(); got != tt.want {
				t.Errorf("HasProxy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPoolManager_SelectProxy covers SelectProxy — the exported bridge the
// fingerprint adapter uses to apply the same proxy-selection logic as the
// standard transport.
func TestPoolManager_SelectProxy(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var pm *PoolManager
		got, err := pm.SelectProxy(context.Background(), &url.URL{Scheme: "https", Host: "example.com"})
		if got != nil || err != nil {
			t.Errorf("SelectProxy on nil receiver = (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("nil proxyFunc", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		// No ProxyURL / ProxyPool / system proxy → proxyFunc is nil.
		if pm.proxyFunc != nil {
			t.Fatal("expected nil proxyFunc for direct-connection config")
		}
		got, err := pm.SelectProxy(context.Background(), &url.URL{Scheme: "https", Host: "example.com"})
		if got != nil || err != nil {
			t.Errorf("SelectProxy with nil proxyFunc = (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("nil target", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{ProxyURL: "http://proxy.example.com:8080"})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		got, err := pm.SelectProxy(context.Background(), nil)
		if got != nil || err != nil {
			t.Errorf("SelectProxy with nil target = (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("static proxy returned", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{ProxyURL: "http://proxy.example.com:8080"})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		got, err := pm.SelectProxy(context.Background(), &url.URL{Scheme: "https", Host: "example.com"})
		if err != nil {
			t.Fatalf("SelectProxy: %v", err)
		}
		if got == nil || got.Host != "proxy.example.com:8080" {
			t.Errorf("SelectProxy returned %v, want proxy.example.com:8080", got)
		}
	})

	t.Run("proxy pool honored", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{
			ProxyPool:         []string{"http://p0.example.com:8080", "http://p1.example.com:8080"},
			ProxyPoolStrategy: proxypool.StrategyRoundRobin,
		})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		got, err := pm.SelectProxy(context.Background(), &url.URL{Scheme: "https", Host: "example.com"})
		if err != nil {
			t.Fatalf("SelectProxy: %v", err)
		}
		if got == nil || got.Host != "p0.example.com:8080" {
			t.Errorf("SelectProxy returned %v, want p0.example.com:8080", got)
		}

		// Per-attempt index on the context routes through SelectIndex.
		ctx := WithProxyAttempt(context.Background(), 1)
		got, err = pm.SelectProxy(ctx, &url.URL{Scheme: "https", Host: "example.com"})
		if err != nil {
			t.Fatalf("SelectProxy with attempt: %v", err)
		}
		if got == nil || got.Host != "p1.example.com:8080" {
			t.Errorf("SelectProxy with attempt=1 returned %v, want p1.example.com:8080", got)
		}
	})
}

// TestPoolManager_ReportProxy covers the ReportProxyFailure/ReportProxySuccess
// forwarding bridge: nil-safe, and recorded against the proxypool when present.
func TestPoolManager_ReportProxy(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var pm *PoolManager
		pm.ReportProxyFailure("127.0.0.1:8080") // must not panic
		pm.ReportProxySuccess("127.0.0.1:8080") // must not panic
	})

	t.Run("no proxy pool", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		pm.ReportProxyFailure("127.0.0.1:8080") // must not panic
		pm.ReportProxySuccess("127.0.0.1:8080") // must not panic
	})

	t.Run("reports to proxy pool", func(t *testing.T) {
		pm, err := NewPoolManager(&Config{
			ProxyPool:             []string{"http://p0.example.com:8080"},
			ProxyPoolStrategy:     proxypool.StrategyRoundRobin,
			ProxyFailureThreshold: 2,
		})
		if err != nil {
			t.Fatalf("NewPoolManager: %v", err)
		}
		defer func() { _ = pm.Close() }()

		if pm.proxyPool == nil {
			t.Fatal("expected proxyPool to be configured")
		}

		// Unknown host is a no-op (ReportFailure/ReportSuccess skip it).
		pm.ReportProxyFailure("unknown.example.com:8080")
		pm.ReportProxySuccess("unknown.example.com:8080")

		// A known host advances/clears the circuit state without error.
		pm.ReportProxyFailure("p0.example.com:8080")
		pm.ReportProxySuccess("p0.example.com:8080")
	})
}

// ============================================================================
// HTTP round trips, metrics, and connection admission behavior
// ============================================================================

func TestPoolManager_HTTPRequest(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true // Allow localhost for testing
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	// Create HTTP client with our pool manager
	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestPoolManager_GetMetrics(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	t.Run("zero state", func(t *testing.T) {
		metrics := pm.GetMetrics()

		// Initially should have zero connections
		if metrics.ActiveConnections != 0 {
			t.Errorf("Expected 0 active connections, got %d", metrics.ActiveConnections)
		}

		if metrics.TotalConnections != 0 {
			t.Errorf("Expected 0 total connections, got %d", metrics.TotalConnections)
		}

		if metrics.ConnectionHitRate != 0 {
			t.Errorf("Expected 0 hit rate with no connections, got %f", metrics.ConnectionHitRate)
		}
	})

	// Hit-rate calculation (folded from the former coverage_test.go
	// TestGetMetrics_HitRateCalculation): cumulative accepted connections vs
	// rejected attempts.
	t.Run("hit rate calculation", func(t *testing.T) {
		pm.acceptedConns.Store(80)
		pm.rejectedConns.Store(20)

		m := pm.GetMetrics()
		wantHitRate := float64(80) / float64(80+20) // 0.8
		if m.ConnectionHitRate != wantHitRate {
			t.Errorf("hit rate = %f, want %f", m.ConnectionHitRate, wantHitRate)
		}
		if m.TotalConnections != 80 {
			t.Errorf("TotalConnections = %d, want 80 (cumulative accepted)", m.TotalConnections)
		}
	})

	// Active gauge (folded from TestGetMetrics_ActiveConnections).
	t.Run("active connections gauge", func(t *testing.T) {
		pm.activeConns.Store(42)

		m := pm.GetMetrics()
		if m.ActiveConnections != 42 {
			t.Errorf("ActiveConnections = %d, want 42", m.ActiveConnections)
		}
		if m.LastUpdate == 0 {
			t.Error("LastUpdate should be non-zero")
		}
	})
}

func TestPoolManager_ConnectionMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	// Make a request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	_ = resp.Body.Close()

	// A successful request must register at least one connection in the metrics.
	metrics := pm.GetMetrics()
	if metrics.TotalConnections < 1 {
		t.Errorf("TotalConnections = %d, want >= 1 after a successful request", metrics.TotalConnections)
	}
}

func TestPoolManager_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.MaxIdleConns = 50
	config.MaxConnsPerHost = 20
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	const numRequests = 20
	var wg sync.WaitGroup
	errChan := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(server.URL)
			if err != nil {
				errChan <- err
				return
			}
			_ = resp.Body.Close()
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Concurrent request failed: %v", err)
	}
}

// TestPoolManager_IdleConnectionReuse asserts real idle-pool reuse: N
// sequential keep-alive requests to one server must ride a single TCP
// connection (one server-side accept), so every request after the first is
// served from the idle pool rather than a fresh dial.
func TestPoolManager_IdleConnectionReuse(t *testing.T) {
	cl := newCountingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	const numRequests = 5
	for i := 0; i < numRequests; i++ {
		resp, err := client.Get("http://" + cl.Addr().String())
		if err != nil {
			t.Fatalf("sequential request %d failed: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	if got := cl.accepted.Load(); got != 1 {
		t.Errorf("server accepted %d connection(s) for %d sequential requests, want 1 (idle connection must be reused)",
			got, numRequests)
	}
}

// TestPoolManager_MaxConnsPerHost_AdmissionControl asserts the admission
// behavior the transport actually enforces for MaxConnsPerHost: concurrent
// requests beyond the cap are QUEUED (no error, no extra connection dialed)
// and served one at a time over the single permitted connection — unlike the
// pool-level MaxTotalConns cap, which rejects with ErrPoolExhausted (see
// TestCreateDialer_PoolExhaustion in dialer_test.go).
func TestPoolManager_MaxConnsPerHost_AdmissionControl(t *testing.T) {
	var inflight, maxInflight atomic.Int64

	cl := newCountingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inflight.Add(1)
		for {
			observed := maxInflight.Load()
			if cur <= observed || maxInflight.CompareAndSwap(observed, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond) // hold the connection so concurrent requests must queue
		inflight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.MaxConnsPerHost = 1
	config.MaxIdleConnsPerHost = 1
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   10 * time.Second,
	}

	const numRequests = 4
	var wg sync.WaitGroup
	errChan := make(chan error, numRequests)
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get("http://" + cl.Addr().String())
			if err != nil {
				errChan <- err
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("queued request failed (MaxConnsPerHost must queue, not reject): %v", err)
	}

	if got := maxInflight.Load(); got > 1 {
		t.Errorf("max concurrent in-flight requests = %d, want <= 1 (MaxConnsPerHost=1 was exceeded)", got)
	}
	if got := cl.accepted.Load(); got != 1 {
		t.Errorf("server accepted %d connection(s), want 1 (dial queue must reuse the single permitted connection)", got)
	}
}

func TestPoolManager_ConcurrentClose(t *testing.T) {
	pm, err := NewPoolManager(nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	const numClosers = 5
	var wg sync.WaitGroup
	wg.Add(numClosers)

	for i := 0; i < numClosers; i++ {
		go func() {
			defer wg.Done()
			_ = pm.Close() // Should be safe to call multiple times
		}()
	}

	wg.Wait()

	// Beyond no-panic: after the race the pool must observably be closed
	// exactly once, and a subsequent dial must be refused.
	if !pm.closed.Load() {
		t.Fatal("pool should be closed after concurrent Close calls")
	}

	dialer := pm.createDialer()
	if _, err := dialer(context.Background(), "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("dial after concurrent Close must fail")
	} else if err.Error() != "connection pool is closed" {
		t.Errorf("dial after concurrent Close error = %q, want 'connection pool is closed'", err.Error())
	}
}

// TestPoolManager_CloseIdleConnections_ClosesServerSide strengthens the
// no-panic check in context_helpers_test.go: after a request completes and its
// connection goes idle, CloseIdleConnections must actually close it — observed
// here server-side as the connection transitioning to the closed state.
func TestPoolManager_CloseIdleConnections_ClosesServerSide(t *testing.T) {
	closed := make(chan struct{}, 1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateClosed {
				select {
				case closed <- struct{}{}:
				default:
				}
			}
		},
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	pm, err := NewPoolManager(config)
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pm.Close() }()

	client := &http.Client{
		Transport: pm.GetTransport(),
		Timeout:   5 * time.Second,
	}

	resp, err := client.Get("http://" + ln.Addr().String())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	// The idle connection must be closed server-side shortly after
	// CloseIdleConnections runs. Retry briefly: returning the connection to
	// the idle pool happens asynchronously in the transport's read loop.
	deadline := time.Now().Add(2 * time.Second)
	for {
		pm.CloseIdleConnections()
		select {
		case <-closed:
			return // server observed the close — done
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("idle connection was not closed server-side within 2s of CloseIdleConnections")
		}
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

// BenchmarkProxyConfiguration benchmarks the performance impact of proxy configuration
func BenchmarkProxyConfiguration(b *testing.B) {
	config := &Config{
		ProxyURL:    "http://proxy.example.com:8080",
		DialTimeout: 10 * time.Second,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm, err := NewPoolManager(config)
		if err != nil {
			b.Fatalf("NewPoolManager failed: %v", err)
		}
		_ = pm.Close()
	}
}

func BenchmarkSystemProxyDetection(b *testing.B) {
	config := &Config{
		EnableSystemProxy: true,
		DialTimeout:       10 * time.Second,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm, err := NewPoolManager(config)
		if err != nil {
			b.Fatalf("NewPoolManager failed: %v", err)
		}
		_ = pm.Close()
	}
}
