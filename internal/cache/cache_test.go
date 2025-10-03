package cache

import (
	"testing"
	"time"
)

// ============================================================================
// CACHE BASIC TESTS
// ============================================================================

func TestCache_New(t *testing.T) {
	cache := New(nil)
	if cache == nil {
		t.Fatal("New() returned nil")
	}
	
	if cache.maxSize <= 0 {
		t.Error("maxSize should be positive")
	}
	
	if cache.maxAge <= 0 {
		t.Error("maxAge should be positive")
	}
}

func TestCache_NewWithConfig(t *testing.T) {
	config := &Config{
		MaxSize: 50 * 1024 * 1024,
		MaxAge:  10 * time.Minute,
	}
	
	cache := New(config)
	if cache == nil {
		t.Fatal("New() returned nil")
	}
	
	if cache.maxSize != config.MaxSize {
		t.Errorf("Expected maxSize %d, got %d", config.MaxSize, cache.maxSize)
	}
	
	if cache.maxAge != config.MaxAge {
		t.Errorf("Expected maxAge %v, got %v", config.MaxAge, cache.maxAge)
	}
}

func TestCache_SetGet(t *testing.T) {
	cache := New(nil)
	
	key := "test-key"
	value := []byte("test value")
	headers := map[string][]string{"Content-Type": {"text/plain"}}
	statusCode := 200
	
	err := cache.Set(key, value, headers, statusCode, 0)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	
	entry, ok := cache.Get(key)
	if !ok {
		t.Fatal("Get() returned false for existing key")
	}
	
	if entry == nil {
		t.Fatal("Get() returned nil entry")
	}
	
	if string(entry.Value) != string(value) {
		t.Errorf("Expected value %s, got %s", value, entry.Value)
	}
	
	if entry.StatusCode != statusCode {
		t.Errorf("Expected status code %d, got %d", statusCode, entry.StatusCode)
	}
}

func TestCache_GetNonExistent(t *testing.T) {
	cache := New(nil)
	
	_, ok := cache.Get("non-existent-key")
	if ok {
		t.Error("Get() returned true for non-existent key")
	}
}

func TestCache_Delete(t *testing.T) {
	cache := New(nil)
	
	key := "test-key"
	value := []byte("test value")
	
	cache.Set(key, value, nil, 200, 0)
	
	// Verify it exists
	_, ok := cache.Get(key)
	if !ok {
		t.Fatal("Entry should exist before delete")
	}
	
	// Delete it
	cache.Delete(key)
	
	// Verify it's gone
	_, ok = cache.Get(key)
	if ok {
		t.Error("Entry should not exist after delete")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := New(nil)
	
	// Add multiple entries
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		value := []byte("value")
		cache.Set(key, value, nil, 200, 0)
	}
	
	// Clear cache
	cache.Clear()
	
	// Verify all entries are gone
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		_, ok := cache.Get(key)
		if ok {
			t.Errorf("Entry %s should not exist after clear", key)
		}
	}
	
	stats := cache.GetStats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", stats.Entries)
	}
}

// ============================================================================
// EXPIRATION TESTS
// ============================================================================

func TestCache_Expiration(t *testing.T) {
	cache := New(&Config{
		MaxSize: 100 * 1024 * 1024,
		MaxAge:  100 * time.Millisecond,
	})
	
	key := "test-key"
	value := []byte("test value")
	
	cache.Set(key, value, nil, 200, 0)
	
	// Should exist immediately
	_, ok := cache.Get(key)
	if !ok {
		t.Fatal("Entry should exist immediately after set")
	}
	
	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	
	// Should be expired
	_, ok = cache.Get(key)
	if ok {
		t.Error("Entry should be expired")
	}
}

func TestCache_CustomTTL(t *testing.T) {
	cache := New(nil)
	
	key := "test-key"
	value := []byte("test value")
	ttl := 100 * time.Millisecond
	
	cache.Set(key, value, nil, 200, ttl)
	
	// Should exist immediately
	_, ok := cache.Get(key)
	if !ok {
		t.Fatal("Entry should exist immediately after set")
	}
	
	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	
	// Should be expired
	_, ok = cache.Get(key)
	if ok {
		t.Error("Entry should be expired")
	}
}

// ============================================================================
// LRU EVICTION TESTS
// ============================================================================

func TestCache_LRUEviction(t *testing.T) {
	cache := New(&Config{
		MaxSize: 100, // Very small cache
		MaxAge:  10 * time.Minute,
	})
	
	// Add entries that exceed cache size
	cache.Set("key1", make([]byte, 40), nil, 200, 0)
	time.Sleep(10 * time.Millisecond) // Ensure different access times
	
	cache.Set("key2", make([]byte, 40), nil, 200, 0)
	time.Sleep(10 * time.Millisecond)
	
	cache.Set("key3", make([]byte, 40), nil, 200, 0)
	
	// key1 should be evicted (least recently used)
	_, ok := cache.Get("key1")
	if ok {
		t.Error("key1 should have been evicted")
	}
	
	// key2 and key3 should still exist
	_, ok = cache.Get("key2")
	if !ok {
		t.Error("key2 should still exist")
	}
	
	_, ok = cache.Get("key3")
	if !ok {
		t.Error("key3 should still exist")
	}
}

// ============================================================================
// KEY GENERATION TESTS
// ============================================================================

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		url     string
		headers map[string]string
		wantLen int
	}{
		{
			name:    "Simple GET",
			method:  "GET",
			url:     "https://example.com",
			headers: nil,
			wantLen: 64, // SHA256 hex length
		},
		{
			name:    "POST with headers",
			method:  "POST",
			url:     "https://example.com/api",
			headers: map[string]string{"Content-Type": "application/json"},
			wantLen: 64,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := GenerateKey(tt.method, tt.url, tt.headers)
			
			if len(key) != tt.wantLen {
				t.Errorf("Expected key length %d, got %d", tt.wantLen, len(key))
			}
			
			// Key should be deterministic
			key2 := GenerateKey(tt.method, tt.url, tt.headers)
			if key != key2 {
				t.Error("Key generation should be deterministic")
			}
		})
	}
}

func TestGenerateKey_Different(t *testing.T) {
	key1 := GenerateKey("GET", "https://example.com", nil)
	key2 := GenerateKey("POST", "https://example.com", nil)
	
	if key1 == key2 {
		t.Error("Different methods should generate different keys")
	}
	
	key3 := GenerateKey("GET", "https://example.com/path1", nil)
	key4 := GenerateKey("GET", "https://example.com/path2", nil)
	
	if key3 == key4 {
		t.Error("Different URLs should generate different keys")
	}
}

// ============================================================================
// STATISTICS TESTS
// ============================================================================

func TestCache_GetStats(t *testing.T) {
	cache := New(nil)
	
	// Add some entries
	cache.Set("key1", []byte("value1"), nil, 200, 0)
	cache.Set("key2", []byte("value2"), nil, 200, 0)
	
	// Access them
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("key2")
	
	stats := cache.GetStats()
	
	if stats.Entries != 2 {
		t.Errorf("Expected 2 entries, got %d", stats.Entries)
	}
	
	if stats.TotalHits != 3 {
		t.Errorf("Expected 3 total hits, got %d", stats.TotalHits)
	}
	
	if stats.Size <= 0 {
		t.Error("Size should be positive")
	}
	
	if stats.MaxSize <= 0 {
		t.Error("MaxSize should be positive")
	}
}

// ============================================================================
// CONCURRENT ACCESS TESTS
// ============================================================================

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := New(nil)
	
	done := make(chan bool, 100)
	
	// Concurrent writes
	for i := 0; i < 50; i++ {
		go func(id int) {
			key := string(rune('a' + id%26))
			value := []byte("value")
			cache.Set(key, value, nil, 200, 0)
			done <- true
		}(i)
	}
	
	// Concurrent reads
	for i := 0; i < 50; i++ {
		go func(id int) {
			key := string(rune('a' + id%26))
			cache.Get(key)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestCache_HitCount(t *testing.T) {
	cache := New(nil)
	
	key := "test-key"
	value := []byte("test value")
	
	cache.Set(key, value, nil, 200, 0)
	
	// Access multiple times
	for i := 0; i < 5; i++ {
		entry, ok := cache.Get(key)
		if !ok {
			t.Fatal("Entry should exist")
		}
		
		if entry.HitCount != int64(i+1) {
			t.Errorf("Expected hit count %d, got %d", i+1, entry.HitCount)
		}
	}
}

