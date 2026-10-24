package engine

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// httpHeaderPool reduces allocations for http.Header maps used in request building.
// Header maps are allocated per-request and can be reused after clearing.
//
// Stores the map value directly rather than *http.Header: boxing a map into any
// is allocation-free (a map value is a single-word pointer to its hmap), whereas
// httpHeaderPool.Put(&h) would force the parameter h to escape to the heap on
// every return, allocating a pointer box.
var httpHeaderPool = sync.Pool{
	New: func() any {
		return make(http.Header, 8)
	},
}

// getHTTPHeader retrieves a cleared http.Header from the pool.
func getHTTPHeader() http.Header {
	h, ok := httpHeaderPool.Get().(http.Header)
	if !ok || h == nil {
		return make(http.Header, 8)
	}
	clear(h)
	return h
}

// putHTTPHeader returns an http.Header to the pool after clearing all entries.
// Maps with more than 64 entries are discarded to prevent memory bloat.
func putHTTPHeader(h http.Header) {
	if h == nil || len(h) > 64 {
		return
	}
	clear(h)
	httpHeaderPool.Put(h)
}

// CloneHeader creates a deep copy of headers using batch allocation.
// Returns a newly allocated header map that can be safely modified.
// SECURITY: Always allocates new slices to prevent data leakage between requests.
// OPTIMIZATION: Uses batch allocation to reduce N allocations (one per header) to 1 allocation.
// For single-value headers (the common case), uses a shared backing array to reduce
// slice header overhead.
func CloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}

	n := len(src)
	dst := make(http.Header, n)

	if n == 0 {
		return dst
	}

	// Fast path: count total values.
	totalValues := 0
	for _, v := range src {
		totalValues += len(v)
	}

	if totalValues == 0 {
		return dst
	}

	// Batch allocate all string values in one slice
	allValues := make([]string, totalValues)
	valueIdx := 0

	for k, v := range src {
		switch len(v) {
		case 0:
			dst[k] = zeroStringSlice
		case 1:
			allValues[valueIdx] = v[0]
			// Cap-1 window (3-index slice), matching captureRequestHeaders: a
			// 2-index window's cap would reach into the neighboring header's
			// values, so appending to this entry could clobber another header.
			dst[k] = allValues[valueIdx : valueIdx+1 : valueIdx+1]
			valueIdx++
		default:
			endIdx := valueIdx + len(v)
			// 3-index window for the same reason as the single-value case: a
			// 2-index cap would reach into the next header's values.
			newVals := allValues[valueIdx:endIdx:endIdx]
			copy(newVals, v)
			dst[k] = newVals
			valueIdx = endIdx
		}
	}
	return dst
}

// zeroStringSlice is a reusable empty slice for headers with no values.
// Read-only: never appended to or mutated.
var zeroStringSlice = []string{}

// queryBuilderPool reduces buffer allocations for building query strings and
// cookie headers. It pools *bytes.Buffer — NOT strings.Builder (whose Reset
// discards the backing array, so a pooled strings.Builder retains only its
// ~32-byte header and every reuse regrows from zero); bytes.Buffer.Reset
// keeps the backing slice. Callers must extract the result with
// strings.Clone before the Put — bytes.Buffer's String aliases the live
// buffer, which the next pool user overwrites.
var queryBuilderPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// getQueryBuilder retrieves a buffer from the pool for query building.
func getQueryBuilder() *bytes.Buffer {
	sb, ok := queryBuilderPool.Get().(*bytes.Buffer)
	if !ok || sb == nil {
		return &bytes.Buffer{}
	}
	sb.Reset()
	return sb
}

// putQueryBuilder returns a buffer to the pool.
// Buffers with large capacity are discarded to prevent memory bloat.
func putQueryBuilder(sb *bytes.Buffer) {
	if sb == nil || sb.Cap() > 4096 {
		return
	}
	sb.Reset()
	queryBuilderPool.Put(sb)
}

// queryKeysPool reduces allocations for the scratch key slice used to sort
// multi-parameter query maps in appendQueryParams. It stores a *[]string —
// the stable pointer lets the backing array be reused across requests
// (boxing a bare slice value into any would allocate a fresh slice header on
// every Put, defeating the pool).
var queryKeysPool = sync.Pool{
	New: func() any {
		s := make([]string, 0, 4)
		return &s
	},
}

// maxQueryKeys caps the pooled scratch slice to prevent memory bloat,
// matching the entry-count guards of the map pools above.
const maxQueryKeys = 64

// getQueryKeys returns a pointer to a zero-length scratch slice for
// collecting map keys. Append through the pointer (*kp = append(*kp, ...))
// so growth updates the shared header.
func getQueryKeys() *[]string {
	kp, ok := queryKeysPool.Get().(*[]string)
	if !ok || kp == nil {
		s := make([]string, 0, 4)
		return &s
	}
	*kp = (*kp)[:0]
	return kp
}

// putQueryKeys returns a scratch key slice to the pool after clearing the
// live string references. clear zeroes len(*kp) slots only: key strings from
// earlier, larger requests remain reachable in the backing array up to cap
// (bounded at maxQueryKeys = 64) until overwritten by later use. That bounded
// residue is accepted — clearing the full cap would touch memory the slice
// no longer logically owns, and 64 stale string headers are not a leak that
// grows with traffic.
func putQueryKeys(kp *[]string) {
	if kp == nil || cap(*kp) > maxQueryKeys {
		return // oversized scratch is left for GC
	}
	clear(*kp)
	*kp = (*kp)[:0]
	queryKeysPool.Put(kp)
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

// maxQueryEscapeSize limits the maximum input size for optimized query escaping.
// Inputs larger than this will use the standard library to avoid integer overflow
// in capacity calculation (len(s)*3) and excessive memory allocation.
const maxQueryEscapeSize = 10 * 1024 * 1024 // 10MB

// appendStringWriter is the sink AppendQueryEscape appends to — satisfied by
// both *strings.Builder and *bytes.Buffer, the two writer types the engine
// and the facade build query/form strings with.
type appendStringWriter interface {
	io.Writer
	io.StringWriter
	io.ByteWriter
}

// AppendQueryEscape appends the URL-query-escaped form of s directly to b.
// Writing straight into the writer avoids the intermediate escaped string
// allocation that b.WriteString(QueryEscape(s)) would incur whenever s
// contains characters that need escaping. Inputs with no escapable bytes are
// written verbatim after a single scan. Uses the RFC 3986 unreserved set —
// note this differs from url.QueryEscape for the space character (%20 here,
// '+' in the stdlib form encoding), which servers decode identically. Very
// large inputs fall back to the standard library.
func AppendQueryEscape(b appendStringWriter, s string) {
	// NOTE: every appendStringWriter implementation (*strings.Builder,
	// *bytes.Buffer) is memory-backed and cannot fail, so the write errors
	// below are explicitly discarded — errcheck requires the discard to be
	// visible now that the receiver is an interface rather than the concrete
	// (previously special-cased) *strings.Builder. Discards are inline (no
	// closures) to keep this hot path allocation-free.

	// SECURITY: delegate very large inputs to the standard library to avoid the
	// per-byte loop cost and keep parity with QueryEscape's overflow guard.
	// url.QueryEscape encodes space as '+', which would silently switch the
	// wire bytes for values crossing the size boundary — map back to %20
	// (QueryEscape emits '+' only for spaces; a literal '+' arrives here as
	// %2B, so the replacement cannot corrupt real content).
	if len(s) > maxQueryEscapeSize {
		_, _ = b.WriteString(strings.ReplaceAll(url.QueryEscape(s), "+", "%20"))
		return
	}

	// Fast scan: locate the first byte that needs escaping.
	firstEscape := -1
	for i := 0; i < len(s); i++ {
		if shouldEscape(s[i]) {
			firstEscape = i
			break
		}
	}
	if firstEscape < 0 {
		_, _ = b.WriteString(s) // nothing to escape — single bulk write
		return
	}

	// Slow path: write the leading verbatim run, then escape the remainder.
	_, _ = b.WriteString(s[:firstEscape])
	const hex = "0123456789ABCDEF"
	for i := firstEscape; i < len(s); i++ {
		c := s[i]
		if !shouldEscape(c) {
			_ = b.WriteByte(c)
			continue
		}
		_ = b.WriteByte('%')
		_ = b.WriteByte(hex[c>>4])
		_ = b.WriteByte(hex[c&0x0F])
	}
}

// appendQueryParams appends query parameters to an existing raw query string.
// This is more efficient than creating url.Values when you have an existing query.
// Optimized to write numeric values directly via strconv.Append* and to escape
// string keys/values straight into the builder (appendQueryEscape), avoiding the
// intermediate string allocations that QueryEscape would incur.
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

	var numBuf [32]byte // stack-allocated buffer for numeric formatting

	// Fast path: a single parameter has no ordering to determine, so the
	// key slice (and its sort) can be skipped entirely — one fewer allocation
	// for the very common single-WithQuery request.
	if len(params) == 1 {
		for key, value := range params {
			if existingQuery != "" {
				sb.WriteByte('&')
			}
			AppendQueryEscape(sb, key)
			sb.WriteByte('=')
			writeQueryParamValue(sb, value, numBuf[:0])
		}
		result := strings.Clone(sb.String()) // copy: the buffer is pooled and reused
		putQueryBuilder(sb)
		return result
	}

	// Sort keys for deterministic output, matching url.Values.Encode and
	// encodeFormFields semantics. Map iteration order is random per request;
	// unsorted keys would make the wire format nondeterministic, which breaks
	// request signing/HMAC, response caching keyed on the URL, and multiplies
	// entries in the URL cache (same params, different orderings).
	// The scratch slice is pooled (getQueryKeys) to avoid one allocation per
	// multi-param request.
	kp := getQueryKeys()
	for key := range params {
		*kp = append(*kp, key)
	}
	slices.Sort(*kp)

	first := existingQuery == ""
	for _, key := range *kp {
		if first {
			first = false
		} else {
			sb.WriteByte('&')
		}
		AppendQueryEscape(sb, key)
		sb.WriteByte('=')

		writeQueryParamValue(sb, params[key], numBuf[:0])
	}
	putQueryKeys(kp)

	result := strings.Clone(sb.String()) // copy: the buffer is pooled and reused
	putQueryBuilder(sb)
	return result
}

// writeQueryParamValue appends a query parameter value to sb.
// Numeric and bool values are written directly via strconv.Append*
// to avoid intermediate string allocations. Strings are URL-escaped straight
// into the builder via appendQueryEscape.
// nil emits nothing, matching FormatQueryParam (which formats nil as "");
// without this guard the default %v branch would render the literal "<nil>".
//
// MAINTENANCE: this type switch has two siblings that MUST stay in sync —
// FormatQueryParam (request.go, inspection/doc path) and the facade's
// queryValueLength (public_options.go). query_param_parity_test.go guards
// the drift; when adding a type, add it to all three.
func writeQueryParamValue(sb appendStringWriter, value any, numBuf []byte) {
	if value == nil {
		return
	}
	switch v := value.(type) {
	case string:
		if v != "" {
			AppendQueryEscape(sb, v)
		}
	case int:
		_, _ = sb.Write(strconv.AppendInt(numBuf, int64(v), 10)) // memory-backed: cannot fail
	case int64:
		_, _ = sb.Write(strconv.AppendInt(numBuf, v, 10)) // memory-backed: cannot fail
	case int32:
		_, _ = sb.Write(strconv.AppendInt(numBuf, int64(v), 10)) // memory-backed: cannot fail
	case uint:
		_, _ = sb.Write(strconv.AppendUint(numBuf, uint64(v), 10)) // memory-backed: cannot fail
	case uint64:
		_, _ = sb.Write(strconv.AppendUint(numBuf, v, 10)) // memory-backed: cannot fail
	case uint32:
		_, _ = sb.Write(strconv.AppendUint(numBuf, uint64(v), 10)) // memory-backed: cannot fail
	case float64:
		_, _ = sb.Write(strconv.AppendFloat(numBuf, v, 'f', -1, 64)) // memory-backed: cannot fail
	case float32:
		_, _ = sb.Write(strconv.AppendFloat(numBuf, float64(v), 'f', -1, 32)) // memory-backed: cannot fail
	case bool:
		if v {
			_, _ = sb.WriteString("true") // memory-backed: cannot fail
		} else {
			_, _ = sb.WriteString("false") // memory-backed: cannot fail
		}
	default:
		if s, ok := value.(fmt.Stringer); ok {
			if strValue := s.String(); strValue != "" {
				AppendQueryEscape(sb, strValue)
			}
		} else if strValue := fmt.Sprintf("%v", value); strValue != "" {
			AppendQueryEscape(sb, strValue)
		}
	}
}
