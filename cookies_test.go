package httpc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCookieBasicOperations(t *testing.T) {
	// Test server that echoes cookies
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo received cookies
		for _, cookie := range r.Cookies() {
			w.Header().Add("X-Received-Cookie", cookie.Name+"="+cookie.Value)
		}

		// Set response cookies
		http.SetCookie(w, &http.Cookie{
			Name:  "session",
			Value: "abc123",
			Path:  "/",
		})
		http.SetCookie(w, &http.Cookie{
			Name:  "user",
			Value: "john",
			Path:  "/",
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true // Allow localhost for testing
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("WithCookie", func(t *testing.T) {
		cookie := &http.Cookie{
			Name:  "test",
			Value: "value123",
		}

		resp, err := client.Get(server.URL, WithCookie(cookie))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		// Check if cookie was sent
		receivedCookies := resp.Headers["X-Received-Cookie"]
		if len(receivedCookies) == 0 {
			t.Error("No cookies were received by server")
		}

		found := false
		for _, c := range receivedCookies {
			if c == "test=value123" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Cookie was not sent to server")
		}
	})

	t.Run("WithCookieValue", func(t *testing.T) {
		resp, err := client.Get(server.URL, WithCookieValue("simple", "cookie"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		receivedCookies := resp.Headers["X-Received-Cookie"]
		found := false
		for _, c := range receivedCookies {
			if c == "simple=cookie" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Simple cookie was not sent to server")
		}
	})

	t.Run("WithCookies", func(t *testing.T) {
		cookies := []*http.Cookie{
			{Name: "cookie1", Value: "value1"},
			{Name: "cookie2", Value: "value2"},
		}

		resp, err := client.Get(server.URL, WithCookies(cookies))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		receivedCookies := resp.Headers["X-Received-Cookie"]
		if len(receivedCookies) < 2 {
			t.Errorf("Expected at least 2 cookies, got %d", len(receivedCookies))
		}
	})

	t.Run("ResponseCookies", func(t *testing.T) {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if len(resp.Cookies) == 0 {
			t.Error("No cookies in response")
		}

		// Check for specific cookies
		sessionCookie := resp.GetCookie("session")
		if sessionCookie == nil {
			t.Error("Session cookie not found")
		} else if sessionCookie.Value != "abc123" {
			t.Errorf("Expected session cookie value 'abc123', got '%s'", sessionCookie.Value)
		}

		userCookie := resp.GetCookie("user")
		if userCookie == nil {
			t.Error("User cookie not found")
		} else if userCookie.Value != "john" {
			t.Errorf("Expected user cookie value 'john', got '%s'", userCookie.Value)
		}
	})

	t.Run("HasCookie", func(t *testing.T) {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if !resp.HasCookie("session") {
			t.Error("HasCookie returned false for existing cookie")
		}

		if resp.HasCookie("nonexistent") {
			t.Error("HasCookie returned true for non-existent cookie")
		}
	})
}

func TestCookieJar(t *testing.T) {
	// Test server that sets and expects cookies
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request: set cookies
			http.SetCookie(w, &http.Cookie{
				Name:  "session_id",
				Value: "xyz789",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Cookie set"))
		} else {
			// Subsequent requests: check if cookie is present
			cookie, err := r.Cookie("session_id")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Cookie not found"))
				return
			}
			if cookie.Value != "xyz789" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Cookie value mismatch"))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Cookie verified"))
		}
	}))
	defer server.Close()

	t.Run("AutomaticCookieManagement", func(t *testing.T) {
		requestCount = 0

		// Create client with cookie jar enabled
		jar, err := NewCookieJar()
		if err != nil {
			t.Fatalf("Failed to create cookie jar: %v", err)
		}

		config := DefaultConfig()
		config.EnableCookies = true
		config.CookieJar = jar
		config.AllowPrivateIPs = true // Allow localhost for testing

		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// First request - server sets cookie
		resp1, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}
		if resp1.Body != "Cookie set" {
			t.Errorf("Unexpected response: %s", resp1.Body)
		}

		// Second request - cookie should be automatically sent
		resp2, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		if resp2.Body != "Cookie verified" {
			t.Errorf("Cookie was not automatically sent. Response: %s", resp2.Body)
		}
	})

	t.Run("DisabledCookieJar", func(t *testing.T) {
		requestCount = 0

		// Create client without cookie jar
		config := DefaultConfig()
		config.EnableCookies = false
		config.AllowPrivateIPs = true // Allow localhost for testing

		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// First request
		_, err = client.Get(server.URL)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}

		// Second request - cookie should NOT be automatically sent
		resp2, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
		if resp2.Body == "Cookie verified" {
			t.Error("Cookie was sent even though cookie jar is disabled")
		}
	})
}

func TestCookieAttributes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "secure_cookie",
			Value:    "secret",
			Path:     "/api",
			Domain:   "example.com",
			Expires:  time.Now().Add(24 * time.Hour),
			MaxAge:   86400,
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true // Allow localhost for testing
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	cookie := resp.GetCookie("secure_cookie")
	if cookie == nil {
		t.Fatal("Cookie not found in response")
	}

	if cookie.Value != "secret" {
		t.Errorf("Expected value 'secret', got '%s'", cookie.Value)
	}
	if cookie.Path != "/api" {
		t.Errorf("Expected path '/api', got '%s'", cookie.Path)
	}
	if !cookie.Secure {
		t.Error("Expected Secure flag to be true")
	}
	if !cookie.HttpOnly {
		t.Error("Expected HttpOnly flag to be true")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("Expected SameSite to be Strict, got %v", cookie.SameSite)
	}
}

func TestMultipleCookieValue(t *testing.T) {
	// Test server that echoes received cookies
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookies := r.Cookies()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Echo back the number of cookies and their names
		fmt.Fprintf(w, `{"count": %d, "cookies": [`, len(cookies))
		for i, cookie := range cookies {
			if i > 0 {
				fmt.Fprintf(w, `, `)
			}
			fmt.Fprintf(w, `{"name": "%s", "value": "%s"}`, cookie.Name, cookie.Value)
		}
		fmt.Fprintf(w, `]}`)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("Multiple WithCookieValue calls", func(t *testing.T) {
		resp, err := client.Get(server.URL,
			WithCookieValue("cookie1", "value1"),
			WithCookieValue("cookie2", "value2"),
			WithCookieValue("cookie3", "value3"),
		)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		var result struct {
			Count   int `json:"count"`
			Cookies []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"cookies"`
		}

		if err := resp.JSON(&result); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if result.Count != 3 {
			t.Errorf("Expected 3 cookies, got %d", result.Count)
			t.Logf("Response body: %s", resp.Body)
		}

		// Verify each cookie
		expectedCookies := map[string]string{
			"cookie1": "value1",
			"cookie2": "value2",
			"cookie3": "value3",
		}

		for _, cookie := range result.Cookies {
			expectedValue, exists := expectedCookies[cookie.Name]
			if !exists {
				t.Errorf("Unexpected cookie: %s", cookie.Name)
			} else if cookie.Value != expectedValue {
				t.Errorf("Cookie %s: expected value '%s', got '%s'", cookie.Name, expectedValue, cookie.Value)
			}
		}
	})

	t.Run("Mixed WithCookie and WithCookieValue", func(t *testing.T) {
		resp, err := client.Get(server.URL,
			WithCookieValue("simple", "value"),
			WithCookie(&http.Cookie{
				Name:  "complex",
				Value: "value2",
				Path:  "/api",
			}),
			WithCookieValue("another", "value3"),
		)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		var result struct {
			Count   int `json:"count"`
			Cookies []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"cookies"`
		}

		if err := resp.JSON(&result); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if result.Count != 3 {
			t.Errorf("Expected 3 cookies, got %d", result.Count)
			t.Logf("Response body: %s", resp.Body)
		}
	})
}
