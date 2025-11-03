package memory

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Manager struct {
	smallBuffers  *BufferPool
	mediumBuffers *BufferPool
	largeBuffers  *BufferPool

	headerPools *HeaderPool

	stats *Stats

	config *Config

	closed int32
	ticker *time.Ticker
	done   chan struct{}
}

type Config struct {
	SmallBufferSize  int
	MediumBufferSize int
	LargeBufferSize  int

	MaxSmallBuffers  int
	MaxMediumBuffers int
	MaxLargeBuffers  int

	CleanupInterval time.Duration

	MemoryPressureThreshold float64
	GCTriggerThreshold      float64
}

type Stats struct {
	SmallBuffersInUse  int64
	MediumBuffersInUse int64
	LargeBuffersInUse  int64
	SmallBuffersTotal  int64
	MediumBuffersTotal int64
	LargeBuffersTotal  int64

	HeadersInUse int64
	HeadersTotal int64

	AllocatedBytes int64
	SystemBytes    int64
	GCCycles       int64
	LastGCTime     int64

	BufferHitRate  float64
	ObjectHitRate  float64
	MemoryPressure float64

	LastUpdate int64
}

type BufferPool struct {
	pool     sync.Pool
	size     int
	maxCount int64
	inUse    int64
	total    int64
}

type HeaderPool struct {
	pool  sync.Pool
	inUse int64
	total int64
}

func DefaultConfig() *Config {
	return &Config{
		SmallBufferSize:         4 * 1024,
		MediumBufferSize:        32 * 1024,
		LargeBufferSize:         256 * 1024,
		MaxSmallBuffers:         1000,
		MaxMediumBuffers:        500,
		MaxLargeBuffers:         100,
		CleanupInterval:         30 * time.Second,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
}

func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config: config,
		stats:  &Stats{},
		done:   make(chan struct{}),
	}

	m.smallBuffers = &BufferPool{
		size:     config.SmallBufferSize,
		maxCount: int64(config.MaxSmallBuffers),
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, config.SmallBufferSize)
			},
		},
	}

	m.mediumBuffers = &BufferPool{
		size:     config.MediumBufferSize,
		maxCount: int64(config.MaxMediumBuffers),
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, config.MediumBufferSize)
			},
		},
	}

	m.largeBuffers = &BufferPool{
		size:     config.LargeBufferSize,
		maxCount: int64(config.MaxLargeBuffers),
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, config.LargeBufferSize)
			},
		},
	}

	m.headerPools = &HeaderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[string]string, 8)
			},
		},
	}

	m.ticker = time.NewTicker(config.CleanupInterval)
	go m.cleanupLoop()

	return m
}

// GetBuffer returns a buffer of appropriate size
func (m *Manager) GetBuffer(size int) []byte {
	var pool *BufferPool

	switch {
	case size <= m.config.SmallBufferSize:
		pool = m.smallBuffers
		atomic.AddInt64(&m.stats.SmallBuffersInUse, 1)
	case size <= m.config.MediumBufferSize:
		pool = m.mediumBuffers
		atomic.AddInt64(&m.stats.MediumBuffersInUse, 1)
	case size <= m.config.LargeBufferSize:
		pool = m.largeBuffers
		atomic.AddInt64(&m.stats.LargeBuffersInUse, 1)
	default:
		// Size too large for pooling, allocate directly
		return make([]byte, size)
	}

	atomic.AddInt64(&pool.inUse, 1)
	return pool.pool.Get().([]byte)
}

func (m *Manager) PutBuffer(buf []byte) {
	if buf == nil {
		return
	}

	if atomic.LoadInt32(&m.closed) == 1 {
		return
	}

	size := cap(buf)
	var pool *BufferPool

	switch {
	case size == m.config.SmallBufferSize:
		pool = m.smallBuffers
		atomic.AddInt64(&m.stats.SmallBuffersInUse, -1)
	case size == m.config.MediumBufferSize:
		pool = m.mediumBuffers
		atomic.AddInt64(&m.stats.MediumBuffersInUse, -1)
	case size == m.config.LargeBufferSize:
		pool = m.largeBuffers
		atomic.AddInt64(&m.stats.LargeBuffersInUse, -1)
	default:
		return
	}

	atomic.AddInt64(&pool.inUse, -1)

	if atomic.LoadInt64(&pool.inUse) < pool.maxCount {
		pool.pool.Put(buf[:0])
	}
}

// GetHeaders returns a header map from the pool
func (m *Manager) GetHeaders() map[string]string {
	atomic.AddInt64(&m.headerPools.inUse, 1)
	atomic.AddInt64(&m.stats.HeadersInUse, 1)
	return m.headerPools.pool.Get().(map[string]string)
}

// PutHeaders returns a header map to the pool
func (m *Manager) PutHeaders(headers map[string]string) {
	if headers == nil {
		return
	}

	// Check if manager is closed
	if atomic.LoadInt32(&m.closed) == 1 {
		return
	}

	// Clear the map but keep capacity
	for k := range headers {
		delete(headers, k)
	}

	atomic.AddInt64(&m.headerPools.inUse, -1)
	atomic.AddInt64(&m.stats.HeadersInUse, -1)

	// Only return to pool if we're not over capacity
	if atomic.LoadInt64(&m.headerPools.inUse) < 1000 { // Reasonable limit
		m.headerPools.pool.Put(headers)
	}
}

func (m *Manager) cleanupLoop() {
	defer m.performCleanup()

	for {
		select {
		case <-m.ticker.C:
			if atomic.LoadInt32(&m.closed) == 1 {
				return
			}
			m.performCleanup()
		case <-m.done:
			return
		}
	}
}

func (m *Manager) performCleanup() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	atomic.StoreInt64(&m.stats.AllocatedBytes, int64(memStats.Alloc))
	atomic.StoreInt64(&m.stats.SystemBytes, int64(memStats.Sys))
	atomic.StoreInt64(&m.stats.GCCycles, int64(memStats.NumGC))
	atomic.StoreInt64(&m.stats.LastGCTime, int64(memStats.LastGC))
	atomic.StoreInt64(&m.stats.LastUpdate, time.Now().Unix())

	memoryPressure := float64(memStats.Alloc) / float64(memStats.Sys)
	m.stats.MemoryPressure = memoryPressure

	if memoryPressure > m.config.GCTriggerThreshold {
		runtime.GC()
	}
}

// GetStats returns current memory statistics
func (m *Manager) GetStats() Stats {
	return Stats{
		SmallBuffersInUse:  atomic.LoadInt64(&m.stats.SmallBuffersInUse),
		MediumBuffersInUse: atomic.LoadInt64(&m.stats.MediumBuffersInUse),
		LargeBuffersInUse:  atomic.LoadInt64(&m.stats.LargeBuffersInUse),
		SmallBuffersTotal:  atomic.LoadInt64(&m.smallBuffers.total),
		MediumBuffersTotal: atomic.LoadInt64(&m.mediumBuffers.total),
		LargeBuffersTotal:  atomic.LoadInt64(&m.largeBuffers.total),
		HeadersInUse:       atomic.LoadInt64(&m.stats.HeadersInUse),
		AllocatedBytes:     atomic.LoadInt64(&m.stats.AllocatedBytes),
		SystemBytes:        atomic.LoadInt64(&m.stats.SystemBytes),
		GCCycles:           atomic.LoadInt64(&m.stats.GCCycles),
		LastGCTime:         atomic.LoadInt64(&m.stats.LastGCTime),
		MemoryPressure:     m.stats.MemoryPressure,
		LastUpdate:         atomic.LoadInt64(&m.stats.LastUpdate),
	}
}

// Close shuts down the memory manager
func (m *Manager) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return nil // Already closed
	}

	close(m.done)
	m.ticker.Stop()

	return nil
}
