package httpc

import (
	"fmt"
	"net/http"
	"strings"
)

// parseCookieHeader parses a Cookie header value into http.Cookie slice.
// Handles the format: "name1=value1; name2=value2"
// Optimized for hot path usage with minimal allocations and efficient parsing.
func parseCookieHeader(cookieHeader string) []*http.Cookie {
	if cookieHeader == "" {
		return nil
	}

	// Pre-allocate with reasonable capacity based on typical usage
	cookies := make([]*http.Cookie, 0, 4)
	headerLen := len(cookieHeader)
	start := 0

	for i := 0; i <= headerLen; i++ {
		if i == headerLen || cookieHeader[i] == ';' {
			if i > start {
				pair := trimSpace(cookieHeader[start:i])
				if pair != "" {
					if idx := strings.IndexByte(pair, '='); idx > 0 && idx < len(pair)-1 {
						name := trimSpaceRight(pair[:idx])
						value := trimSpaceLeft(pair[idx+1:])

						// Only create cookie if both name and value are valid
						if name != "" {
							cookies = append(cookies, &http.Cookie{
								Name:  name,
								Value: value,
							})
						}
					}
				}
			}
			start = i + 1
		}
	}
	return cookies
}

// trimSpace trims leading and trailing spaces without allocation.
// Optimized for hot path usage in cookie parsing with minimal branching.
func trimSpace(s string) string {
	// Fast path: check if trimming is needed
	if len(s) == 0 {
		return s
	}

	start := 0
	end := len(s)

	// Find first non-space character
	for start < end && s[start] == ' ' {
		start++
	}

	// Find last non-space character
	for end > start && s[end-1] == ' ' {
		end--
	}

	return s[start:end]
}

// trimSpaceLeft trims leading spaces without allocation.
// Optimized for cookie parsing hot path.
func trimSpaceLeft(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	return s
}

// trimSpaceRight trims trailing spaces without allocation.
// Optimized for cookie parsing hot path.
func trimSpaceRight(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// itoa converts int to string without allocation for small numbers.
// Faster than fmt.Sprintf or strconv.Itoa for common cases.
func itoa(i int) string {
	return itoa64(int64(i))
}

// itoa64 converts int64 to string without allocation for small numbers.
func itoa64(i int64) string {
	if i >= 0 && i < 100 {
		return small[i]
	}
	return fmt.Sprintf("%d", i)
}

// small is a lookup table for common small integers (0-99).
// This avoids allocation for the most common cases.
var small = [100]string{
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
	"10", "11", "12", "13", "14", "15", "16", "17", "18", "19",
	"20", "21", "22", "23", "24", "25", "26", "27", "28", "29",
	"30", "31", "32", "33", "34", "35", "36", "37", "38", "39",
	"40", "41", "42", "43", "44", "45", "46", "47", "48", "49",
	"50", "51", "52", "53", "54", "55", "56", "57", "58", "59",
	"60", "61", "62", "63", "64", "65", "66", "67", "68", "69",
	"70", "71", "72", "73", "74", "75", "76", "77", "78", "79",
	"80", "81", "82", "83", "84", "85", "86", "87", "88", "89",
	"90", "91", "92", "93", "94", "95", "96", "97", "98", "99",
}
