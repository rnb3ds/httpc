package httpc

import (
	"net/http"
	"strings"
)

// parseCookieHeader parses a Cookie header value into http.Cookie slice.
func parseCookieHeader(cookieHeader string) []*http.Cookie {
	if cookieHeader == "" {
		return nil
	}

	cookies := make([]*http.Cookie, 0, 4)
	headerLen := len(cookieHeader)
	start := 0

	for i := 0; i <= headerLen; i++ {
		if i == headerLen || cookieHeader[i] == ';' {
			if i > start {
				pair := trimSpace(cookieHeader[start:i])
				if pair != "" {
					if idx := strings.IndexByte(pair, '='); idx > 0 {
						name := trimSpaceRight(pair[:idx])
						value := trimSpaceLeft(pair[idx+1:])

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
func trimSpace(s string) string {
	if len(s) == 0 {
		return s
	}

	start := 0
	end := len(s)

	for start < end && s[start] == ' ' {
		start++
	}

	for end > start && s[end-1] == ' ' {
		end--
	}

	return s[start:end]
}

// trimSpaceLeft trims leading spaces without allocation.
func trimSpaceLeft(s string) string {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return s[i:]
}

// trimSpaceRight trims trailing spaces without allocation.
func trimSpaceRight(s string) string {
	if len(s) == 0 {
		return s
	}
	// Find last non-space character
	i := len(s) - 1
	for i >= 0 && s[i] == ' ' {
		i--
	}
	if i < 0 {
		return ""
	}
	return s[:i+1]
}
