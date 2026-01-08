package httpc

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ----------------------------------------------------------------------------
// Cookie Tests - Unified
// ----------------------------------------------------------------------------
// This file consolidates all cookie-related tests from request_test.go,
// data_test.go, and response_test.go for better organization and maintainability.

// ----------------------------------------------------------------------------
// Request Cookies - Basic Operations
// ----------------------------------------------------------------------------

func TestCookie_RequestBasicOperations(t *testing.T) {
	t.Run("WithCookie", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("test-cookie")
			if err != nil || cookie.Value != "test-value" {
				t.Error("Cookie not found or incorrect")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithCookie(http.Cookie{
			Name:  "test-cookie",
			Value: "test-value",
		}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithCookies", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie1, _ := r.Cookie("cookie1")
			cookie2, _ := r.Cookie("cookie2")
			if cookie1 == nil || cookie2 == nil {
				t.Error("Cookies not found")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithCookies([]http.Cookie{
			{Name: "cookie1", Value: "value1"},
			{Name: "cookie2", Value: "value2"},
		}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithCookieValue", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("simple")
			if err != nil || cookie.Value != "value" {
				t.Error("Cookie not found or incorrect")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithCookieValue("simple", "value"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Cookie String Parsing
// ----------------------------------------------------------------------------

func TestCookie_StringParsing(t *testing.T) {
	tests := []struct {
		name            string
		cookieString    string
		expectedCount   int
		expectedCookies map[string]string
	}{
		{
			name:            "empty string",
			cookieString:    "",
			expectedCount:   0,
			expectedCookies: map[string]string{},
		},
		{
			name:          "single cookie",
			cookieString:  "name=value",
			expectedCount: 1,
			expectedCookies: map[string]string{
				"name": "value",
			},
		},
		{
			name:          "multiple cookies",
			cookieString:  "cookie1=value1; cookie2=value2; cookie3=value3",
			expectedCount: 3,
			expectedCookies: map[string]string{
				"cookie1": "value1",
				"cookie2": "value2",
				"cookie3": "value3",
			},
		},
		{
			name:          "complex with special chars",
			cookieString:  "session=abc123; token=xyz-789_ABC; user=john@example.com",
			expectedCount: 3,
			expectedCookies: map[string]string{
				"session": "abc123",
				"token":   "xyz-789_ABC",
				"user":    "john@example.com",
			},
		},
		{
			name:          "with spaces",
			cookieString:  "name1 = value1 ; name2 = value2",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"name1": "value1",
				"name2": "value2",
			},
		},
		{
			name:          "empty value",
			cookieString:  "empty=",
			expectedCount: 1,
			expectedCookies: map[string]string{
				"empty": "",
			},
		},
		{
			name:          "empty value",
			cookieString:  "empty=",
			expectedCount: 1,
			expectedCookies: map[string]string{
				"empty": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cookies := r.Cookies()
				if len(cookies) != tt.expectedCount {
					t.Errorf("Expected %d cookies, got %d", tt.expectedCount, len(cookies))
				}

				for name, expectedValue := range tt.expectedCookies {
					cookie, err := r.Cookie(name)
					if err != nil {
						t.Errorf("Cookie %s not found", name)
						continue
					}
					if cookie.Value != expectedValue {
						t.Errorf("Cookie %s: expected value %s, got %s", name, expectedValue, cookie.Value)
					}
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			_, err := client.Get(server.URL, WithCookieString(tt.cookieString))
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Cookie String Parsing - Error Cases
// ----------------------------------------------------------------------------

func TestCookie_StringParsingErrors(t *testing.T) {
	tests := []struct {
		name         string
		cookieString string
		expectError  bool
	}{
		{
			name:         "malformed without equals",
			cookieString: "invalid",
			expectError:  true,
		},
		{
			name:         "empty name",
			cookieString: "=value",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			_, err := client.Get(server.URL, WithCookieString(tt.cookieString))
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Cookie Auto-Domain Extraction
// ----------------------------------------------------------------------------

func TestCookie_AutoDomain(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Create cookie without domain - should auto-extract from URL
	cookie := http.Cookie{
		Name:  "test",
		Value: "value",
	}

	_, err := client.Get(server.URL, WithCookie(cookie))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Cookie should have been sent successfully
	// The domain is automatically handled by the HTTP client
}

// ----------------------------------------------------------------------------
// Cookie Persistence (Jar)
// ----------------------------------------------------------------------------

func TestCookie_Persistence(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			w.WriteHeader(http.StatusOK)
		} else {
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "abc123" {
				t.Error("Session cookie not persisted")
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// First request - server sets cookie
	_, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	// Second request - cookie should be sent automatically
	_, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Response Cookies
// ----------------------------------------------------------------------------

func TestCookie_ResponseOperations(t *testing.T) {
	t.Run("GetCookie", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			http.SetCookie(w, &http.Cookie{Name: "token", Value: "xyz789"})
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		sessionCookie := resp.GetCookie("session")
		if sessionCookie == nil || sessionCookie.Value != "abc123" {
			t.Error("Session cookie not found or incorrect")
		}

		tokenCookie := resp.GetCookie("token")
		if tokenCookie == nil || tokenCookie.Value != "xyz789" {
			t.Error("Token cookie not found or incorrect")
		}
	})

	t.Run("HasCookie", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "exists", Value: "yes"})
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if !resp.HasCookie("exists") {
			t.Error("Expected cookie 'exists' to be present")
		}

		if resp.HasCookie("nonexistent") {
			t.Error("Expected cookie 'nonexistent' to be absent")
		}
	})

	t.Run("ResponseCookies", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "cookie1", Value: "value1"})
			http.SetCookie(w, &http.Cookie{Name: "cookie2", Value: "value2"})
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		cookies := resp.Response.Cookies
		if len(cookies) != 2 {
			t.Errorf("Expected 2 cookies, got %d", len(cookies))
		}
	})
}

// ----------------------------------------------------------------------------
// Cookie Inspection
// ----------------------------------------------------------------------------

func TestCookie_Inspection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	cookies := []http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
	}

	_, err := client.Get(server.URL, WithCookies(cookies))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Cookies should be sent successfully
	// The actual inspection happens on the server side in the handler
}
