package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Entry represents a cached response
type Entry struct {
	Key        string
	Value      []byte
	Headers    map[string][]string
	StatusCode int
	ExpiresAt  int64 // Unix timestamp in nanoseconds
	CreatedAt  int64
	AccessedAt int64
	HitCount   int64
}

// Cache provides a thread-safe LRU cache for HTTP responses
type Cache struct {
	entries   sync.Map // map[string]*Entry
	maxSize   int64
	maxAge    time.Duration
	currentSize int64
	mu        sync.RWMutex
}

// Config defines cache configuration
type Config struct {
	MaxSize int64         // Maximum cache size in bytes
	MaxAge  time.Duration // Maximum age of cached entries
}

// DefaultConfig returns default cache configuration
func DefaultConfig() *Config {
	return &Config{
		MaxSize: 100 * 1024 * 1024, // 100MB
		MaxAge:  5 * time.Minute,
	}
}

// New creates a new cache
func New(config *Config) *Cache {
	if config == nil {
		config = DefaultConfig()
	}

	c := &Cache{
		maxSize: config.MaxSize,
		maxAge:  config.MaxAge,
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

// GenerateKey generates a cache key from request parameters
func GenerateKey(method, url string, headers map[string]string) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte(url))
	
	// Include relevant headers in key generation
	for k, v := range headers {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}
	
	return hex.EncodeToString(h.Sum(nil))
}

// Get retrieves a cached entry
func (c *Cache) Get(key string) (*Entry, bool) {
	value, ok := c.entries.Load(key)
	if !ok {
		return nil, false
	}

	entry := value.(*Entry)

	// Check if entry has expired
	now := time.Now().UnixNano()
	if entry.ExpiresAt > 0 && entry.ExpiresAt < now {
		c.entries.Delete(key)
		return nil, false
	}

	// Update access time and hit count
	entry.AccessedAt = now
	entry.HitCount++

	return entry, true
}

// Set stores an entry in the cache
func (c *Cache) Set(key string, value []byte, headers map[string][]string, statusCode int, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	entrySize := int64(len(value))
	if c.currentSize+entrySize > c.maxSize {
		c.evictLRU(entrySize)
	}

	now := time.Now().UnixNano()
	expiresAt := int64(0)
	if ttl > 0 {
		expiresAt = now + ttl.Nanoseconds()
	} else if c.maxAge > 0 {
		expiresAt = now + c.maxAge.Nanoseconds()
	}

	entry := &Entry{
		Key:        key,
		Value:      value,
		Headers:    headers,
		StatusCode: statusCode,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		AccessedAt: now,
		HitCount:   0,
	}

	c.entries.Store(key, entry)
	c.currentSize += entrySize

	return nil
}

// Delete removes an entry from the cache
func (c *Cache) Delete(key string) {
	if value, ok := c.entries.LoadAndDelete(key); ok {
		entry := value.(*Entry)
		c.mu.Lock()
		c.currentSize -= int64(len(entry.Value))
		c.mu.Unlock()
	}
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries.Range(func(key, value interface{}) bool {
		c.entries.Delete(key)
		return true
	})
	c.currentSize = 0
}

// evictLRU evicts least recently used entries to make space
func (c *Cache) evictLRU(needed int64) {
	type entryWithKey struct {
		key       string
		entry     *Entry
		size      int64
	}

	var entries []entryWithKey

	// Collect all entries
	c.entries.Range(func(key, value interface{}) bool {
		k := key.(string)
		e := value.(*Entry)
		entries = append(entries, entryWithKey{
			key:   k,
			entry: e,
			size:  int64(len(e.Value)),
		})
		return true
	})

	// Sort by access time (oldest first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].entry.AccessedAt > entries[j].entry.AccessedAt {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Evict entries until we have enough space
	freed := int64(0)
	for _, e := range entries {
		if c.currentSize-freed+needed <= c.maxSize {
			break
		}
		c.entries.Delete(e.key)
		freed += e.size
	}

	c.currentSize -= freed
}

// cleanupLoop periodically removes expired entries
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries
func (c *Cache) cleanup() {
	now := time.Now().UnixNano()
	var toDelete []string

	c.entries.Range(func(key, value interface{}) bool {
		entry := value.(*Entry)
		if entry.ExpiresAt > 0 && entry.ExpiresAt < now {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, key := range toDelete {
		c.Delete(key)
	}
}

// Stats returns cache statistics
type Stats struct {
	Entries     int64
	Size        int64
	MaxSize     int64
	HitRate     float64
	TotalHits   int64
	TotalMisses int64
}

// GetStats returns current cache statistics
func (c *Cache) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalHits int64
	var entryCount int64

	c.entries.Range(func(key, value interface{}) bool {
		entry := value.(*Entry)
		totalHits += entry.HitCount
		entryCount++
		return true
	})

	stats := Stats{
		Entries:   entryCount,
		Size:      c.currentSize,
		MaxSize:   c.maxSize,
		TotalHits: totalHits,
	}

	return stats
}

