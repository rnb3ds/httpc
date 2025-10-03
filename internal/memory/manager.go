package memory

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Manager provides intelligent memory management with monitoring and optimization
type Manager struct {
	// Buffer pools for different sizes
	smallBuffers  *BufferPool // 1KB - 8KB
	mediumBuffers *BufferPool // 8KB - 64KB
	largeBuffers  *BufferPool // 64KB - 512KB

	// Object pools
	headerPools   *HeaderPool
	requestPools  *RequestPool
	responsePools *ResponsePool

	// Memory statistics
	stats *Stats

	// Configuration
	config *Config

	// Lifecycle
	closed int32
	ticker *time.Ticker
	done   chan struct{}
}

// Config defines memory manager configuration
type Config struct {
	// Buffer pool sizes
	SmallBufferSize  int // Default: 4KB
	MediumBufferSize int // Default: 32KB
	LargeBufferSize  int // Default: 256KB

	// Pool limits
	MaxSmallBuffers  int // Default: 1000
	MaxMediumBuffers int // Default: 500
	MaxLargeBuffers  int // Default: 100

	// Cleanup intervals
	CleanupInterval time.Duration // Default: 30s

	// Memory pressure thresholds
	MemoryPressureThreshold float64 // Default: 0.8 (80% of available memory)
	GCTriggerThreshold      float64 // Default: 0.9 (90% of available memory)
}

// Stats provides memory usage statistics
type Stats struct {
	// Buffer pool statistics
	SmallBuffersInUse  int64
	MediumBuffersInUse int64
	LargeBuffersInUse  int64
	SmallBuffersTotal  int64
	MediumBuffersTotal int64
	LargeBuffersTotal  int64

	// Object pool statistics
	HeadersInUse   int64
	RequestsInUse  int64
	ResponsesInUse int64
	HeadersTotal   int64
	RequestsTotal  int64
	ResponsesTotal int64

	// Memory statistics
	AllocatedBytes int64
	SystemBytes    int64
	GCCycles       int64
	LastGCTime     int64

	// Performance metrics
	BufferHitRate  float64
	ObjectHitRate  float64
	MemoryPressure float64

	// Timestamps
	LastUpdate int64
}

// BufferPool manages buffers of a specific size range
type BufferPool struct {
	pool     sync.Pool
	size     int
	maxCount int64
	inUse    int64
	total    int64
}

// HeaderPool manages header map objects
type HeaderPool struct {
	pool  sync.Pool
	inUse int64
	total int64
}

// RequestPool manages request objects
type RequestPool struct {
	pool  sync.Pool
	inUse int64
	total int64
}

// ResponsePool manages response objects
type ResponsePool struct {
	pool  sync.Pool
	inUse int64
	total int64
}

// DefaultConfig returns default memory manager configuration
func DefaultConfig() *Config {
	return &Config{
		SmallBufferSize:         4 * 1024,   // 4KB
		MediumBufferSize:        32 * 1024,  // 32KB
		LargeBufferSize:         256 * 1024, // 256KB
		MaxSmallBuffers:         1000,
		MaxMediumBuffers:        500,
		MaxLargeBuffers:         100,
		CleanupInterval:         30 * time.Second,
		MemoryPressureThreshold: 0.8,
		GCTriggerThreshold:      0.9,
	}
}

// NewManager creates a new memory manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config: config,
		stats:  &Stats{},
		done:   make(chan struct{}),
	}

	// Initialize buffer pools
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

	// Initialize object pools
	m.headerPools = &HeaderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[string]string, 8)
			},
		},
	}

	m.requestPools = &RequestPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &PooledRequest{
					Headers:     make(map[string]string, 8),
					QueryParams: make(map[string]interface{}, 4),
				}
			},
		},
	}

	m.responsePools = &ResponsePool{
		pool: sync.Pool{
			New: func() interface{} {
				return &PooledResponse{}
			},
		},
	}

	// Start background cleanup
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

// PutBuffer returns a buffer to the appropriate pool
func (m *Manager) PutBuffer(buf []byte) {
	if buf == nil {
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
		// Not from our pools, just let GC handle it
		return
	}

	// Check if we should return to pool based on current usage
	if atomic.LoadInt64(&pool.inUse) < pool.maxCount {
		atomic.AddInt64(&pool.inUse, -1)
		pool.pool.Put(buf[:cap(buf)]) // Reset length to capacity
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

	// Clear the map but keep capacity
	for k := range headers {
		delete(headers, k)
	}

	atomic.AddInt64(&m.headerPools.inUse, -1)
	atomic.AddInt64(&m.stats.HeadersInUse, -1)
	m.headerPools.pool.Put(headers)
}

// PooledRequest represents a pooled request object
type PooledRequest struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]interface{}
	Body        interface{}
	Timeout     time.Duration
	MaxRetries  int
	Context     interface{} // context.Context
}

// Reset clears the request for reuse
func (r *PooledRequest) Reset() {
	r.Method = ""
	r.URL = ""
	r.Body = nil
	r.Timeout = 0
	r.MaxRetries = 0
	r.Context = nil

	// Clear maps but keep capacity
	for k := range r.Headers {
		delete(r.Headers, k)
	}
	for k := range r.QueryParams {
		delete(r.QueryParams, k)
	}
}

// PooledResponse represents a pooled response object
type PooledResponse struct {
	StatusCode    int
	Status        string
	Headers       map[string][]string
	Body          string
	RawBody       []byte
	ContentLength int64
	Proto         string
	Duration      time.Duration
	Attempts      int
}

// Reset clears the response for reuse
func (r *PooledResponse) Reset() {
	r.StatusCode = 0
	r.Status = ""
	r.Headers = nil
	r.Body = ""
	r.RawBody = nil
	r.ContentLength = 0
	r.Proto = ""
	r.Duration = 0
	r.Attempts = 0
}

// cleanupLoop performs periodic cleanup and memory pressure monitoring
func (m *Manager) cleanupLoop() {
	for {
		// Check if manager is closed before waiting
		if atomic.LoadInt32(&m.closed) == 1 {
			return
		}

		select {
		case <-m.ticker.C:
			// Double-check closed state after ticker fires
			if atomic.LoadInt32(&m.closed) == 1 {
				return
			}
			m.performCleanup()
		case <-m.done:
			return
		}
	}
}

// performCleanup performs memory cleanup and monitoring
func (m *Manager) performCleanup() {
	// Update memory statistics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	atomic.StoreInt64(&m.stats.AllocatedBytes, int64(memStats.Alloc))
	atomic.StoreInt64(&m.stats.SystemBytes, int64(memStats.Sys))
	atomic.StoreInt64(&m.stats.GCCycles, int64(memStats.NumGC))
	atomic.StoreInt64(&m.stats.LastGCTime, int64(memStats.LastGC))
	atomic.StoreInt64(&m.stats.LastUpdate, time.Now().Unix())

	// Calculate memory pressure
	memoryPressure := float64(memStats.Alloc) / float64(memStats.Sys)
	m.stats.MemoryPressure = memoryPressure

	// Trigger GC if memory pressure is high
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
		RequestsInUse:      atomic.LoadInt64(&m.stats.RequestsInUse),
		ResponsesInUse:     atomic.LoadInt64(&m.stats.ResponsesInUse),
		AllocatedBytes:     atomic.LoadInt64(&m.stats.AllocatedBytes),
		SystemBytes:        atomic.LoadInt64(&m.stats.SystemBytes),
		GCCycles:           atomic.LoadInt64(&m.stats.GCCycles),
		LastGCTime:         atomic.LoadInt64(&m.stats.LastGCTime),
		MemoryPressure:     m.stats.MemoryPressure,
		LastUpdate:         atomic.LoadInt64(&m.stats.LastUpdate),
	}
}

// GetPooledRequest returns a pooled request object
func (m *Manager) GetPooledRequest() *PooledRequest {
	atomic.AddInt64(&m.requestPools.inUse, 1)
	atomic.AddInt64(&m.stats.RequestsInUse, 1)
	return m.requestPools.pool.Get().(*PooledRequest)
}

// PutPooledRequest returns a pooled request object
func (m *Manager) PutPooledRequest(req *PooledRequest) {
	if req == nil {
		return
	}

	req.Reset()
	atomic.AddInt64(&m.requestPools.inUse, -1)
	atomic.AddInt64(&m.stats.RequestsInUse, -1)
	m.requestPools.pool.Put(req)
}

// GetPooledResponse returns a pooled response object
func (m *Manager) GetPooledResponse() *PooledResponse {
	atomic.AddInt64(&m.responsePools.inUse, 1)
	atomic.AddInt64(&m.stats.ResponsesInUse, 1)
	return m.responsePools.pool.Get().(*PooledResponse)
}

// PutPooledResponse returns a pooled response object
func (m *Manager) PutPooledResponse(resp *PooledResponse) {
	if resp == nil {
		return
	}

	resp.Reset()
	atomic.AddInt64(&m.responsePools.inUse, -1)
	atomic.AddInt64(&m.stats.ResponsesInUse, -1)
	m.responsePools.pool.Put(resp)
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
