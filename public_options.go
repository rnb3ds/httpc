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

// WithHeader sets a request header with security validation
func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		// 基本安全检查
		if err := validateHeaderSafety(key, value); err != nil {
			// 静默忽略不安全的头部，避免暴露错误信息
			return
		}

		r.Headers[key] = value
	}
}

// validateHeaderSafety 执行基本的头部安全检查
func validateHeaderSafety(key, value string) error {
	// 检查CRLF注入
	if strings.ContainsAny(key, "\r\n\x00") || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("header contains invalid characters")
	}

	// 检查长度限制
	if len(key) > 256 || len(value) > 8192 {
		return fmt.Errorf("header too long")
	}

	// 检查空键
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("header key cannot be empty")
	}

	return nil
}

// WithHeaderMap sets multiple request headers
func WithHeaderMap(headers map[string]string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		for k, v := range headers {
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

// WithBasicAuth sets basic authentication with input validation
func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		// 验证输入
		if strings.TrimSpace(username) == "" {
			return // 静默忽略空用户名
		}

		// 检查是否包含危险字符
		if strings.ContainsAny(username, "\r\n\x00:") || strings.ContainsAny(password, "\r\n\x00") {
			return // 静默忽略包含危险字符的凭据
		}

		// 限制长度以防止过长的凭据
		if len(username) > 255 || len(password) > 255 {
			return // 静默忽略过长的凭据
		}

		auth := fmt.Sprintf("%s:%s", username, password)
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		r.Headers["Authorization"] = "Basic " + encoded
	}
}

// WithBearerToken sets bearer token authentication
func WithBearerToken(token string) RequestOption {
	return WithHeader("Authorization", "Bearer "+token)
}

// WithQuery sets a single query parameter with validation
func WithQuery(key string, value interface{}) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}

		// 验证查询参数键
		if strings.TrimSpace(key) == "" {
			return // 静默忽略空键
		}

		// 检查键中的危险字符
		if strings.ContainsAny(key, "\r\n\x00&=") {
			return // 静默忽略包含危险字符的键
		}

		// 限制键长度
		if len(key) > 256 {
			return // 静默忽略过长的键
		}

		// 验证值
		if value != nil {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > 8192 { // 限制查询参数值的长度
				return // 静默忽略过长的值
			}
		}

		r.QueryParams[key] = value
	}
}

// WithQueryMap sets multiple query parameters from a map
func WithQueryMap(params map[string]interface{}) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		for k, v := range params {
			r.QueryParams[k] = v
		}
	}
}

// WithJSON sets the request body as JSON and sets appropriate Content-Type
func WithJSON(data interface{}) RequestOption {
	return func(r *Request) {
		r.Body = data
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/json"
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

// WithFile uploads a single file with security validation
func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *Request) {
		// 验证字段名
		if strings.TrimSpace(fieldName) == "" {
			return // 静默忽略空字段名
		}

		// 验证文件名
		if strings.TrimSpace(filename) == "" {
			return // 静默忽略空文件名
		}

		// 清理文件名，防止路径遍历
		cleanFilename := filepath.Base(filename)
		if cleanFilename == "." || cleanFilename == ".." {
			return // 静默忽略危险文件名
		}

		// 检查文件大小限制 (50MB)
		if len(content) > 50*1024*1024 {
			return // 静默忽略过大文件
		}

		// 检查字段名和文件名中的危险字符
		if strings.ContainsAny(fieldName, "\r\n\x00\"'<>") ||
			strings.ContainsAny(cleanFilename, "\r\n\x00") {
			return // 静默忽略包含危险字符的名称
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
func WithBody(body interface{}) RequestOption {
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

// WithCookie adds a cookie to the request
func WithCookie(cookie *http.Cookie) RequestOption {
	return func(r *Request) {
		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0)
		}
		r.Cookies = append(r.Cookies, cookie)
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

// WithCookieValue adds a simple cookie with the given name and value
func WithCookieValue(name, value string) RequestOption {
	return WithCookie(&http.Cookie{
		Name:  name,
		Value: value,
	})
}
