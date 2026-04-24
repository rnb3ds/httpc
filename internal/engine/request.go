package engine

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/cybergodev/httpc/internal/types"
	"github.com/cybergodev/httpc/internal/validation"
)

// stringsReaderPool reduces allocations for strings.Reader used in request bodies
var stringsReaderPool = sync.Pool{
	New: func() any { return &strings.Reader{} },
}

// bytesReaderPool reduces allocations for bytes.Reader used in request bodies
var bytesReaderPool = sync.Pool{
	New: func() any { return &bytes.Reader{} },
}

// stringBuilderPool reduces allocations for strings.Builder used in escapeQuotes
var stringBuilderPool = sync.Pool{
	New: func() any {
		sb := &strings.Builder{}
		return sb
	},
}

// maxMultipartBufferSize limits the maximum buffer size returned to the pool
// to prevent memory bloat from large file uploads (256KB)
const maxMultipartBufferSize = 256 * 1024

// maxJSONBufferSize limits the maximum buffer size for JSON encoding (1MB)
const maxJSONBufferSize = 1024 * 1024

// mimeHeaderPool reduces allocations for textproto.MIMEHeader in multipart uploads
var mimeHeaderPool = sync.Pool{
	New: func() any {
		h := make(textproto.MIMEHeader, 4) // Pre-allocate for typical multipart headers
		return &h
	},
}

// getMIMEHeader retrieves a textproto.MIMEHeader from the pool
func getMIMEHeader() *textproto.MIMEHeader {
	h, ok := mimeHeaderPool.Get().(*textproto.MIMEHeader)
	if !ok || h == nil {
		tmp := make(textproto.MIMEHeader, 4)
		return &tmp
	}
	// Clear for reuse
	for k := range *h {
		delete(*h, k)
	}
	return h
}

// putMIMEHeader returns a textproto.MIMEHeader to the pool
func putMIMEHeader(h *textproto.MIMEHeader) {
	if h == nil || len(*h) > 16 {
		return // Don't pool large headers
	}
	// Clear values for GC and security
	for k, v := range *h {
		for i := range v {
			v[i] = ""
		}
		delete(*h, k)
	}
	mimeHeaderPool.Put(h)
}

// multipartBufferPool reduces allocations for multipart form data buffers
var multipartBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 8*1024))
	},
}

// jsonBufferPool reduces allocations for JSON encoding buffers
var jsonBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 512))
	},
}

// pooledStringsReader wraps a strings.Reader and returns it to the pool on EOF or Close.
type pooledStringsReader struct {
	reader *strings.Reader
}

func (r *pooledStringsReader) Read(p []byte) (n int, err error) {
	// SAFETY: Check for nil reader to prevent panic after EOF
	// io.Reader contract allows multiple reads after EOF
	if r.reader == nil {
		return 0, io.EOF
	}
	n, err = r.reader.Read(p)
	if err == io.EOF {
		// Reset and return to pool when fully read
		r.reader.Reset("")
		stringsReaderPool.Put(r.reader)
		r.reader = nil
		stringsReaderWrapperPool.Put(r)
	}
	return n, err
}

func (r *pooledStringsReader) Close() error {
	if r.reader != nil {
		r.reader.Reset("")
		stringsReaderPool.Put(r.reader)
		r.reader = nil
	}
	stringsReaderWrapperPool.Put(r)
	return nil
}

// pooledBytesReader wraps a bytes.Reader and returns it to the pool on EOF or Close.
type pooledBytesReader struct {
	reader *bytes.Reader
}

func (r *pooledBytesReader) Read(p []byte) (n int, err error) {
	// SAFETY: Check for nil reader to prevent panic after EOF
	// io.Reader contract allows multiple reads after EOF
	if r.reader == nil {
		return 0, io.EOF
	}
	n, err = r.reader.Read(p)
	if err == io.EOF {
		// Reset and return to pool when fully read
		r.reader.Reset(nil)
		bytesReaderPool.Put(r.reader)
		r.reader = nil
		bytesReaderWrapperPool.Put(r)
	}
	return n, err
}

func (r *pooledBytesReader) Close() error {
	if r.reader != nil {
		r.reader.Reset(nil)
		bytesReaderPool.Put(r.reader)
		r.reader = nil
	}
	bytesReaderWrapperPool.Put(r)
	return nil
}

// stringsReaderWrapperPool reduces allocations for pooledStringsReader wrapper structs.
var stringsReaderWrapperPool = sync.Pool{
	New: func() any { return &pooledStringsReader{} },
}

// bytesReaderWrapperPool reduces allocations for pooledBytesReader wrapper structs.
var bytesReaderWrapperPool = sync.Pool{
	New: func() any { return &pooledBytesReader{} },
}

// getPooledStringsReader gets a strings.Reader from the pool and wraps it
func getPooledStringsReader(s string) io.Reader {
	reader, ok := stringsReaderPool.Get().(*strings.Reader)
	if !ok || reader == nil {
		reader = &strings.Reader{}
	}
	reader.Reset(s)
	wrapper, _ := stringsReaderWrapperPool.Get().(*pooledStringsReader)
	if wrapper == nil {
		wrapper = &pooledStringsReader{}
	}
	wrapper.reader = reader
	return wrapper
}

// getPooledBytesReader gets a bytes.Reader from the pool and wraps it
func getPooledBytesReader(b []byte) io.Reader {
	reader, ok := bytesReaderPool.Get().(*bytes.Reader)
	if !ok || reader == nil {
		reader = &bytes.Reader{}
	}
	reader.Reset(b)
	wrapper, _ := bytesReaderWrapperPool.Get().(*pooledBytesReader)
	if wrapper == nil {
		wrapper = &pooledBytesReader{}
	}
	wrapper.reader = reader
	return wrapper
}

// urlCache provides a thread-safe LRU-like cache for parsed URLs
// to avoid expensive url.Parse() calls for repeated URLs.
//
// SECURITY: The cache uses a sanitized cache key (without sensitive query parameters)
// to prevent credential leakage. The actual parsed URL (with all parameters) is still
// returned to the caller, but the cache key excludes sensitive data.
type urlCache struct {
	mu      sync.RWMutex
	entries map[string]*url.URL
	keys    []string // Track insertion order for LRU eviction
	maxSize int
}

// globalURLCache is the shared URL cache for all requests
var globalURLCache = &urlCache{
	entries: make(map[string]*url.URL, 256),
	keys:    make([]string, 0, 256),
	maxSize: 1024,
}

// sanitizeCacheKey creates a cache-safe version of the URL by removing sensitive
// query parameters. This prevents credentials from being stored in cache keys.
func sanitizeCacheKey(rawURL string) string {
	// Fast path: check if URL contains query parameters
	if !strings.Contains(rawURL, "?") {
		return rawURL // No query params, safe to use as-is
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL // Fallback to original on parse error
	}

	// Check if any sensitive params exist
	query := parsed.Query()
	hasSensitive := false
	for key := range query {
		if validation.IsSensitiveQueryParam(key) {
			hasSensitive = true
			break
		}
	}

	if !hasSensitive {
		return rawURL // No sensitive params, safe to use as-is
	}

	// Clone URL and remove sensitive params
	cloned := cloneURL(parsed)
	newQuery := cloned.Query()
	for key := range newQuery {
		if validation.IsSensitiveQueryParam(key) {
			newQuery.Set(key, "[REDACTED]")
		}
	}
	cloned.RawQuery = newQuery.Encode()
	return cloned.String()
}

// Get retrieves a parsed URL from cache or parses and caches it.
// SECURITY: Uses sanitized cache key to prevent credential leakage.
func (c *urlCache) Get(rawURL string) (*url.URL, error) {
	// SECURITY: Use sanitized key for cache lookup
	cacheKey := sanitizeCacheKey(rawURL)

	// Fast path: read lock for cache hit
	c.mu.RLock()
	if parsed, ok := c.entries[cacheKey]; ok {
		c.mu.RUnlock()
		// SECURITY: Return a clone to prevent modification of cached entry
		return cloneURL(parsed), nil
	}
	c.mu.RUnlock()

	// Slow path: parse and cache
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if parsed, ok := c.entries[cacheKey]; ok {
		return cloneURL(parsed), nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// SECURITY: Evict oldest entry if cache is full
	// Check len(c.keys) > 0 to prevent index out of bounds in race conditions
	if len(c.entries) >= c.maxSize && len(c.keys) > 0 {
		// Remove oldest key (simple FIFO eviction)
		oldestKey := c.keys[0]
		delete(c.entries, oldestKey)
		c.keys = c.keys[1:]
		// Compact backing array when capacity is significantly larger than length
		// to prevent unbounded memory growth under sustained unique URL traffic
		if cap(c.keys) > len(c.keys)*2 {
			newKeys := make([]string, len(c.keys), len(c.keys)*2)
			copy(newKeys, c.keys)
			c.keys = newKeys
		}
	}

	// Store the parsed URL directly — callers always receive a clone
	c.entries[cacheKey] = parsed
	c.keys = append(c.keys, cacheKey)

	return cloneURL(parsed), nil
}

// cloneURL creates a deep copy of a URL
// to ensure cached entries remain immutable
func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	clone := &url.URL{
		Scheme:      u.Scheme,
		Opaque:      u.Opaque,
		Host:        u.Host,
		Path:        u.Path,
		RawPath:     u.RawPath,
		OmitHost:    u.OmitHost,
		ForceQuery:  u.ForceQuery,
		RawQuery:    u.RawQuery,
		Fragment:    u.Fragment,
		RawFragment: u.RawFragment,
	}
	if u.User != nil {
		username := u.User.Username()
		password, hasPassword := u.User.Password()
		if hasPassword {
			clone.User = url.UserPassword(username, password)
		} else {
			clone.User = url.User(username)
		}
	}
	return clone
}

// clearURLCache clears the global URL cache to release memory.
// This is useful for long-running applications that want to free memory
// when the URL patterns change or during low-activity periods.
// Thread-safe: can be called concurrently with cache operations.
func clearURLCache() {
	globalURLCache.clear()
}

// getURLCacheSize returns the current number of entries in the URL cache.
// Useful for monitoring cache usage in production environments.
func getURLCacheSize() int {
	return globalURLCache.size()
}

// clear removes all entries from the cache
func (c *urlCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*url.URL, 256)
	c.keys = make([]string, 0, 256)
}

// size returns the current number of cached entries
func (c *urlCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// getMultipartBuffer gets a bytes.Buffer from the pool for multipart form data
func getMultipartBuffer() *bytes.Buffer {
	buf, ok := multipartBufferPool.Get().(*bytes.Buffer)
	if !ok || buf == nil {
		return bytes.NewBuffer(make([]byte, 0, 8*1024))
	}
	buf.Reset()
	return buf
}

// putMultipartBuffer returns a bytes.Buffer to the pool.
// SECURITY: Resets the buffer before returning to prevent data leakage.
// Buffers larger than maxMultipartBufferSize are discarded to prevent memory bloat.
func putMultipartBuffer(buf *bytes.Buffer) {
	if buf == nil || buf.Cap() > maxMultipartBufferSize {
		return // Discard large buffers
	}
	// SECURITY: Reset clears the buffer and allows GC to collect old data
	buf.Reset()
	multipartBufferPool.Put(buf)
}

// pooledMultipartBuffer wraps a bytes.Buffer and returns it to the pool when fully read.
// This enables buffer reuse for multipart form data without premature recycling.
type pooledMultipartBuffer struct {
	buf   *bytes.Buffer
	owned bool // Tracks if buffer still needs to be returned to pool
}

// multipartBufferWrapperPool reduces allocations for pooledMultipartBuffer wrapper structs.
var multipartBufferWrapperPool = sync.Pool{
	New: func() any { return &pooledMultipartBuffer{} },
}

// getPooledMultipartBufferWrapper creates a pooledMultipartBuffer from the pool.
func getPooledMultipartBufferWrapper(buf *bytes.Buffer) *pooledMultipartBuffer {
	wrapper, _ := multipartBufferWrapperPool.Get().(*pooledMultipartBuffer)
	if wrapper == nil {
		wrapper = &pooledMultipartBuffer{}
	}
	wrapper.buf = buf
	wrapper.owned = true
	return wrapper
}

func (r *pooledMultipartBuffer) Read(p []byte) (n int, err error) {
	if r.buf == nil {
		return 0, io.EOF
	}
	n, err = r.buf.Read(p)
	if err == io.EOF && r.owned {
		// Return to pool when fully read
		putMultipartBuffer(r.buf)
		r.buf = nil
		r.owned = false
		multipartBufferWrapperPool.Put(r)
	}
	return n, err
}

func (r *pooledMultipartBuffer) Close() error {
	if r.buf != nil && r.owned {
		putMultipartBuffer(r.buf)
		r.buf = nil
		r.owned = false
	}
	multipartBufferWrapperPool.Put(r)
	return nil
}

// getJSONBuffer retrieves a bytes.Buffer from the pool for JSON encoding.
func getJSONBuffer() *bytes.Buffer {
	buf, ok := jsonBufferPool.Get().(*bytes.Buffer)
	if !ok || buf == nil {
		return bytes.NewBuffer(make([]byte, 0, 512))
	}
	buf.Reset()
	return buf
}

// putJSONBuffer returns a bytes.Buffer to the JSON pool.
// SECURITY: Resets the buffer before returning to prevent data leakage.
// Buffers larger than maxJSONBufferSize are discarded to prevent memory bloat.
func putJSONBuffer(buf *bytes.Buffer) {
	if buf == nil || buf.Cap() > maxJSONBufferSize {
		return
	}
	buf.Reset()
	jsonBufferPool.Put(buf)
}

// pooledJSONBuffer wraps a bytes.Buffer for JSON data and returns it to the pool when fully read.
type pooledJSONBuffer struct {
	buf   *bytes.Buffer
	owned bool
}

// jsonBufferWrapperPool reduces allocations for pooledJSONBuffer wrapper structs.
var jsonBufferWrapperPool = sync.Pool{
	New: func() any { return &pooledJSONBuffer{} },
}

// getPooledJSONBufferWrapper creates a pooledJSONBuffer from the pool.
func getPooledJSONBufferWrapper(buf *bytes.Buffer) *pooledJSONBuffer {
	wrapper, _ := jsonBufferWrapperPool.Get().(*pooledJSONBuffer)
	if wrapper == nil {
		wrapper = &pooledJSONBuffer{}
	}
	wrapper.buf = buf
	wrapper.owned = true
	return wrapper
}

func (r *pooledJSONBuffer) Read(p []byte) (n int, err error) {
	if r.buf == nil {
		return 0, io.EOF
	}
	n, err = r.buf.Read(p)
	if err == io.EOF && r.owned {
		putJSONBuffer(r.buf)
		r.buf = nil
		r.owned = false
		jsonBufferWrapperPool.Put(r)
	}
	return n, err
}

func (r *pooledJSONBuffer) Close() error {
	if r.buf != nil && r.owned {
		putJSONBuffer(r.buf)
		r.buf = nil
		r.owned = false
	}
	jsonBufferWrapperPool.Put(r)
	return nil
}

type requestProcessor struct {
	config *Config
}

func newRequestProcessor(config *Config) *requestProcessor {
	return &requestProcessor{
		config: config,
	}
}

func (p *requestProcessor) Build(req *Request) (*http.Request, error) {
	if req.Method() == "" {
		req.SetMethod("GET")
	}

	if req.Context() == nil {
		req.SetContext(backgroundCtx)
	}

	// Use cached URL parsing to avoid expensive url.Parse() calls
	parsedURL, err := globalURLCache.Get(req.URL())
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if len(req.QueryParams()) > 0 {
		// parsedURL is already a clone from the cache, safe to modify directly.
		parsedURL.RawQuery = appendQueryParams(parsedURL.RawQuery, req.QueryParams())
	}

	var body io.Reader
	var contentType string

	if req.Body() != nil {
		switch v := req.Body().(type) {
		case string:
			body = getPooledStringsReader(v)
			contentType = "text/plain"
		case []byte:
			body = getPooledBytesReader(v)
			contentType = "application/octet-stream"
		case io.Reader:
			body = v
		default:
			existingContentType := ""
			if req.Headers() != nil {
				existingContentType = req.Headers()["Content-Type"]
			}

			if existingContentType == "application/xml" {
				xmlData, err := xml.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("marshal XML failed: %w", err)
				}
				body = getPooledBytesReader(xmlData)
				contentType = "application/xml"
			} else if fd, ok := v.(*types.FormData); ok {
				// Use pooled buffer for multipart form data
				buf := getMultipartBuffer()
				writer := multipart.NewWriter(buf)

				for key, value := range fd.Fields {
					if err := writer.WriteField(key, value); err != nil {
						putMultipartBuffer(buf)
						return nil, fmt.Errorf("write form field failed: %w", err)
					}
				}

				for key, fileData := range fd.Files {
					if fileData == nil {
						continue
					}

					var part io.Writer
					var err error

					if fileData.ContentType != "" {
						h := getMIMEHeader()
						sb, ok := stringBuilderPool.Get().(*strings.Builder)
						if !ok || sb == nil {
							sb = &strings.Builder{}
						}
						sb.Reset()
						escapedKey := escapeQuotes(key)
						escapedFilename := escapeQuotes(fileData.Filename)
						sb.Grow(21 + len(escapedKey) + 12 + len(escapedFilename) + 2)
						sb.WriteString(`form-data; name="`)
						sb.WriteString(escapedKey)
						sb.WriteString(`"; filename="`)
						sb.WriteString(escapedFilename)
						sb.WriteByte('"')
						contentDisposition := sb.String()
						stringBuilderPool.Put(sb)

						h.Set("Content-Disposition", contentDisposition)
						h.Set("Content-Type", fileData.ContentType)
						part, err = writer.CreatePart(*h)
						putMIMEHeader(h)
					} else {
						part, err = writer.CreateFormFile(key, fileData.Filename)
					}

					if err != nil {
						putMultipartBuffer(buf)
						return nil, fmt.Errorf("create form file failed: %w", err)
					}

					if _, err := part.Write(fileData.Content); err != nil {
						putMultipartBuffer(buf)
						return nil, fmt.Errorf("write file content failed: %w", err)
					}
				}

				if err := writer.Close(); err != nil {
					putMultipartBuffer(buf)
					return nil, fmt.Errorf("close multipart writer failed: %w", err)
				}

				body = getPooledMultipartBufferWrapper(buf)
				contentType = writer.FormDataContentType()
			} else {
				// Use pooled buffer for JSON encoding to reduce allocations
				buf := getJSONBuffer()
				encoder := json.NewEncoder(buf)
				if err := encoder.Encode(v); err != nil {
					putJSONBuffer(buf)
					return nil, fmt.Errorf("marshal JSON failed: %w", err)
				}
				// Trim trailing newline added by json.Encoder.Encode
				// to maintain compatibility with json.Marshal behavior
				if b := buf.Bytes(); len(b) > 0 && b[len(b)-1] == '\n' {
					buf.Truncate(len(b) - 1)
				}
				body = getPooledJSONBufferWrapper(buf)
				contentType = "application/json"
			}
		}
	}

	// Construct http.Request directly to avoid:
	//   1. parsedURL.String() allocation (URL to string)
	//   2. url.Parse re-parsing that string back to *url.URL
	//   3. io.NopCloser wrapper for body readers that implement io.ReadCloser
	headerSize := max(len(p.config.Headers)+len(req.Headers())+2, 8) // +2 for Content-Type, User-Agent

	var bodyRC io.ReadCloser
	if body != nil {
		if rc, ok := body.(io.ReadCloser); ok {
			bodyRC = rc
		} else {
			bodyRC = io.NopCloser(body)
		}
	}

	method := req.Method()
	httpReq := &http.Request{
		Method:     method,
		URL:        parsedURL,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header, headerSize),
		Body:       bodyRC,
		Host:       parsedURL.Host,
	}

	// Set Content-Length from known body types
	p.setContentLength(httpReq, body)

	httpReq = httpReq.WithContext(req.Context())

	if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	for key, value := range p.config.Headers {
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}

	for key, value := range req.Headers() {
		httpReq.Header.Set(key, value)
	}

	if httpReq.Header.Get("User-Agent") == "" && p.config.UserAgent != "" {
		httpReq.Header.Set("User-Agent", p.config.UserAgent)
	}

	// Add cookies to the request
	// Note: If EnableCookies is true and a CookieJar is configured,
	// the cookies will be managed by the jar automatically.
	// We still add them here for immediate use in this request.
	cookies := req.Cookies()
	for i := range cookies {
		httpReq.AddCookie(&cookies[i])
	}

	return httpReq, nil
}

// setContentLength sets Content-Length on the http.Request for known body types.
// This avoids the stdlib's reflection-based detection when constructing requests directly.
func (p *requestProcessor) setContentLength(req *http.Request, body io.Reader) {
	switch v := body.(type) {
	case *pooledStringsReader:
		if v.reader != nil {
			req.ContentLength = int64(v.reader.Len())
		}
	case *pooledBytesReader:
		if v.reader != nil {
			req.ContentLength = int64(v.reader.Len())
		}
	case *pooledJSONBuffer:
		if v.buf != nil {
			req.ContentLength = int64(v.buf.Len())
		}
	case *pooledMultipartBuffer:
		if v.buf != nil {
			req.ContentLength = int64(v.buf.Len())
		}
	}
}

// escapeQuotes escapes backslashes and double quotes in filenames per RFC 7578.
// Optimized to use pooled strings.Builder for better performance.
func escapeQuotes(s string) string {
	// Fast path: no escapes needed - use direct byte scanning
	var hasEscape bool
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' || s[i] == '"' {
			hasEscape = true
			break
		}
	}
	if !hasEscape {
		return s
	}

	// Slow path: build escaped string using pooled builder
	sb, ok := stringBuilderPool.Get().(*strings.Builder)
	if !ok || sb == nil {
		sb = &strings.Builder{}
	}
	sb.Reset()
	sb.Grow(len(s) + len(s)/10) // Pre-allocate ~10% extra for escapes

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			sb.WriteString("\\\\")
		case '"':
			sb.WriteString("\\\"")
		default:
			sb.WriteByte(s[i])
		}
	}

	result := sb.String()
	stringBuilderPool.Put(sb)
	return result
}

// FormatQueryParam converts a value to string for query parameters.
// Optimized to avoid fmt.Sprintf allocations for common types.
func FormatQueryParam(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case bool:
		return strconv.FormatBool(val)
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}
