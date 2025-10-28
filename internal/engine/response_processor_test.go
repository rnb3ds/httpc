package engine

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/memory"
)

// ============================================================================
// RESPONSE PROCESSOR TESTS
// ============================================================================

func TestResponseProcessor_Process(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024, // 50MB
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewResponseProcessor(config, memManager)

	tests := []struct {
		name         string
		httpResponse *http.Response
		validate     func(*testing.T, *Response)
	}{
		{
			name: "Simple JSON response",
			httpResponse: &http.Response{
				StatusCode:    200,
				Status:        "200 OK",
				ContentLength: 27, // Set ContentLength field
				Header: http.Header{
					"Content-Type":   []string{"application/json"},
					"Content-Length": []string{"27"},
				},
				Body:    io.NopCloser(strings.NewReader(`{"message":"success","code":200}`)),
				Request: &http.Request{}, // Add Request to avoid nil pointer
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 200 {
					t.Errorf("Expected status code 200, got %d", resp.StatusCode)
				}
				if resp.Status != "200 OK" {
					t.Errorf("Expected status '200 OK', got '%s'", resp.Status)
				}
				if resp.Body != `{"message":"success","code":200}` {
					t.Errorf("Expected body '..success..', got '%s'", resp.Body)
				}
				if len(resp.RawBody) == 0 {
					t.Error("RawBody should not be empty")
				}
				if resp.ContentLength != 27 {
					t.Errorf("Expected content length 27, got %d", resp.ContentLength)
				}
			},
		},
		{
			name: "Error response",
			httpResponse: &http.Response{
				StatusCode: 404,
				Status:     "404 Not Found",
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body:    io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
				Request: &http.Request{}, // Add Request to avoid nil pointer
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 404 {
					t.Errorf("Expected status code 404, got %d", resp.StatusCode)
				}
				if resp.Status != "404 Not Found" {
					t.Errorf("Expected status '404 Not Found', got '%s'", resp.Status)
				}
				if !strings.Contains(resp.Body, "not found") {
					t.Error("Response body should contain 'not found'")
				}
			},
		},
		{
			name: "Response with cookies",
			httpResponse: &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header: http.Header{
					"Set-Cookie": []string{
						"session_id=abc123; Path=/; HttpOnly",
						"user_pref=dark_mode; Path=/",
					},
				},
				Body:    io.NopCloser(strings.NewReader("OK")),
				Request: &http.Request{}, // Add Request to avoid nil pointer
			},
			validate: func(t *testing.T, resp *Response) {
				if len(resp.Cookies) != 2 {
					t.Errorf("Expected 2 cookies, got %d", len(resp.Cookies))
				}

				foundSession := false
				foundPref := false
				for _, cookie := range resp.Cookies {
					if cookie.Name == "session_id" && cookie.Value == "abc123" {
						foundSession = true
					}
					if cookie.Name == "user_pref" && cookie.Value == "dark_mode" {
						foundPref = true
					}
				}

				if !foundSession {
					t.Error("Session cookie not found")
				}
				if !foundPref {
					t.Error("Preference cookie not found")
				}
			},
		},
		{
			name: "Empty response body",
			httpResponse: &http.Response{
				StatusCode: 204,
				Status:     "204 No Content",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    &http.Request{}, // Add Request to avoid nil pointer
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 204 {
					t.Errorf("Expected status code 204, got %d", resp.StatusCode)
				}
				if resp.Body != "" {
					t.Errorf("Expected empty body, got '%s'", resp.Body)
				}
				if len(resp.RawBody) != 0 {
					t.Errorf("Expected empty RawBody, got %d bytes", len(resp.RawBody))
				}
			},
		},
		{
			name: "Binary response",
			httpResponse: &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header: http.Header{
					"Content-Type": []string{"application/octet-stream"},
				},
				Body:    io.NopCloser(bytes.NewReader([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})), // PNG header
				Request: &http.Request{},                                                                       // Add Request to avoid nil pointer
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 200 {
					t.Errorf("Expected status code 200, got %d", resp.StatusCode)
				}

				expectedBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
				if !bytes.Equal(resp.RawBody, expectedBytes) {
					t.Errorf("Expected binary data %v, got %v", expectedBytes, resp.RawBody)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := processor.Process(tt.httpResponse)
			if err != nil {
				t.Fatalf("Failed to process response: %v", err)
			}

			tt.validate(t, resp)
		})
	}
}

func TestResponseProcessor_LargeResponse(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   1024, // 1KB limit for testing
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewResponseProcessor(config, memManager)

	// Create response exceeding the limit
	largeData := strings.Repeat("A", 2048) // 2KB data
	httpResponse := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body:    io.NopCloser(strings.NewReader(largeData)),
		Request: &http.Request{}, // Add Request to avoid nil pointer
	}

	_, err := processor.Process(httpResponse)
	if err == nil {
		t.Error("Expected error for large response, got nil")
	}

	if !strings.Contains(err.Error(), "response body too large") {
		t.Errorf("Expected 'response body too large' error, got: %v", err)
	}
}

func TestResponseProcessor_HeaderProcessing(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewResponseProcessor(config, memManager)

	httpResponse := &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type":    []string{"application/json"},
			"Content-Length":  []string{"13"},
			"X-Custom-Header": []string{"custom-value"},
			"Cache-Control":   []string{"no-cache, no-store"},
			"Set-Cookie":      []string{"session=abc123"},
		},
		Body:    io.NopCloser(strings.NewReader(`{"test":true}`)),
		Request: &http.Request{}, // Add Request to avoid nil pointer
	}

	resp, err := processor.Process(httpResponse)
	if err != nil {
		t.Fatalf("Failed to process response: %v", err)
	}

	// Check if headers are correctly copied
	if len(resp.Headers["Content-Type"]) == 0 || resp.Headers["Content-Type"][0] != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %v", resp.Headers["Content-Type"])
	}

	if len(resp.Headers["X-Custom-Header"]) == 0 || resp.Headers["X-Custom-Header"][0] != "custom-value" {
		t.Errorf("Expected X-Custom-Header 'custom-value', got %v", resp.Headers["X-Custom-Header"])
	}

	if len(resp.Headers["Cache-Control"]) == 0 || resp.Headers["Cache-Control"][0] != "no-cache, no-store" {
		t.Errorf("Expected Cache-Control 'no-cache, no-store', got %v", resp.Headers["Cache-Control"])
	}

	// Check if Content-Length is correctly set
	if resp.ContentLength != 13 {
		t.Errorf("Expected content length 13, got %d", resp.ContentLength)
	}
}

func TestResponseProcessor_CookieProcessing(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewResponseProcessor(config, memManager)

	tests := []struct {
		name       string
		setCookies []string
		validate   func(*testing.T, []*http.Cookie)
	}{
		{
			name: "Simple cookie",
			setCookies: []string{
				"session_id=abc123",
			},
			validate: func(t *testing.T, cookies []*http.Cookie) {
				if len(cookies) != 1 {
					t.Errorf("Expected 1 cookie, got %d", len(cookies))
					return
				}

				cookie := cookies[0]
				if cookie.Name != "session_id" {
					t.Errorf("Expected cookie name 'session_id', got '%s'", cookie.Name)
				}
				if cookie.Value != "abc123" {
					t.Errorf("Expected cookie value 'abc123', got '%s'", cookie.Value)
				}
			},
		},
		{
			name: "Cookie with attributes",
			setCookies: []string{
				"session_id=abc123; Path=/; HttpOnly; Secure",
			},
			validate: func(t *testing.T, cookies []*http.Cookie) {
				if len(cookies) != 1 {
					t.Errorf("Expected 1 cookie, got %d", len(cookies))
					return
				}

				cookie := cookies[0]
				if cookie.Name != "session_id" {
					t.Errorf("Expected cookie name 'session_id', got '%s'", cookie.Name)
				}
				if cookie.Value != "abc123" {
					t.Errorf("Expected cookie value 'abc123', got '%s'", cookie.Value)
				}
				if cookie.Path != "/" {
					t.Errorf("Expected cookie path '/', got '%s'", cookie.Path)
				}
				if !cookie.HttpOnly {
					t.Error("Expected HttpOnly cookie")
				}
				if !cookie.Secure {
					t.Error("Expected Secure cookie")
				}
			},
		},
		{
			name: "Multiple cookies",
			setCookies: []string{
				"session_id=abc123; Path=/",
				"user_pref=dark_mode; Path=/settings",
				"lang=en; Domain=.example.com",
			},
			validate: func(t *testing.T, cookies []*http.Cookie) {
				if len(cookies) != 3 {
					t.Errorf("Expected 3 cookies, got %d", len(cookies))
					return
				}

				cookieMap := make(map[string]*http.Cookie)
				for _, cookie := range cookies {
					cookieMap[cookie.Name] = cookie
				}

				if session, ok := cookieMap["session_id"]; ok {
					if session.Value != "abc123" {
						t.Errorf("Expected session_id value 'abc123', got '%s'", session.Value)
					}
					if session.Path != "/" {
						t.Errorf("Expected session_id path '/', got '%s'", session.Path)
					}
				} else {
					t.Error("session_id cookie not found")
				}

				if pref, ok := cookieMap["user_pref"]; ok {
					if pref.Value != "dark_mode" {
						t.Errorf("Expected user_pref value 'dark_mode', got '%s'", pref.Value)
					}
					if pref.Path != "/settings" {
						t.Errorf("Expected user_pref path '/settings', got '%s'", pref.Path)
					}
				} else {
					t.Error("user_pref cookie not found")
				}

				if lang, ok := cookieMap["lang"]; ok {
					if lang.Value != "en" {
						t.Errorf("Expected lang value 'en', got '%s'", lang.Value)
					}
					if lang.Domain != ".example.com" {
						t.Errorf("Expected lang domain '.example.com', got '%s'", lang.Domain)
					}
				} else {
					t.Error("lang cookie not found")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpResponse := &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header: http.Header{
					"Set-Cookie": tt.setCookies,
				},
				Body:    io.NopCloser(strings.NewReader("OK")),
				Request: &http.Request{}, // Add Request to avoid nil pointer
			}

			resp, err := processor.Process(httpResponse)
			if err != nil {
				t.Fatalf("Failed to process response: %v", err)
			}

			tt.validate(t, resp.Cookies)
		})
	}
}

func TestResponseProcessor_ErrorHandling(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewResponseProcessor(config, memManager)

	tests := []struct {
		name         string
		httpResponse *http.Response
		expectError  bool
	}{
		{
			name:         "Nil response",
			httpResponse: nil,
			expectError:  true,
		},
		{
			name: "Response with nil body",
			httpResponse: &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       nil,
				Request:    &http.Request{}, // Add Request to avoid nil pointer
			},
			expectError: false, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processor.Process(tt.httpResponse)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestResponseProcessor_ContentLengthHandling(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewResponseProcessor(config, memManager)

	tests := []struct {
		name           string
		contentLength  string
		body           string
		expectedLength int64
	}{
		{
			name:           "Correct content length",
			contentLength:  "13",
			body:           "Hello, World!",
			expectedLength: 13,
		},
		{
			name:           "No content length header",
			contentLength:  "",
			body:           "Hello, World!",
			expectedLength: 0, // Should be 0 when header is missing
		},
		{
			name:           "Invalid content length",
			contentLength:  "invalid",
			body:           "Hello, World!",
			expectedLength: 0, // Should be 0 when header is invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.contentLength != "" {
				headers.Set("Content-Length", tt.contentLength)
			}

			httpResponse := &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}

			resp, err := processor.Process(httpResponse)
			if err != nil {
				t.Fatalf("Failed to process response: %v", err)
			}

			if resp.ContentLength != tt.expectedLength {
				t.Errorf("Expected content length %d, got %d", tt.expectedLength, resp.ContentLength)
			}
		})
	}
}
