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
)

// bufferPool reuses byte buffers for response body reading
var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, defaultBufferSize))
	},
}

// getBuffer retrieves a buffer from the pool
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
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

	resp := &Response{}
	resp.SetStatusCode(httpResp.StatusCode)
	resp.SetStatus(httpResp.Status)
	resp.SetHeaders(httpResp.Header)
	// Use zero-copy conversion for body string since body is already a fresh copy.
	// This saves one allocation compared to string(body).
	resp.SetBody(bytesToString(body))
	resp.SetRawBody(body)
	resp.SetContentLength(contentLength)
	resp.SetProto(httpResp.Proto)
	resp.SetCookies(httpResp.Cookies())

	return resp, nil
}

// bytesToString performs a zero-allocation conversion from []byte to string.
// SAFE because the input slice must be a fresh copy that will not be modified.
func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// readBody reads and optionally decompresses the response body with size limits.
// Uses a buffer pool to reduce heap allocations for response bodies.
func (p *ResponseProcessor) readBody(httpResp *http.Response) ([]byte, error) {
	if httpResp.Body == nil {
		return nil, nil
	}

	reader := io.Reader(httpResp.Body)

	if encoding := httpResp.Header.Get("Content-Encoding"); encoding != "" {
		var err error
		reader, err = p.createDecompressor(reader, encoding)
		if err != nil {
			return nil, fmt.Errorf("failed to create decompressor for %s: %w", encoding, err)
		}
	}

	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 {
		reader = io.LimitReader(reader, maxSize+1)
	}

	// Use pooled buffer for reading
	buf := getBuffer()
	defer putBuffer(buf)

	_, err := io.Copy(buf, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	body := buf.Bytes()

	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 && int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	// Must copy the bytes since buffer is returned to pool
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
