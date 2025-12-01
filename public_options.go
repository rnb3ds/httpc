package httpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"
)

// WithHeader adds a custom header to the request.
// Returns an error if the header key or value is invalid.
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

// WithHeaderMap adds multiple headers to the request.
// Returns an error if any header key or value is invalid.
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

// WithUserAgent sets the User-Agent header for the request.
func WithUserAgent(userAgent string) RequestOption {
	return WithHeader("User-Agent", userAgent)
}

// WithContentType sets the Content-Type header for the request.
func WithContentType(contentType string) RequestOption {
	return WithHeader("Content-Type", contentType)
}

// WithAccept sets the Accept header for the request.
func WithAccept(accept string) RequestOption {
	return WithHeader("Accept", accept)
}

// WithJSONAccept sets the Accept header to application/json.
func WithJSONAccept() RequestOption {
	return WithAccept("application/json")
}

// WithXMLAccept sets the Accept header to application/xml.
func WithXMLAccept() RequestOption {
	return WithAccept("application/xml")
}

const (
	maxCredLen     = 255
	maxTokenLen    = 2048
	maxKeyLen      = 256
	maxValueLen    = 8192
	maxFilenameLen = 256
)

// WithBasicAuth adds HTTP Basic Authentication to the request.
// The credentials are base64-encoded and added to the Authorization header.
// Returns an error if username is empty or credentials contain invalid characters.
func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) error {
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}
		if err := validateCredential(username, maxCredLen, true); err != nil {
			return fmt.Errorf("invalid username: %w", err)
		}
		if err := validateCredential(password, maxCredLen, false); err != nil {
			return fmt.Errorf("invalid password: %w", err)
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}

		r.Headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
		return nil
	}
}

// WithBearerToken adds a Bearer token to the Authorization header.
// Returns an error if token is empty, too long, or contains invalid characters.
func WithBearerToken(token string) RequestOption {
	return func(r *Request) error {
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		if err := validateToken(token); err != nil {
			return err
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Authorization"] = "Bearer " + token
		return nil
	}
}

// WithQuery adds a single query parameter to the request URL.
// Returns an error if key is empty, too long, or contains invalid characters.
func WithQuery(key string, value any) RequestOption {
	return func(r *Request) error {
		if err := validateQueryKey(key); err != nil {
			return err
		}

		if value != nil {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > maxValueLen {
				return fmt.Errorf("query value too long (max %d)", maxValueLen)
			}
		}

		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any)
		}
		r.QueryParams[key] = value
		return nil
	}
}

// WithQueryMap adds multiple query parameters to the request URL.
// Returns an error if any key is empty, too long, or contains invalid characters.
func WithQueryMap(params map[string]any) RequestOption {
	return func(r *Request) error {
		if r.QueryParams == nil {
			r.QueryParams = make(map[string]any, len(params))
		}
		for k, v := range params {
			if err := validateQueryKey(k); err != nil {
				return fmt.Errorf("invalid key %s: %w", k, err)
			}

			if v != nil {
				valueStr := fmt.Sprintf("%v", v)
				if len(valueStr) > maxValueLen {
					return fmt.Errorf("query value too long for key %s (max %d)", k, maxValueLen)
				}
			}
			r.QueryParams[k] = v
		}
		return nil
	}
}

// WithJSON sets the request body to JSON-encoded data and sets Content-Type to application/json.
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

// WithXML sets the request body to XML-encoded data and sets Content-Type to application/xml.
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

// WithFile adds a file upload to the request.
// Returns an error if fieldName or filename is empty or contains invalid characters.
func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *Request) error {
		if fieldName == "" {
			return fmt.Errorf("field name cannot be empty")
		}
		if filename == "" {
			return fmt.Errorf("filename cannot be empty")
		}
		if err := validateFieldName(fieldName); err != nil {
			return fmt.Errorf("invalid field name: %w", err)
		}
		if err := validateFieldName(filename); err != nil {
			return fmt.Errorf("invalid filename: %w", err)
		}

		cleanFilename := filepath.Base(filename)
		if cleanFilename == "." || cleanFilename == ".." || cleanFilename == "" {
			return fmt.Errorf("invalid filename")
		}

		r.Body = &FormData{
			Fields: make(map[string]string),
			Files: map[string]*FileData{
				fieldName: {
					Filename: cleanFilename,
					Content:  content,
				},
			},
		}
		return nil
	}
}

// WithTimeout sets the request timeout.
// Returns ErrInvalidTimeout if timeout is negative or exceeds 30 minutes.
func WithTimeout(timeout time.Duration) RequestOption {
	return func(r *Request) error {
		if timeout < 0 {
			return fmt.Errorf("%w: cannot be negative", ErrInvalidTimeout)
		}
		if timeout > 30*time.Minute {
			return fmt.Errorf("%w: exceeds 30 minutes", ErrInvalidTimeout)
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

// WithMaxRetries sets the maximum number of retry attempts.
// Returns ErrInvalidRetry if maxRetries is not between 0 and 10.
func WithMaxRetries(maxRetries int) RequestOption {
	return func(r *Request) error {
		if maxRetries < 0 || maxRetries > 10 {
			return fmt.Errorf("%w: must be 0-10, got %d", ErrInvalidRetry, maxRetries)
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

const (
	maxCookieNameLen   = 256
	maxCookieValueLen  = 4096
	maxCookieDomainLen = 255
	maxCookiePathLen   = 1024
)

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

// WithCookieValue adds a cookie with the given name and value to the request.
// The cookie is created with secure defaults: HttpOnly=true, SameSite=Lax.
// For HTTPS requests, consider using WithCookie with Secure=true.
// Returns an error if name is empty or contains invalid characters.
func WithCookieValue(name, value string) RequestOption {
	return func(r *Request) error {
		cookie := &http.Cookie{
			Name:     name,
			Value:    value,
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		}

		if err := validateCookie(cookie); err != nil {
			return err
		}

		if r.Cookies == nil {
			r.Cookies = make([]*http.Cookie, 0, 1)
		}
		r.Cookies = append(r.Cookies, cookie)
		return nil
	}
}

func validateCredential(cred string, maxLen int, checkColon bool) error {
	credLen := len(cred)
	if credLen > maxLen {
		return fmt.Errorf("too long (max %d)", maxLen)
	}

	for i := range credLen {
		c := cred[i]
		if c == '\r' || c == '\n' || c == 0 || (checkColon && c == ':') {
			return fmt.Errorf("contains invalid characters")
		}
	}
	return nil
}

func validateToken(token string) error {
	tokenLen := len(token)
	if tokenLen > maxTokenLen {
		return fmt.Errorf("token too long (max %d)", maxTokenLen)
	}

	for i := range tokenLen {
		c := token[i]
		if c == '\r' || c == '\n' || c == 0 {
			return fmt.Errorf("token contains invalid characters")
		}
	}
	return nil
}

func validateQueryKey(key string) error {
	if key == "" {
		return fmt.Errorf("query key cannot be empty")
	}
	keyLen := len(key)
	if keyLen > maxKeyLen {
		return fmt.Errorf("query key too long (max %d)", maxKeyLen)
	}

	for i := range keyLen {
		c := key[i]
		if c == '\r' || c == '\n' || c == 0 || c == '&' || c == '=' {
			return fmt.Errorf("query key contains invalid characters")
		}
	}
	return nil
}

func validateFieldName(name string) error {
	nameLen := len(name)
	if nameLen > maxFilenameLen {
		return fmt.Errorf("too long (max %d)", maxFilenameLen)
	}

	for i := range nameLen {
		c := name[i]
		if c == '\r' || c == '\n' || c == 0 || c == '"' || c == '\'' || c == '<' || c == '>' || c == '&' {
			return fmt.Errorf("contains invalid characters")
		}
	}
	return nil
}

func validateCookie(cookie *http.Cookie) error {
	if cookie.Name == "" {
		return fmt.Errorf("cookie name cannot be empty")
	}
	nameLen := len(cookie.Name)
	if nameLen > maxCookieNameLen {
		return fmt.Errorf("cookie name too long (max %d)", maxCookieNameLen)
	}

	for i := range nameLen {
		c := cookie.Name[i]
		if c == '\r' || c == '\n' || c == 0 || c == ';' || c == ',' {
			return fmt.Errorf("cookie name contains invalid characters")
		}
	}

	valueLen := len(cookie.Value)
	if valueLen > maxCookieValueLen {
		return fmt.Errorf("cookie value too long (max %d)", maxCookieValueLen)
	}

	for i := range valueLen {
		c := cookie.Value[i]
		if c == '\r' || c == '\n' || c == 0 {
			return fmt.Errorf("cookie value contains invalid characters")
		}
	}

	if cookie.Domain != "" {
		domainLen := len(cookie.Domain)
		if domainLen > maxCookieDomainLen {
			return fmt.Errorf("cookie domain too long (max %d)", maxCookieDomainLen)
		}
		if cookie.Domain == "." {
			return fmt.Errorf("cookie domain cannot be just a dot")
		}
		for i := range domainLen {
			c := cookie.Domain[i]
			if c == '\r' || c == '\n' || c == 0 || c == ';' || c == ',' {
				return fmt.Errorf("cookie domain contains invalid characters")
			}
		}
	}

	if cookie.Path != "" {
		pathLen := len(cookie.Path)
		if pathLen == 0 || cookie.Path[0] != '/' {
			return fmt.Errorf("cookie path must start with /")
		}
		if pathLen > maxCookiePathLen {
			return fmt.Errorf("cookie path too long (max %d)", maxCookiePathLen)
		}
		for i := range pathLen {
			c := cookie.Path[i]
			if c == '\r' || c == '\n' || c == 0 || c == ';' {
				return fmt.Errorf("cookie path contains invalid characters")
			}
		}
	}
	return nil
}
