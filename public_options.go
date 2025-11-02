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

// WithHeader sets a request header with proper validation.
// Use ValidateConfig to catch invalid headers at configuration time.
func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		// Strict validation - skip invalid headers
		if err := validateHeaderKeyValue(key, value); err != nil {
			// Invalid headers are silently ignored to prevent breaking existing code
			return
		}

		r.Headers[key] = value
	}
}

// WithHeaderMap sets multiple request headers with validation
func WithHeaderMap(headers map[string]string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		for k, v := range headers {
			// Apply strict validation
			if err := validateHeaderKeyValue(k, v); err != nil {
				// Skip invalid headers silently
				continue
			}
			r.Headers[k] = v
		}
	}
}

// WithUserAgent sets the User-Agent header
func WithUserAgent(userAgent string) RequestOption {
	return WithHeader("User-Agent", userAgent)
}

// WithContentType sets the Content-Type header
func WithContentType(contentType string) RequestOption {
	return WithHeader("Content-Type", contentType)
}

// WithAccept sets the Accept header
func WithAccept(accept string) RequestOption {
	return WithHeader("Accept", accept)
}

// WithJSONAccept sets Accept header to application/json
func WithJSONAccept() RequestOption {
	return WithAccept("application/json")
}

// WithXMLAccept sets Accept header to application/xml
func WithXMLAccept() RequestOption {
	return WithAccept("application/xml")
}

// WithBasicAuth sets basic authentication with enhanced security
func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		// Enhanced validation
		if strings.TrimSpace(username) == "" {
			return
		}

		// Check for dangerous characters and length limits
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

// WithBearerToken sets bearer token authentication with validation
func WithBearerToken(token string) RequestOption {
	return func(r *Request) {
		// Validate token
		if strings.TrimSpace(token) == "" {
			return
		}

		// Check for dangerous characters and reasonable length limits
		if strings.ContainsAny(token, "\r\n\x00") || len(token) > 2048 {
			return
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Authorization"] = "Bearer " + token
	}
}

// WithQuery sets a single query parameter with validation
func WithQuery(key string, value any) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		// Validate query parameter key and value
		if strings.TrimSpace(key) != "" && len(key) <= 256 &&
			!strings.ContainsAny(key, "\r\n\x00&=") {
			// Validate value size
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

// WithQueryMap sets multiple query parameters from a map with validation
func WithQueryMap(params map[string]any) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		for k, v := range params {
			// Apply same validation as WithQuery
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

// WithJSON sets the request body as JSON and sets appropriate Content-Type
func WithJSON(data any) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/json"
	}
}

// WithXML sets the request body as XML and sets appropriate Content-Type
func WithXML(data any) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/xml"
	}
}

// WithText sets the request body as plain text and sets appropriate Content-Type
func WithText(content string) RequestOption {
	return func(r *Request) {
		r.Body = content
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "text/plain"
	}
}

// WithForm sets the request body as form data and sets appropriate Content-Type
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

// WithFormData sets the request body as multipart form data
func WithFormData(data *FormData) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		// Content-Type will be set by the transport layer with boundary
	}
}

// WithFile uploads a single file with enhanced security validation
func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *Request) {
		// Enhanced validation
		if strings.TrimSpace(fieldName) == "" || strings.TrimSpace(filename) == "" {
			return
		}

		// Validate field name and filename lengths and characters
		if len(fieldName) > 256 || len(filename) > 256 {
			return
		}

		// Check for dangerous characters
		if strings.ContainsAny(fieldName, "\r\n\x00\"'<>&") ||
			strings.ContainsAny(filename, "\r\n\x00\"'<>&") {
			return
		}

		cleanFilename := filepath.Base(filename)
		if cleanFilename == "." || cleanFilename == ".." || cleanFilename == "" {
			return
		}

		// Check for null bytes in content (potential security issue)
		if len(content) > 0 && content[0] == 0 {
			// Allow but be cautious with binary files starting with null
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

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) RequestOption {
	return func(r *Request) {
		r.Timeout = timeout
	}
}

// WithContext sets the request context
func WithContext(ctx context.Context) RequestOption {
	return func(r *Request) {
		r.Context = ctx
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(maxRetries int) RequestOption {
	return func(r *Request) {
		r.MaxRetries = maxRetries
	}
}

// WithBody sets the raw request body
func WithBody(body any) RequestOption {
	return func(r *Request) {
		r.Body = body
	}
}

// WithBinary sets the request body as binary data with optional content type
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

// WithCookie adds a cookie to the request with validation
func WithCookie(cookie *http.Cookie) RequestOption {
	return func(r *Request) {
		if cookie == nil {
			return
		}

		// Validate cookie name and value
		if strings.TrimSpace(cookie.Name) == "" {
			return
		}

		// Check for dangerous characters in cookie name and value
		if strings.ContainsAny(cookie.Name, "\r\n\x00;,") ||
			strings.ContainsAny(cookie.Value, "\r\n\x00") {
			return
		}

		// Limit cookie name and value length
		if len(cookie.Name) > 256 || len(cookie.Value) > 4096 {
			return
		}

		// Enhance security by setting secure defaults if not specified
		if cookie.Secure == false && cookie.HttpOnly == false {
			// Create a copy to avoid modifying the original
			secureCookie := *cookie
			secureCookie.HttpOnly = true // Prevent XSS attacks
			if r.Cookies == nil {
				r.Cookies = make([]*http.Cookie, 0)
			}
			r.Cookies = append(r.Cookies, &secureCookie)
		} else {
			if r.Cookies == nil {
				r.Cookies = make([]*http.Cookie, 0)
			}
			r.Cookies = append(r.Cookies, cookie)
		}
	}
}

// WithCookies adds multiple cookies to the request
func WithCookies(cookies []*http.Cookie) RequestOption {
	return func(r *Request) {
		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0, len(cookies))
		}
		r.Cookies = append(r.Cookies, cookies...)
	}
}

// WithCookieValue adds a simple cookie with the given name and value with secure defaults
func WithCookieValue(name, value string) RequestOption {
	// Pre-validate before creating cookie
	if strings.TrimSpace(name) == "" ||
		strings.ContainsAny(name, "\r\n\x00;,") ||
		strings.ContainsAny(value, "\r\n\x00") ||
		len(name) > 256 || len(value) > 4096 {
		return func(r *Request) {} // Return no-op function for invalid input
	}

	return WithCookie(&http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,                 // Secure default to prevent XSS
		SameSite: http.SameSiteLaxMode, // CSRF protection
	})
}
