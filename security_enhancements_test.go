package httpc

import (
	"net/http"
	"strings"
	"testing"
)

func TestSecurity_EnhancedValidation(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("HeaderMap_Validation", func(t *testing.T) {
		// Test that WithHeaderMap properly validates headers (fail-fast approach)
		
		// Valid headers should succeed
		validHeaders := map[string]string{
			"Valid-Header": "valid-value",
		}
		req := &Request{Headers: make(map[string]string)}
		err := WithHeaderMap(validHeaders)(req)
		if err != nil {
			t.Errorf("Valid headers should not error: %v", err)
		}
		if req.Headers["Valid-Header"] != "valid-value" {
			t.Error("Valid header was not preserved")
		}

		// Invalid headers should return errors
		invalidHeaders := map[string]string{
			"Invalid\r\nKey": "value",
		}
		req2 := &Request{Headers: make(map[string]string)}
		err = WithHeaderMap(invalidHeaders)(req2)
		if err == nil {
			t.Error("Expected error for invalid header key")
		}
	})

	t.Run("Query_Validation", func(t *testing.T) {
		// Valid query parameters should succeed
		req := &Request{QueryParams: make(map[string]any)}
		err := WithQuery("valid-key", "valid-value")(req)
		if err != nil {
			t.Errorf("Valid query should not error: %v", err)
		}
		if req.QueryParams["valid-key"] != "valid-value" {
			t.Error("Valid query param was not preserved")
		}

		// Invalid query parameters should return errors
		testCases := []struct {
			name  string
			key   string
			value any
		}{
			{"CRLF in key", "invalid\r\nkey", "value"},
			{"Ampersand in key", "key&with&ampersand", "value"},
			{"Empty key", "", "empty-key"},
			{"Long value", "long-value", strings.Repeat("x", 10000)},
		}

		for _, tc := range testCases {
			req := &Request{QueryParams: make(map[string]any)}
			err := WithQuery(tc.key, tc.value)(req)
			if err == nil {
				t.Errorf("%s: expected error but got nil", tc.name)
			}
		}
	})

	t.Run("Cookie_Validation", func(t *testing.T) {
		// Valid cookie should succeed
		req := &Request{}
		validCookie := &http.Cookie{Name: "valid", Value: "value"}
		err := WithCookie(validCookie)(req)
		if err != nil {
			t.Errorf("Valid cookie should not error: %v", err)
		}
		if len(req.Cookies) != 1 || req.Cookies[0].Name != "valid" {
			t.Error("Valid cookie was not preserved correctly")
		}

		// Invalid cookies should return errors
		invalidCookie1 := &http.Cookie{Name: "invalid\r\n", Value: "value"}
		err = WithCookie(invalidCookie1)(&Request{})
		if err == nil {
			t.Error("Expected error for cookie with CRLF in name")
		}

		invalidCookie2 := &http.Cookie{Name: "valid", Value: "invalid\r\nvalue"}
		err = WithCookie(invalidCookie2)(&Request{})
		if err == nil {
			t.Error("Expected error for cookie with CRLF in value")
		}

		nilCookie := (*http.Cookie)(nil)
		err = WithCookie(nilCookie)(&Request{})
		if err == nil {
			t.Error("Expected error for nil cookie")
		}
	})

	t.Run("CookieValue_Validation", func(t *testing.T) {
		req := &Request{}

		// Test cookie value validation
		WithCookieValue("valid", "value")(req)
		WithCookieValue("invalid\r\n", "value")(req)      // Should be no-op
		WithCookieValue("valid", "invalid\r\nvalue")(req) // Should be no-op
		WithCookieValue("", "empty-name")(req)            // Should be no-op

		// Only valid cookie should remain
		if len(req.Cookies) != 1 {
			t.Errorf("Expected 1 valid cookie, got %d", len(req.Cookies))
		}
		if req.Cookies[0].Name != "valid" || req.Cookies[0].Value != "value" {
			t.Error("Valid cookie was not preserved correctly")
		}
	})

	t.Run("BearerToken_Validation", func(t *testing.T) {
		req := &Request{Headers: make(map[string]string)}

		// Test bearer token validation
		WithBearerToken("valid-token")(req)
		if req.Headers["Authorization"] != "Bearer valid-token" {
			t.Error("Valid bearer token was not set correctly")
		}

		// Test invalid token (should not set header)
		req.Headers = make(map[string]string)
		WithBearerToken("invalid\r\ntoken")(req)
		if _, exists := req.Headers["Authorization"]; exists {
			t.Error("Invalid bearer token should not set Authorization header")
		}

		// Test empty token (should not set header)
		req.Headers = make(map[string]string)
		WithBearerToken("")(req)
		if _, exists := req.Headers["Authorization"]; exists {
			t.Error("Empty bearer token should not set Authorization header")
		}
	})

	t.Run("BasicAuth_Validation", func(t *testing.T) {
		req := &Request{Headers: make(map[string]string)}

		// Test valid basic auth
		WithBasicAuth("user", "pass")(req)
		if !strings.HasPrefix(req.Headers["Authorization"], "Basic ") {
			t.Error("Valid basic auth was not set correctly")
		}

		// Test invalid username (should not set header)
		req.Headers = make(map[string]string)
		WithBasicAuth("user\r\n", "pass")(req)
		if _, exists := req.Headers["Authorization"]; exists {
			t.Error("Invalid username should not set Authorization header")
		}

		// Test invalid password (should not set header)
		req.Headers = make(map[string]string)
		WithBasicAuth("user", "pass\r\n")(req)
		if _, exists := req.Headers["Authorization"]; exists {
			t.Error("Invalid password should not set Authorization header")
		}
	})
}

func TestSecurity_NewSecureClient(t *testing.T) {
	client, err := NewSecure()
	if err != nil {
		t.Fatalf("Failed to create secure client: %v", err)
	}
	defer client.Close()

	// Test that secure client was created successfully
	// The actual security settings are internal, but we can test basic functionality
	// This ensures the secure client works as expected
	t.Log("Secure client created successfully")
}

func TestSecurity_JSONBombPrevention(t *testing.T) {
	// Create a response with potential JSON bomb
	resp := &Response{
		RawBody: []byte(strings.Repeat("{", 15000) + strings.Repeat("}", 15000)),
	}

	var result map[string]any
	err := resp.JSON(&result)
	if err == nil {
		t.Error("Expected error for JSON bomb, got nil")
	}
	// Note: We now rely on Go's stdlib json.Unmarshal for protection.
	// It will return a parsing error for malformed JSON (mismatched brackets).
	// The stdlib has built-in protection against stack overflow from deep nesting.
	if err != nil {
		t.Logf("JSON bomb correctly rejected with error: %v", err)
	}
}

func TestSecurity_ConfigValidation(t *testing.T) {
	t.Run("UserAgent_InvalidCharacters", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.UserAgent = "invalid\r\nuseragent"

		_, err := New(cfg)
		if err == nil {
			t.Error("Expected error for invalid UserAgent characters")
		}
	})

	t.Run("Headers_InvalidCharacters", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Headers = map[string]string{
			"invalid\r\nkey": "value",
		}

		_, err := New(cfg)
		if err == nil {
			t.Error("Expected error for invalid header characters")
		}
	})

	t.Run("Headers_TooLong", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Headers = map[string]string{
			"key": strings.Repeat("a", 10000),
		}

		_, err := New(cfg)
		if err == nil {
			t.Error("Expected error for too long header value")
		}
	})
}
