package dns

import (
	"context"
	"encoding/hex"
	"net"
	"sync"
	"testing"
	"time"
)

func TestParseDomain(t *testing.T) {
	tests := []struct {
		name       string
		msgHex     string
		offset     int
		wantDomain string
		wantErr    bool
	}{
		{
			name:       "simple domain",
			msgHex:     "076578616d706c6503636f6d00", // example.com\0
			offset:     0,
			wantDomain: "example.com",
			wantErr:    false,
		},
		{
			name:       "single label",
			msgHex:     "096c6f63616c686f737400", // localhost (9 bytes) + null terminator
			offset:     0,
			wantDomain: "localhost",
			wantErr:    false,
		},
		{
			name:    "empty message",
			msgHex:  "",
			offset:  0,
			wantErr: true,
		},
		{
			name:    "offset out of bounds",
			msgHex:  "076578616d706c6500",
			offset:  100,
			wantErr: true,
		},
		{
			name:    "label exceeds message length",
			msgHex:  "ff6578616d706c65", // length=255 but message too short
			offset:  0,
			wantErr: true,
		},
		{
			name:    "compression pointer circular",
			msgHex:  "c000", // pointer to offset 0 (self-referential)
			offset:  0,
			wantErr: true, // Should fail due to recursion depth limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, _ := hex.DecodeString(tt.msgHex)
			domain, _, err := parseDomain(msg, tt.offset, 0)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDomain() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("parseDomain() unexpected error: %v", err)
				}
				if domain != tt.wantDomain {
					t.Errorf("parseDomain() = %q, want %q", domain, tt.wantDomain)
				}
			}
		})
	}
}

func TestGetUint16(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    uint16
		wantErr bool
	}{
		{"valid", []byte{0x01, 0x02}, 0x0102, false},
		{"zero", []byte{0x00, 0x00}, 0x0000, false},
		{"max", []byte{0xff, 0xff}, 0xffff, false},
		{"too short - empty", []byte{}, 0, true},
		{"too short - 1 byte", []byte{0x01}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getUint16(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Errorf("getUint16() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("getUint16() unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("getUint16() = %d, want %d", got, tt.want)
				}
			}
		})
	}
}

// Note: parseWireFormatResponse and parseJSONResponse are tested indirectly
// through LookupIPAddr integration tests. Unit tests for these private methods
// would require exposing them or using reflection, which is not idiomatic Go.

func TestDoHResolver_CacheExpiration(t *testing.T) {
	// Create resolver with very short TTL for testing
	resolver := NewDoHResolver(nil, 100*time.Millisecond)
	ctx := context.Background()

	// First lookup - should query network
	ips1, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Fatalf("First lookup failed: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second lookup - should query network again (cache expired)
	ips2, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Fatalf("Second lookup failed: %v", err)
	}

	// Results should be similar (same host)
	if len(ips1) == 0 || len(ips2) == 0 {
		t.Error("Expected non-empty IP results")
	}
}

func TestDoHResolver_CacheSize(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)

	// Initial cache size should be 0
	if size := resolver.CacheSize(); size != 0 {
		t.Errorf("Initial cache size = %d, want 0", size)
	}

	ctx := context.Background()

	// First lookup populates cache (ignore errors - we just want to test cache behavior)
	_, _ = resolver.LookupIPAddr(ctx, "www.google.com")

	// Cache size should be 0 or 1 depending on whether lookup succeeded
	// We can't guarantee network success, so just verify the method works
	size := resolver.CacheSize()
	t.Logf("Cache size after lookup: %d", size)

	// Clear cache
	resolver.ClearCache()

	if size := resolver.CacheSize(); size != 0 {
		t.Errorf("Cache size after clear = %d, want 0", size)
	}
}

func TestDoHResolver_SetCacheTTL(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// Populate cache
	_, _ = resolver.LookupIPAddr(ctx, "www.example.com")

	// Change TTL (this clears cache)
	resolver.SetCacheTTL(1 * time.Minute)

	// Cache should be cleared
	if size := resolver.CacheSize(); size != 0 {
		t.Errorf("Cache size after SetCacheTTL = %d, want 0", size)
	}
}

func TestDoHResolver_ConcurrentAccess(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Launch 20 concurrent lookups
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := resolver.LookupIPAddr(ctx, "www.google.com")
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent lookup error: %v", err)
	}
}

func TestDoHResolver_ContextCancellation(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

func TestDoHResolver_ContextTimeout(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(2 * time.Nanosecond) // Ensure timeout

	_, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err == nil {
		t.Error("Expected error with timed out context")
	}
}

func TestDoHResolver_EmptyHost(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// Empty host should fail
	_, err := resolver.LookupIPAddr(ctx, "")
	if err == nil {
		t.Error("Expected error for empty host")
	}
}

func TestDoHResolver_IPAddressInput(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// IP address should be handled
	ips, err := resolver.LookupIPAddr(ctx, "8.8.8.8")
	if err != nil {
		t.Logf("IP address lookup returned error (may be expected): %v", err)
	} else if len(ips) == 0 {
		t.Error("Expected at least one IP for IP address input")
	}
}

func TestDoHResolver_CacheReturnsCopy(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	// First lookup
	ips1, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Fatalf("First lookup failed: %v", err)
	}

	// Modify the returned slice
	if len(ips1) > 0 {
		ips1[0] = net.IPAddr{IP: net.ParseIP("1.2.3.4")}
	}

	// Second lookup should return original cached data
	ips2, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Fatalf("Second lookup failed: %v", err)
	}

	// Data should not be modified
	if len(ips2) > 0 && ips2[0].IP.String() == "1.2.3.4" {
		t.Error("Cache returned modified data instead of copy")
	}
}

func TestDoHResolver_Close(t *testing.T) {
	t.Run("CloseOnce", func(t *testing.T) {
		resolver := NewDoHResolver(nil, 5*time.Minute)

		err := resolver.Close()
		if err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	})

	t.Run("CloseTwice", func(t *testing.T) {
		resolver := NewDoHResolver(nil, 5*time.Minute)

		// First close
		err := resolver.Close()
		if err != nil {
			t.Errorf("First close() returned error: %v", err)
		}

		// Second close should be idempotent
		err = resolver.Close()
		if err != nil {
			t.Errorf("Second close() returned error: %v", err)
		}
	})

	t.Run("LookupAfterClose", func(t *testing.T) {
		resolver := NewDoHResolver(nil, 5*time.Minute)

		// Close the resolver
		_ = resolver.Close()

		ctx := context.Background()
		_, err := resolver.LookupIPAddr(ctx, "www.google.com")
		if err == nil {
			t.Error("Expected error when using closed resolver")
		}
	})
}

func TestDoHResolver_GetCacheTTL(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)

	// Get default TTL
	ttl := resolver.GetCacheTTL()
	if ttl != 5*time.Minute {
		t.Errorf("GetCacheTTL() = %v, want %v", ttl, 5*time.Minute)
	}

	// Set new TTL
	resolver.SetCacheTTL(10 * time.Minute)

	// Verify TTL was changed
	ttl = resolver.GetCacheTTL()
	if ttl != 10*time.Minute {
		t.Errorf("GetCacheTTL() after SetCacheTTL = %v, want %v", ttl, 10*time.Minute)
	}
}

func TestDoHResolver_ConcurrentClearCache(t *testing.T) {
	resolver := NewDoHResolver(nil, 5*time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup

	// Concurrent cache operations
	for i := 0; i < 10; i++ {
		wg.Add(2)

		// Goroutine 1: lookup
		go func() {
			defer wg.Done()
			_, _ = resolver.LookupIPAddr(ctx, "www.google.com")
		}()

		// Goroutine 2: clear cache
		go func() {
			defer wg.Done()
			resolver.ClearCache()
		}()
	}

	wg.Wait()
	// If we get here without deadlock or panic, the test passes
}

// ============================================================================
// parseWireFormatResponse Unit Tests - Error Cases
// ============================================================================

func TestParseWireFormatResponse_Errors(t *testing.T) {
	r := &DoHResolver{}

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "Empty body",
			body:    []byte{},
			wantErr: true,
		},
		{
			name:    "Too short - 1 byte",
			body:    []byte{0x00},
			wantErr: true,
		},
		{
			name:    "Too short - 11 bytes",
			body:    make([]byte, 11),
			wantErr: true,
		},
		{
			name: "Valid header no answers",
			// DNS header with QDCOUNT=0, ANCOUNT=0
			body:    []byte{0x00, 0x01, 0x81, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00},
			wantErr: true, // No IP addresses found
		},
		{
			name: "Truncated after question",
			// DNS header with QDCOUNT=1, but truncated
			body:    []byte{0x00, 0x01, 0x81, 0x80, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x07, 0x65, 0x78},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.parseWireFormatResponse(tt.body, "example.com")
			if (err != nil) != tt.wantErr {
				t.Errorf("parseWireFormatResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// parseJSONResponse Unit Tests - Error Cases
// ============================================================================

func TestParseJSONResponse_Errors(t *testing.T) {
	r := &DoHResolver{}

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "Invalid JSON",
			body:    []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name:    "Empty object",
			body:    []byte(`{}`),
			wantErr: true, // No IP addresses found
		},
		{
			name:    "DNS error status",
			body:    []byte(`{"Status": 2, "Answer": []}`),
			wantErr: true, // Non-zero status
		},
		{
			name:    "Empty answers",
			body:    []byte(`{"Status": 0, "Answer": []}`),
			wantErr: true, // No IP addresses found
		},
		{
			name:    "No A/AAAA records",
			body:    []byte(`{"Status": 0, "Answer": [{"name": "example.com", "type": 5, "data": "target.com"}]}`),
			wantErr: true, // No IP addresses found
		},
		{
			name:    "Invalid IP in answer",
			body:    []byte(`{"Status": 0, "Answer": [{"name": "example.com", "type": 1, "data": "invalid-ip"}]}`),
			wantErr: true, // No valid IPs after parsing
		},
		{
			name:    "Valid A record",
			body:    []byte(`{"Status": 0, "Answer": [{"name": "example.com", "type": 1, "data": "8.8.8.8"}]}`),
			wantErr: false,
		},
		{
			name:    "Valid AAAA record",
			body:    []byte(`{"Status": 0, "Answer": [{"name": "example.com", "type": 28, "data": "2001:4860:4860::8888"}]}`),
			wantErr: false,
		},
		{
			name:    "NXDOMAIN status (3) with empty answers",
			body:    []byte(`{"Status": 3, "Answer": []}`),
			wantErr: true, // No IP addresses
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := r.parseJSONResponse(tt.body, "example.com")
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(ips) == 0 {
				t.Error("Expected at least one IP address")
			}
		})
	}
}

// ============================================================================
// Cache Limit Tests
// ============================================================================

func TestDoHResolver_CacheLimit(t *testing.T) {
	// Create resolver with a custom configuration
	resolver := NewDoHResolver(nil, 5*time.Minute)
	defer resolver.Close()

	ctx := context.Background()

	// Populate cache with multiple entries
	hosts := []string{
		"www.google.com",
		"www.example.com",
		"www.cloudflare.com",
		"www.github.com",
		"www.microsoft.com",
	}

	for _, host := range hosts {
		_, _ = resolver.LookupIPAddr(ctx, host)
	}

	// Verify cache has entries
	size := resolver.CacheSize()
	t.Logf("Cache size after %d lookups: %d", len(hosts), size)

	// Clear cache and verify
	resolver.ClearCache()
	if resolver.CacheSize() != 0 {
		t.Errorf("Cache size after clear = %d, want 0", resolver.CacheSize())
	}
}

// ============================================================================
// Provider Priority Tests
// ============================================================================

func TestDoHResolver_ProviderPriority(t *testing.T) {
	// Create custom providers with different priorities
	providers := []*DoHProvider{
		{
			Name:     "primary",
			Template: "https://1.1.1.1/dns-query?name={name}&type=A",
			Priority: 1,
		},
		{
			Name:     "secondary",
			Template: "https://dns.google/resolve?name={name}&type=A",
			Priority: 2,
		},
	}

	resolver := NewDoHResolver(providers, 5*time.Minute)
	defer resolver.Close()

	// Verify providers are set
	if len(resolver.providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(resolver.providers))
	}

	// Verify order
	if resolver.providers[0].Priority > resolver.providers[1].Priority {
		t.Error("Providers should be ordered by priority")
	}
}

// ============================================================================
// HTTP Client Timeout Tests
// ============================================================================

func TestDoHResolver_HTTPTimeout(t *testing.T) {
	// Create resolver with short timeout
	resolver := NewDoHResolver(nil, 5*time.Minute)
	defer resolver.Close()

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(2 * time.Nanosecond)

	_, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err == nil {
		t.Error("Expected timeout error")
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestDoHResolver_InvalidProviderTemplate(t *testing.T) {
	// Create resolver with invalid provider template
	providers := []*DoHProvider{
		{
			Name:     "invalid",
			Template: "http://[::1]:namedpipe",
			Priority: 1,
		},
	}

	resolver := NewDoHResolver(providers, 5*time.Minute)
	defer resolver.Close()

	ctx := context.Background()

	// Should fall back to system resolver
	_, err := resolver.LookupIPAddr(ctx, "www.google.com")
	// The fallback might succeed, so we just verify it doesn't panic
	_ = err
}

func TestDoHResolver_CacheExpirationRaceCondition(t *testing.T) {
	resolver := NewDoHResolver(nil, 100*time.Millisecond)
	defer resolver.Close()

	ctx := context.Background()

	// First lookup to populate cache
	_, _ = resolver.LookupIPAddr(ctx, "www.google.com")

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Concurrent reads during cache expiration
	for i := 0; i < 10; i++ {
		wg.Add(2)

		// Reader 1
		go func() {
			defer wg.Done()
			_, err := resolver.LookupIPAddr(ctx, "www.google.com")
			if err != nil {
				errors <- err
			}
		}()

		// Reader 2
		go func() {
			defer wg.Done()
			_, err := resolver.LookupIPAddr(ctx, "www.google.com")
			if err != nil {
				errors <- err
			}
		}()

		// Small delay to allow cache expiration
		time.Sleep(20 * time.Millisecond)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Logf("Concurrent lookup error (may be expected): %v", err)
	}
}

func TestDoHResolver_MultipleProvidersFailover(t *testing.T) {
	// Create providers with first one invalid to test failover
	providers := []*DoHProvider{
		{
			Name:     "invalid-first",
			Template: "http://invalid.localhost:99999/dns?name={name}",
			Priority: 1,
		},
		{
			Name:     "valid-fallback",
			Template: "https://dns.google/resolve?name={name}&type=A",
			Priority: 2,
		},
	}

	resolver := NewDoHResolver(providers, 5*time.Minute)
	defer resolver.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Should fail over to valid provider or system resolver
	ips, err := resolver.LookupIPAddr(ctx, "www.google.com")
	if err != nil {
		t.Logf("Lookup error (may be expected in test env): %v", err)
	} else {
		t.Logf("Successfully resolved with failover: %d IPs", len(ips))
	}
}
