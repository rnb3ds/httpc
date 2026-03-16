package httpc

import (
	"net/http"
	"testing"

	"github.com/cybergodev/httpc/internal/validation"
)

// ============================================================================
// SESSION MANAGER TESTS
// ============================================================================

func TestNewSessionManager(t *testing.T) {
	session := NewSessionManager()
	if session == nil {
		t.Fatal("Expected non-nil SessionManager")
	}
	if len(session.cookies) != 0 {
		t.Error("Expected empty cookies map")
	}
	if len(session.headers) != 0 {
		t.Error("Expected empty headers map")
	}
}

func TestNewSessionManagerWithSecurity(t *testing.T) {
	securityConfig := validation.StrictCookieSecurityConfig()
	session := NewSessionManagerWithSecurity(securityConfig)

	if session == nil {
		t.Fatal("Expected non-nil SessionManager")
	}
	if session.cookieSecurity == nil {
		t.Error("Expected cookieSecurity to be set")
	}
}

func TestSessionManager_SetCookieSecurity(t *testing.T) {
	session := NewSessionManager()

	// Initially no security config
	if session.cookieSecurity != nil {
		t.Error("Expected no cookie security initially")
	}

	// Set security config
	securityConfig := validation.StrictCookieSecurityConfig()
	session.SetCookieSecurity(securityConfig)

	if session.cookieSecurity == nil {
		t.Error("Expected cookieSecurity to be set")
	}
}

func TestSessionManager_CookieSecurityValidation(t *testing.T) {
	// Create session with strict security
	securityConfig := validation.StrictCookieSecurityConfig()
	session := NewSessionManagerWithSecurity(securityConfig)

	// Try to set insecure cookie - should fail
	insecureCookie := &http.Cookie{
		Name:     "session",
		Value:    "test123",
		Secure:   false, // Should be true for strict config
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	err := session.SetCookie(insecureCookie)
	if err == nil {
		t.Error("Expected error for insecure cookie with strict security")
	}

	// Try to set secure cookie - should succeed
	secureCookie := &http.Cookie{
		Name:     "session",
		Value:    "test123",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	}

	err = session.SetCookie(secureCookie)
	if err != nil {
		t.Errorf("Expected no error for secure cookie, got: %v", err)
	}
}

func TestSessionManager_SetCookie(t *testing.T) {
	session := NewSessionManager()

	// Test nil cookie
	err := session.SetCookie(nil)
	if err == nil {
		t.Error("Expected error for nil cookie")
	}

	// Test valid cookie
	cookie := &http.Cookie{
		Name:  "test",
		Value: "value",
	}
	err = session.SetCookie(cookie)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify cookie was stored
	stored := session.GetCookie("test")
	if stored == nil {
		t.Error("Expected cookie to be stored")
	}
	if stored.Value != "value" {
		t.Errorf("Expected value 'value', got %s", stored.Value)
	}
}

func TestSessionManager_SetCookies(t *testing.T) {
	session := NewSessionManager()

	cookies := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
	}

	err := session.SetCookies(cookies)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	allCookies := session.GetCookies()
	if len(allCookies) != 2 {
		t.Errorf("Expected 2 cookies, got %d", len(allCookies))
	}
}

func TestSessionManager_DeleteCookie(t *testing.T) {
	session := NewSessionManager()

	// Add cookie
	cookie := &http.Cookie{Name: "test", Value: "value"}
	_ = session.SetCookie(cookie)

	// Delete it
	session.DeleteCookie("test")

	// Verify it's gone
	stored := session.GetCookie("test")
	if stored != nil {
		t.Error("Expected cookie to be deleted")
	}
}

func TestSessionManager_ClearCookies(t *testing.T) {
	session := NewSessionManager()

	// Add multiple cookies
	_ = session.SetCookie(&http.Cookie{Name: "c1", Value: "v1"})
	_ = session.SetCookie(&http.Cookie{Name: "c2", Value: "v2"})

	// Clear all
	session.ClearCookies()

	// Verify empty
	allCookies := session.GetCookies()
	if len(allCookies) != 0 {
		t.Errorf("Expected 0 cookies after clear, got %d", len(allCookies))
	}
}

func TestSessionManager_SetHeader(t *testing.T) {
	session := NewSessionManager()

	// Test valid header
	err := session.SetHeader("X-Custom", "value")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test invalid header (with CRLF)
	err = session.SetHeader("X-Bad", "value\r\nX-Injected: malicious")
	if err == nil {
		t.Error("Expected error for header with CRLF")
	}
}

func TestSessionManager_SetHeaders(t *testing.T) {
	session := NewSessionManager()

	headers := map[string]string{
		"X-Header-1": "value1",
		"X-Header-2": "value2",
	}

	err := session.SetHeaders(headers)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	allHeaders := session.GetHeaders()
	if len(allHeaders) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(allHeaders))
	}
}

func TestSessionManager_DeleteHeader(t *testing.T) {
	session := NewSessionManager()

	_ = session.SetHeader("X-Test", "value")
	session.DeleteHeader("X-Test")

	allHeaders := session.GetHeaders()
	if _, exists := allHeaders["X-Test"]; exists {
		t.Error("Expected header to be deleted")
	}
}

func TestSessionManager_ClearHeaders(t *testing.T) {
	session := NewSessionManager()

	_ = session.SetHeader("X-Header-1", "value1")
	_ = session.SetHeader("X-Header-2", "value2")

	session.ClearHeaders()

	allHeaders := session.GetHeaders()
	if len(allHeaders) != 0 {
		t.Errorf("Expected 0 headers after clear, got %d", len(allHeaders))
	}
}

func TestSessionManager_PrepareOptions(t *testing.T) {
	session := NewSessionManager()

	// Test empty session
	options := session.PrepareOptions()
	if options != nil {
		t.Error("Expected nil options for empty session")
	}

	// Add cookies and headers
	_ = session.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})
	_ = session.SetHeader("Authorization", "Bearer token")

	options = session.PrepareOptions()
	if len(options) < 2 {
		t.Errorf("Expected at least 2 options, got %d", len(options))
	}
}

func TestSessionManager_UpdateFromResult(t *testing.T) {
	session := NewSessionManager()

	// Test nil result
	session.UpdateFromResult(nil)

	// Test result with cookies
	result := &Result{
		Response: &ResponseInfo{
			Cookies: []*http.Cookie{
				{Name: "server-cookie", Value: "server-value"},
			},
		},
	}

	session.UpdateFromResult(result)

	cookie := session.GetCookie("server-cookie")
	if cookie == nil {
		t.Error("Expected cookie from result")
	}
	if cookie.Value != "server-value" {
		t.Errorf("Expected value 'server-value', got %s", cookie.Value)
	}
}

func TestSessionManager_SecurityValidation(t *testing.T) {
	// Test that SetCookieSecurity affects subsequent SetCookie calls
	session := NewSessionManager()

	// Set security config after creation
	securityConfig := validation.DefaultCookieSecurityConfig()
	securityConfig.RequireSecure = true
	session.SetCookieSecurity(securityConfig)

	// This should fail because cookie is not secure
	insecureCookie := &http.Cookie{
		Name:   "test",
		Value:  "value",
		Secure: false,
	}

	err := session.SetCookie(insecureCookie)
	if err == nil {
		t.Error("Expected error for insecure cookie with RequireSecure=true")
	}
}
