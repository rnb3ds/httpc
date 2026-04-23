package engine

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	// maxPooledHeaderSize limits the number of headers that will be pooled
	// to prevent memory bloat from large header sets
	maxPooledHeaderSize = 32
)

// headerPool reduces allocations for http.Header objects
var headerPool = sync.Pool{
	New: func() any {
		h := make(http.Header, 8)
		return &h
	},
}

// getHeader retrieves an http.Header from the pool.
// The returned header is cleared and ready for use.
func getHeader() *http.Header {
	h, ok := headerPool.Get().(*http.Header)
	if !ok || h == nil {
		tmp := make(http.Header, 8)
		return &tmp
	}
	// Clear the map for reuse
	for k := range *h {
		delete(*h, k)
	}
	return h
}

// putHeader returns an http.Header to the pool.
// Headers with too many entries are discarded to prevent memory bloat.
// SECURITY: Always clears all header values to prevent sensitive data leakage,
// regardless of whether the header is pooled or discarded.
func putHeader(h *http.Header) {
	if h == nil {
		return
	}

	// Record size before clearing — check must happen while entries exist
	oversized := len(*h) > maxPooledHeaderSize

	// SECURITY: Always clear all values to prevent sensitive data leakage
	for k, v := range *h {
		// Clear all string values to allow GC and prevent data leakage
		for i := range v {
			v[i] = ""
		}
		(*h)[k] = v[:0]
		delete(*h, k)
	}

	// Only pool headers within size limits
	if oversized {
		return // Don't pool large headers (already cleared above)
	}

	headerPool.Put(h)
}

// cloneHeader creates a deep copy of headers using batch allocation.
// Returns a newly allocated header map that can be safely modified.
// SECURITY: Always allocates new slices to prevent data leakage between requests.
// The header map itself uses pooled allocation, but values are always copied.
// OPTIMIZATION: Uses batch allocation to reduce N allocations (one per header) to 1 allocation.
func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}

	dst := *getHeader()

	// Count total values for batch allocation
	totalValues := 0
	for _, v := range src {
		totalValues += len(v)
	}

	if totalValues == 0 {
		return dst
	}

	// Batch allocate all strings in one slice
	allValues := make([]string, totalValues)
	valueIdx := 0

	for k, v := range src {
		if len(v) == 0 {
			dst[k] = []string{}
			continue
		}

		// Slice into the batch allocation for this header's values
		endIdx := valueIdx + len(v)
		newVals := allValues[valueIdx:endIdx]
		copy(newVals, v)
		dst[k] = newVals
		valueIdx = endIdx
	}
	return dst
}

// queryBuilderPool reduces allocations for building query strings
var queryBuilderPool = sync.Pool{
	New: func() any {
		sb := &strings.Builder{}
		return sb
	},
}

// getQueryBuilder retrieves a strings.Builder from the pool for query building.
func getQueryBuilder() *strings.Builder {
	sb, ok := queryBuilderPool.Get().(*strings.Builder)
	if !ok || sb == nil {
		return &strings.Builder{}
	}
	sb.Reset()
	return sb
}

// putQueryBuilder returns a strings.Builder to the pool.
// Builders with large capacity are discarded to prevent memory bloat.
func putQueryBuilder(sb *strings.Builder) {
	if sb == nil || sb.Cap() > 4096 {
		return
	}
	sb.Reset()
	queryBuilderPool.Put(sb)
}

// shouldEscape reports whether the byte needs escaping for URL query encoding.
// Fast path: only check for characters that actually need escaping.
func shouldEscape(c byte) bool {
	// RFC 3986: unreserved characters are A-Z, a-z, 0-9, '-', '.', '_', '~'
	// These are the only characters that DON'T need escaping
	return (c < 'A' || c > 'Z') &&
		(c < 'a' || c > 'z') &&
		(c < '0' || c > '9') &&
		c != '-' && c != '.' && c != '_' && c != '~'
}

// queryEscapePool pools byte slices for query escaping.
var queryEscapePool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 64)
		return &b
	},
}

// maxQueryEscapeSize limits the maximum input size for optimized query escaping.
// Inputs larger than this will use the standard library to avoid integer overflow
// in capacity calculation (len(s)*3) and excessive memory allocation.
const maxQueryEscapeSize = 10 * 1024 * 1024 // 10MB

// queryEscape performs URL query escaping with minimal allocations.
// Returns the original string if no escaping is needed (zero allocation).
func queryEscape(s string) string {
	// SECURITY: For very large inputs, use standard library to avoid:
	// 1. Integer overflow in len(s)*3 capacity calculation (32-bit systems)
	// 2. Excessive memory allocation
	if len(s) > maxQueryEscapeSize {
		// Count escapes to determine if we can return original
		for i := 0; i < len(s); i++ {
			if shouldEscape(s[i]) {
				// Use standard library for large inputs that need escaping
				return url.QueryEscape(s)
			}
		}
		return s // No escaping needed
	}

	// Fast path: scan for characters that need escaping
	var needsEscape bool
	for i := 0; i < len(s); i++ {
		if shouldEscape(s[i]) {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		return s // Zero allocation for strings that don't need escaping
	}

	// Slow path: escape using pooled buffer
	// Safe from overflow: len(s) <= maxQueryEscapeSize (10MB), so len(s)*3 <= 30MB
	bufPtr, ok := queryEscapePool.Get().(*[]byte)
	if !ok || bufPtr == nil {
		tmp := make([]byte, 0, len(s)*3)
		bufPtr = &tmp
	}
	buf := (*bufPtr)[:0]

	for i := 0; i < len(s); i++ {
		c := s[i]
		if !shouldEscape(c) {
			buf = append(buf, c)
		} else {
			buf = append(buf, '%')
			buf = append(buf, "0123456789ABCDEF"[c>>4])
			buf = append(buf, "0123456789ABCDEF"[c&0x0F])
		}
	}

	result := string(buf)
	// Always return bufPtr to pool; detach large buffers to prevent bloat.
	if cap(buf) <= 1024 {
		*bufPtr = buf
	} else {
		*bufPtr = nil
	}
	queryEscapePool.Put(bufPtr)
	return result
}

// encodeQueryParams efficiently encodes query parameters without allocating url.Values.
// This avoids the intermediate map allocation and uses pooled strings.Builder.
// Returns an empty string if params is nil or empty.
func encodeQueryParams(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}

	sb := getQueryBuilder()
	first := true

	// Pre-calculate approximate size to reduce reallocations
	estimatedSize := len(params) * 32 // rough estimate: key=value per param
	sb.Grow(estimatedSize)

	for key, value := range params {
		if first {
			first = false
		} else {
			sb.WriteByte('&')
		}
		sb.WriteString(queryEscape(key))
		sb.WriteByte('=')

		// Format value without allocation using formatQueryParam
		strValue := formatQueryParam(value)
		if strValue != "" {
			sb.WriteString(queryEscape(strValue))
		}
	}

	result := sb.String()
	putQueryBuilder(sb)
	return result
}

// appendQueryParams appends query parameters to an existing raw query string.
// This is more efficient than creating url.Values when you have an existing query.
func appendQueryParams(existingQuery string, params map[string]any) string {
	if len(params) == 0 {
		return existingQuery
	}

	sb := getQueryBuilder()

	// Pre-calculate size
	estimatedSize := len(existingQuery) + len(params)*32
	sb.Grow(estimatedSize)

	// Write existing query first
	if existingQuery != "" {
		sb.WriteString(existingQuery)
	}

	first := existingQuery == ""
	for key, value := range params {
		if first {
			first = false
		} else {
			sb.WriteByte('&')
		}
		sb.WriteString(queryEscape(key))
		sb.WriteByte('=')

		strValue := formatQueryParam(value)
		if strValue != "" {
			sb.WriteString(queryEscape(strValue))
		}
	}

	result := sb.String()
	putQueryBuilder(sb)
	return result
}
