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

const (
	maxBodyPreview   = 200 // Maximum body preview length in String()
	truncationMarker = "...[truncated]"
)

// sensitiveHeaders contains header names that should be masked in String() output.
var sensitiveHeaders = map[string]bool{
	"Authorization":       true,
	"Cookie":              true,
	"Set-Cookie":          true,
	"X-Api-Key":           true,
	"X-Auth-Token":        true,
	"Proxy-Authorization": true,
}

type Result struct {
	Request  *RequestInfo
	Response *ResponseInfo
	Meta     *RequestMeta
}

type RequestInfo struct {
	URL     string
	Method  string
	Headers http.Header
	Cookies []*http.Cookie
}

type ResponseInfo struct {
	StatusCode    int
	Status        string
	Proto         string
	Headers       http.Header
	Body          string
	RawBody       []byte
	ContentLength int64
	Cookies       []*http.Cookie
}

type RequestMeta struct {
	Duration      time.Duration
	Attempts      int
	RedirectChain []string
	RedirectCount int
}

func (r *Result) Body() string {
	if r == nil || r.Response == nil {
		return ""
	}
	return r.Response.Body
}

func (r *Result) RawBody() []byte {
	if r == nil || r.Response == nil {
		return nil
	}
	return r.Response.RawBody
}

func (r *Result) StatusCode() int {
	if r == nil || r.Response == nil {
		return 0
	}
	return r.Response.StatusCode
}

// Proto returns the HTTP protocol version (e.g., "HTTP/1.1", "HTTP/2.0").
func (r *Result) Proto() string {
	if r == nil || r.Response == nil {
		return ""
	}
	return r.Response.Proto
}

func (r *Result) RequestCookies() []*http.Cookie {
	if r == nil || r.Request == nil {
		return nil
	}
	return r.Request.Cookies
}

func (r *Result) ResponseCookies() []*http.Cookie {
	if r == nil || r.Response == nil {
		return nil
	}
	return r.Response.Cookies
}

// Unmarshal parses the JSON-encoded response body and stores the result
// in the value pointed to by v. It follows the same conventions as json.Unmarshal.
//
// Returns ErrResponseBodyEmpty if the body is nil or empty.
// Returns ErrResponseBodyTooLarge if the body exceeds 50MB.
func (r *Result) Unmarshal(v any) error {
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

func (r *Result) HasCookie(name string) bool {
	return r.GetCookie(name) != nil
}

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

func (r *Result) HasRequestCookie(name string) bool {
	return r.GetRequestCookie(name) != nil
}

func (r *Result) String() string {
	if r == nil || r.Response == nil {
		return "Result{}"
	}

	// Pre-calculate approximate size to reduce allocations
	estimatedSize := 128 // Base size for status, content length
	if r.Meta != nil {
		estimatedSize += 64 // Duration and attempts
	}
	if len(r.Response.Headers) > 0 {
		estimatedSize += 32 + len(r.Response.Headers)*16 // Headers
	}
	if len(r.Response.Cookies) > 0 {
		estimatedSize += 32 // Cookies count
	}
	if len(r.Response.Body) > 0 {
		bodyPreview := min(len(r.Response.Body), maxBodyPreview)
		estimatedSize += 16 + bodyPreview
	}

	var b strings.Builder
	b.Grow(estimatedSize)

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
		b.WriteString(" [")
		first := true
		for key := range r.Response.Headers {
			if !first {
				b.WriteString(", ")
			}
			first = false
			if sensitiveHeaders[key] {
				b.WriteString(key)
				b.WriteString(": ***")
			} else {
				b.WriteString(key)
			}
		}
		b.WriteByte(']')
	}

	if len(r.Response.Cookies) > 0 {
		b.WriteString(", Cookies: ")
		b.WriteString(strconv.Itoa(len(r.Response.Cookies)))
	}

	if len(r.Response.Body) > 0 {
		b.WriteString(", Body: ")
		if len(r.Response.Body) > maxBodyPreview {
			b.WriteString(r.Response.Body[:maxBodyPreview])
			b.WriteString(truncationMarker)
		} else {
			b.WriteString(r.Response.Body)
		}
	}

	b.WriteByte('}')

	return b.String()
}

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
