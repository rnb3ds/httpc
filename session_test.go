package httpc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybergodev/httpc/internal/validation"
)

// ============================================================================
// SESSION MANAGER TESTS
// ============================================================================

// TestNewSessionManager construction with defaults is covered by
// TestNewSessionManagerDefault (DefaultSessionConfig is its input), so no
// separate minimal-constructor test is kept here.

// (TestNewSessionManagerWithConfig was folded into TestNewSessionManagerDefault
// below as its "with cookie security config" subtest — the two constructors'
// only difference was whether CookieSecurity was preconfigured.)

// TestSessionManager_SetCookie_DefensiveCopy pins the M1 regression: the
// session must store a COPY of the caller's *http.Cookie — storing the
// pointer would let later caller-side mutations race the session's locked
// read paths (which snapshot values under RLock) and break the snapshot
// semantics of GetCookies/prepareOptions. storeCookies and
// captureFromOptions already copy; SetCookie/SetCookies must match.
func TestSessionManager_SetCookie_DefensiveCopy(t *testing.T) {
	sm, err := NewSessionManager(DefaultSessionConfig())
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	t.Run("SetCookie", func(t *testing.T) {
		c := &http.Cookie{Name: "sid", Value: "original"}
		if err := sm.SetCookie(c); err != nil {
			t.Fatalf("SetCookie error: %v", err)
		}
		c.Value = "mutated-by-caller"
		got := sm.GetCookie("sid")
		if got == nil {
			t.Fatal("GetCookie returned nil")
		}
		if got.Value != "original" {
			t.Errorf("stored cookie tracks the caller's pointer: got %q, want %q", got.Value, "original")
		}
	})

	t.Run("SetCookies", func(t *testing.T) {
		c := &http.Cookie{Name: "batch", Value: "original"}
		if err := sm.SetCookies([]*http.Cookie{c}); err != nil {
			t.Fatalf("SetCookies error: %v", err)
		}
		c.Value = "mutated-by-caller"
		got := sm.GetCookie("batch")
		if got == nil {
			t.Fatal("GetCookie returned nil")
		}
		if got.Value != "original" {
			t.Errorf("stored cookie tracks the caller's pointer: got %q, want %q", got.Value, "original")
		}
	})
}

func TestNewSessionManagerDefault(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		// NewSessionManagerDefault is a shortcut for NewSessionManager(DefaultSessionConfig()).
		session, err := NewSessionManagerDefault()
		if err != nil {
			t.Fatalf("NewSessionManagerDefault error: %v", err)
		}
		if session == nil {
			t.Fatal("Expected non-nil SessionManager")
		}
		if len(session.cookies) != 0 {
			t.Error("Expected empty cookies map")
		}
		if len(session.headers) != 0 {
			t.Error("Expected empty headers map")
		}
		if session.cookieSecurity != nil {
			t.Error("Expected nil cookieSecurity with default config")
		}
	})

	t.Run("with cookie security config", func(t *testing.T) {
		// Formerly TestNewSessionManagerWithConfig: NewSessionManager stores a
		// preconfigured CookieSecurity policy.
		securityConfig := validation.StrictCookieSecurityConfig()
		cfg := DefaultSessionConfig()
		cfg.CookieSecurity = securityConfig
		session, err := NewSessionManager(cfg)

		if err != nil {
			t.Fatalf("NewSessionManager error: %v", err)
		}
		if session == nil {
			t.Fatal("Expected non-nil SessionManager")
		}
		if session.cookieSecurity == nil {
			t.Error("Expected cookieSecurity to be set")
		}
	})
}

// TestSessionManagerZeroValue exercises the write methods on a zero-value
// SessionManager (&SessionManager{} instead of NewSessionManager): the maps
// are initialized lazily on first write, so a mis-constructed manager behaves
// as an empty session instead of panicking with "assignment to entry in nil
// map" (SEC-003 regression guard).
func TestSessionManagerZeroValue(t *testing.T) {
	s := &SessionManager{}

	if err := s.SetHeader("Accept", "application/json"); err != nil {
		t.Fatalf("SetHeader on zero value: %v", err)
	}
	if err := s.SetHeaders(map[string]string{"X-A": "b"}); err != nil {
		t.Fatalf("SetHeaders on zero value: %v", err)
	}
	if err := s.SetCookie(&http.Cookie{Name: "sid", Value: "1"}); err != nil {
		t.Fatalf("SetCookie on zero value: %v", err)
	}
	if err := s.SetCookies([]*http.Cookie{{Name: "c2", Value: "v"}}); err != nil {
		t.Fatalf("SetCookies on zero value: %v", err)
	}
	s.UpdateFromCookies([]*http.Cookie{{Name: "c3", Value: "v"}})
	s.UpdateFromResult(&Result{Response: &ResponseInfo{Cookies: []*http.Cookie{{Name: "c4", Value: "v"}}}})
	s.captureFromOptions([]RequestOption{
		WithHeader("X-Zero", "1"),
		WithCookies([]http.Cookie{{Name: "c5", Value: "v"}}),
	})

	if got := s.GetCookie("sid"); got == nil || got.Value != "1" {
		t.Errorf("GetCookie(sid) = %+v, want value %q", got, "1")
	}
	if v, ok := s.GetHeaders()["X-A"]; !ok || v != "b" {
		t.Errorf("GetHeaders()[X-A] = %q, %v", v, ok)
	}
	for _, name := range []string{"c2", "c3", "c4", "c5"} {
		if s.GetCookie(name) == nil {
			t.Errorf("cookie %s missing after write on zero value", name)
		}
	}

	// Reads on a never-written zero value must also work (nil-map reads).
	s2 := &SessionManager{}
	if h := s2.GetHeaders(); len(h) != 0 {
		t.Errorf("GetHeaders on untouched zero value = %v, want empty", h)
	}
	if c := s2.GetCookies(); c != nil {
		t.Errorf("GetCookies on untouched zero value = %v, want nil", c)
	}
	if c := s2.GetCookie("missing"); c != nil {
		t.Errorf("GetCookie on untouched zero value = %v, want nil", c)
	}
}

// (TestSessionManager_SetCookieSecurity and TestSessionManager_SecurityValidation_SetCookieSecurity
// were folded into TestSessionManager_CookieSecurityValidation below — all three
// tested the same contract: the effective cookie-security policy (however it was
// installed) gates SetCookie.)

func TestSessionManager_CookieSecurityValidation(t *testing.T) {
	t.Run("constructor-configured policy rejects insecure, accepts secure", func(t *testing.T) {
		// Create session with strict security
		securityConfig := validation.StrictCookieSecurityConfig()
		cfg := DefaultSessionConfig()
		cfg.CookieSecurity = securityConfig
		session, err := NewSessionManager(cfg)
		if err != nil {
			t.Fatalf("NewSessionManager error: %v", err)
		}

		// Try to set insecure cookie - should fail
		insecureCookie := &http.Cookie{
			Name:     "session",
			Value:    "test123",
			Secure:   false, // Should be true for strict config
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}

		err = session.SetCookie(insecureCookie)
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
	})

	t.Run("setter replaces nil policy", func(t *testing.T) {
		// Formerly TestSessionManager_SetCookieSecurity: a default session has
		// no policy until SetCookieSecurity installs one.
		session, err := NewSessionManagerDefault()
		if err != nil {
			t.Fatalf("NewSessionManager error: %v", err)
		}

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
	})

	t.Run("setter-installed policy enforced on subsequent SetCookie", func(t *testing.T) {
		// Formerly TestSessionManager_SecurityValidation_SetCookieSecurity:
		// SetCookieSecurity affects subsequent SetCookie calls.
		session, err := NewSessionManagerDefault()
		if err != nil {
			t.Fatalf("NewSessionManager error: %v", err)
		}

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

		err = session.SetCookie(insecureCookie)
		if err == nil {
			t.Error("Expected error for insecure cookie with RequireSecure=true")
		}
	})
}

// newTestSession returns an initialized SessionManager, collapsing the repeated
// NewSessionManager + error-check boilerplate shared by the accessor tests.
// (Not used by TestNewSessionManager itself, which validates the constructor.)
func newTestSession(t *testing.T) *SessionManager {
	t.Helper()
	session, err := NewSessionManagerDefault()
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	return session
}

// TestSessionManager_CRUDOperations table-drives the eight one-assert
// accessor round-trips (formerly eight standalone SetCookie/SetCookies/
// DeleteCookie/ClearCookies/SetHeader/SetHeaders/DeleteHeader/ClearHeaders
// funcs; each original assertion is preserved verbatim in its row).
func TestSessionManager_CRUDOperations(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, session *SessionManager)
	}{
		{"SetCookie", func(t *testing.T, session *SessionManager) {
			// Test nil cookie
			if err := session.SetCookie(nil); err == nil {
				t.Error("Expected error for nil cookie")
			}

			// Test valid cookie
			cookie := &http.Cookie{
				Name:  "test",
				Value: "value",
			}
			if err := session.SetCookie(cookie); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify cookie was stored
			stored := session.GetCookie("test")
			if stored == nil {
				t.Fatal("Expected cookie to be stored")
			}
			if stored.Value != "value" {
				t.Errorf("Expected value 'value', got %s", stored.Value)
			}
		}},
		{"SetCookies", func(t *testing.T, session *SessionManager) {
			cookies := []*http.Cookie{
				{Name: "cookie1", Value: "value1"},
				{Name: "cookie2", Value: "value2"},
			}

			if err := session.SetCookies(cookies); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			allCookies := session.GetCookies()
			if len(allCookies) != 2 {
				t.Errorf("Expected 2 cookies, got %d", len(allCookies))
			}
		}},
		{"DeleteCookie", func(t *testing.T, session *SessionManager) {
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
		}},
		{"ClearCookies", func(t *testing.T, session *SessionManager) {
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
		}},
		{"SetHeader", func(t *testing.T, session *SessionManager) {
			// Valid header
			if err := session.SetHeader("X-Custom", "value"); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Invalid header (with CRLF)
			if err := session.SetHeader("X-Bad", "value\r\nX-Injected: malicious"); err == nil {
				t.Error("Expected error for header with CRLF")
			}

			// Oversize key / value are rejected by ValidateHeaderKeyValue limits.
			if err := session.SetHeader("X-"+strings.Repeat("k", validation.MaxHeaderKeyLen), "value"); err == nil {
				t.Error("Expected error for oversize header key")
			}
			if err := session.SetHeader("X-Key", strings.Repeat("v", validation.MaxValueLen+1)); err == nil {
				t.Error("Expected error for oversize header value")
			}
		}},
		{"SetHeaders", func(t *testing.T, session *SessionManager) {
			headers := map[string]string{
				"X-Header-1": "value1",
				"X-Header-2": "value2",
			}

			if err := session.SetHeaders(headers); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			allHeaders := session.GetHeaders()
			if len(allHeaders) != 2 {
				t.Errorf("Expected 2 headers, got %d", len(allHeaders))
			}
		}},
		{"DeleteHeader", func(t *testing.T, session *SessionManager) {
			_ = session.SetHeader("X-Test", "value")
			session.DeleteHeader("X-Test")

			allHeaders := session.GetHeaders()
			if _, exists := allHeaders["X-Test"]; exists {
				t.Error("Expected header to be deleted")
			}
		}},
		{"ClearHeaders", func(t *testing.T, session *SessionManager) {
			_ = session.SetHeader("X-Header-1", "value1")
			_ = session.SetHeader("X-Header-2", "value2")

			session.ClearHeaders()

			allHeaders := session.GetHeaders()
			if len(allHeaders) != 0 {
				t.Errorf("Expected 0 headers after clear, got %d", len(allHeaders))
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, newTestSession(t))
		})
	}
}

// TestSessionManager_HeaderKeyCasing pins the canonical-key storage contract
// for session headers: keys are stored in http.CanonicalHeaderKey form, the
// same header under two casings collapses onto one entry (last write wins),
// and DeleteHeader removes case-insensitively.
func TestSessionManager_HeaderKeyCasing(t *testing.T) {
	t.Run("SetHeader canonicalizes and collapses casings", func(t *testing.T) {
		session := newTestSession(t)

		if err := session.SetHeader("Cache-Control", "no-store"); err != nil {
			t.Fatalf("SetHeader: %v", err)
		}
		if err := session.SetHeader("cache-control", "max-age=0"); err != nil {
			t.Fatalf("SetHeader: %v", err)
		}

		allHeaders := session.GetHeaders()
		if len(allHeaders) != 1 {
			t.Fatalf("Expected 1 header after two casings, got %d (%v)", len(allHeaders), allHeaders)
		}
		if v, ok := allHeaders["Cache-Control"]; !ok || v != "max-age=0" {
			t.Errorf("GetHeaders()[Cache-Control] = %q, %v; want %q, true", v, ok, "max-age=0")
		}
	})

	t.Run("SetHeaders collapses mixed-case duplicates", func(t *testing.T) {
		session := newTestSession(t)

		err := session.SetHeaders(map[string]string{
			"Accept-Language": "en-US",
			"accept-language": "zh-CN", // 'a'(0x61) sorts after 'A'(0x41), must win
		})
		if err != nil {
			t.Fatalf("SetHeaders: %v", err)
		}

		allHeaders := session.GetHeaders()
		if len(allHeaders) != 1 {
			t.Fatalf("Expected 1 header, got %d (%v)", len(allHeaders), allHeaders)
		}
		if v := allHeaders["Accept-Language"]; v != "zh-CN" {
			t.Errorf("Accept-Language = %q, want later-sorted %q", v, "zh-CN")
		}
	})

	t.Run("DeleteHeader is case-insensitive", func(t *testing.T) {
		session := newTestSession(t)

		if err := session.SetHeader("Cache-Control", "no-store"); err != nil {
			t.Fatalf("SetHeader: %v", err)
		}
		session.DeleteHeader("cache-control")

		allHeaders := session.GetHeaders()
		if _, exists := allHeaders["Cache-Control"]; exists {
			t.Error("DeleteHeader with different casing failed to remove the header")
		}
	})
}

// (TestSessionManager_ClearHeaders was folded into
// TestSessionManager_CRUDOperations/ClearHeaders above.)

func TestSessionManager_prepareOptions(t *testing.T) {
	session, err := NewSessionManagerDefault()
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	// Test empty session
	options := session.prepareOptions()
	if options != nil {
		t.Error("Expected nil options for empty session")
	}

	// Add cookies and headers
	_ = session.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})
	_ = session.SetHeader("Authorization", "Bearer token")

	options = session.prepareOptions()
	if len(options) < 2 {
		t.Errorf("Expected at least 2 options, got %d", len(options))
	}

	// Verify options actually work when applied to a request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("Authorization header not applied")
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value != "abc123" {
			t.Errorf("Session cookie not applied: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer func() { _ = client.Close() }()

	resp, err := client.Get(server.URL, options...)
	if err != nil {
		t.Fatalf("Request with session options failed: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode())
	}
}

func TestSessionManager_UpdateFromResult(t *testing.T) {
	session, err := NewSessionManagerDefault()
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

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
		t.Fatal("Expected cookie from result")
	}
	if cookie.Value != "server-value" {
		t.Errorf("Expected value 'server-value', got %s", cookie.Value)
	}
}

// ----------------------------------------------------------------------------
// UpdateFromCookies / SetCookies edge cases
// ----------------------------------------------------------------------------

func TestSessionManager_UpdateFromCookies(t *testing.T) {
	session, err := NewSessionManagerDefault()
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	t.Run("empty slice", func(t *testing.T) {
		session.UpdateFromCookies([]*http.Cookie{})
	})

	t.Run("nil cookie skipped", func(t *testing.T) {
		session.UpdateFromCookies([]*http.Cookie{
			nil,
			{Name: "test", Value: "val"},
		})
		c := session.GetCookie("test")
		if c == nil || c.Value != "val" {
			t.Error("should have stored non-nil cookie")
		}
	})
}

func TestSessionManager_SetCookies_NilElement(t *testing.T) {
	session, err := NewSessionManagerDefault()
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}

	// Slice with nil element at index > 0 should return error
	err = session.SetCookies([]*http.Cookie{
		{Name: "a", Value: "1"},
		nil,
	})
	if err == nil {
		t.Error("expected error for nil cookie element")
	}
}

// ----------------------------------------------------------------------------
// Nil-receiver safety
// ----------------------------------------------------------------------------

// TestSessionManager_NilReceiverSafety verifies every SessionManager method
// either returns an error or is a safe no-op when called on a nil receiver.
func TestSessionManager_NilReceiverSafety(t *testing.T) {
	var s *SessionManager // nil receiver

	t.Run("SetCookieSecurity does not panic", func(t *testing.T) {
		s.SetCookieSecurity(nil)
	})

	t.Run("SetHeader returns error", func(t *testing.T) {
		if err := s.SetHeader("key", "val"); err == nil {
			t.Error("expected error on nil receiver")
		}
	})

	t.Run("SetHeaders returns error", func(t *testing.T) {
		if err := s.SetHeaders(map[string]string{"k": "v"}); err == nil {
			t.Error("expected error on nil receiver")
		}
	})

	t.Run("SetHeaders with invalid header returns error", func(t *testing.T) {
		sm, _ := NewSessionManagerDefault()
		badHeaders := map[string]string{"": "empty-key"}
		if err := sm.SetHeaders(badHeaders); err == nil {
			t.Error("expected error for empty header key")
		}
	})

	t.Run("DeleteHeader does not panic", func(t *testing.T) {
		s.DeleteHeader("key")
	})

	t.Run("ClearHeaders does not panic", func(t *testing.T) {
		s.ClearHeaders()
	})

	t.Run("GetHeaders returns nil", func(t *testing.T) {
		if h := s.GetHeaders(); h != nil {
			t.Errorf("expected nil, got %v", h)
		}
	})

	t.Run("SetCookie returns error", func(t *testing.T) {
		if err := s.SetCookie(&http.Cookie{Name: "k", Value: "v"}); err == nil {
			t.Error("expected error on nil receiver")
		}
	})

	t.Run("SetCookies returns error", func(t *testing.T) {
		if err := s.SetCookies([]*http.Cookie{{Name: "k", Value: "v"}}); err == nil {
			t.Error("expected error on nil receiver")
		}
	})

	t.Run("DeleteCookie does not panic", func(t *testing.T) {
		s.DeleteCookie("key")
	})

	t.Run("ClearCookies does not panic", func(t *testing.T) {
		s.ClearCookies()
	})

	t.Run("GetCookies returns nil", func(t *testing.T) {
		if c := s.GetCookies(); c != nil {
			t.Errorf("expected nil, got %v", c)
		}
	})

	t.Run("GetCookie returns nil", func(t *testing.T) {
		if c := s.GetCookie("key"); c != nil {
			t.Errorf("expected nil, got %v", c)
		}
	})
}

// Moved from quality_regression_test.go (dissolved grab-bag file):
// TestCaptureFromOptionsSkipsInvalidCookies guards the ValidateCookie gate in
// captureFromOptions: cookies extracted from built-in cookie options must pass
// well-formedness validation before entering the session store.
func TestCaptureFromOptionsSkipsInvalidCookies(t *testing.T) {
	sm, err := NewSessionManagerDefault()
	if err != nil {
		t.Fatal(err)
	}

	// A custom RequestOptionFunc is not statically extractable: capture no
	// longer executes user options, so its cookies must NOT enter the session.
	sm.captureFromOptions([]RequestOption{
		RequestOptionFunc(func(r RequestMutator) error {
			r.SetCookies([]http.Cookie{{Name: "custom", Value: "x"}})
			return nil
		}),
	})
	if got := sm.GetCookie("custom"); got != nil {
		t.Errorf("custom option cookies must not be captured, got %v", got)
	}

	// A built-in cookie option IS statically extracted, and its invalid cookies
	// are rejected by the same ValidateCookie gate storeCookies uses.
	sm.captureFromOptions([]RequestOption{
		WithCookies([]http.Cookie{
			{Name: "good", Value: "fine"},
			{Name: "bad;name", Value: "invalid"}, // control/separator chars in name
		}),
	})
	if got := sm.GetCookie("good"); got == nil {
		t.Error("valid cookie from built-in option should be captured")
	}
	if got := sm.GetCookie("bad;name"); got != nil {
		t.Errorf("invalid cookie should be rejected, got %v", got)
	}
}
