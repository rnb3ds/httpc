package connection

import (
	"testing"
)

// TestNewPoolManager_ProxyDispatcherActivation pins the exact conditions under
// which the per-proxy dispatcher is created. Everything else (shared transport
// with forced HTTP/1 for multi-proxy pools) is the default.
func TestNewPoolManager_ProxyDispatcherActivation(t *testing.T) {
	tests := []struct {
		name         string
		proxyURL     string
		pool         []string
		poolHTTP2    bool
		enableHTTP2  bool
		wantDispatch bool
	}{
		{
			name:         "flag on with multi-proxy pool and HTTP/2",
			pool:         []string{"http://p1.example.com:8080", "http://p2.example.com:8080"},
			poolHTTP2:    true,
			enableHTTP2:  true,
			wantDispatch: true,
		},
		{
			name:        "flag off (default)",
			pool:        []string{"http://p1.example.com:8080", "http://p2.example.com:8080"},
			enableHTTP2: true,
		},
		{
			name:      "flag on but HTTP/2 disabled",
			pool:      []string{"http://p1.example.com:8080", "http://p2.example.com:8080"},
			poolHTTP2: true,
		},
		{
			name:        "flag on but single-proxy pool",
			pool:        []string{"http://p1.example.com:8080"},
			poolHTTP2:   true,
			enableHTTP2: true,
		},
		{
			name:        "flag on but ProxyURL wins over the pool",
			proxyURL:    "http://manual.example.com:8080",
			pool:        []string{"http://p1.example.com:8080", "http://p2.example.com:8080"},
			poolHTTP2:   true,
			enableHTTP2: true,
		},
		{
			name:      "flag on without any pool",
			poolHTTP2: true, enableHTTP2: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.ProxyURL = tt.proxyURL
			cfg.ProxyPool = tt.pool
			cfg.EnableProxyPoolHTTP2 = tt.poolHTTP2
			cfg.EnableHTTP2 = tt.enableHTTP2

			pm, err := NewPoolManager(cfg)
			if err != nil {
				t.Fatalf("NewPoolManager() failed: %v", err)
			}
			defer func() { _ = pm.Close() }()

			if got := pm.proxyDispatcher != nil; got != tt.wantDispatch {
				t.Errorf("dispatcher active = %v, want %v", got, tt.wantDispatch)
			}

			// GetRoundTripper must agree with the dispatcher state: the
			// dispatcher instance when active, the shared transport otherwise.
			if tt.wantDispatch {
				if pm.GetRoundTripper() != pm.proxyDispatcher {
					t.Error("GetRoundTripper() does not return the dispatcher")
				}
			} else if pm.GetRoundTripper() != pm.transport {
				t.Error("GetRoundTripper() does not return the shared transport")
			}
		})
	}
}
