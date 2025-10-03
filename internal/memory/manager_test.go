package memory

import (
	"sync"
	"testing"
	"time"
)

// ============================================================================
// MEMORY MANAGER UNIT TESTS
// ============================================================================

func TestManager_New(t *testing.T) {
	t.Run("With default config", func(t *testing.T) {
		m := NewManager(nil)
		defer m.Close()

		if m.config == nil {
			t.Error("Config should not be nil")
		}

		if m.smallBuffers == nil {
			t.Error("Small buffers pool should not be nil")
		}

		if m.mediumBuffers == nil {
			t.Error("Medium buffers pool should not be nil")
		}

		if m.largeBuffers == nil {
			t.Error("Large buffers pool should not be nil")
		}

		if m.stats == nil {
			t.Error("Stats should not be nil")
		}
	})

	t.Run("With custom config", func(t *testing.T) {
		config := &Config{
			SmallBufferSize:  2 * 1024,
			MediumBufferSize: 16 * 1024,
			LargeBufferSize:  128 * 1024,
			MaxSmallBuffers:  500,
			MaxMediumBuffers: 250,
			MaxLargeBuffers:  50,
			CleanupInterval:  10 * time.Second,
		}

		m := NewManager(config)
		defer m.Close()

		if m.config.SmallBufferSize != 2*1024 {
			t.Errorf("Expected SmallBufferSize 2048, got %d", m.config.SmallBufferSize)
		}

		if m.smallBuffers.size != 2*1024 {
			t.Errorf("Expected small buffer size 2048, got %d", m.smallBuffers.size)
		}
	})
}

func TestManager_GetBuffer_SmallBuffer(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	size := 2 * 1024 // 2KB
	buf := m.GetBuffer(size)

	if buf == nil {
		t.Fatal("Buffer should not be nil")
	}

	if cap(buf) < size {
		t.Errorf("Expected buffer capacity >= %d, got %d", size, cap(buf))
	}

	stats := m.GetStats()
	if stats.SmallBuffersInUse != 1 {
		t.Errorf("Expected 1 small buffer in use, got %d", stats.SmallBuffersInUse)
	}

	// Return buffer
	m.PutBuffer(buf)

	stats = m.GetStats()
	if stats.SmallBuffersInUse != 0 {
		t.Errorf("Expected 0 small buffers in use after return, got %d", stats.SmallBuffersInUse)
	}
}

func TestManager_GetBuffer_MediumBuffer(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	size := 16 * 1024 // 16KB
	buf := m.GetBuffer(size)

	if buf == nil {
		t.Fatal("Buffer should not be nil")
	}

	if cap(buf) < size {
		t.Errorf("Expected buffer capacity >= %d, got %d", size, cap(buf))
	}

	stats := m.GetStats()
	if stats.MediumBuffersInUse != 1 {
		t.Errorf("Expected 1 medium buffer in use, got %d", stats.MediumBuffersInUse)
	}
}

func TestManager_GetBuffer_LargeBuffer(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	size := 128 * 1024 // 128KB
	buf := m.GetBuffer(size)

	if buf == nil {
		t.Fatal("Buffer should not be nil")
	}

	if cap(buf) < size {
		t.Errorf("Expected buffer capacity >= %d, got %d", size, cap(buf))
	}

	stats := m.GetStats()
	if stats.LargeBuffersInUse != 1 {
		t.Errorf("Expected 1 large buffer in use, got %d", stats.LargeBuffersInUse)
	}
}

func TestManager_GetBuffer_VeryLarge(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	size := 1024 * 1024 // 1MB - too large for pooling
	buf := m.GetBuffer(size)

	if buf == nil {
		t.Fatal("Buffer should not be nil")
	}

	if len(buf) != size {
		t.Errorf("Expected buffer length %d, got %d", size, len(buf))
	}

	// This should not affect pool stats
	stats := m.GetStats()
	if stats.SmallBuffersInUse != 0 || stats.MediumBuffersInUse != 0 || stats.LargeBuffersInUse != 0 {
		t.Error("Very large buffer should not use pools")
	}
}

func TestManager_PutBuffer_Nil(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	// Should not panic
	m.PutBuffer(nil)
}

func TestManager_PutBuffer_WrongSize(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	// Create a buffer with non-standard size
	buf := make([]byte, 1234)

	// Should not panic, just won't be returned to pool
	m.PutBuffer(buf)

	stats := m.GetStats()
	if stats.SmallBuffersInUse != 0 {
		t.Error("Non-pooled buffer should not affect stats")
	}
}

func TestManager_GetHeaders(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	headers := m.GetHeaders()

	if headers == nil {
		t.Fatal("Headers should not be nil")
	}

	stats := m.GetStats()
	if stats.HeadersInUse != 1 {
		t.Errorf("Expected 1 header in use, got %d", stats.HeadersInUse)
	}

	// Add some data
	headers["Content-Type"] = "application/json"
	headers["Authorization"] = "Bearer token"

	// Return headers
	m.PutHeaders(headers)

	// Should be cleared
	if len(headers) != 0 {
		t.Errorf("Headers should be cleared, got %d entries", len(headers))
	}

	stats = m.GetStats()
	if stats.HeadersInUse != 0 {
		t.Errorf("Expected 0 headers in use after return, got %d", stats.HeadersInUse)
	}
}

func TestManager_GetPooledRequest(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	req := m.GetPooledRequest()

	if req == nil {
		t.Fatal("Request should not be nil")
	}

	if req.Headers == nil {
		t.Error("Request headers should not be nil")
	}

	if req.QueryParams == nil {
		t.Error("Request query params should not be nil")
	}

	stats := m.GetStats()
	if stats.RequestsInUse != 1 {
		t.Errorf("Expected 1 request in use, got %d", stats.RequestsInUse)
	}

	// Set some data
	req.Method = "POST"
	req.URL = "https://example.com"
	req.Headers["Content-Type"] = "application/json"

	// Return request
	m.PutPooledRequest(req)

	// Should be reset
	if req.Method != "" {
		t.Error("Request method should be reset")
	}

	if req.URL != "" {
		t.Error("Request URL should be reset")
	}

	if len(req.Headers) != 0 {
		t.Error("Request headers should be cleared")
	}

	stats = m.GetStats()
	if stats.RequestsInUse != 0 {
		t.Errorf("Expected 0 requests in use after return, got %d", stats.RequestsInUse)
	}
}

func TestManager_GetPooledResponse(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	resp := m.GetPooledResponse()

	if resp == nil {
		t.Fatal("Response should not be nil")
	}

	stats := m.GetStats()
	if stats.ResponsesInUse != 1 {
		t.Errorf("Expected 1 response in use, got %d", stats.ResponsesInUse)
	}

	// Set some data
	resp.StatusCode = 200
	resp.Status = "OK"
	resp.Body = "test body"

	// Return response
	m.PutPooledResponse(resp)

	// Should be reset
	if resp.StatusCode != 0 {
		t.Error("Response status code should be reset")
	}

	if resp.Status != "" {
		t.Error("Response status should be reset")
	}

	if resp.Body != "" {
		t.Error("Response body should be reset")
	}

	stats = m.GetStats()
	if stats.ResponsesInUse != 0 {
		t.Errorf("Expected 0 responses in use after return, got %d", stats.ResponsesInUse)
	}
}

func TestManager_ConcurrentBufferAccess(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	numGoroutines := 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get and return buffers
			buf := m.GetBuffer(2 * 1024)
			time.Sleep(1 * time.Millisecond)
			m.PutBuffer(buf)
		}()
	}

	wg.Wait()

	stats := m.GetStats()
	if stats.SmallBuffersInUse != 0 {
		t.Errorf("Expected 0 buffers in use after concurrent access, got %d", stats.SmallBuffersInUse)
	}
}

func TestManager_GetStats(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()

	// Use various resources
	buf1 := m.GetBuffer(2 * 1024)
	buf2 := m.GetBuffer(16 * 1024)
	headers := m.GetHeaders()
	req := m.GetPooledRequest()

	stats := m.GetStats()

	if stats.SmallBuffersInUse != 1 {
		t.Errorf("Expected 1 small buffer in use, got %d", stats.SmallBuffersInUse)
	}

	if stats.MediumBuffersInUse != 1 {
		t.Errorf("Expected 1 medium buffer in use, got %d", stats.MediumBuffersInUse)
	}

	if stats.HeadersInUse != 1 {
		t.Errorf("Expected 1 header in use, got %d", stats.HeadersInUse)
	}

	if stats.RequestsInUse != 1 {
		t.Errorf("Expected 1 request in use, got %d", stats.RequestsInUse)
	}

	// Return resources
	m.PutBuffer(buf1)
	m.PutBuffer(buf2)
	m.PutHeaders(headers)
	m.PutPooledRequest(req)

	stats = m.GetStats()

	if stats.SmallBuffersInUse != 0 {
		t.Errorf("Expected 0 small buffers in use, got %d", stats.SmallBuffersInUse)
	}

	if stats.MediumBuffersInUse != 0 {
		t.Errorf("Expected 0 medium buffers in use, got %d", stats.MediumBuffersInUse)
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager(nil)

	err := m.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got: %v", err)
	}

	// Close again should be idempotent
	err = m.Close()
	if err != nil {
		t.Errorf("Expected no error on double close, got: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.SmallBufferSize != 4*1024 {
		t.Errorf("Expected SmallBufferSize 4096, got %d", config.SmallBufferSize)
	}

	if config.MediumBufferSize != 32*1024 {
		t.Errorf("Expected MediumBufferSize 32768, got %d", config.MediumBufferSize)
	}

	if config.LargeBufferSize != 256*1024 {
		t.Errorf("Expected LargeBufferSize 262144, got %d", config.LargeBufferSize)
	}

	if config.CleanupInterval != 30*time.Second {
		t.Errorf("Expected CleanupInterval 30s, got %v", config.CleanupInterval)
	}
}

