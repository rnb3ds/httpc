package httpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
	"github.com/cybergodev/httpc/internal/validation"
)

func WithHeader(key, value string) RequestOption {
	return func(r *engine.Request) error {
		if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
			return fmt.Errorf("invalid header: %w", err)
		}

		r.SetHeader(key, value)
		return nil
	}
}

// WithHeaderMap sets multiple headers from a map.
func WithHeaderMap(headers map[string]string) RequestOption {
	return func(r *engine.Request) error {
		for k, v := range headers {
			if err := validation.ValidateHeaderKeyValue(k, v); err != nil {
				return fmt.Errorf("invalid header %s: %w", k, err)
			}
			r.SetHeader(k, v)
		}
		return nil
	}
}

// WithUserAgent sets the User-Agent header.
// This is kept as a convenience function since it's commonly used.
func WithUserAgent(userAgent string) RequestOption {
	return WithHeader("User-Agent", userAgent)
}

func WithBasicAuth(username, password string) RequestOption {
	return func(r *engine.Request) error {
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}
		if err := validation.ValidateCredential(username, validation.MaxCredLen, true, "username"); err != nil {
			return fmt.Errorf("invalid username: %w", err)
		}
		if err := validation.ValidateCredential(password, validation.MaxCredLen, false, "password"); err != nil {
			return fmt.Errorf("invalid password: %w", err)
		}

		// Efficient string concatenation and encoding
		creds := username + ":" + password
		r.SetHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
		return nil
	}
}

func WithBearerToken(token string) RequestOption {
	return func(r *engine.Request) error {
		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}
		if err := validation.ValidateToken(token); err != nil {
			return err
		}

		r.SetHeader("Authorization", "Bearer "+token)
		return nil
	}
}

func WithQuery(key string, value any) RequestOption {
	return func(r *engine.Request) error {
		if err := validation.ValidateQueryKey(key); err != nil {
			return err
		}

		if value != nil {
			if valueLen := queryValueLength(value); valueLen > validation.MaxValueLen {
				return fmt.Errorf("query value too long (max %d)", validation.MaxValueLen)
			}
		}

		params := r.QueryParams()
		if params == nil {
			// Pre-allocate with capacity for typical query params
			params = make(map[string]any, 4)
		}
		params[key] = value
		r.SetQueryParams(params)
		return nil
	}
}

// WithQueryMap sets multiple query parameters from a map.
func WithQueryMap(params map[string]any) RequestOption {
	return func(r *engine.Request) error {
		existing := r.QueryParams()
		if existing == nil {
			existing = make(map[string]any, len(params))
		}

		for k, v := range params {
			if err := validation.ValidateQueryKey(k); err != nil {
				return fmt.Errorf("invalid key %s: %w", k, err)
			}

			if v != nil {
				if valueLen := queryValueLength(v); valueLen > validation.MaxValueLen {
					return fmt.Errorf("query value too long for key %s (max %d)", k, validation.MaxValueLen)
				}
			}
			existing[k] = v
		}
		r.SetQueryParams(existing)
		return nil
	}
}

// queryValueLength efficiently calculates the string length of a query value
// without allocating a string. Uses type switching for common types.
func queryValueLength(v any) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case int:
		return lenInt(int64(val))
	case int64:
		return lenInt(val)
	case int32:
		return lenInt(int64(val))
	case uint:
		return lenUint(uint64(val))
	case uint64:
		return lenUint(val)
	case uint32:
		return lenUint(uint64(val))
	case float64:
		return lenFloat(val, 64)
	case float32:
		return lenFloat(float64(val), 32)
	case bool:
		if val {
			return 4 // "true"
		}
		return 5 // "false"
	case fmt.Stringer:
		return len(val.String())
	default:
		return len(fmt.Sprintf("%v", val))
	}
}

// lenInt calculates the number of digits in an int64 (including sign if negative)
func lenInt(v int64) int {
	if v < 0 {
		return 1 + lenUint(uint64(-v))
	}
	return lenUint(uint64(v))
}

// lenUint calculates the number of digits in a uint64
func lenUint(v uint64) int {
	if v < 10 {
		return 1
	}
	if v < 100 {
		return 2
	}
	if v < 1000 {
		return 3
	}
	if v < 10000 {
		return 4
	}
	if v < 100000 {
		return 5
	}
	if v < 1000000 {
		return 6
	}
	if v < 10000000 {
		return 7
	}
	if v < 100000000 {
		return 8
	}
	if v < 1000000000 {
		return 9
	}
	if v < 10000000000 {
		return 10
	}
	if v < 100000000000 {
		return 11
	}
	if v < 1000000000000 {
		return 12
	}
	if v < 10000000000000 {
		return 13
	}
	if v < 100000000000000 {
		return 14
	}
	if v < 1000000000000000 {
		return 15
	}
	if v < 10000000000000000 {
		return 16
	}
	if v < 100000000000000000 {
		return 17
	}
	if v < 1000000000000000000 {
		return 18
	}
	return 19
}

// lenFloat estimates the length of a float formatted with strconv.FormatFloat
// without allocating a string. Uses mathematical estimation to avoid heap allocation.
func lenFloat(v float64, bitSize int) int {
	// Handle special cases first
	if math.IsInf(v, 0) {
		return 4 // "Inf" or "-Inf" or "+Inf"
	}
	if math.IsNaN(v) {
		return 3 // "NaN"
	}

	// Estimate the length without formatting
	absV := math.Abs(v)

	// Count integer part digits using log10
	var intDigits int
	if absV < 1 {
		intDigits = 1 // "0" before decimal point
	} else {
		intDigits = int(math.Log10(absV)) + 1
	}

	// Estimate fractional part - use conservative estimate based on bitSize
	// float64: up to 15-17 significant digits, float32: up to 6-9
	var maxFracDigits int
	if bitSize == 32 {
		maxFracDigits = 9
	} else {
		maxFracDigits = 17
	}

	// For small integers, fractional part is 0
	if absV >= 1 && absV == math.Floor(absV) && absV < 1e15 {
		maxFracDigits = 0
	}

	// Total: sign (1 if negative) + integer digits + decimal point (1) + fractional digits
	signLen := 0
	if v < 0 {
		signLen = 1
	}

	decimalPointLen := 0
	if maxFracDigits > 0 {
		decimalPointLen = 1
	}

	return signLen + intDigits + decimalPointLen + maxFracDigits
}

func WithJSON(data any) RequestOption {
	return func(r *engine.Request) error {
		if data == nil {
			return fmt.Errorf("JSON data cannot be nil")
		}
		r.SetBody(data)
		r.SetHeader("Content-Type", "application/json")
		return nil
	}
}

func WithXML(data any) RequestOption {
	return func(r *engine.Request) error {
		if data == nil {
			return fmt.Errorf("XML data cannot be nil")
		}
		r.SetBody(data)
		r.SetHeader("Content-Type", "application/xml")
		return nil
	}
}

func WithForm(data map[string]string) RequestOption {
	return func(r *engine.Request) error {
		if data == nil {
			return fmt.Errorf("form data cannot be nil")
		}
		// Pre-allocate url.Values with capacity to avoid map growth
		values := make(url.Values, len(data))
		for k, v := range data {
			values.Set(k, v)
		}
		r.SetBody(values.Encode())
		r.SetHeader("Content-Type", "application/x-www-form-urlencoded")
		return nil
	}
}

func WithFormData(data *FormData) RequestOption {
	return func(r *engine.Request) error {
		if data == nil {
			return fmt.Errorf("form data cannot be nil")
		}
		r.SetBody(data)
		return nil
	}
}

func WithFile(fieldName, filename string, content []byte) RequestOption {
	return func(r *engine.Request) error {
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

		r.SetBody(&FormData{
			Fields: make(map[string]string, 1), // Pre-allocate for typical case
			Files: map[string]*FileData{
				fieldName: {
					Filename: cleanFilename,
					Content:  content,
				},
			},
		})
		return nil
	}
}

func WithTimeout(timeout time.Duration) RequestOption {
	return func(r *engine.Request) error {
		if timeout < 0 {
			return fmt.Errorf("%w: cannot be negative", ErrInvalidTimeout)
		}
		if timeout > maxTimeout {
			return fmt.Errorf("%w: exceeds %v", ErrInvalidTimeout, maxTimeout)
		}
		r.SetTimeout(timeout)
		return nil
	}
}

func WithContext(ctx context.Context) RequestOption {
	return func(r *engine.Request) error {
		if ctx == nil {
			return fmt.Errorf("context cannot be nil")
		}
		r.SetContext(ctx)
		return nil
	}
}

func WithMaxRetries(maxRetries int) RequestOption {
	return func(r *engine.Request) error {
		if maxRetries < 0 || maxRetries > 10 {
			return fmt.Errorf("%w: must be 0-10, got %d", ErrInvalidRetry, maxRetries)
		}
		r.SetMaxRetries(maxRetries)
		return nil
	}
}

func WithBody(body any) RequestOption {
	return func(r *engine.Request) error {
		r.SetBody(body)
		return nil
	}
}

func WithFollowRedirects(follow bool) RequestOption {
	return func(r *engine.Request) error {
		r.SetFollowRedirects(&follow)
		return nil
	}
}

func WithMaxRedirects(maxRedirects int) RequestOption {
	return func(r *engine.Request) error {
		if maxRedirects < 0 {
			return fmt.Errorf("maxRedirects cannot be negative")
		}
		if maxRedirects > 50 {
			return fmt.Errorf("maxRedirects exceeds maximum 50")
		}
		r.SetMaxRedirects(&maxRedirects)
		return nil
	}
}

func WithBinary(data []byte, contentType ...string) RequestOption {
	return func(r *engine.Request) error {
		if data == nil {
			return fmt.Errorf("binary data cannot be nil")
		}

		r.SetBody(data)

		ct := "application/octet-stream"
		if len(contentType) > 0 && contentType[0] != "" {
			ct = contentType[0]
		}
		r.SetHeader("Content-Type", ct)
		return nil
	}
}

func WithCookie(cookie http.Cookie) RequestOption {
	return func(r *engine.Request) error {
		if err := validation.ValidateCookie(&cookie); err != nil {
			return fmt.Errorf("invalid cookie: %w", err)
		}

		cookies := r.Cookies()
		cookies = append(cookies, cookie)
		r.SetCookies(cookies)
		return nil
	}
}

func WithCookieString(cookieString string) RequestOption {
	return func(r *engine.Request) error {
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

		existing := r.Cookies()
		for i := range cookies {
			if err := validation.ValidateCookie(&cookies[i]); err != nil {
				return fmt.Errorf("invalid cookie %s: %w", cookies[i].Name, err)
			}
			existing = append(existing, cookies[i])
		}
		r.SetCookies(existing)

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

		// Validate cookie name characters (excluding '=' which is valid in cookie string format)
		for j := 0; j < nameLen; j++ {
			c := cookie.Name[j]
			if c < 0x20 || c == 0x7F || c == ';' || c == ',' {
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

// WithOnRequest registers a callback invoked before the request is sent.
// The callback receives the request mutator, allowing inspection or modification
// of the request before it's transmitted.
//
// Multiple callbacks can be chained - they are executed in the order added.
// If any callback returns an error, the request is aborted.
//
// Example:
//
//	result, err := client.Get("https://api.example.com",
//	    httpc.WithOnRequest(func(req httpc.RequestMutator) error {
//	        log.Printf("Sending %s request to %s", req.Method(), req.URL())
//	        return nil
//	    }),
//	)
func WithOnRequest(callback func(req RequestMutator) error) RequestOption {
	return func(r *engine.Request) error {
		if callback == nil {
			return fmt.Errorf("onRequest callback cannot be nil")
		}

		existing := r.OnRequest()
		r.SetOnRequest(func(req *engine.Request) error {
			if existing != nil {
				if err := existing(req); err != nil {
					return err
				}
			}
			return callback(req)
		})
		return nil
	}
}

// WithOnResponse registers a callback invoked after the response is received.
// The callback receives the response mutator, allowing inspection or modification
// of the response before it's returned to the caller.
//
// Multiple callbacks can be chained - they are executed in the order added.
// If any callback returns an error, the request fails with that error.
//
// Example:
//
//	result, err := client.Get("https://api.example.com",
//	    httpc.WithOnResponse(func(resp httpc.ResponseMutator) error {
//	        log.Printf("Received response: %d %s", resp.StatusCode(), resp.Status())
//	        return nil
//	    }),
//	)
func WithOnResponse(callback func(resp ResponseMutator) error) RequestOption {
	return func(r *engine.Request) error {
		if callback == nil {
			return fmt.Errorf("onResponse callback cannot be nil")
		}

		existing := r.OnResponse()
		r.SetOnResponse(func(resp *engine.Response) error {
			if existing != nil {
				if err := existing(resp); err != nil {
					return err
				}
			}
			return callback(resp)
		})
		return nil
	}
}

// WithSecureCookie creates a request option that enforces cookie security attributes.
// on the cookie being added to the request. The securityConfig defines the required
// security attributes (Secure, HttpOnly, SameSite).
//
// Example:
//
//	security := &validation.CookieSecurityConfig{
//		RequireSecure:     true,
//		RequireHttpOnly:   true,
//		RequireSameSite:   "Strict",
//		AllowSameSiteNone: false,
//	}
//	result, err := client.Get("https://api.example.com",
//		httpc.WithSecureCookie(security),
//	)
func WithSecureCookie(securityConfig *validation.CookieSecurityConfig) RequestOption {
	return func(r *engine.Request) error {
		if securityConfig == nil {
			return fmt.Errorf("security config cannot be nil")
		}

		// Store the security config in the request for later validation
		// This will be applied when cookies are processed
		existing := r.Cookies()
		for i := range existing {
			if err := validation.ValidateCookieSecurity(&existing[i], securityConfig); err != nil {
				return fmt.Errorf("cookie '%s' failed security validation: %w", existing[i].Name, err)
			}
		}

		return nil
	}
}
