package httpc

import (
	"fmt"
	"maps"
	"net/http"
	"sync"

	"github.com/cybergodev/httpc/internal/engine"
	"github.com/cybergodev/httpc/internal/validation"
)

// SessionManager manages session state including cookies and headers
// for DomainClient instances. It provides thread-safe access to session data.
type SessionManager struct {
	mu             sync.RWMutex
	cookies        map[string]*http.Cookie
	headers        map[string]string
	cookieSecurity *validation.CookieSecurityConfig
}

// NewSessionManager creates a new SessionManager with empty session state.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		cookies: make(map[string]*http.Cookie),
		headers: make(map[string]string),
	}
}

// NewSessionManagerWithSecurity creates a new SessionManager with cookie security validation.
// The cookieSecurity config is used to validate cookies when they are set.
func NewSessionManagerWithSecurity(cookieSecurity *validation.CookieSecurityConfig) *SessionManager {
	return &SessionManager{
		cookies:        make(map[string]*http.Cookie),
		headers:        make(map[string]string),
		cookieSecurity: cookieSecurity,
	}
}

// SetCookieSecurity sets the cookie security configuration.
// This affects all subsequent SetCookie calls.
func (s *SessionManager) SetCookieSecurity(config *validation.CookieSecurityConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cookieSecurity = config
}

// SetHeader adds or updates a header in the session.
// Returns an error if the header key or value is invalid.
func (s *SessionManager) SetHeader(key, value string) error {
	if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.headers[key] = value
	return nil
}

// SetHeaders adds or updates multiple headers in the session.
// Returns an error if any header key or value is invalid.
func (s *SessionManager) SetHeaders(headers map[string]string) error {
	for k, v := range headers {
		if err := validation.ValidateHeaderKeyValue(k, v); err != nil {
			return fmt.Errorf("invalid header %s: %w", k, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	maps.Copy(s.headers, headers)
	return nil
}

// DeleteHeader removes a header from the session.
func (s *SessionManager) DeleteHeader(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.headers, key)
}

// ClearHeaders removes all headers from the session.
func (s *SessionManager) ClearHeaders() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.headers = make(map[string]string)
}

// GetHeaders returns a copy of all session headers.
func (s *SessionManager) GetHeaders() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	headers := make(map[string]string, len(s.headers))
	maps.Copy(headers, s.headers)
	return headers
}

// SetCookie adds or updates a cookie in the session.
// Returns an error if the cookie is nil or invalid.
// If cookie security is configured, validates against security requirements.
func (s *SessionManager) SetCookie(cookie *http.Cookie) error {
	if cookie == nil {
		return fmt.Errorf("cookie cannot be nil")
	}
	if err := validation.ValidateCookie(cookie); err != nil {
		return fmt.Errorf("invalid cookie: %w", err)
	}

	// Apply cookie security validation if configured
	if s.cookieSecurity != nil {
		if err := validation.ValidateCookieSecurity(cookie, s.cookieSecurity); err != nil {
			return fmt.Errorf("cookie security validation failed: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cookies[cookie.Name] = cookie
	return nil
}

// SetCookies adds or updates multiple cookies in the session.
// Returns an error if any cookie is nil or invalid.
// If cookie security is configured, validates against security requirements.
func (s *SessionManager) SetCookies(cookies []*http.Cookie) error {
	for i, cookie := range cookies {
		if cookie == nil {
			return fmt.Errorf("cookie at index %d is nil", i)
		}
		if err := validation.ValidateCookie(cookie); err != nil {
			return fmt.Errorf("invalid cookie at index %d: %w", i, err)
		}

		// Apply cookie security validation if configured
		if s.cookieSecurity != nil {
			if err := validation.ValidateCookieSecurity(cookie, s.cookieSecurity); err != nil {
				return fmt.Errorf("cookie security validation failed at index %d: %w", i, err)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cookie := range cookies {
		s.cookies[cookie.Name] = cookie
	}
	return nil
}

// DeleteCookie removes a cookie from the session by name.
func (s *SessionManager) DeleteCookie(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cookies, name)
}

// ClearCookies removes all cookies from the session.
func (s *SessionManager) ClearCookies() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cookies = make(map[string]*http.Cookie)
}

// GetCookies returns a copy of all session cookies.
// Optimized to reduce allocations by pre-allocating slice with exact capacity.
func (s *SessionManager) GetCookies() []*http.Cookie {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n := len(s.cookies)
	if n == 0 {
		return nil
	}
	cookies := make([]*http.Cookie, 0, n)
	for _, cookie := range s.cookies {
		cookieCopy := *cookie
		cookies = append(cookies, &cookieCopy)
	}
	return cookies
}

// GetCookie returns a copy of a cookie by name, or nil if not found.
func (s *SessionManager) GetCookie(name string) *http.Cookie {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cookie, ok := s.cookies[name]; ok {
		cookieCopy := *cookie
		return &cookieCopy
	}
	return nil
}

// PrepareOptions creates RequestOptions from the current session state.
// This is used to apply session cookies and headers to outgoing requests.
func (s *SessionManager) PrepareOptions() []RequestOption {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cookieCount := len(s.cookies)
	headerCount := len(s.headers)

	if cookieCount == 0 && headerCount == 0 {
		return nil
	}

	options := make([]RequestOption, 0, 2)

	if cookieCount > 0 {
		for _, cookie := range s.cookies {
			options = append(options, WithCookie(*cookie))
		}
	}

	if headerCount > 0 {
		headersCopy := make(map[string]string, headerCount)
		maps.Copy(headersCopy, s.headers)
		options = append(options, WithHeaders(headersCopy))
	}

	return options
}

// UpdateFromResult updates session cookies from a Result.
func (s *SessionManager) UpdateFromResult(result *Result) {
	if result == nil || result.Response == nil || len(result.Response.Cookies) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cookie := range result.Response.Cookies {
		if cookie != nil {
			s.cookies[cookie.Name] = cookie
		}
	}
}

// CaptureFromOptions extracts cookies and headers from RequestOptions
// and stores them in the session.
func (s *SessionManager) CaptureFromOptions(options []RequestOption) {
	if len(options) == 0 {
		return
	}

	// Use engine.Request which implements RequestMutator
	tempReq := &engine.Request{}

	for _, opt := range options {
		if opt != nil {
			// Best effort: ignore errors during option capture for session persistence.
			// Errors from individual options are handled during actual request execution.
			_ = opt(tempReq)
		}
	}

	cookies := tempReq.Cookies()
	headers := tempReq.Headers()

	if len(cookies) == 0 && len(headers) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range cookies {
		cookie := &cookies[i]
		s.cookies[cookie.Name] = cookie
	}

	for key, value := range headers {
		s.headers[key] = value
	}
}
