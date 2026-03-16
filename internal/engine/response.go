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

	if encoding := httpResp.Header.Get("Content-Encoding"); encoding != "" {
		isCompressed = true
		var err error
		// SECURITY: Limit compressed data size before decompression to prevent zip bombs
		compressedLr = getLimitReader(httpResp.Body, maxCompressedSize+1)
		reader, err = p.createDecompressor(compressedLr, encoding)
		if err != nil {
			putLimitReader(compressedLr)
			return nil, fmt.Errorf("failed to create decompressor for %s: %w", encoding, err)
		}
	}

	// Apply decompressed size limit using pooled reader
	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 {
		decompressedLr = getLimitReader(reader, maxSize+1)
		reader = decompressedLr
	}

	// Use pooled buffer for reading
	buf := getBuffer()
	bufferStolen := false // Track if we stole the buffer to skip putBuffer

	defer func() {
		// Only return buffer to pool if we didn't steal it
		if !bufferStolen {
			putBuffer(buf)
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

	// Optimization: For small responses, "steal" the buffer directly
	// to avoid an extra allocation and copy. Create a fresh buffer for the pool.
	if len(body) <= bufferStealThreshold {
		// Mark buffer as stolen so defer skips putBuffer
		bufferStolen = true
		// Put a fresh buffer into the pool to replace the one we're stealing
		bufferPool.Put(bytes.NewBuffer(make([]byte, 0, defaultBufferSize)))
		// Return the stolen buffer directly (caller takes ownership)
		return body, nil
	}

	// For larger responses, copy to avoid holding large buffers
	result := make([]byte, len(body))
	copy(result, body)
	return result, nil
}

// createDecompressor creates an appropriate decompressor based on the encoding type.
func (p *ResponseProcessor) createDecompressor(reader io.Reader, encoding string) (io.Reader, error) {
	switch encoding {
	case "gzip":
		return gzip.NewReader(reader)
	case "deflate":
		return flate.NewReader(reader), nil
	case "br":
		return nil, fmt.Errorf("brotli decompression not supported")
	case "compress", "x-compress":
		return nil, fmt.Errorf("LZW compression not supported")
	case "identity", "":
		return reader, nil
	default:
		return reader, nil
	}
}
