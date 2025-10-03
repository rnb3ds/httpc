package httpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// WithHeader sets a request header
func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers[key] = value
	}
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

// WithXMLAccept sets Accept header to application/xml
func WithXMLAccept() RequestOption {
	return WithAccept("application/xml")
}

// WithBasicAuth sets basic authentication
func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
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

// WithQuery sets a single query parameter
func WithQuery(key string, value interface{}) RequestOption {
	return func(r *Request) {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
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

// WithXML sets the request body as XML and sets appropriate Content-Type
func WithXML(data interface{}) RequestOption {
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

// WithFile uploads a single file
func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *Request) {
		formData := &FormData{
			Fields: make(map[string]string),
			Files: map[string]*FileData{
				fieldName: {
					Filename: filename,
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

// WithCookieValue adds a simple cookie with the given name and value to the request.
// This is a convenience method for simple cookies without additional attributes.
// For cookies with attributes (Path, Domain, Secure, etc.), use WithCookie instead.
//
// Example:
//
//	client.Get(url, httpc.WithCookieValue("session_id", "abc123"))
func WithCookieValue(name, value string) RequestOption {
	return WithCookie(&http.Cookie{
		Name:  name,
		Value: value,
	})
}
