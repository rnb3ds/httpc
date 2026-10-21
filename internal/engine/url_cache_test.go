package engine

import (
	"fmt"
	"net/url"
	"testing"
)

// ============================================================================
// URL CACHE TESTS
//
// Consolidates every urlCache test into one file, in two sections:
//   - Get/SENSITIVE: the high-level cache API and the sensitive-URL
//     never-cached contract
//   - evictOldest: the entries FIFO and keys-slice compaction
// ============================================================================

// TestURLCache_SensitiveNotShared validates that URLs carrying sensitive
// query parameters are never shared through the cache: each lookup must
// return a parse of its own URL, never another URL's credentials-bearing
// entry, and no entry is retained for them. (The previous sanitized-key
// design merged ?token=A and ?token=B onto one entry and returned request
// A's token for request B — cross-request credential leakage.)
func TestURLCache_SensitiveNotShared(t *testing.T) {
	clearURLCache()

	// These URLs differ only in the sensitive "token" param — each lookup
	// must parse its own value.
	url1 := "https://api.example.com/data?token=secretA&page=1"
	url2 := "https://api.example.com/data?token=secretB&page=1"

	parsed1, err := globalURLCache.Get(url1)
	if err != nil {
		t.Fatalf("Failed to parse URL1: %v", err)
	}
	parsed2, err := globalURLCache.Get(url2)
	if err != nil {
		t.Fatalf("Failed to parse URL2: %v", err)
	}

	q1 := parsed1.Query()
	q2 := parsed2.Query()
	if q1.Get("token") != "secretA" {
		t.Errorf("URL1 token = %q, want %q", q1.Get("token"), "secretA")
	}
	if q2.Get("token") != "secretB" {
		t.Errorf("URL2 token = %q, want %q (its own value, never URL1's)", q2.Get("token"), "secretB")
	}

	// Credential-bearing URLs are cached nowhere.
	if getURLCacheSize() != 0 {
		t.Errorf("Expected 0 cache entries for sensitive URLs, got %d", getURLCacheSize())
	}

	// Userinfo URLs are sensitive too, and credentials stay stripped.
	u, err := globalURLCache.Get("https://user:pass@example.com/data")
	if err != nil {
		t.Fatalf("Failed to parse userinfo URL: %v", err)
	}
	if u.User != nil {
		t.Errorf("userinfo not stripped: %v", u.User)
	}
	if getURLCacheSize() != 0 {
		t.Errorf("userinfo URL must not be cached, got %d entries", getURLCacheSize())
	}

	// URLs with different non-sensitive params are cached separately
	url3 := "https://api.example.com/data?page=2"
	_, err = globalURLCache.Get(url3)
	if err != nil {
		t.Fatalf("Failed to parse URL3: %v", err)
	}
	if getURLCacheSize() != 1 {
		t.Errorf("Expected 1 cache entry after adding non-sensitive variant, got %d", getURLCacheSize())
	}
}

// TestURLCache_Get validates cache hit/miss and eviction behavior.
func TestURLCache_Get(t *testing.T) {
	clearURLCache()

	// Cache miss -> parse and store
	u, err := globalURLCache.Get("https://example.com/page1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if u == nil {
		t.Fatal("Expected URL, got nil")
	}

	// Cache hit
	u2, err := globalURLCache.Get("https://example.com/page1")
	if err != nil {
		t.Fatalf("Unexpected error on cache hit: %v", err)
	}
	if u2.String() != u.String() {
		t.Errorf("Cache hit returned different URL: %s vs %s", u2.String(), u.String())
	}

	// Invalid URL
	_, err = globalURLCache.Get("://invalid")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	clearURLCache()
}

// TestURLCache_BoundedGrowth verifies that a burst of unique URL variants
// cannot grow the cache beyond maxSize: FIFO eviction keeps entries bounded,
// and evicted URLs still resolve correctly afterwards (a post-eviction
// lookup simply re-parses — equivalent to a cache miss).
func TestURLCache_BoundedGrowth(t *testing.T) {
	cache := &urlCache{
		entries: make(map[string]*url.URL, 16),
		keys:    make([]string, 0, 16),
		maxSize: 64,
	}

	// Fill the cache with unique entries
	for i := 0; i < 64; i++ {
		urlStr := fmt.Sprintf("https://host%d.example.com/path", i)
		if _, err := cache.Get(urlStr); err != nil {
			t.Fatalf("Get(%q) failed: %v", urlStr, err)
		}
	}

	// Add many URL variants to stress eviction
	for i := 0; i < 2500; i++ {
		urlStr := fmt.Sprintf("https://host0.example.com/path?v=%d", i)
		if _, err := cache.Get(urlStr); err != nil {
			t.Fatalf("Get(%q) failed: %v", urlStr, err)
		}
	}

	cache.mu.RLock()
	entriesLen := len(cache.entries)
	cache.mu.RUnlock()

	if entriesLen > 64 {
		t.Errorf("entries grew to %d, expected max 64", entriesLen)
	}

	// Verify the cache still works correctly after heavy eviction: an evicted
	// URL re-parses and is re-cached.
	u, err := cache.Get("https://host0.example.com/path?v=42")
	if err != nil {
		t.Fatalf("Get after eviction failed: %v", err)
	}
	if u == nil {
		t.Fatal("expected non-nil URL after eviction")
	}
}

// TestURLCache_DuplicateLookupNoGrowth verifies that re-looking up the same
// URL never creates duplicate entries.
func TestURLCache_DuplicateLookupNoGrowth(t *testing.T) {
	cache := &urlCache{
		entries: make(map[string]*url.URL, 16),
		keys:    make([]string, 0, 16),
		maxSize: 64,
	}

	urlStr := "https://example.com/api/v1"
	for i := 0; i < 100; i++ {
		if _, err := cache.Get(urlStr); err != nil {
			t.Fatalf("Get failed on iteration %d: %v", i, err)
		}
	}

	cache.mu.RLock()
	entriesLen := len(cache.entries)
	cache.mu.RUnlock()

	if entriesLen != 1 {
		t.Errorf("expected 1 entry after 100 identical lookups, got %d", entriesLen)
	}
}

// TestURLCache_EvictOldest covers the entries FIFO eviction: when entries
// exceed maxSize the oldest key is dropped and the keys slice is advanced.
func TestURLCache_EvictOldest(t *testing.T) {
	oldest := &url.URL{Path: "/oldest"}
	newest := &url.URL{Path: "/newest"}
	c := &urlCache{
		entries: map[string]*url.URL{"k1": oldest, "k2": newest},
		keys:    []string{"k1", "k2"},
		maxSize: 1, // tiny so eviction triggers with two entries
	}

	c.evictOldest()

	if _, ok := c.entries["k1"]; ok {
		t.Error("oldest entry (k1) should be evicted")
	}
	if _, ok := c.entries["k2"]; !ok {
		t.Error("newest entry (k2) should be retained")
	}
	if len(c.keys) != 1 || c.keys[0] != "k2" {
		t.Errorf("keys slice should advance to [k2], got %v", c.keys)
	}

	// The keys-slice compaction branch: after eviction leaves a slice whose
	// capacity is more than double its length, the backing array is shrunk.
	big := &url.URL{Path: "/big"}
	c3 := &urlCache{
		entries: map[string]*url.URL{"k": big},
		keys:    append(make([]string, 0, 16), "k"), // cap 16 >> len 1
		maxSize: 1,
	}
	c3.evictOldest()
	if _, ok := c3.entries["k"]; ok {
		t.Error("entry should be evicted at maxSize")
	}
	if cap(c3.keys) > 2*len(c3.keys) {
		t.Errorf("keys backing array should be shrunk, cap=%d len=%d", cap(c3.keys), len(c3.keys))
	}
}
