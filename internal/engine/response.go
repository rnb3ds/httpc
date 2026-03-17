package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"sync"
	"unsafe"
)

// gzipReaderPool pools gzip.Reader objects to reduce allocations during decompression.
// Each gzip.NewReader allocates a reader struct and internal buffers.
var gzipReaderPool = sync.Pool{
	New: func() any {
		// Create a dummy reader to initialize the pool
		// The actual reader will be reset with the real source
		reader, _ := gzip.NewReader(bytes.NewReader(nil))
		return reader
	},
}

// flateReaderPool pools flate.Reader objects (flate.Resetter interface).
// flate.NewReader returns an io.ReadCloser that can be reset.
var flateReaderPool = sync.Pool{
	New: func() any {
		// Create a dummy reader to initialize the pool
		return flate.NewReader(bytes.NewReader(nil))
	},
}

const (
	// defaultBufferSize is the initial size for buffer pool buffers
	defaultBufferSize = 4 * 1024 // 4KB - good balance for most responses
	// maxBufferSize caps the buffer size to prevent memory bloat
	maxBufferSize = 512 * 1024 // 512KB
	// bufferStealThreshold is the size below which we "steal" the buffer
	// instead of copying, reducing allocations for small responses
	bufferStealThreshold = 16 * 1024 // 16KB

	// SECURITY: maxCompressedSize limits the size of compressed response data
	// to prevent decompression bomb (zip bomb) attacks. A highly compressed
	// malicious payload could exhaust memory during decompression.
	maxCompressedSize = 100 * 1024 * 1024 // 100MB compressed data limit

	// smallBufferThreshold is the threshold for using pre-allocated small buffers
	// This avoids pool overhead for very small responses
	smallBufferThreshold = 512 // 512 bytes
)

// bufferPool reuses byte buffers for response body reading
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, defaultBufferSize))
	},
}

// smallBufferPool provides pre-allocated small byte slices for tiny responses
// This avoids the overhead of the buffer pool for very small responses
var smallBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, smallBufferThreshold)
		return &buf
	},
}

// responsePool reuses Response objects to reduce allocations in the hot path
var responsePool = sync.Pool{
	New: func() any {
		return &Response{}
	},
}

// limitReaderPool reduces allocations for limit readers
var limitReaderPool = sync.Pool{
	New: func() any {
		return &pooledLimitReader{}
	},
}

// pooledLimitReader is a reusable io.Reader that limits the number of bytes read
type pooledLimitReader struct {
	r io.Reader
	n int64
}

func (l *pooledLimitReader) Read(p []byte) (n int, err error) {
	if l.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.n {
		p = p[0:l.n]
	}
	n, err = l.r.Read(p)
	l.n -= int64(n)
	return
}

func (l *pooledLimitReader) Reset(r io.Reader, n int64) {
	l.r = r
	l.n = n
}

// getLimitReader retrieves a pooledLimitReader from the pool
func getLimitReader(r io.Reader, n int64) *pooledLimitReader {
	lr, ok := limitReaderPool.Get().(*pooledLimitReader)
	if !ok || lr == nil {
		lr = &pooledLimitReader{}
	}
	lr.Reset(r, n)
	return lr
}

// putLimitReader returns a pooledLimitReader to the pool
func putLimitReader(lr *pooledLimitReader) {
	if lr == nil {
		return
	}
	lr.r = nil
	lr.n = 0
	limitReaderPool.Put(lr)
}

// getBuffer retrieves a buffer from the pool with safe type assertion.
// Returns a new buffer if the pool contains an unexpected type (defensive).
func getBuffer() *bytes.Buffer {
	pooled := bufferPool.Get()
	buf, ok := pooled.(*bytes.Buffer)
	if !ok || buf == nil {
		// Defensive: create new buffer if pool returns wrong type
		return bytes.NewBuffer(make([]byte, 0, defaultBufferSize))
	}
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool if it's not too large
func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() <= maxBufferSize {
		bufferPool.Put(buf)
	}
	// Let large buffers be garbage collected to prevent memory bloat
}

// getResponse retrieves a Response object from the pool
func getResponse() *Response {
	resp, ok := responsePool.Get().(*Response)
	if !ok || resp == nil {
		return &Response{}
	}
	return resp
}

type ResponseProcessor struct {
	config *Config
}

func NewResponseProcessor(config *Config) *ResponseProcessor {
	return &ResponseProcessor{
		config: config,
	}
}

func (p *ResponseProcessor) Process(httpResp *http.Response) (*Response, error) {
	if httpResp == nil {
		return nil, fmt.Errorf("HTTP response is nil")
	}

	wasCompressed := httpResp.Header.Get("Content-Encoding") != ""

	body, err := p.readBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	contentLength := httpResp.ContentLength
	// Strict content-length validation: skip for HEAD requests (no body expected)
	// and compressed responses (body size differs from Content-Length header)
	if !wasCompressed && p.config.StrictContentLength && contentLength > 0 && contentLength != int64(len(body)) {
		// Safe nil check with short-circuit evaluation before accessing Method
		if httpResp.Request == nil || httpResp.Request.Method != "HEAD" {
			return nil, fmt.Errorf("content-length mismatch: expected %d, got %d", contentLength, len(body))
		}
	}

	if wasCompressed {
		contentLength = int64(len(body))
	}

	// Use pooled Response object to reduce allocations
	resp := getResponse()
	resp.SetStatusCode(httpResp.StatusCode)
	resp.SetStatus(httpResp.Status)
	resp.SetHeaders(httpResp.Header)
	// SECURITY: readBody returns a freshly allocated copy (not pooled buffer),
	// so zero-copy string conversion is safe here.
	resp.SetBody(bytesToString(body))
	resp.SetRawBody(body)
	resp.SetContentLength(contentLength)
	resp.SetProto(httpResp.Proto)
	resp.SetCookies(httpResp.Cookies())

	return resp, nil
}

// bytesToString performs a zero-allocation conversion from []byte to string.
// SAFE because the input slice must be a fresh copy that will not be modified.
// IMPORTANT: The input slice must be newly allocated (not from a pool) and must
// not be modified after this function returns, as the returned string shares
// the same underlying memory.
func bytesToString(b []byte) string {
	// Handle nil slice and empty slice to prevent panic
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// readBody reads and optionally decompresses the response body with size limits.
// Uses buffer and limit reader pools to reduce heap allocations.
// SECURITY: Implements protection against decompression bomb attacks.
func (p *ResponseProcessor) readBody(httpResp *http.Response) ([]byte, error) {
	if httpResp.Body == nil {
		return nil, nil
	}

	reader := io.Reader(httpResp.Body)
	isCompressed := false
	var compressedLr *pooledLimitReader
	var decompressedLr *pooledLimitReader
	var decompressor io.ReadCloser // Track decompressor for cleanup

	if encoding := httpResp.Header.Get("Content-Encoding"); encoding != "" {
		isCompressed = true
		var err error
		// SECURITY: Limit compressed data size before decompression to prevent zip bombs
		compressedLr = getLimitReader(httpResp.Body, maxCompressedSize+1)
		decompressor, err = p.createDecompressor(compressedLr, encoding)
		if err != nil {
			putLimitReader(compressedLr)
			return nil, fmt.Errorf("failed to create decompressor for %s: %w", encoding, err)
		}
		reader = decompressor
	}

	// Apply decompressed size limit using pooled reader
	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 {
		decompressedLr = getLimitReader(reader, maxSize+1)
		reader = decompressedLr
	}

	// Optimization: Use Content-Length hint for buffer pre-allocation when available
	// This reduces buffer growth overhead for responses with known size
	contentLength := httpResp.ContentLength
	var buf *bytes.Buffer
	fromPool := false // Track if buffer came from pool

	// For responses with known content length that fit in stolen threshold,
	// allocate directly to avoid pool overhead
	if !isCompressed && contentLength > 0 && contentLength <= int64(bufferStealThreshold) {
		// Direct allocation: we know the exact size needed
		body := make([]byte, 0, contentLength)
		buf = bytes.NewBuffer(body)
	} else {
		buf = getBuffer()
		fromPool = true
	}

	defer func() {
		// Only return buffer to pool if it came from pool and we're not stealing it
		if fromPool && buf != nil && buf.Cap() <= maxBufferSize {
			putBuffer(buf)
		}
		if decompressor != nil {
			_ = decompressor.Close() // Close returns pooled reader to pool
		}
		if compressedLr != nil {
			putLimitReader(compressedLr)
		}
		if decompressedLr != nil {
			putLimitReader(decompressedLr)
		}
	}()

	_, err := io.Copy(buf, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	body := buf.Bytes()

	// SECURITY: Check compressed size limit for zip bomb protection
	if isCompressed && len(body) > maxCompressedSize {
		return nil, fmt.Errorf("compressed response body exceeds security limit of %d bytes (potential zip bomb)", maxCompressedSize)
	}

	// Check decompressed size limit
	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 && int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	// Optimization path for small responses (most common case)
	// For directly allocated buffers (fromPool=false), just return the bytes directly
	// For pooled buffers within steal threshold, steal the backing array
	if len(body) <= bufferStealThreshold {
		if !fromPool {
			// Buffer was directly allocated, return it directly (no copy needed)
			return body, nil
		}
		// Buffer came from pool - steal the backing array
		// Set buf to nil so defer doesn't try to return it
		result := body
		buf = nil // Prevent defer from returning buffer to pool
		// Put a fresh buffer into the pool to replace the one we're stealing
		bufferPool.Put(bytes.NewBuffer(make([]byte, 0, defaultBufferSize)))
		return result, nil
	}

	// For larger responses, copy to avoid holding large buffers
	result := make([]byte, len(body))
	copy(result, body)
	return result, nil
}

// createDecompressor creates an appropriate decompressor based on the encoding type.
// Uses pooled readers for gzip and deflate to reduce allocations.
func (p *ResponseProcessor) createDecompressor(reader io.Reader, encoding string) (io.ReadCloser, error) {
	switch encoding {
	case "gzip":
		// Try to get a pooled gzip reader
		if pooled, ok := gzipReaderPool.Get().(*gzip.Reader); ok && pooled != nil {
			if err := pooled.Reset(reader); err != nil {
				// Reset failed, put back and create new
				gzipReaderPool.Put(pooled)
				return gzip.NewReader(reader)
			}
			return &pooledGzipReader{Reader: pooled}, nil
		}
		return gzip.NewReader(reader)
	case "deflate":
		// Try to get a pooled flate reader
		if pooled, ok := flateReaderPool.Get().(io.ReadCloser); ok && pooled != nil {
			// Check if it also implements flate.Resetter
			if resetter, ok := pooled.(flate.Resetter); ok {
				if err := resetter.Reset(reader, nil); err != nil {
					// Reset failed, put back and create new
					flateReaderPool.Put(pooled)
					return flate.NewReader(reader), nil
				}
				return &pooledFlateReader{reader: pooled}, nil
			}
			// Doesn't implement Resetter, put back and create new
			flateReaderPool.Put(pooled)
		}
		return flate.NewReader(reader), nil
	case "br":
		return nil, fmt.Errorf("brotli decompression not supported")
	case "compress", "x-compress":
		return nil, fmt.Errorf("LZW compression not supported")
	case "identity", "":
		return io.NopCloser(reader), nil
	default:
		return io.NopCloser(reader), nil
	}
}

// pooledGzipReader wraps a pooled gzip.Reader and returns it to the pool on Close.
type pooledGzipReader struct {
	*gzip.Reader
}

func (r *pooledGzipReader) Close() error {
	if r.Reader == nil {
		return nil
	}
	err := r.Reader.Close()
	// Reset to nil reader for safety before returning to pool
	r.Reader.Reset(bytes.NewReader(nil))
	gzipReaderPool.Put(r.Reader)
	r.Reader = nil
	return err
}

// pooledFlateReader wraps a pooled flate reader and returns it to the pool on Close.
// flate.NewReader returns an io.ReadCloser that also implements flate.Resetter.
type pooledFlateReader struct {
	reader io.ReadCloser
}

func (r *pooledFlateReader) Read(p []byte) (n int, err error) {
	if r.reader == nil {
		return 0, io.EOF
	}
	return r.reader.Read(p)
}

func (r *pooledFlateReader) Close() error {
	if r.reader == nil {
		return nil
	}
	// Get the Resetter interface to reset and return to pool
	if resetter, ok := r.reader.(flate.Resetter); ok {
		resetter.Reset(bytes.NewReader(nil), nil)
		flateReaderPool.Put(resetter)
	} else {
		// SECURITY: If the reader doesn't implement Resetter, close it directly
		// to prevent resource leaks. This shouldn't happen with standard library,
		// but we handle it defensively for custom implementations.
		_ = r.reader.Close()
	}
	r.reader = nil
	return nil
}

// ClearResponsePools clears all sync.Pool instances used by the response package.
// This is primarily useful for testing and debugging to ensure a clean state.
// Note: sync.Pool is automatically managed by the GC, so this is typically not needed
// in production code. The pools will be repopulated on next use.
func ClearResponsePools() {
	gzipReaderPool = sync.Pool{
		New: func() any {
			reader, _ := gzip.NewReader(bytes.NewReader(nil))
			return reader
		},
	}
	flateReaderPool = sync.Pool{
		New: func() any {
			return flate.NewReader(bytes.NewReader(nil))
		},
	}
	bufferPool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, defaultBufferSize))
		},
	}
	smallBufferPool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, smallBufferThreshold)
			return &buf
		},
	}
	responsePool = sync.Pool{
		New: func() any {
			return &Response{}
		},
	}
	limitReaderPool = sync.Pool{
		New: func() any {
			return &pooledLimitReader{}
		},
	}
}
