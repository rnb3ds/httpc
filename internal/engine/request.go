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
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/cybergodev/httpc/internal/types"
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

// pooledStringsReader wraps a strings.Reader and returns it to the pool on EOF
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
	}
	return n, err
}

// pooledBytesReader wraps a bytes.Reader and returns it to the pool on EOF
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
	}
	return n, err
}

// getPooledStringsReader gets a strings.Reader from the pool and wraps it
func getPooledStringsReader(s string) io.Reader {
	reader, ok := stringsReaderPool.Get().(*strings.Reader)
	if !ok || reader == nil {
		reader = &strings.Reader{}
	}
	reader.Reset(s)
	return &pooledStringsReader{reader: reader}
}

// getPooledBytesReader gets a bytes.Reader from the pool and wraps it
func getPooledBytesReader(b []byte) io.Reader {
	reader, ok := bytesReaderPool.Get().(*bytes.Reader)
	if !ok || reader == nil {
		reader = &bytes.Reader{}
	}
	reader.Reset(b)
	return &pooledBytesReader{reader: reader}
}

// urlCache provides a thread-safe LRU-like cache for parsed URLs
// to avoid expensive url.Parse() calls for repeated URLs
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

// Get retrieves a parsed URL from cache or parses and caches it
func (c *urlCache) Get(rawURL string) (*url.URL, error) {
	// Fast path: read lock for cache hit
	c.mu.RLock()
	if parsed, ok := c.entries[rawURL]; ok {
		c.mu.RUnlock()
		return parsed, nil
	}
	c.mu.RUnlock()

	// Slow path: parse and cache
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if parsed, ok := c.entries[rawURL]; ok {
		return parsed, nil
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
	}

	// Clone the URL to ensure cached entries are immutable
	// This prevents modifications from affecting cached values
	cloned := cloneURL(parsed)
	c.entries[rawURL] = cloned
	c.keys = append(c.keys, rawURL)

	return cloned, nil
}

// cloneURL creates a deep copy of a URL to ensure cached entries remain immutable
func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	cloned := &url.URL{
		Scheme:      u.Scheme,
		Opaque:      u.Opaque,
		User:        u.User,
		Host:        u.Host,
		Path:        u.Path,
		RawPath:     u.RawPath,
		OmitHost:    u.OmitHost,
		ForceQuery:  u.ForceQuery,
		RawQuery:    u.RawQuery,
		Fragment:    u.Fragment,
		RawFragment: u.RawFragment,
	}
	return cloned
}

// ClearURLCache clears the global URL cache to release memory.
// This is useful for long-running applications that want to free memory
// when the URL patterns change or during low-activity periods.
// Thread-safe: can be called concurrently with cache operations.
func ClearURLCache() {
	globalURLCache.clear()
}

// GetURLCacheSize returns the current number of entries in the URL cache.
// Useful for monitoring cache usage in production environments.
func GetURLCacheSize() int {
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

// ClearRequestPools clears all sync.Pool instances used by the request package.
// This is primarily useful for testing and debugging to ensure a clean state.
// Note: sync.Pool is automatically managed by the GC, so this is typically not needed
// in production code. The pools will be repopulated on next use.
func ClearRequestPools() {
	stringsReaderPool = sync.Pool{
		New: func() any { return &strings.Reader{} },
	}
	bytesReaderPool = sync.Pool{
		New: func() any { return &bytes.Reader{} },
	}
	stringBuilderPool = sync.Pool{
		New: func() any {
			sb := &strings.Builder{}
			return sb
		},
	}
}

// FormData and FileData are now defined in internal/types package.
// Use types.FormData and types.FileData for type checking.

type RequestProcessor struct {
	config *Config
}

func NewRequestProcessor(config *Config) *RequestProcessor {
	return &RequestProcessor{
		config: config,
	}
}

func (p *RequestProcessor) Build(req *Request) (*http.Request, error) {
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
		// Clone the cached URL before modifying it with query params
		// This ensures the cached version remains immutable
		parsedURL = cloneURL(parsedURL)
		query := parsedURL.Query()
		for key, value := range req.QueryParams() {
			query.Add(key, formatQueryParam(value))
		}
		parsedURL.RawQuery = query.Encode()
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
			} else if isFormData(v) {
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)

				fieldsVal := reflect.ValueOf(v).Elem().FieldByName("Fields")
				filesVal := reflect.ValueOf(v).Elem().FieldByName("Files")

				if fieldsVal.IsValid() && fieldsVal.Kind() == reflect.Map {
					for _, key := range fieldsVal.MapKeys() {
						value := fieldsVal.MapIndex(key).String()
						if err := writer.WriteField(key.String(), value); err != nil {
							return nil, fmt.Errorf("write form field failed: %w", err)
						}
					}
				}

				if filesVal.IsValid() && filesVal.Kind() == reflect.Map {
					for _, key := range filesVal.MapKeys() {
						fileDataValue := filesVal.MapIndex(key)
						if !fileDataValue.IsValid() || fileDataValue.IsNil() {
							continue
						}
						fileDataElem := fileDataValue.Elem()

						filename := ""
						var content []byte
						contentType := ""

						if f := fileDataElem.FieldByName("Filename"); f.IsValid() && f.Kind() == reflect.String {
							filename = f.String()
						}
						if f := fileDataElem.FieldByName("Content"); f.IsValid() && f.Kind() == reflect.Slice {
							content = f.Bytes()
						}
						if f := fileDataElem.FieldByName("ContentType"); f.IsValid() && f.Kind() == reflect.String {
							contentType = f.String()
						}

						var part io.Writer
						var err error

						if contentType != "" {
							h := make(textproto.MIMEHeader)
							h.Set("Content-Disposition",
								fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
									escapeQuotes(key.String()), escapeQuotes(filename)))
							h.Set("Content-Type", contentType)
							part, err = writer.CreatePart(h)
						} else {
							part, err = writer.CreateFormFile(key.String(), filename)
						}

						if err != nil {
							return nil, fmt.Errorf("create form file failed: %w", err)
						}

						if _, err := part.Write(content); err != nil {
							return nil, fmt.Errorf("write file content failed: %w", err)
						}
					}
				}

				if err := writer.Close(); err != nil {
					return nil, fmt.Errorf("close multipart writer failed: %w", err)
				}

				body = &buf
				contentType = writer.FormDataContentType()
			} else {
				jsonData, err := json.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("marshal JSON failed: %w", err)
				}
				body = getPooledBytesReader(jsonData)
				contentType = "application/json"
			}
		}
	}

	httpReq, err := http.NewRequest(req.Method(), parsedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request failed: %w", err)
	}

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

// formatQueryParam converts a value to string for query parameters.
// Optimized to avoid fmt.Sprintf allocations for common types.
func formatQueryParam(v any) string {
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

func isFormData(v any) bool {
	if v == nil {
		return false
	}
	// Check if it's a pointer to types.FormData
	if _, ok := v.(*types.FormData); ok {
		return true
	}
	// Fallback to reflection for compatible types from different packages
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Ptr {
		return false
	}
	t = t.Elem()
	if t.Kind() != reflect.Struct {
		return false
	}
	if t.Name() != "FormData" {
		return false
	}
	_, hasFields := t.FieldByName("Fields")
	_, hasFiles := t.FieldByName("Files")
	return hasFields && hasFiles
}
