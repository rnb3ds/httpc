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
