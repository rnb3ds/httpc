package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"sync"
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

	// SECURITY: defaultMaxDecompressedSize is the default limit for decompressed
	// response body size when MaxResponseBodySize is not explicitly configured.
	// This provides a safety net against compression bombs where 100MB of
	// highly compressible data (e.g., zeros) could decompress to many gigabytes.
	defaultMaxDecompressedSize = 500 * 1024 * 1024 // 500MB decompressed data limit
)

// bufferPool reuses byte buffers for response body reading
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, defaultBufferSize))
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

// getResponse retrieves a Response object from the pool.
// SECURITY: Resets all fields to zero values to prevent data leakage from previous requests.
func getResponse() *Response {
	resp, ok := responsePool.Get().(*Response)
	if !ok || resp == nil {
		return &Response{}
	}
	// SECURITY: Clear all fields to prevent sensitive data leakage
	*resp = Response{}
	return resp
}

type responseProcessor struct {
	config *Config
}

func newResponseProcessor(config *Config) *responseProcessor {
	return &responseProcessor{
		config: config,
	}
}

func (p *responseProcessor) Process(httpResp *http.Response) (*Response, error) {
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
	resp.SetRawBody(body)
	// Body string is lazily converted on first access via Body() to avoid
	// doubling memory when caller only uses RawBody
	resp.SetContentLength(contentLength)
	resp.SetProto(httpResp.Proto)
	// Only parse cookies when Set-Cookie header is present to avoid unnecessary allocation
	if _, ok := httpResp.Header["Set-Cookie"]; ok {
		resp.SetCookies(httpResp.Cookies())
	}

	return resp, nil
}

// readBody reads and optionally decompresses the response body with size limits.
// Uses buffer and limit reader pools to reduce heap allocations.
//
// # SECURITY CONTRACT
//
// This function MUST return a freshly allocated []byte.
// The returned slice must not be retained by any other reference (pool or shared buffer).
//
// SECURITY: Implements protection against decompression bomb attacks.
func (p *responseProcessor) readBody(httpResp *http.Response) ([]byte, error) {
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

	// SECURITY: Apply decompressed size limit using pooled reader
	// Use MaxDecompressedBodySize if set, else MaxResponseBodySize, else default
	maxSize := p.config.MaxDecompressedBodySize
	if maxSize <= 0 {
		maxSize = p.config.MaxResponseBodySize
		if maxSize <= 0 {
			maxSize = defaultMaxDecompressedSize
		}
	}
	decompressedLr = getLimitReader(reader, maxSize+1)
	reader = decompressedLr

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

	// SECURITY: After decompression, check body size for zip bomb protection.
	// The compressed input is already limited by compressedLr above.
	// This catches cases where small compressed payloads decompress to excessively large output.
	if isCompressed && int64(len(body)) > maxCompressedSize {
		return nil, fmt.Errorf("decompressed response body exceeds security limit of %d bytes (potential zip bomb)", maxCompressedSize)
	}

	// Check decompressed size limit (uses same maxSize calculation as limit reader above)
	if int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	// Optimization path for small responses (most common case)
	// For directly allocated buffers (fromPool=false), just return the bytes directly
	// For pooled buffers within steal threshold, steal the backing array
	if len(body) <= bufferStealThreshold {
		if !fromPool {
			return body, nil
		}
		// stealThreshold: for bodies under 4KB, copy is cheaper than
		// allocating a 4KB replacement buffer. Steal for larger ones.
		if len(body) <= defaultBufferSize/2 {
			result := make([]byte, len(body))
			copy(result, body)
			return result, nil
		}
		// Steal: detach buffer from pool and return backing array directly
		result := body
		buf = nil
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
func (p *responseProcessor) createDecompressor(reader io.Reader, encoding string) (io.ReadCloser, error) {
	switch encoding {
	case "gzip":
		// Try to get a pooled gzip reader
		if pooled, ok := gzipReaderPool.Get().(*gzip.Reader); ok && pooled != nil {
			if err := pooled.Reset(reader); err != nil {
				// SECURITY: Reset failed - discard the reader instead of returning to pool.
				// A reader in error state may cause issues for subsequent users.
				// The discarded reader will be garbage collected.
				return gzip.NewReader(reader)
			}
			wrapper, _ := gzipReaderWrapperPool.Get().(*pooledGzipReader)
			if wrapper == nil {
				wrapper = &pooledGzipReader{}
			}
			wrapper.Reader = pooled
			return wrapper, nil
		}
		return gzip.NewReader(reader)
	case "deflate":
		// Try to get a pooled flate reader
		if pooled, ok := flateReaderPool.Get().(io.ReadCloser); ok && pooled != nil {
			// Check if it also implements flate.Resetter
			if resetter, ok := pooled.(flate.Resetter); ok {
				if err := resetter.Reset(reader, nil); err != nil {
					// SECURITY: Reset failed - discard the reader instead of returning to pool.
					// A reader in error state may cause issues for subsequent users.
					// The discarded reader will be garbage collected.
					return flate.NewReader(reader), nil
				}
				wrapper, _ := flateReaderWrapperPool.Get().(*pooledFlateReader)
				if wrapper == nil {
					wrapper = &pooledFlateReader{}
				}
				wrapper.reader = pooled
				return wrapper, nil
			}
			// Doesn't implement Resetter, discard and create new
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

// gzipReaderWrapperPool reduces allocations for the pooledGzipReader wrapper struct.
var gzipReaderWrapperPool = sync.Pool{
	New: func() any { return &pooledGzipReader{} },
}

func (r *pooledGzipReader) Close() error {
	if r.Reader == nil {
		return nil
	}
	err := r.Reader.Close()
	// Reset to nil reader for safety before returning to pool
	_ = r.Reader.Reset(bytes.NewReader(nil)) // reset before returning to pool
	gzipReaderPool.Put(r.Reader)
	r.Reader = nil
	// Return wrapper to pool
	gzipReaderWrapperPool.Put(r)
	return err
}

// pooledFlateReader wraps a pooled flate reader and returns it to the pool on Close.
// flate.NewReader returns an io.ReadCloser that also implements flate.Resetter.
type pooledFlateReader struct {
	reader io.ReadCloser
}

// flateReaderWrapperPool reduces allocations for the pooledFlateReader wrapper struct.
var flateReaderWrapperPool = sync.Pool{
	New: func() any { return &pooledFlateReader{} },
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
		_ = resetter.Reset(bytes.NewReader(nil), nil) // reset before returning to pool
		flateReaderPool.Put(resetter)
	} else {
		// SECURITY: If the reader doesn't implement Resetter, close it directly
		// to prevent resource leaks. This shouldn't happen with standard library,
		// but we handle it defensively for custom implementations.
		_ = r.reader.Close()
	}
	r.reader = nil
	// Return wrapper to pool
	flateReaderWrapperPool.Put(r)
	return nil
}

// ReleaseResponse returns a Response to the pool for reuse.
// Call this when the Response data has been consumed and copied elsewhere.
// After calling this, the Response must not be used.
func ReleaseResponse(r *Response) {
	if r == nil {
		return
	}
	if r.rawBodyReader != nil {
		_ = r.rawBodyReader.Close()
	}
	if r.cancelFunc != nil {
		r.cancelFunc()
	}
	*r = Response{}
	responsePool.Put(r)
}

// clearResponsePools clears all sync.Pool instances used by the response package.
// This is primarily useful for testing and debugging to ensure a clean state.
// Note: sync.Pool is automatically managed by the GC, so this is typically not needed
// in production code. The pools will be repopulated on next use.
func clearResponsePools() {
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
