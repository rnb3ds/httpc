package httpc

import (
	"net/http"
	"sync"
)

// cookieSlicePool reduces allocations for cookie slices
var cookieSlicePool = sync.Pool{
	New: func() any {
		slice := make([]*http.Cookie, 0, 8)
		return &slice
	},
}

// getCookiesSlice retrieves a cookie slice from the pool
func getCookiesSlice() *[]*http.Cookie {
	slice, ok := cookieSlicePool.Get().(*[]*http.Cookie)
	if !ok || slice == nil {
		s := make([]*http.Cookie, 0, 8)
		return &s
	}
	*slice = (*slice)[:0]
	return slice
}

// putCookiesSlice returns a cookie slice to the pool
func putCookiesSlice(slice *[]*http.Cookie) {
	if cap(*slice) > 64 {
		return // Don't pool large slices
	}
	*slice = (*slice)[:0]
	cookieSlicePool.Put(slice)
}

// parseCookieHeader parses a Cookie header value into http.Cookie slice.
// Optimized to minimize string allocations by using index-based trimming.
func parseCookieHeader(cookieHeader string) []*http.Cookie {
	if cookieHeader == "" {
		return nil
	}

	// Use pooled slice for parsing
	cookiesPtr := getCookiesSlice()
	defer func() {
		// Return slice to pool if we're returning nil or a different slice
		if len(*cookiesPtr) == 0 {
			putCookiesSlice(cookiesPtr)
		}
	}()

	cookies := *cookiesPtr
	headerLen := len(cookieHeader)
	start := 0

	for i := 0; i <= headerLen; i++ {
		if i == headerLen || cookieHeader[i] == ';' {
			if i > start {
				// Trim whitespace from the pair using indices (no allocation)
				pairStart, pairEnd := trimSpaceIndices(cookieHeader, start, i)
				if pairStart < pairEnd {
					pair := cookieHeader[pairStart:pairEnd]
					if idx := findEqual(pair); idx > 0 {
						// Trim whitespace from name
						nameStart, nameEnd := trimSpaceIndices(pair, 0, idx)
						// Trim whitespace from value
						valueStart, valueEnd := trimSpaceIndices(pair, idx+1, len(pair))

						if nameStart < nameEnd {
							cookies = append(cookies, &http.Cookie{
								Name:  pair[nameStart:nameEnd],
								Value: pair[valueStart:valueEnd],
							})
						}
					}
				}
			}
			start = i + 1
		}
	}

	if len(cookies) == 0 {
		return nil
	}

	// Return the slice; caller is responsible for not pooling it
	*cookiesPtr = cookies
	return cookies
}

// trimSpaceIndices returns the start and end indices of s[low:high] after trimming whitespace.
// This avoids allocating a new string.
func trimSpaceIndices(s string, low, high int) (int, int) {
	// Trim leading whitespace
	for low < high && isWhitespace(s[low]) {
		low++
	}
	// Trim trailing whitespace
	for high > low && isWhitespace(s[high-1]) {
		high--
	}
	return low, high
}

// isWhitespace reports whether byte c is an ASCII whitespace character.
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// findEqual finds the first '=' byte in s, returning -1 if not found.
func findEqual(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return i
		}
	}
	return -1
}
