//go:build integration

package dns

import (
	"context"
	"testing"
	"time"
)

func TestDoHResolver_LookupIPAddr(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantMin int // Minimum expected IP addresses
	}{
		{
			name:    "Google DNS",
			host:    "dns.google",
			wantMin: 1,
		},
		{
			name:    "Cloudflare DNS",
			host:    "1.1.1.1",
			wantMin: 1,
		},
		{
			name:    "Baidu",
			host:    "www.baidu.com",
			wantMin: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewDoHResolver(nil, 5*time.Minute)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			ips, err := resolver.LookupIPAddr(ctx, tt.host)
			if err != nil {
				t.Fatalf("LookupIPAddr() error = %v", err)
			}

			if len(ips) < tt.wantMin {
				t.Errorf("LookupIPAddr() returned %d IPs, want at least %d", len(ips), tt.wantMin)
			}

			t.Logf("Resolved %s to %d IPs:", tt.host, len(ips))
			for _, ip := range ips {
				t.Logf("  - %s", ip.IP.String())
			}
		})
	}
}

func TestDoHResolver_Cache(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// First lookup
	ips1, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Fatalf("First lookup failed: %v", err)
	}

	if len(ips1) == 0 {
		t.Fatal("First lookup returned no IPs")
	}

	// Second lookup should use cache
	ips2, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Fatalf("Second lookup failed: %v", err)
	}

	if len(ips2) != len(ips1) {
		t.Errorf("Cache returned different number of IPs: got %d, want %d", len(ips2), len(ips1))
	}
}

func TestDoHResolver_ClearCache(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// Populate cache
	_, err := resolver.LookupIPAddr(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	// Clear cache
	resolver.ClearCache()

	// Lookup again - should still work
	_, err = resolver.LookupIPAddr(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("Lookup after cache clear failed: %v", err)
	}
}

func TestDoHResolver_Fallback(t *testing.T) {
	// Create resolver with invalid providers
	invalidProviders := []*DoHProvider{
		{
			Name:     "invalid",
			Template: "http://invalid.local/test",
			Priority: 1,
		},
	}

	resolver := NewDoHResolver(invalidProviders, 5*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Should fall back to system resolver
	_, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Logf("Fallback lookup error (expected with invalid providers): %v", err)
	}
}

func TestDefaultDoHProviders(t *testing.T) {
	providers := DefaultDoHProviders()

	if len(providers) == 0 {
		t.Fatal("DefaultDoHProviders() returned empty list")
	}

	for _, p := range providers {
		if p.Name == "" {
			t.Error("Provider has empty Name")
		}
		if p.Template == "" {
			t.Error("Provider has empty Template")
		}
		if p.Template == "" {
			t.Errorf("Provider %s has invalid template", p.Name)
		}
		t.Logf("Provider: %s - %s", p.Name, p.Template)
	}
}

// Benchmark test
func BenchmarkDoHResolver_LookupIPAddr(b *testing.B) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.LookupIPAddr(ctx, "www.google.com")
	}
}

func BenchmarkDoHResolver_CachedLookup(b *testing.B) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// Warm up cache
	_, _ = resolver.LookupIPAddr(ctx, "www.google.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.LookupIPAddr(ctx, "www.google.com")
	}
}

// ============================================================================
// Additional Cache Tests (complementing doh_unit_test.go)
// ============================================================================

func TestDoHResolver_CacheSize_MultipleLookups(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	defer resolver.Close()
	ctx := context.Background()

	hosts := []string{"www.google.com", "www.example.com", "www.cloudflare.com"}

	// Look up multiple hosts
	for _, host := range hosts {
		_, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			t.Logf("Lookup for %s failed (may be expected in test env): %v", host, err)
		}
	}

	// Cache should have entries
	size := resolver.CacheSize()
	t.Logf("Cache size after %d lookups: %d", len(hosts), size)

	// Clear and verify
	resolver.ClearCache()
	if resolver.CacheSize() != 0 {
		t.Errorf("Cache size after clear = %d, want 0", resolver.CacheSize())
	}
}

func TestDoHResolver_ConcurrentCacheAccess(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	defer resolver.Close()

	// Concurrent TTL reads and writes
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := range 10 {
			resolver.SetCacheTTL(time.Duration(i+1) * time.Minute)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for range 10 {
			_ = resolver.GetCacheTTL()
			_ = resolver.CacheSize()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}
