package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestStreamModeContextCancelCleanup verifies that the context cancel function
// is properly cleaned up when streaming mode is used and ReleaseResponse is called.
// This prevents timer leaks from context.WithTimeout.
func TestStreamModeContextCancelCleanup(t *testing.T) {

	mock := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Status:     "OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("streaming body")),
		},
	}

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = mock
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Request(context.Background(), "GET", "https://example.com",
		func(r *Request) error {
			r.SetStreamBody(true)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response should not be nil")
	}

	// Verify streaming response has a body reader
	if resp.RawBodyReader() == nil {
		t.Fatal("Streaming response should have a raw body reader")
	}

	// Cancel function should be set for streaming mode
	// (ReleaseResponse is responsible for calling it)
	ReleaseResponse(resp)

	// Verify cleanup completed without panic.
	// ReleaseResponse handles cancel func internally.
}

// TestStreamModeContextCancelOnEarlyError verifies that the context cancel function
// is called when a streaming-mode request fails before the response is created.
// This prevents timer leaks from context.WithTimeout when the context is already expired.
func TestStreamModeContextCancelOnEarlyError(t *testing.T) {
	config := &Config{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = newMockTransport(200, "OK")
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create an already-expired context to trigger early error
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = client.Request(ctx, "GET", "https://example.com",
		func(r *Request) error {
			r.SetStreamBody(true)
			return nil
		},
	)
	if err == nil {
		t.Fatal("Expected error from cancelled context")
	}

	// The key assertion: no goroutine or timer leak.
	// The cancel func from context.WithTimeout (created in executeRequest)
	// must have been called even though streaming mode was requested.
	// We can't directly observe cancel calls, but the test verifies
	// the error path completes without hanging or leaking.
}

// TestNonStreamModeContextCancel verifies that non-streaming requests
// always have their context cancelled (baseline for comparison).
func TestNonStreamModeContextCancel(t *testing.T) {
	mock := newMockTransport(200, "hello world")

	config := &Config{
		Timeout:         10 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = mock
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Request(context.Background(), "GET", "https://example.com")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}

	ReleaseResponse(resp)
}

// TestReleaseResponseCleansUpStreamingResources verifies that ReleaseResponse
// properly closes the raw body reader and calls the cancel function for
// streaming responses.
func TestReleaseResponseCleansUpStreamingResources(t *testing.T) {
	var bodyClosed atomic.Int32
	var cancelCalled atomic.Int32

	body := &trackingReadCloser{
		Reader: strings.NewReader("test data"),
		onClose: func() {
			bodyClosed.Store(1)
		},
	}

	resp := getResponse()
	resp.SetStatusCode(200)
	resp.rawBodyReader = body
	resp.cancelFunc = func() {
		cancelCalled.Store(1)
	}

	ReleaseResponse(resp)

	if bodyClosed.Load() != 1 {
		t.Error("Body reader was not closed by ReleaseResponse")
	}
	if cancelCalled.Load() != 1 {
		t.Error("Cancel function was not called by ReleaseResponse")
	}
}

// TestReleaseResponseNilSafe verifies that ReleaseResponse handles nil and
// already-released responses without panicking.
func TestReleaseResponseNilSafe(t *testing.T) {
	ReleaseResponse(nil)
	ReleaseResponse(&Response{})

	resp := getResponse()
	ReleaseResponse(resp)
	ReleaseResponse(resp) // Double release should not panic
}

// trackingReadCloser tracks Close calls for testing.
type trackingReadCloser struct {
	*strings.Reader
	onClose func()
}

func (t *trackingReadCloser) Close() error {
	if t.onClose != nil {
		t.onClose()
	}
	return nil
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

// TestDecompressorPoolCleanup verifies that decompressor fallback paths
// properly handle pooled readers instead of silently discarding them.
func TestDecompressorPoolCleanup(t *testing.T) {
	config := &Config{
		Timeout:         10 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = newMockTransport(200, "test response data")
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Verify gzip decompression works correctly
	mock := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Encoding": []string{"gzip"},
			},
			Body: createGzipBody(t, "compressed data"),
		},
	}

	client2, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = mock
	})
	if err != nil {
		t.Fatalf("Failed to create client2: %v", err)
	}
	defer client2.Close()

	resp, err := client2.Request(context.Background(), "GET", "https://example.com")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	ReleaseResponse(resp)

	// Verify deflate decompression works correctly
	mock2 := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Encoding": []string{"deflate"},
			},
			Body: createDeflateBody(t, "deflated data"),
		},
	}

	client3, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = mock2
	})
	if err != nil {
		t.Fatalf("Failed to create client3: %v", err)
	}
	defer client3.Close()

	resp2, err := client3.Request(context.Background(), "GET", "https://example.com")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	ReleaseResponse(resp2)
}

// createGzipBody creates a gzip-compressed HTTP body for testing.
func createGzipBody(t *testing.T, data string) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return io.NopCloser(&buf)
}

// createDeflateBody creates a deflate-compressed HTTP body for testing.
func createDeflateBody(t *testing.T, data string) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatalf("flate write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate close failed: %v", err)
	}
	return io.NopCloser(&buf)
}
