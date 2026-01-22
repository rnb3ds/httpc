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

func WithTimeout(timeout time.Duration) RequestOption {
	return func(r *Request) error {
		if timeout < 0 {
			return fmt.Errorf("%w: cannot be negative", ErrInvalidTimeout)
		}
		if timeout > maxTimeout {
			return fmt.Errorf("%w: exceeds %v", ErrInvalidTimeout, maxTimeout)
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

func WithFollowRedirects(follow bool) RequestOption {
	return func(r *Request) error {
		r.FollowRedirects = &follow
		return nil
	}
}

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
		if err := validation.ValidateCookie(&cookie); err != nil {
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
			if err := validation.ValidateCookie(&cookies[i]); err != nil {
				return fmt.Errorf("invalid cookie at index %d: %w", i, err)
			}
			r.Cookies = append(r.Cookies, cookies[i])
		}
		return nil
	}
}

func WithCookieValue(name, value string) RequestOption {
	return func(r *Request) error {
		cookie := http.Cookie{
			Name:  name,
			Value: value,
		}

		if err := validation.ValidateCookie(&cookie); err != nil {
			return err
		}

		if r.Cookies == nil {
			r.Cookies = make([]http.Cookie, 0, 1)
		}
		r.Cookies = append(r.Cookies, cookie)
		return nil
	}
}

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
			if err := validation.ValidateCookie(&cookies[i]); err != nil {
				return fmt.Errorf("invalid cookie %s: %w", cookies[i].Name, err)
			}
			r.Cookies = append(r.Cookies, cookies[i])
		}

		return nil
	}
}

func parseCookieString(cookieString string) ([]http.Cookie, error) {
	// Quick validation for common malformed cases
	if strings.IndexByte(cookieString, '=') < 0 {
		return nil, fmt.Errorf("malformed cookie: missing '=' separator")
	}

	if strings.HasPrefix(cookieString, "=") {
		return nil, fmt.Errorf("malformed cookie: empty name before '='")
	}

	parsedCookies := parseCookieHeader(cookieString)
	if parsedCookies == nil {
		return nil, nil
	}

	cookies := make([]http.Cookie, 0, len(parsedCookies))
	for _, cookie := range parsedCookies {
		nameLen := len(cookie.Name)
		if nameLen > validation.MaxCookieNameLen {
			return nil, fmt.Errorf("cookie name too long: %s", cookie.Name)
		}
		if len(cookie.Value) > validation.MaxCookieValueLen {
			return nil, fmt.Errorf("cookie value too long for %s", cookie.Name)
		}

		// Validate cookie name characters
		for j := 0; j < nameLen; j++ {
			c := cookie.Name[j]
			if c < 0x20 || c == 0x7F || c == ';' || c == ',' || c == '=' {
				return nil, fmt.Errorf("invalid character in cookie name: %s", cookie.Name)
			}
		}

		cookies = append(cookies, http.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
		})
	}

	return cookies, nil
}
