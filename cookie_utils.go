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
				pair := strings.TrimSpace(cookieHeader[start:i])
				if pair != "" {
					if idx := strings.IndexByte(pair, '='); idx > 0 {
						name := strings.TrimSpace(pair[:idx])
						value := strings.TrimSpace(pair[idx+1:])

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
