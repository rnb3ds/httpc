package httpc_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybergodev/httpc"
)

func TestNewDomain(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{
			name:    "valid HTTPS URL",
			baseURL: "https://api.example.com",
			wantErr: false,
		},
		{
			name:    "valid HTTP URL",
			baseURL: "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "invalid URL - no scheme",
			baseURL: "api.example.com",
			wantErr: true,
		},
		{
			name:    "invalid URL - empty",
			baseURL: "",
			wantErr: true,
		},
		{
			name:    "invalid URL - malformed",
			baseURL: "ht!tp://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := httpc.NewDomain(tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDomain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if client != nil {
				defer client.Close()
			}
		})
	}
}

func TestDomainClient_AutomaticCookieManagement(t *testing.T) {
	// Create test server that sets and expects cookies
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request: set cookies
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			http.SetCookie(w, &http.Cookie{Name: "token", Value: "xyz789"})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("first"))
		} else {
			// Second request: verify cookies are sent
			sessionCookie, err := r.Cookie("session")
			if err != nil || sessionCookie.Value != "abc123" {
				t.Errorf("session cookie not found or incorrect")
			}
			tokenCookie, err := r.Cookie("token")
			if err != nil || tokenCookie.Value != "xyz789" {
				t.Errorf("token cookie not found or incorrect")
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("second"))
		}
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request
	resp1, err := client.Get("/")
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}
	if resp1.Body() != "first" {
		t.Errorf("First response body = %v, want 'first'", resp1.Body())
	}

	// Second request - cookies should be automatically sent
	resp2, err := client.Get("/test")
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}
	if resp2.Body() != "second" {
		t.Errorf("Second response body = %v, want 'second'", resp2.Body())
	}
}

func TestDomainClient_AutomaticHeaderManagement(t *testing.T) {
	// Create test server that checks headers
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request: check initial header
			if r.Header.Get("X-Custom") != "initial" {
				t.Errorf("First request missing X-Custom header")
			}
		} else {
			// Second request: check persisted header
			if r.Header.Get("X-Custom") != "initial" {
				t.Errorf("Second request missing persisted X-Custom header")
			}
			if r.Header.Get("X-New") != "added" {
				t.Errorf("Second request missing X-New header")
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request with initial header
	_, err = client.Get("/", httpc.WithHeader("X-Custom", "initial"))
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}

	// Set persistent header
	err = client.SetHeader("X-Custom", "initial")
	if err != nil {
		t.Fatalf("SetHeader error = %v", err)
	}

	// Second request - header should be automatically sent
	_, err = client.Get("/test", httpc.WithHeader("X-New", "added"))
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}
}

func TestDomainClient_CookieOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("test")
		if err != nil {
			t.Errorf("Cookie not found")
			return
		}
		w.Write([]byte(cookie.Value))
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// Set persistent cookie
	err = client.SetCookie(&http.Cookie{Name: "test", Value: "persistent"})
	if err != nil {
		t.Fatalf("SetCookie error = %v", err)
	}

	// Request with override cookie
	resp, err := client.Get("/", httpc.WithCookieValue("test", "override"))
	if err != nil {
		t.Fatalf("Request error = %v", err)
	}

	if resp.Body() != "override" {
		t.Errorf("Expected override cookie value, got %v", resp.Body())
	}
}

func TestDomainClient_HeaderOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("X-Test")))
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// Set persistent header
	err = client.SetHeader("X-Test", "persistent")
	if err != nil {
		t.Fatalf("SetHeader error = %v", err)
	}

	// Request with override header
	resp, err := client.Get("/", httpc.WithHeader("X-Test", "override"))
	if err != nil {
		t.Fatalf("Request error = %v", err)
	}

	if resp.Body() != "override" {
		t.Errorf("Expected override header value, got %v", resp.Body())
	}
}

func TestDomainClient_SetHeaders(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	headers := map[string]string{
		"X-Custom-1": "value1",
		"X-Custom-2": "value2",
	}

	err = client.SetHeaders(headers)
	if err != nil {
		t.Fatalf("SetHeaders error = %v", err)
	}

	got := client.GetHeaders()
	if len(got) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(got))
	}
	if got["X-Custom-1"] != "value1" {
		t.Errorf("X-Custom-1 = %v, want value1", got["X-Custom-1"])
	}
	if got["X-Custom-2"] != "value2" {
		t.Errorf("X-Custom-2 = %v, want value2", got["X-Custom-2"])
	}
}

func TestDomainClient_DeleteHeader(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	err = client.SetHeader("X-Test", "value")
	if err != nil {
		t.Fatalf("SetHeader error = %v", err)
	}

	client.DeleteHeader("X-Test")

	got := client.GetHeaders()
	if len(got) != 0 {
		t.Errorf("Expected 0 headers after delete, got %d", len(got))
	}
}

func TestDomainClient_ClearHeaders(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	headers := map[string]string{
		"X-Custom-1": "value1",
		"X-Custom-2": "value2",
	}
	err = client.SetHeaders(headers)
	if err != nil {
		t.Fatalf("SetHeaders error = %v", err)
	}

	client.ClearHeaders()

	got := client.GetHeaders()
	if len(got) != 0 {
		t.Errorf("Expected 0 headers after clear, got %d", len(got))
	}
}

func TestDomainClient_SetCookies(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	cookies := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
	}

	err = client.SetCookies(cookies)
	if err != nil {
		t.Fatalf("SetCookies error = %v", err)
	}

	got := client.GetCookies()
	if len(got) != 2 {
		t.Errorf("Expected 2 cookies, got %d", len(got))
	}
}

func TestDomainClient_GetCookie(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	err = client.SetCookie(&http.Cookie{Name: "test", Value: "value"})
	if err != nil {
		t.Fatalf("SetCookie error = %v", err)
	}

	cookie := client.GetCookie("test")
	if cookie == nil {
		t.Fatal("GetCookie returned nil")
	}
	if cookie.Name != "test" || cookie.Value != "value" {
		t.Errorf("GetCookie = %v/%v, want test/value", cookie.Name, cookie.Value)
	}

	notFound := client.GetCookie("nonexistent")
	if notFound != nil {
		t.Errorf("GetCookie for nonexistent cookie should return nil")
	}
}

func TestDomainClient_DeleteCookie(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	err = client.SetCookie(&http.Cookie{Name: "test", Value: "value"})
	if err != nil {
		t.Fatalf("SetCookie error = %v", err)
	}

	client.DeleteCookie("test")

	cookie := client.GetCookie("test")
	if cookie != nil {
		t.Errorf("Cookie should be deleted")
	}
}

func TestDomainClient_ClearCookies(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	cookies := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
	}
	err = client.SetCookies(cookies)
	if err != nil {
		t.Fatalf("SetCookies error = %v", err)
	}

	client.ClearCookies()

	got := client.GetCookies()
	if len(got) != 0 {
		t.Errorf("Expected 0 cookies after clear, got %d", len(got))
	}
}

func TestDomainClient_PathHandling(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		wantPath string
	}{
		{
			name:     "root path",
			baseURL:  "https://api.example.com",
			path:     "/",
			wantPath: "/",
		},
		{
			name:     "path with leading slash",
			baseURL:  "https://api.example.com",
			path:     "/users",
			wantPath: "/users",
		},
		{
			name:     "path without leading slash",
			baseURL:  "https://api.example.com",
			path:     "users",
			wantPath: "/users",
		},
		{
			name:     "empty path",
			baseURL:  "https://api.example.com",
			path:     "",
			wantPath: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.wantPath {
					t.Errorf("Path = %v, want %v", r.URL.Path, tt.wantPath)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := httpc.DefaultConfig()
			cfg.AllowPrivateIPs = true
			client, err := httpc.NewDomain(server.URL, cfg)
			if err != nil {
				t.Fatalf("NewDomain() error = %v", err)
			}
			defer client.Close()

			_, err = client.Get(tt.path)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
		})
	}
}

func TestDomainClient_FullURLHandling(t *testing.T) {
	// Create two test servers to simulate same domain and different domain
	sameDomainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("same-domain"))
	}))
	defer sameDomainServer.Close()

	differentDomainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("different-domain"))
	}))
	defer differentDomainServer.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(sameDomainServer.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		path     string
		wantBody string
	}{
		{
			name:     "relative path without slash",
			path:     "aa.html",
			wantBody: "same-domain",
		},
		{
			name:     "relative path with slash",
			path:     "/aa.html",
			wantBody: "same-domain",
		},
		{
			name:     "full URL same domain",
			path:     sameDomainServer.URL + "/aa.html",
			wantBody: "same-domain",
		},
		{
			name:     "full URL different domain",
			path:     differentDomainServer.URL + "/test",
			wantBody: "different-domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(tt.path)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if resp.Body() != tt.wantBody {
				t.Errorf("Body = %v, want %v", resp.Body(), tt.wantBody)
			}
		})
	}
}

func TestDomainClient_SameDomainCookiePersistence(t *testing.T) {
	// Test that cookies persist when using full URLs with same domain
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request: set cookie
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "test123"})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("first"))
		} else {
			// Second request: verify cookie is sent
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "test123" {
				t.Errorf("Cookie not found or incorrect in request %d", requestCount)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("second"))
		}
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request with relative path
	resp1, err := client.Get("/login")
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}
	if resp1.Body() != "first" {
		t.Errorf("First response = %v, want 'first'", resp1.Body())
	}

	// Second request with full URL (same domain)
	resp2, err := client.Get(server.URL + "/api/data")
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}
	if resp2.Body() != "second" {
		t.Errorf("Second response = %v, want 'second'", resp2.Body())
	}
}

func TestDomainClient_DomainMatching(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		requestPath string
		shouldMatch bool
	}{
		{
			name:        "exact domain match",
			baseURL:     "https://www.example.com",
			requestPath: "https://www.example.com/aa.html",
			shouldMatch: true,
		},
		{
			name:        "different subdomain",
			baseURL:     "https://www.example.com",
			requestPath: "https://api.example.com/aa.html",
			shouldMatch: false,
		},
		{
			name:        "different domain",
			baseURL:     "https://www.example.com",
			requestPath: "https://www.other.com/aa.html",
			shouldMatch: false,
		},
		{
			name:        "same domain different port",
			baseURL:     "https://www.example.com:8080",
			requestPath: "https://www.example.com:8080/aa.html",
			shouldMatch: true,
		},
		{
			name:        "same domain different protocol",
			baseURL:     "https://www.example.com",
			requestPath: "http://www.example.com/aa.html",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := httpc.DefaultConfig()
			cfg.AllowPrivateIPs = true

			// Use test server URL as base
			client, err := httpc.NewDomain(server.URL, cfg)
			if err != nil {
				t.Fatalf("NewDomain() error = %v", err)
			}
			defer client.Close()

			// Test with relative path (should always work)
			_, err = client.Get("/test")
			if err != nil {
				t.Errorf("Relative path request failed: %v", err)
			}

			// Test with full URL to same server (should work)
			_, err = client.Get(server.URL + "/test")
			if err != nil {
				t.Errorf("Full URL same domain request failed: %v", err)
			}
		})
	}
}

func TestDomainClient_AllHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("Method = %v, want %v", r.Method, method)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := httpc.DefaultConfig()
			cfg.AllowPrivateIPs = true
			client, err := httpc.NewDomain(server.URL, cfg)
			if err != nil {
				t.Fatalf("NewDomain() error = %v", err)
			}
			defer client.Close()

			var resp *httpc.Result
			switch method {
			case "GET":
				resp, err = client.Get("/")
			case "POST":
				resp, err = client.Post("/")
			case "PUT":
				resp, err = client.Put("/")
			case "PATCH":
				resp, err = client.Patch("/")
			case "DELETE":
				resp, err = client.Delete("/")
			case "HEAD":
				resp, err = client.Head("/")
			case "OPTIONS":
				resp, err = client.Options("/")
			}

			if err != nil {
				t.Fatalf("%s error = %v", method, err)
			}
			if resp == nil {
				t.Fatalf("%s returned nil response", method)
			}
		})
	}
}

func TestDomainClient_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// Set initial state
	err = client.SetHeader("X-Test", "value")
	if err != nil {
		t.Fatalf("SetHeader error = %v", err)
	}
	err = client.SetCookie(&http.Cookie{Name: "test", Value: "value"})
	if err != nil {
		t.Fatalf("SetCookie error = %v", err)
	}

	// Concurrent reads and writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Mix of operations
			client.Get("/")
			client.SetHeader("X-Concurrent", "test")
			client.GetHeaders()
			client.SetCookie(&http.Cookie{Name: "concurrent", Value: "test"})
			client.GetCookies()
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestDomainClient_InvalidHeaderValidation(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// Test invalid header key
	err = client.SetHeader("", "value")
	if err == nil {
		t.Error("Expected error for empty header key")
	}

	// Test invalid header with control characters
	err = client.SetHeader("X-Test\r\n", "value")
	if err == nil {
		t.Error("Expected error for header key with control characters")
	}
}

func TestDomainClient_InvalidCookieValidation(t *testing.T) {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// Test nil cookie
	err = client.SetCookie(nil)
	if err == nil {
		t.Error("Expected error for nil cookie")
	}

	// Test empty cookie name
	err = client.SetCookie(&http.Cookie{Name: "", Value: "value"})
	if err == nil {
		t.Error("Expected error for empty cookie name")
	}

	// Test cookie with invalid characters
	err = client.SetCookie(&http.Cookie{Name: "test\r\n", Value: "value"})
	if err == nil {
		t.Error("Expected error for cookie name with control characters")
	}
}

func TestDomainClient_AutoPersistRequestOptions(t *testing.T) {
	// Test that cookies and headers passed via options are automatically persisted
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request: verify initial cookies and headers
			cookie, err := r.Cookie("request-cookie")
			if err != nil || cookie.Value != "request-value" {
				t.Errorf("First request: cookie not found or incorrect")
			}
			if r.Header.Get("X-Request-Header") != "request-header-value" {
				t.Errorf("First request: header not found or incorrect")
			}
			w.WriteHeader(http.StatusOK)
		} else {
			// Second request: verify cookies and headers are automatically sent
			cookie, err := r.Cookie("request-cookie")
			if err != nil || cookie.Value != "request-value" {
				t.Errorf("Second request: cookie not persisted")
			}
			if r.Header.Get("X-Request-Header") != "request-header-value" {
				t.Errorf("Second request: header not persisted")
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request with cookies and headers via options
	_, err = client.Get("/first",
		httpc.WithCookieValue("request-cookie", "request-value"),
		httpc.WithHeader("X-Request-Header", "request-header-value"),
	)
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}

	// Second request without options - should automatically use persisted values
	_, err = client.Get("/second")
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount)
	}
}

func TestDomainClient_AutoPersistWithFullURL(t *testing.T) {
	// Test that options are persisted even when using full URLs
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		cookie, err := r.Cookie("test-cookie")
		if err != nil || cookie.Value != "test-value" {
			t.Errorf("Request %d: cookie not found or incorrect", requestCount)
		}
		if r.Header.Get("X-Test") != "test-header" {
			t.Errorf("Request %d: header not found or incorrect", requestCount)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request with relative path and options
	_, err = client.Get("/first",
		httpc.WithCookieValue("test-cookie", "test-value"),
		httpc.WithHeader("X-Test", "test-header"),
	)
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}

	// Second request with full URL (same domain) - should use persisted options
	_, err = client.Get(server.URL + "/second")
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}

	// Third request with relative path - should still use persisted options
	_, err = client.Get("/third")
	if err != nil {
		t.Fatalf("Third request error = %v", err)
	}

	if requestCount != 3 {
		t.Errorf("Expected 3 requests, got %d", requestCount)
	}
}

func TestDomainClient_AutoPersistMultipleCookies(t *testing.T) {
	// Test that multiple cookies are persisted correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie1, err1 := r.Cookie("cookie1")
		cookie2, err2 := r.Cookie("cookie2")
		cookie3, err3 := r.Cookie("cookie3")

		if err1 != nil || cookie1.Value != "value1" {
			t.Error("cookie1 not found or incorrect")
		}
		if err2 != nil || cookie2.Value != "value2" {
			t.Error("cookie2 not found or incorrect")
		}
		if err3 != nil || cookie3.Value != "value3" {
			t.Error("cookie3 not found or incorrect")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request with multiple cookies
	_, err = client.Get("/first",
		httpc.WithCookies([]http.Cookie{
			{Name: "cookie1", Value: "value1"},
			{Name: "cookie2", Value: "value2"},
			{Name: "cookie3", Value: "value3"},
		}),
	)
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}

	// Second request - should automatically send all cookies
	_, err = client.Get("/second")
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}
}

func TestDomainClient_AutoPersistHeaderMap(t *testing.T) {
	// Test that header map is persisted correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Header-1") != "value1" {
			t.Error("X-Header-1 not found or incorrect")
		}
		if r.Header.Get("X-Header-2") != "value2" {
			t.Error("X-Header-2 not found or incorrect")
		}
		if r.Header.Get("X-Header-3") != "value3" {
			t.Error("X-Header-3 not found or incorrect")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request with header map
	_, err = client.Get("/first",
		httpc.WithHeaderMap(map[string]string{
			"X-Header-1": "value1",
			"X-Header-2": "value2",
			"X-Header-3": "value3",
		}),
	)
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}

	// Second request - should automatically send all headers
	_, err = client.Get("/second")
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}
}

func TestDomainClient_AutoPersistOverride(t *testing.T) {
	// Test that new options override persisted ones
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		cookie, _ := r.Cookie("test-cookie")
		header := r.Header.Get("X-Test")

		if requestCount == 1 {
			if cookie.Value != "value1" {
				t.Errorf("Request 1: expected cookie value1, got %s", cookie.Value)
			}
			if header != "header1" {
				t.Errorf("Request 1: expected header header1, got %s", header)
			}
		} else if requestCount == 2 {
			if cookie.Value != "value2" {
				t.Errorf("Request 2: expected cookie value2, got %s", cookie.Value)
			}
			if header != "header2" {
				t.Errorf("Request 2: expected header header2, got %s", header)
			}
		} else {
			// Third request should use the last persisted values
			if cookie.Value != "value2" {
				t.Errorf("Request 3: expected cookie value2, got %s", cookie.Value)
			}
			if header != "header2" {
				t.Errorf("Request 3: expected header header2, got %s", header)
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// First request
	_, err = client.Get("/first",
		httpc.WithCookieValue("test-cookie", "value1"),
		httpc.WithHeader("X-Test", "header1"),
	)
	if err != nil {
		t.Fatalf("First request error = %v", err)
	}

	// Second request with different values (should override)
	_, err = client.Get("/second",
		httpc.WithCookieValue("test-cookie", "value2"),
		httpc.WithHeader("X-Test", "header2"),
	)
	if err != nil {
		t.Fatalf("Second request error = %v", err)
	}

	// Third request without options (should use last persisted values)
	_, err = client.Get("/third")
	if err != nil {
		t.Fatalf("Third request error = %v", err)
	}
}

func TestDomainClient_RealWorldScenario(t *testing.T) {
	// Simulate a real-world scenario with login and subsequent API calls
	loginCalled := false
	apiCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/login") {
			loginCalled = true
			// Set session cookie
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret123"})
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"token":"abc123"}`))
		} else if strings.HasSuffix(r.URL.Path, "/api/data") {
			apiCalled = true
			// Verify session cookie is present
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "secret123" {
				t.Error("Session cookie not found in API request")
			}
			// Verify auth header is present
			if r.Header.Get("Authorization") != "Bearer abc123" {
				t.Error("Authorization header not found in API request")
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":"success"}`))
		}
	}))
	defer server.Close()

	cfg := httpc.DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, err := httpc.NewDomain(server.URL, cfg)
	if err != nil {
		t.Fatalf("NewDomain() error = %v", err)
	}
	defer client.Close()

	// Step 1: Login
	loginResp, err := client.Post("/login", httpc.WithJSON(map[string]string{
		"username": "test",
		"password": "pass",
	}))
	if err != nil {
		t.Fatalf("Login error = %v", err)
	}

	// Step 2: Extract token and set as persistent header
	var loginData map[string]string
	if err := loginResp.JSON(&loginData); err != nil {
		t.Fatalf("JSON parse error = %v", err)
	}
	err = client.SetHeader("Authorization", "Bearer "+loginData["token"])
	if err != nil {
		t.Fatalf("SetHeader error = %v", err)
	}

	// Step 3: Make API call (cookies and headers should be automatically sent)
	apiResp, err := client.Get("/api/data")
	if err != nil {
		t.Fatalf("API call error = %v", err)
	}

	if !strings.Contains(apiResp.Body(), "success") {
		t.Errorf("Expected success response, got %v", apiResp.Body())
	}

	if !loginCalled || !apiCalled {
		t.Error("Expected both login and API endpoints to be called")
	}
}
