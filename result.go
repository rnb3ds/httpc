package httpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Result represents the complete outcome of an HTTP request.
// It provides clear separation between request information, response data, and metadata.
//
// Thread Safety:
// Result objects are immutable after creation and safe to read from
// multiple goroutines concurrently. Do not modify Result fields directly.
type Result struct {
	Request  *RequestInfo
	Response *ResponseInfo
	Meta     *RequestMeta
}

// RequestInfo contains information about the HTTP request that was sent.
type RequestInfo struct {
	Method  string
	URL     string
	Headers http.Header
	Cookies []*http.Cookie
}

// ResponseInfo contains the HTTP response data received from the server.
type ResponseInfo struct {
	StatusCode    int
	Status        string
	Headers       http.Header
	Body          string
	RawBody       []byte
	ContentLength int64
	Cookies       []*http.Cookie
}

// RequestMeta contains metadata about the request execution.
type RequestMeta struct {
	Duration      time.Duration
	Attempts      int
	RedirectChain []string
	RedirectCount int
}

// Body returns the response body as a string.
// This is a convenience method equivalent to accessing Result.Response.Body.
func (r *Result) Body() string {
	if r == nil || r.Response == nil {
		return ""
	}
	return r.Response.Body
}

// RawBody returns the response body as raw bytes.
// This is a convenience method equivalent to accessing Result.Response.RawBody.
func (r *Result) RawBody() []byte {
	if r == nil || r.Response == nil {
		return nil
	}
	return r.Response.RawBody
}

// StatusCode returns the HTTP status code.
// This is a convenience method equivalent to accessing Result.Response.StatusCode.
func (r *Result) StatusCode() int {
	if r == nil || r.Response == nil {
		return 0
	}
	return r.Response.StatusCode
}

// RequestCookies returns the cookies that were sent with the request.
// This is a convenience method equivalent to accessing Result.Request.Cookies.
func (r *Result) RequestCookies() []*http.Cookie {
	if r == nil || r.Request == nil {
		return nil
	}
	return r.Request.Cookies
}

// ResponseCookies returns the cookies received in the response.
// This is a convenience method equivalent to accessing Result.Response.Cookies.
func (r *Result) ResponseCookies() []*http.Cookie {
	if r == nil || r.Response == nil {
		return nil
	}
	return r.Response.Cookies
}

// JSON unmarshals the response body into the provided interface.
// Returns ErrResponseBodyEmpty if the body is nil or empty.
// Returns ErrResponseBodyTooLarge if the body exceeds 50MB.
func (r *Result) JSON(v any) error {
	if r == nil || r.Response == nil {
		return ErrResponseBodyEmpty
	}

	bodyLen := len(r.Response.RawBody)
	if bodyLen == 0 {
		return ErrResponseBodyEmpty
	}

	if bodyLen > maxJSONSize {
		return fmt.Errorf("%w: %d bytes exceeds 50MB", ErrResponseBodyTooLarge, bodyLen)
	}

	return json.Unmarshal(r.Response.RawBody, v)
}

// IsSuccess returns true if the response status code indicates success (2xx).
func (r *Result) IsSuccess() bool {
	if r == nil || r.Response == nil {
		return false
	}
	code := r.Response.StatusCode
	return code >= 200 && code < 300
}

// IsRedirect returns true if the response status code indicates a redirect (3xx).
func (r *Result) IsRedirect() bool {
	if r == nil || r.Response == nil {
		return false
	}
	code := r.Response.StatusCode
	return code >= 300 && code < 400
}

// IsClientError returns true if the response status code indicates a client error (4xx).
func (r *Result) IsClientError() bool {
	if r == nil || r.Response == nil {
		return false
	}
	code := r.Response.StatusCode
	return code >= 400 && code < 500
}

// IsServerError returns true if the response status code indicates a server error (5xx).
func (r *Result) IsServerError() bool {
	if r == nil || r.Response == nil {
		return false
	}
	code := r.Response.StatusCode
	return code >= 500 && code < 600
}

// GetCookie returns a specific cookie from the response by name.
// This returns cookies from the server's Set-Cookie header (response cookies).
func (r *Result) GetCookie(name string) *http.Cookie {
	if r == nil || r.Response == nil {
		return nil
	}
	for _, cookie := range r.Response.Cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// HasCookie checks if a specific cookie exists in the response.
func (r *Result) HasCookie(name string) bool {
	return r.GetCookie(name) != nil
}

// GetRequestCookie returns a specific cookie from the request by name.
func (r *Result) GetRequestCookie(name string) *http.Cookie {
	if r == nil || r.Request == nil {
		return nil
	}
	for _, cookie := range r.Request.Cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// HasRequestCookie checks if a specific cookie was sent in the request.
func (r *Result) HasRequestCookie(name string) bool {
	return r.GetRequestCookie(name) != nil
}

// String returns a formatted string representation of the result.
func (r *Result) String() string {
	if r == nil || r.Response == nil {
		return "Result{}"
	}

	var b strings.Builder
	b.Grow(256)

	b.WriteString("Result{Status: ")
	b.WriteString(strconv.Itoa(r.Response.StatusCode))
	b.WriteByte(' ')
	b.WriteString(r.Response.Status)
	b.WriteString(", ContentLength: ")
	b.WriteString(strconv.FormatInt(r.Response.ContentLength, 10))

	if r.Meta != nil {
		b.WriteString(", Duration: ")
		b.WriteString(r.Meta.Duration.String())
		b.WriteString(", Attempts: ")
		b.WriteString(strconv.Itoa(r.Meta.Attempts))
	}

	if len(r.Response.Headers) > 0 {
		b.WriteString(", Headers: ")
		b.WriteString(strconv.Itoa(len(r.Response.Headers)))
	}

	if len(r.Response.Cookies) > 0 {
		b.WriteString(", Cookies: ")
		b.WriteString(strconv.Itoa(len(r.Response.Cookies)))
	}

	if len(r.Response.Body) > 0 {
		b.WriteString(", Body: \n")
		b.WriteString(r.Response.Body)
	}

	b.WriteByte('}')

	return b.String()
}

// Html returns the response body as HTML content.
// This is an alias for Body() method.
func (r *Result) Html() string {
	return r.Body()
}

// SaveToFile saves the response body to a file.
// Returns ErrResponseBodyEmpty if the body is nil or empty.
func (r *Result) SaveToFile(filePath string) error {
	if r == nil || r.Response == nil || r.Response.RawBody == nil {
		return ErrResponseBodyEmpty
	}

	if err := prepareFilePath(filePath); err != nil {
		return fmt.Errorf("file path validation failed: %w", err)
	}

	cleanPath := filepath.Clean(filePath)
	if err := os.WriteFile(cleanPath, r.Response.RawBody, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
