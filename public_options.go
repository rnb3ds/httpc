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
	return func(r *Request) error {
		if err := validateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("invalid header: %w", err)
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers[key] = value
		return nil
	}
}

func WithHeaderMap(headers map[string]string) RequestOption {
	return func(r *Request) error {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		for k, v := range headers {
			if err := validateHeaderKeyValue(k, v); err != nil {
				return fmt.Errorf("invalid header %s: %w", k, err)
			}
			r.Headers[k] = v
		}
		return nil
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
	return func(r *Request) error {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Accept"] = "application/json"
		return nil
	}
}

func WithXMLAccept() RequestOption {
	return func(r *Request) error {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Accept"] = "application/xml"
		return nil
	}
}

func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) error {
		if strings.TrimSpace(username) == "" {
			return fmt.Errorf("username cannot be empty")
		}

		if strings.ContainsAny(username, "\r\n\x00:") {
			return fmt.Errorf("username contains invalid characters")
		}
		if strings.ContainsAny(password, "\r\n\x00") {
			return fmt.Errorf("password contains invalid characters")
		}
		if len(username) > 255 {
			return fmt.Errorf("username too long (max 255 characters)")
		}
		if len(password) > 255 {
			return fmt.Errorf("password too long (max 255 characters)")
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		auth := fmt.Sprintf("%s:%s", username, password)
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		r.Headers["Authorization"] = "Basic " + encoded
		return nil
	}
}

func WithBearerToken(token string) RequestOption {
	return func(r *Request) error {
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("token cannot be empty")
		}

		if strings.ContainsAny(token, "\r\n\x00") {
			return fmt.Errorf("token contains invalid characters")
		}
		if len(token) > 2048 {
			return fmt.Errorf("token too long (max 2048 characters)")
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Authorization"] = "Bearer " + token
		return nil
	}
}

func WithQuery(key string, value any) RequestOption {
	return func(r *Request) error {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("query key cannot be empty")
		}
		if len(key) > 256 {
			return fmt.Errorf("query key too long (max 256 characters)")
		}
		if strings.ContainsAny(key, "\r\n\x00&=") {
			return fmt.Errorf("query key contains invalid characters")
		}
		
		if value != nil {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > 8192 {
				return fmt.Errorf("query value too long (max 8192 characters)")
			}
		}
		
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		r.QueryParams[key] = value
		return nil
	}
}

func WithQueryMap(params map[string]any) RequestOption {
	return func(r *Request) error {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		for k, v := range params {
			if strings.TrimSpace(k) == "" {
				return fmt.Errorf("query key cannot be empty")
			}
			if len(k) > 256 {
				return fmt.Errorf("query key '%s' too long (max 256 characters)", k)
			}
			if strings.ContainsAny(k, "\r\n\x00&=") {
				return fmt.Errorf("query key '%s' contains invalid characters", k)
			}
			
			if v != nil {
				valueStr := fmt.Sprintf("%v", v)
				if len(valueStr) > 8192 {
					return fmt.Errorf("query value for key '%s' too long (max 8192 characters)", k)
				}
			}
			r.QueryParams[k] = v
		}
		return nil
	}
}

func WithJSON(data any) RequestOption {
	return func(r *Request) error {
		if data == nil {
			return fmt.Errorf("JSON data cannot be nil")
		}
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/json"
		return nil
	}
}

func WithXML(data any) RequestOption {
	return func(r *Request) error {
		if data == nil {
			return fmt.Errorf("XML data cannot be nil")
		}
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/xml"
		return nil
	}
}

func WithText(content string) RequestOption {
	return func(r *Request) error {
		r.Body = content
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "text/plain"
		return nil
	}
}

func WithForm(data map[string]string) RequestOption {
	return func(r *Request) error {
		if data == nil {
			return fmt.Errorf("form data cannot be nil")
		}
		values := url.Values{}
		for k, v := range data {
			values.Set(k, v)
		}
		r.Body = values.Encode()
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		return nil
	}
}

func WithFormData(data *FormData) RequestOption {
	return func(r *Request) error {
		if data == nil {
			return fmt.Errorf("form data cannot be nil")
		}
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		return nil
	}
}

func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *Request) error {
		if strings.TrimSpace(fieldName) == "" {
			return fmt.Errorf("field name cannot be empty")
		}
		if strings.TrimSpace(filename) == "" {
			return fmt.Errorf("filename cannot be empty")
		}

		if len(fieldName) > 256 {
			return fmt.Errorf("field name too long (max 256 characters)")
		}
		if len(filename) > 256 {
			return fmt.Errorf("filename too long (max 256 characters)")
		}

		if strings.ContainsAny(fieldName, "\r\n\x00\"'<>&") {
			return fmt.Errorf("field name contains invalid characters")
		}
		if strings.ContainsAny(filename, "\r\n\x00\"'<>&") {
			return fmt.Errorf("filename contains invalid characters")
		}

		cleanFilename := filepath.Base(filename)
		if cleanFilename == "." || cleanFilename == ".." || cleanFilename == "" {
			return fmt.Errorf("invalid filename")
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
		return nil
	}
}

func WithTimeout(timeout time.Duration) RequestOption {
	return func(r *Request) error {
		if timeout < 0 {
			return fmt.Errorf("timeout cannot be negative")
		}
		if timeout > 30*time.Minute {
			return fmt.Errorf("timeout too large (max 30 minutes)")
		}
		r.Timeout = timeout
		return nil
	}
}

func WithContext(ctx context.Context) RequestOption {
	return func(r *Request) error {
		if ctx == nil {
			return fmt.Errorf("context cannot be nil")
		}
		r.Context = ctx
		return nil
	}
}

func WithMaxRetries(maxRetries int) RequestOption {
	return func(r *Request) error {
		if maxRetries < 0 {
			return fmt.Errorf("max retries cannot be negative")
		}
		if maxRetries > 10 {
			return fmt.Errorf("max retries too large (max 10)")
		}
		r.MaxRetries = maxRetries
		return nil
	}
}

func WithBody(body any) RequestOption {
	return func(r *Request) error {
		r.Body = body
		return nil
	}
}

func WithBinary(data []byte, contentType ...string) RequestOption {
	return func(r *Request) error {
		if data == nil {
			return fmt.Errorf("binary data cannot be nil")
		}
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		ct := "application/octet-stream"
		if len(contentType) > 0 && contentType[0] != "" {
			ct = contentType[0]
		}
		r.Headers["Content-Type"] = ct
		return nil
	}
}

func WithCookie(cookie *http.Cookie) RequestOption {
	return func(r *Request) error {
		if cookie == nil {
			return fmt.Errorf("cookie cannot be nil")
		}

		if err := validateCookie(cookie); err != nil {
			return fmt.Errorf("invalid cookie: %w", err)
		}

		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0, 1)
		}
		r.Cookies = append(r.Cookies, cookie)
		return nil
	}
}

// validateCookie performs comprehensive cookie validation
func validateCookie(cookie *http.Cookie) error {
	if strings.TrimSpace(cookie.Name) == "" {
		return fmt.Errorf("cookie name cannot be empty")
	}

	// Check for invalid characters that could enable header injection
	if strings.ContainsAny(cookie.Name, "\r\n\x00;,") {
		return fmt.Errorf("cookie name contains invalid characters")
	}
	if strings.ContainsAny(cookie.Value, "\r\n\x00") {
		return fmt.Errorf("cookie value contains invalid characters")
	}

	// Check size limits to prevent DoS
	if len(cookie.Name) > 256 {
		return fmt.Errorf("cookie name too long (max 256 characters)")
	}
	if len(cookie.Value) > 4096 {
		return fmt.Errorf("cookie value too long (max 4096 characters)")
	}

	// Validate Domain if present
	if cookie.Domain != "" {
		if strings.ContainsAny(cookie.Domain, "\r\n\x00;,") {
			return fmt.Errorf("cookie domain contains invalid characters")
		}
		if len(cookie.Domain) > 255 {
			return fmt.Errorf("cookie domain too long (max 255 characters)")
		}
	}

	// Validate Path if present
	if cookie.Path != "" {
		if strings.ContainsAny(cookie.Path, "\r\n\x00;") {
			return fmt.Errorf("cookie path contains invalid characters")
		}
		if len(cookie.Path) > 1024 {
			return fmt.Errorf("cookie path too long (max 1024 characters)")
		}
	}

	return nil
}

func WithCookies(cookies []*http.Cookie) RequestOption {
	return func(r *Request) error {
		if len(cookies) == 0 {
			return nil
		}
		
		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0, len(cookies))
		}
		
		for i, cookie := range cookies {
			if cookie == nil {
				return fmt.Errorf("cookie at index %d is nil", i)
			}
			if err := validateCookie(cookie); err != nil {
				return fmt.Errorf("invalid cookie at index %d: %w", i, err)
			}
			r.Cookies = append(r.Cookies, cookie)
		}
		return nil
	}
}

func WithCookieValue(name, value string) RequestOption {
	return func(r *Request) error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("cookie name cannot be empty")
		}
		if strings.ContainsAny(name, "\r\n\x00;,") {
			return fmt.Errorf("cookie name contains invalid characters")
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("cookie value contains invalid characters")
		}
		if len(name) > 256 {
			return fmt.Errorf("cookie name too long (max 256 characters)")
		}
		if len(value) > 4096 {
			return fmt.Errorf("cookie value too long (max 4096 characters)")
		}

		cookie := &http.Cookie{
			Name:     name,
			Value:    value,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}

		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0, 1)
		}
		r.Cookies = append(r.Cookies, cookie)
		return nil
	}
}
