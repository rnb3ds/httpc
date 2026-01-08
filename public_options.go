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

	"github.com/cybergodev/httpc/internal/validation"
)

// WithHeader adds a custom header to the request.
// Returns an error if the header key or value is invalid.
func WithHeader(key, value string) RequestOption {
	return func(r *Request) error {
		if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
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
			if err := validation.ValidateHeaderKeyValue(k, v); err != nil {
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



// validateCookie validates HTTP cookies using consolidated validation.
// Prevents cookie injection and enforces RFC 6265 compliance.
func validateCookie(cookie *http.Cookie) error {
	if err := validation.ValidateCookieName(cookie.Name); err != nil {
		return err
	}

	if err := validation.ValidateCookieValue(cookie.Value); err != nil {
		return err
	}

	// Validate domain if set
	if cookie.Domain != "" {
		domainLen := len(cookie.Domain)
		if domainLen > validation.MaxCookieDomainLen {
			return fmt.Errorf("cookie domain too long (max %d)", validation.MaxCookieDomainLen)
		}
		for i, r := range cookie.Domain {
			if r < 0x20 || r == 0x7F {
				return fmt.Errorf("cookie domain contains invalid characters at position %d", i)
			}
		}
	}

	// Validate path if set
	if cookie.Path != "" {
		pathLen := len(cookie.Path)
		if pathLen > validation.MaxCookiePathLen {
			return fmt.Errorf("cookie path too long (max %d)", validation.MaxCookiePathLen)
		}
		for i, r := range cookie.Path {
			if r < 0x20 || r == 0x7F {
				return fmt.Errorf("cookie path contains invalid characters at position %d", i)
			}
		}
	}

	return nil
}

// WithBasicAuth adds HTTP Basic Authentication to the request.
// The credentials are base64-encoded and added to the Authorization header.
// Validates credentials to prevent injection attacks and enforce size limits.
// Returns an error if username is empty or credentials contain invalid characters.
func WithBasicAuth(username, password string) RequestOption {
	return func(r *Request) error {
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}
		if err := validation.ValidateCredential(username, validation.MaxCredLen, true, "username"); err != nil {
			return fmt.Errorf("invalid username: %w", err)
		}
		if err := validation.ValidateCredential(password, validation.MaxCredLen, false, "password"); err != nil {
			return fmt.Errorf("invalid password: %w", err)
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string, 1)
		}

		// Efficient string concatenation and encoding
		creds := username + ":" + password
		r.Headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
		return nil
	}
}

// WithBearerToken adds a Bearer token to the Authorization header.
// Validates token to prevent injection attacks and enforce size limits.
// Returns an error if token is empty, too long, or contains invalid characters.
func WithBearerToken(token string) RequestOption {
	return func(r *Request) error {
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		if err := validation.ValidateToken(token); err != nil {
			return err
		}

		if r.Headers == nil {
			r.Headers = make(map[string]string, 1)
		}
		r.Headers["Authorization"] = "Bearer " + token
		return nil
	}
}

// WithQuery adds a single query parameter to the request URL.
// Returns an error if key is empty, too long, or contains invalid characters.
func WithQuery(key string, value any) RequestOption {
	return func(r *Request) error {
		if err := validation.ValidateQueryKey(key); err != nil {
			return err
		}

		if value != nil {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > validation.MaxValueLen {
				return fmt.Errorf("query value too long (max %d)", validation.MaxValueLen)
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
			if err := validation.ValidateQueryKey(k); err != nil {
				return fmt.Errorf("invalid key %s: %w", k, err)
			}

			if v != nil {
				valueStr := fmt.Sprintf("%v", v)
				if len(valueStr) > validation.MaxValueLen {
					return fmt.Errorf("query value too long for key %s (max %d)", k, validation.MaxValueLen)
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
		if err := validation.ValidateFieldName(fieldName, "field name"); err != nil {
			return fmt.Errorf("invalid field name: %w", err)
		}
		if err := validation.ValidateFieldName(filename, "filename"); err != nil {
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

// WithFollowRedirects enables or disables automatic redirect following for this request.
// This overrides the client's FollowRedirects configuration.
// When disabled, the client returns the redirect response (3xx) without following it.
func WithFollowRedirects(follow bool) RequestOption {
	return func(r *Request) error {
		r.FollowRedirects = &follow
		return nil
	}
}

// WithMaxRedirects sets the maximum number of redirects to follow for this request.
// This overrides the client's MaxRedirects configuration.
// Set to 0 to allow unlimited redirects (not recommended).
// Returns an error if maxRedirects is negative or exceeds 50.
func WithMaxRedirects(maxRedirects int) RequestOption {
	return func(r *Request) error {
		if maxRedirects < 0 {
			return fmt.Errorf("maxRedirects cannot be negative")
		}
		if maxRedirects > 50 {
			return fmt.Errorf("maxRedirects exceeds maximum of 50")
		}
		r.MaxRedirects = &maxRedirects
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

func WithCookie(cookie http.Cookie) RequestOption {
	return func(r *Request) error {
		if err := validateCookie(&cookie); err != nil {
			return fmt.Errorf("invalid cookie: %w", err)
		}

		if r.Cookies == nil {
			r.Cookies = make([]http.Cookie, 0, 1)
		}
		r.Cookies = append(r.Cookies, cookie)
		return nil
	}
}

func WithCookies(cookies []http.Cookie) RequestOption {
	return func(r *Request) error {
		if len(cookies) == 0 {
			return nil
		}

		if r.Cookies == nil {
			r.Cookies = make([]http.Cookie, 0, len(cookies))
		}

		for i := range cookies {
			if err := validateCookie(&cookies[i]); err != nil {
				return fmt.Errorf("invalid cookie at index %d: %w", i, err)
			}
			r.Cookies = append(r.Cookies, cookies[i])
		}
		return nil
	}
}

// WithCookieValue adds a cookie with the given name and value to the request.
// The cookie is created with secure defaults: HttpOnly=true, SameSite=Lax.
// The domain should be set explicitly using WithCookie if needed.
// For HTTPS requests, consider using WithCookie with Secure=true.
// Returns an error if name is empty or contains invalid characters.
func WithCookieValue(name, value string) RequestOption {
	return func(r *Request) error {
		cookie := http.Cookie{
			Name:     name,
			Value:    value,
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		}

		if err := validateCookie(&cookie); err != nil {
			return err
		}

		if r.Cookies == nil {
			r.Cookies = make([]http.Cookie, 0, 1)
		}
		r.Cookies = append(r.Cookies, cookie)
		return nil
	}
}

// WithCookieString parses a cookie string and adds all cookies to the request.
// The cookie string should be in the format: "name1=value1; name2=value2; name3=value3"
// This is commonly used when copying cookies from browser developer tools or server responses.
// Each cookie is created with secure defaults: HttpOnly=true, SameSite=Lax.
// Returns an error if the cookie string is malformed or contains invalid cookie names/values.
//
// Example:
//
//	WithCookieString("BPSID=4418ECBB1281B550; PSTM=1733760779; BDS=kUwNTVFcEUBUItoc")
func WithCookieString(cookieString string) RequestOption {
	return func(r *Request) error {
		if cookieString == "" {
			return nil
		}

		cookies, err := parseCookieString(cookieString)
		if err != nil {
			return fmt.Errorf("failed to parse cookie string: %w", err)
		}

		if len(cookies) == 0 {
			return nil
		}

		if r.Cookies == nil {
			r.Cookies = make([]http.Cookie, 0, len(cookies))
		}

		for i := range cookies {
			if err := validateCookie(&cookies[i]); err != nil {
				return fmt.Errorf("invalid cookie %s: %w", cookies[i].Name, err)
			}
			r.Cookies = append(r.Cookies, cookies[i])
		}

		return nil
	}
}

// parseCookieString parses a cookie string and returns http.Cookie objects with secure defaults.
func parseCookieString(cookieString string) ([]http.Cookie, error) {
	if cookieString == "" {
		return nil, nil
	}

	cookies := make([]http.Cookie, 0, 4)
	start := 0
	cookieLen := len(cookieString)

	for i := 0; i <= cookieLen; i++ {
		if i == cookieLen || cookieString[i] == ';' {
			pair := trimSpace(cookieString[start:i])

			if pair != "" {
				idx := strings.IndexByte(pair, '=')
				if idx == -1 {
					return nil, fmt.Errorf("malformed cookie pair: %s (missing '=')", pair)
				}

				name := trimSpaceRight(pair[:idx])
				value := trimSpaceLeft(pair[idx+1:])

				if name == "" {
					return nil, fmt.Errorf("empty cookie name in pair: %s", pair)
				}

				nameLen := len(name)
				if nameLen > validation.MaxCookieNameLen {
					return nil, fmt.Errorf("cookie name too long: %s", name)
				}
				if len(value) > validation.MaxCookieValueLen {
					return nil, fmt.Errorf("cookie value too long for %s", name)
				}

				// Validate cookie name characters
				for j := 0; j < nameLen; j++ {
					c := name[j]
					if c < 0x20 || c == 0x7F || c == ';' || c == ',' || c == '=' {
						return nil, fmt.Errorf("invalid character in cookie name: %s", name)
					}
				}

				cookies = append(cookies, http.Cookie{
					Name:     name,
					Value:    value,
					HttpOnly: true,
					Secure:   false,
					SameSite: http.SameSiteLaxMode,
				})
			}
			start = i + 1
		}
	}

	return cookies, nil
}
