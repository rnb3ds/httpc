package httpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		if err := validateHeaderKeyValue(key, value); err != nil {
			return
		}

		r.Headers[key] = value
	}
}

func WithHeaderMap(headers map[string]string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		for k, v := range headers {
			if err := validateHeaderKeyValue(k, v); err != nil {
				continue
			}
			r.Headers[k] = v
		}
	}
}

func WithUserAgent(userAgent string) RequestOption {
	return WithHeader("User-Agent", userAgent)
}

func WithContentType(contentType string) RequestOption {
	return WithHeader("Content-Type", contentType)
}

func WithAccept(accept string) RequestOption {
	return WithHeader("Accept", accept)
}

func WithJSONAccept() RequestOption {
	return WithAccept("application/json")
}

func WithXMLAccept() RequestOption {
	return WithAccept("application/xml")
}

func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		if strings.TrimSpace(username) == "" {
			return
		}

		if strings.ContainsAny(username, "\r\n\x00:") ||
			strings.ContainsAny(password, "\r\n\x00") ||
			len(username) > 255 || len(password) > 255 {
			return
		}

		auth := fmt.Sprintf("%s:%s", username, password)
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		r.Headers["Authorization"] = "Basic " + encoded
	}
}

func WithBearerToken(token string) RequestOption {
	return func(r *Request) {
		if strings.TrimSpace(token) == "" {
			return
		}

		if strings.ContainsAny(token, "\r\n\x00") || len(token) > 2048 {
			return
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Authorization"] = "Bearer " + token
	}
}

func WithQuery(key string, value any) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		if strings.TrimSpace(key) != "" && len(key) <= 256 &&
			!strings.ContainsAny(key, "\r\n\x00&=") {
			if value != nil {
				valueStr := fmt.Sprintf("%v", value)
				if len(valueStr) <= 8192 {
					r.QueryParams[key] = value
				}
			} else {
				r.QueryParams[key] = value
			}
		}
	}
}

func WithQueryMap(params map[string]any) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		for k, v := range params {
			if strings.TrimSpace(k) != "" && len(k) <= 256 &&
				!strings.ContainsAny(k, "\r\n\x00&=") {
				if v != nil {
					valueStr := fmt.Sprintf("%v", v)
					if len(valueStr) <= 8192 {
						r.QueryParams[k] = v
					}
				} else {
					r.QueryParams[k] = v
				}
			}
		}
	}
}

func WithJSON(data any) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/json"
	}
}

func WithXML(data any) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/xml"
	}
}

func WithText(content string) RequestOption {
	return func(r *Request) {
		r.Body = content
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "text/plain"
	}
}

func WithForm(data map[string]string) RequestOption {
	return func(r *Request) {
		values := url.Values{}
		for k, v := range data {
			values.Set(k, v)
		}
		r.Body = values.Encode()
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
}

func WithFormData(data *FormData) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
	}
}

func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *Request) {
		if strings.TrimSpace(fieldName) == "" || strings.TrimSpace(filename) == "" {
			return
		}

		if len(fieldName) > 256 || len(filename) > 256 {
			return
		}

		if strings.ContainsAny(fieldName, "\r\n\x00\"'<>&") ||
			strings.ContainsAny(filename, "\r\n\x00\"'<>&") {
			return
		}

		cleanFilename := filepath.Base(filename)
		if cleanFilename == "." || cleanFilename == ".." || cleanFilename == "" {
			return
		}

		formData := &FormData{
			Fields: make(map[string]string),
			Files: map[string]*FileData{
				fieldName: {
					Filename: cleanFilename,
					Content:  content,
				},
			},
		}
		r.Body = formData
	}
}

func WithTimeout(timeout time.Duration) RequestOption {
	return func(r *Request) {
		r.Timeout = timeout
	}
}

func WithContext(ctx context.Context) RequestOption {
	return func(r *Request) {
		r.Context = ctx
	}
}

func WithMaxRetries(maxRetries int) RequestOption {
	return func(r *Request) {
		r.MaxRetries = maxRetries
	}
}

func WithBody(body any) RequestOption {
	return func(r *Request) {
		r.Body = body
	}
}

func WithBinary(data []byte, contentType ...string) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		ct := "application/octet-stream"
		if len(contentType) > 0 && contentType[0] != "" {
			ct = contentType[0]
		}
		r.Headers["Content-Type"] = ct
	}
}

func WithCookie(cookie *http.Cookie) RequestOption {
	return func(r *Request) {
		if cookie == nil {
			return
		}

		if strings.TrimSpace(cookie.Name) == "" {
			return
		}

		if strings.ContainsAny(cookie.Name, "\r\n\x00;,") ||
			strings.ContainsAny(cookie.Value, "\r\n\x00") {
			return
		}

		if len(cookie.Name) > 256 || len(cookie.Value) > 4096 {
			return
		}

		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0)
		}
		r.Cookies = append(r.Cookies, cookie)
	}
}

func WithCookies(cookies []*http.Cookie) RequestOption {
	return func(r *Request) {
		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0, len(cookies))
		}
		r.Cookies = append(r.Cookies, cookies...)
	}
}

func WithCookieValue(name, value string) RequestOption {
	if strings.TrimSpace(name) == "" ||
		strings.ContainsAny(name, "\r\n\x00;,") ||
		strings.ContainsAny(value, "\r\n\x00") ||
		len(name) > 256 || len(value) > 4096 {
		return func(r *Request) {}
	}

	return WithCookie(&http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
