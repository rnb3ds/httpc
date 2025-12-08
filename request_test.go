package httpc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// REQUEST TESTS - Headers, cookies, query params, body, options
// Consolidates: options_test.go, cookie_*.go, request_headers_test.go
// ============================================================================

// ----------------------------------------------------------------------------
// Headers
// ----------------------------------------------------------------------------

func TestRequest_Headers(t *testing.T) {
	t.Run("WithHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Custom-Header") != "custom-value" {
				t.Error("Expected X-Custom-Header: custom-value")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithHeader("X-Custom-Header", "custom-value"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithHeaderMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Header-1") != "value1" {
				t.Error("Expected X-Header-1: value1")
			}
			if r.Header.Get("X-Header-2") != "value2" {
				t.Error("Expected X-Header-2: value2")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		headers := map[string]string{
			"X-Header-1": "value1",
			"X-Header-2": "value2",
		}
		_, err := client.Get(server.URL, WithHeaderMap(headers))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithUserAgent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") != "custom-agent/1.0" {
				t.Errorf("Expected User-Agent: custom-agent/1.0, got %s", r.Header.Get("User-Agent"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithUserAgent("custom-agent/1.0"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithJSONAccept", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/json" {
				t.Error("Expected Accept: application/json")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithJSONAccept())
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithXMLAccept", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/xml" {
				t.Error("Expected Accept: application/xml")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithXMLAccept())
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("RequestHeadersInspection", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL,
			WithHeader("X-Custom", "test-value"),
			WithHeader("X-Another", "another-value"),
		)
		if err != nil {
			t.Fatal(err)
		}

		// Verify request headers are captured
		if resp.Request == nil || resp.Request.Headers == nil {
			t.Fatal("Request headers not captured")
		}

		if resp.Request.Headers.Get("X-Custom") != "test-value" {
			t.Error("X-Custom header not captured correctly")
		}
		if resp.Request.Headers.Get("X-Another") != "another-value" {
			t.Error("X-Another header not captured correctly")
		}
	})
}

// ----------------------------------------------------------------------------
// Authentication
// ----------------------------------------------------------------------------

func TestRequest_Authentication(t *testing.T) {
	t.Run("WithBasicAuth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok || username != "user" || password != "pass" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL, WithBasicAuth("user", "pass"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
	})

	t.Run("WithBearerToken", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token-123" {
				t.Errorf("Expected Bearer token, got %s", auth)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithBearerToken("test-token-123"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Query Parameters
// ----------------------------------------------------------------------------

func TestRequest_QueryParameters(t *testing.T) {
	t.Run("WithQueryMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key1") != "value1" {
				t.Error("Expected key1=value1")
			}
			if r.URL.Query().Get("key2") != "value2" {
				t.Error("Expected key2=value2")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		params := map[string]any{
			"key1": "value1",
			"key2": "value2",
		}
		_, err := client.Get(server.URL, WithQueryMap(params))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithQuery", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("search") != "test query" {
				t.Error("Expected search=test query")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithQuery("search", "test query"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Body Content
// ----------------------------------------------------------------------------

func TestRequest_Body(t *testing.T) {
	type TestData struct {
		Message string `json:"message" xml:"message"`
		Code    int    `json:"code" xml:"code"`
	}

	t.Run("WithJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("Expected Content-Type: application/json")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{Message: "test", Code: 200}
		_, err := client.Post(server.URL, WithJSON(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Error("Expected Content-Type: application/xml")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{Message: "test", Code: 200}
		_, err := client.Post(server.URL, WithXML(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithBody", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if string(body) != "raw body content" {
				t.Errorf("Expected 'raw body content', got %s", string(body))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody([]byte("raw body content")))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithForm", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Error("Expected Content-Type: application/x-www-form-urlencoded")
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("Failed to parse form: %v", err)
			}
			if r.FormValue("field1") != "value1" {
				t.Error("Expected field1=value1")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithForm(map[string]string{
			"field1": "value1",
			"field2": "value2",
		}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithFile", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		content := []byte("test content")
		_, err := client.Post(server.URL, WithFile("file", "test.txt", content))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithFormData", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Error("Expected Content-Type: multipart/form-data")
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			if r.FormValue("field1") != "value1" {
				t.Error("Expected field1=value1")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Fields: map[string]string{"field1": "value1"},
			Files: map[string]*FileData{
				"file": {Filename: "test.txt", Content: []byte("content")},
			},
		}
		_, err := client.Post(server.URL, WithFormData(formData))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Cookies - Basic Operations
// ----------------------------------------------------------------------------

func TestRequest_Cookies(t *testing.T) {
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

		cookie := http.Cookie{Name: "test-cookie", Value: "test-value"}
		_, err := client.Get(server.URL, WithCookie(cookie))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithCookies", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie1, _ := r.Cookie("cookie1")
			cookie2, _ := r.Cookie("cookie2")
			if cookie1 == nil || cookie1.Value != "value1" {
				t.Error("cookie1 not found or incorrect")
			}
			if cookie2 == nil || cookie2.Value != "value2" {
				t.Error("cookie2 not found or incorrect")
			}
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
	})

	t.Run("WithCookieValue", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "abc123" {
				t.Error("Cookie not found or incorrect")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithCookieValue("session", "abc123"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Cookie String Parsing
// ----------------------------------------------------------------------------

func TestRequest_CookieString(t *testing.T) {
	tests := []struct {
		name            string
		cookieString    string
		expectedCount   int
		expectedCookies map[string]string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "empty string",
			cookieString:    "",
			expectedCount:   0,
			expectedCookies: map[string]string{},
		},
		{
			name:          "single cookie",
			cookieString:  "BSID=4418ECBB1281B550",
			expectedCount: 1,
			expectedCookies: map[string]string{
				"BSID": "4418ECBB1281B550",
			},
		},
		{
			name:          "multiple cookies",
			cookieString:  "BSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc",
			expectedCount: 3,
			expectedCookies: map[string]string{
				"BSID": "4418ECBB1281B550",
				"PSTM": "1733760779",
				"BS":   "kUwNTVFcEUBUItoc",
			},
		},
		{
			name:          "complex with special chars",
			cookieString:  "BID=01E8D701159F774:FG=1; MCITY=-257%3A; BUPN=12314753",
			expectedCount: 3,
			expectedCookies: map[string]string{
				"BID":   "01E8D701159F774:FG=1",
				"MCITY": "-257%3A",
				"BUPN":  "12314753",
			},
		},
		{
			name:          "with spaces",
			cookieString:  "session = abc123 ; token = xyz789 ",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"session": "abc123",
				"token":   "xyz789",
			},
		},
		{
			name:          "empty value",
			cookieString:  "empty_cookie=; normal_cookie=value",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"empty_cookie":  "",
				"normal_cookie": "value",
			},
		},
		{
			name:          "malformed without equals",
			cookieString:  "invalid_cookie_without_equals",
			expectError:   true,
			errorContains: "malformed cookie pair",
		},
		{
			name:          "empty name",
			cookieString:  "=value_without_name",
			expectError:   true,
			errorContains: "empty cookie name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &Request{}
			option := WithCookieString(tt.cookieString)
			err := option(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(req.Cookies) != tt.expectedCount {
				t.Errorf("expected %d cookies, got %d", tt.expectedCount, len(req.Cookies))
			}

			for expectedName, expectedValue := range tt.expectedCookies {
				found := false
				for _, cookie := range req.Cookies {
					if cookie.Name == expectedName {
						found = true
						if cookie.Value != expectedValue {
							t.Errorf("cookie %s: expected value %q, got %q", expectedName, expectedValue, cookie.Value)
						}
						break
					}
				}
				if !found {
					t.Errorf("expected cookie %s not found", expectedName)
				}
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Cookie Auto-Domain Extraction
// ----------------------------------------------------------------------------

func TestRequest_CookieAutoDomain(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.EnableCookies = true
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	t.Run("WithCookie auto-domain", func(t *testing.T) {
		cookie := http.Cookie{Name: "session", Value: "abc123"}
		resp, err := client.Get(server.URL, WithCookie(cookie))
		if err != nil {
			t.Fatal(err)
		}
		if !resp.HasRequestCookie("session") {
			t.Error("Cookie should have been sent")
		}

		// Verify persistence
		resp2, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if !resp2.HasRequestCookie("session") {
			t.Error("Cookie should persist with auto-set domain")
		}
	})

	t.Run("WithCookies auto-domain", func(t *testing.T) {
		cookies := []http.Cookie{
			{Name: "cookie1", Value: "value1"},
			{Name: "cookie2", Value: "value2"},
		}
		resp, err := client.Get(server.URL, WithCookies(cookies))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cookies {
			if !resp.HasRequestCookie(c.Name) {
				t.Errorf("Cookie %s should have been sent", c.Name)
			}
		}
	})

	t.Run("WithCookieString auto-domain", func(t *testing.T) {
		cookieString := "CONSENT=YES+; PREF=dark_mode; ID=abc123"
		resp, err := client.Get(server.URL, WithCookieString(cookieString))
		if err != nil {
			t.Fatal(err)
		}
		expectedCookies := []string{"CONSENT", "PREF", "ID"}
		for _, name := range expectedCookies {
			if !resp.HasRequestCookie(name) {
				t.Errorf("Cookie %s should have been sent", name)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Request Cookies Inspection
// ----------------------------------------------------------------------------

func TestRequest_CookiesInspection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL,
		WithCookieValue("cookie1", "value1"),
		WithCookieValue("cookie2", "value2"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Test HasRequestCookie
	if !resp.HasRequestCookie("cookie1") {
		t.Error("HasRequestCookie should return true for cookie1")
	}
	if resp.HasRequestCookie("nonexistent") {
		t.Error("HasRequestCookie should return false for nonexistent cookie")
	}

	// Test GetRequestCookie
	cookie := resp.GetRequestCookie("cookie1")
	if cookie == nil {
		t.Fatal("GetRequestCookie should return cookie1")
	}
	if cookie.Value != "value1" {
		t.Errorf("Expected value1, got %s", cookie.Value)
	}

	// Test RequestCookies
	cookies := resp.RequestCookies()
	if len(cookies) < 2 {
		t.Errorf("Expected at least 2 cookies, got %d", len(cookies))
	}
}

// ----------------------------------------------------------------------------
// Timeout & Retry Options
// ----------------------------------------------------------------------------

func TestRequest_TimeoutAndRetry(t *testing.T) {
	t.Run("WithMaxRetries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithMaxRetries(3))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Combined Options
// ----------------------------------------------------------------------------

func TestRequest_CombinedOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify all options applied
		if r.Header.Get("X-Custom") != "value" {
			t.Error("Header not set")
		}
		if r.URL.Query().Get("param") != "test" {
			t.Error("Query param not set")
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value != "abc123" {
			t.Error("Cookie not set")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	_, err := client.Get(server.URL,
		WithHeader("X-Custom", "value"),
		WithQuery("param", "test"),
		WithCookieValue("session", "abc123"),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}
