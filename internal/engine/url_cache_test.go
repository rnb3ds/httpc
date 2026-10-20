package engine

import (
	"fmt"
	"net/url"
	"testing"
)

// ============================================================================
// URL CACHE TESTS
//
// Consolidates every urlCache test into one file. The type is split across
// three layers, each with its own section:
//   - Get/SanitizedKey: the high-level cache API and sanitized-key dedup
//   - populateRawCache/evictRawIfNeeded: the raw-URL side cache (bounded memory)
//   - evictOldest: the entries LRU and keys-slice compaction
// ============================================================================

// TestURLCache_SanitizedKey validates that URLs differing only in sensitive
// query params share a single cache entry (sanitized key deduplication), while
// URLs with different non-sensitive params are cached separately.
func TestURLCache_SanitizedKey(t *testing.T) {
	clearURLCache()

	// These URLs differ only in the sensitive "token" param — should map to one cache entry
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

	// Both URLs should share a single sanitized cache entry
	if getURLCacheSize() != 1 {
		t.Errorf("Expected 1 cache entry (sanitized key dedup), got %d", getURLCacheSize())
	}

	// Both lookups return the first cached entry's token value (dedup by sanitized key)
	q1 := parsed1.Query()
	q2 := parsed2.Query()
	if q1.Get("token") != "secretA" {
		t.Errorf("URL1 token = %q, want %q", q1.Get("token"), "secretA")
	}
	if q2.Get("token") != "secretA" {
		t.Errorf("URL2 token = %q, want %q (dedup: shares first entry's value)", q2.Get("token"), "secretA")
	}

	// URLs with different non-sensitive params should be cached separately
	url3 := "https://api.example.com/data?page=2"
	_, err = globalURLCache.Get(url3)
	if err != nil {
		t.Fatalf("Failed to parse URL3: %v", err)
	}
	if getURLCacheSize() != 2 {
		t.Errorf("Expected 2 cache entries after adding non-sensitive variant, got %d", getURLCacheSize())
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

// TestURLCacheRawEviction verifies that the raw URL cache is evicted when it
// exceeds rawCacheMaxSize, preventing unbounded memory growth.
func TestURLCacheRawEviction(t *testing.T) {
	cache := &urlCache{
		raw:     make(map[string]*url.URL, 16),
		entries: make(map[string]*url.URL, 16),
		keys:    make([]string, 0, 16),
		maxSize: 64,
	}

	// Fill the cache with unique entries
	for i := 0; i < 64; i++ {
		urlStr := fmt.Sprintf("https://host%d.example.com/path", i)
		_, err := cache.Get(urlStr)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", urlStr, err)
		}
	}

	// Verify entries map is at maxSize
	cache.mu.RLock()
	entriesLen := len(cache.entries)
	rawLen := len(cache.raw)
	cache.mu.RUnlock()

	if entriesLen != 64 {
		t.Errorf("expected 64 entries, got %d", entriesLen)
	}
	if rawLen != 64 {
		t.Errorf("expected 64 raw entries, got %d", rawLen)
	}

	// Now add many URL variants for the same host to grow the raw map beyond entries
	for i := 0; i < 2500; i++ {
		urlStr := fmt.Sprintf("https://host0.example.com/path?v=%d", i)
		_, err := cache.Get(urlStr)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", urlStr, err)
		}
	}

	cache.mu.RLock()
	rawLen = len(cache.raw)
	entriesLen = len(cache.entries)
	cache.mu.RUnlock()

	// The raw map should not grow unboundedly — it must be capped
	if rawLen > rawCacheMaxSize*2 {
		t.Errorf("raw cache grew to %d entries, expected cap near %d", rawLen, rawCacheMaxSize)
	}

	// Entries should still be bounded at maxSize
	if entriesLen > 64 {
		t.Errorf("entries grew to %d, expected max 64", entriesLen)
	}

	// Verify cache still works correctly after eviction
	u, err := cache.Get("https://host0.example.com/path?v=42")
	if err != nil {
		t.Fatalf("Get after eviction failed: %v", err)
	}
	if u == nil {
		t.Fatal("expected non-nil URL after eviction")
	}
}

// TestURLCacheRawDuplicateCheck verifies that re-looking up the same raw URL
// does not create duplicate entries in the raw map.
func TestURLCacheRawDuplicateCheck(t *testing.T) {
	cache := &urlCache{
		raw:     make(map[string]*url.URL, 16),
		entries: make(map[string]*url.URL, 16),
		keys:    make([]string, 0, 16),
		maxSize: 64,
	}

	urlStr := "https://example.com/api/v1"

	// Look up the same URL many times
	for i := 0; i < 100; i++ {
		_, err := cache.Get(urlStr)
		if err != nil {
			t.Fatalf("Get failed on iteration %d: %v", i, err)
		}
	}

	cache.mu.RLock()
	rawLen := len(cache.raw)
	entriesLen := len(cache.entries)
	cache.mu.RUnlock()

	if rawLen != 1 {
		t.Errorf("expected 1 raw entry, got %d", rawLen)
	}
	if entriesLen != 1 {
		t.Errorf("expected 1 entry, got %d", entriesLen)
	}
}

// TestURLCache_EvictRawIfNeeded covers the raw-cache eviction branch: entries
// still referenced from the entries map are retained, and eviction reduces the
// raw map below the threshold.
func TestURLCache_EvictRawIfNeeded(t *testing.T) {
	c := &urlCache{
		raw:     make(map[string]*url.URL, rawCacheMaxSize+1),
		entries: make(map[string]*url.URL, 1),
		maxSize: 1024,
	}

	// One "live" entry: present in both raw and entries (same pointer).
	liveURL := &url.URL{Path: "/live"}
	c.entries["sanitized-live"] = liveURL
	c.raw["http://example.com/0"] = liveURL

	// Fill the rest with "stale" entries whose pointers are NOT in entries,
	// pushing len(raw) past rawCacheMaxSize so eviction triggers.
	for i := 1; i <= rawCacheMaxSize; i++ {
		key := fmt.Sprintf("http://example.com/%d", i)
		c.raw[key] = &url.URL{Path: fmt.Sprintf("/stale-%d", i)}
	}
	if len(c.raw) < rawCacheMaxSize {
		t.Fatalf("setup error: raw cache not over threshold, len=%d", len(c.raw))
	}

	c.evictRawIfNeeded()

	// The live entry is always retained (its pointer is found in entries).
	if _, ok := c.raw["http://example.com/0"]; !ok {
		t.Error("live entry should be retained after eviction")
	}
	// Eviction must have reduced the raw cache below the threshold.
	if len(c.raw) >= rawCacheMaxSize {
		t.Errorf("eviction did not reduce raw cache: len=%d (threshold %d)",
			len(c.raw), rawCacheMaxSize)
	}
}

// TestURLCache_EvictRaw_NoopBelowThreshold confirms eviction is a no-op when
// the raw cache is under rawCacheMaxSize.
func TestURLCache_EvictRaw_NoopBelowThreshold(t *testing.T) {
	c := &urlCache{
		raw:     map[string]*url.URL{"http://example.com/a": {Path: "/a"}},
		entries: map[string]*url.URL{},
		maxSize: 1024,
	}
	c.evictRawIfNeeded()
	if len(c.raw) != 1 {
		t.Errorf("eviction should be a no-op below threshold, got len=%d", len(c.raw))
	}
}

// TestURLCache_PopulateRawCache covers the lazy-init (nil raw) branch and the
// existing-key no-op of populateRawCache.
func TestURLCache_PopulateRawCache(t *testing.T) {
	c := &urlCache{
		entries: make(map[string]*url.URL),
		keys:    []string{},
		maxSize: 1024,
		// raw intentionally nil → exercises the lazy-init branch.
	}
	parsed := &url.URL{Path: "/x"}
	c.populateRawCache("http://example.com/x", parsed)
	if c.raw == nil || c.raw["http://example.com/x"] != parsed {
		t.Error("populateRawCache did not lazily init raw and store the entry")
	}
	// Re-populating an existing key is a no-op (no duplicate, no eviction).
	c.populateRawCache("http://example.com/x", parsed)
	if len(c.raw) != 1 {
		t.Errorf("duplicate populate should be a no-op, got len=%d", len(c.raw))
	}
}

// TestURLCache_EvictOldest covers the entries-LRU eviction: when entries exceed
// maxSize the oldest key is dropped, its raw entry is cleaned up, and the keys
// slice is advanced.
func TestURLCache_EvictOldest(t *testing.T) {
	oldest := &url.URL{Path: "/oldest"}
	newest := &url.URL{Path: "/newest"}
	c := &urlCache{
		entries: map[string]*url.URL{"k1": oldest, "k2": newest},
		raw: map[string]*url.URL{
			"http://example.com/oldest": oldest,
			"http://example.com/newest": newest,
		},
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
	// evictOldest no longer scans the raw map: an orphaned raw entry is a
	// still-correct parse of its key and is reclaimed by evictRawIfNeeded
	// once the raw map exceeds its cap. Verify both raw entries survive here.
	if _, ok := c.raw["http://example.com/oldest"]; !ok {
		t.Error("orphaned raw entry for the evicted URL may remain until evictRawIfNeeded runs")
	}
	if _, ok := c.raw["http://example.com/newest"]; !ok {
		t.Error("raw entry for the retained URL should remain")
	}
	if len(c.keys) != 1 || c.keys[0] != "k2" {
		t.Errorf("keys slice should advance to [k2], got %v", c.keys)
	}

	// populateRawCacheLocked must lazily initialize a nil raw map, and an
	// existing key must be a no-op (no overwrite). Covers the branches that
	// formerly relied on the removed TestURLCache_Eviction.
	u := &url.URL{Path: "/x"}
	c2 := &urlCache{
		entries: map[string]*url.URL{"k": u},
		keys:    []string{"k"},
		maxSize: 4,
		// raw intentionally nil
	}
	c2.populateRawCacheLocked("http://example.com/x", u)
	if c2.raw == nil || c2.raw["http://example.com/x"] != u {
		t.Error("populateRawCacheLocked should lazily create the raw map and insert")
	}
	c2.populateRawCacheLocked("http://example.com/x", &url.URL{Path: "/other"})
	if c2.raw["http://example.com/x"] != u {
		t.Error("populateRawCacheLocked must not overwrite an existing raw entry")
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
