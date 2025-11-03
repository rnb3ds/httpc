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
		// Test that WithHeaderMap properly validates headers
		headers := map[string]string{
			"Valid-Header":   "valid-value",
			"Invalid\r\nKey": "value",                    // Should be filtered out
			"Valid-Key":      "invalid\r\nvalue",         // Should be filtered out
			"Too-Long-Key":   strings.Repeat("a", 10000), // Should be filtered out
		}

		req := &Request{Headers: make(map[string]string)}
		WithHeaderMap(headers)(req)

		// Only valid header should remain
		if len(req.Headers) != 1 {
			t.Errorf("Expected 1 valid header, got %d", len(req.Headers))
		}
		if req.Headers["Valid-Header"] != "valid-value" {
			t.Error("Valid header was not preserved")
		}
	})

	t.Run("Query_Validation", func(t *testing.T) {
		req := &Request{QueryParams: make(map[string]any)}

		// Test various query parameter validations
		WithQuery("valid-key", "valid-value")(req)
		WithQuery("invalid\r\nkey", "value")(req)                // Should be filtered
		WithQuery("key&with&ampersand", "value")(req)            // Should be filtered
		WithQuery("", "empty-key")(req)                          // Should be filtered
		WithQuery("long-value", strings.Repeat("x", 10000))(req) // Should be filtered

		// Only valid query should remain
		if len(req.QueryParams) != 1 {
			t.Errorf("Expected 1 valid query param, got %d", len(req.QueryParams))
		}
		if req.QueryParams["valid-key"] != "valid-value" {
			t.Error("Valid query param was not preserved")
		}
	})

	t.Run("Cookie_Validation", func(t *testing.T) {
		req := &Request{}

		// Test cookie validation
		validCookie := &http.Cookie{Name: "valid", Value: "value"}
		invalidCookie1 := &http.Cookie{Name: "invalid\r\n", Value: "value"}
		invalidCookie2 := &http.Cookie{Name: "valid", Value: "invalid\r\nvalue"}
		nilCookie := (*http.Cookie)(nil)

		WithCookie(validCookie)(req)
		WithCookie(invalidCookie1)(req)
		WithCookie(invalidCookie2)(req)
		WithCookie(nilCookie)(req)

		// Only valid cookie should remain
		if len(req.Cookies) != 1 {
			t.Errorf("Expected 1 valid cookie, got %d", len(req.Cookies))
		}
		if req.Cookies[0].Name != "valid" || req.Cookies[0].Value != "value" {
			t.Error("Valid cookie was not preserved correctly")
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
	if !strings.Contains(err.Error(), "JSON structure too complex") &&
		!strings.Contains(err.Error(), "JSON nesting too deep") {
		t.Errorf("Expected JSON bomb error, got: %v", err)
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
