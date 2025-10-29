package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/memory"
)

// ============================================================================
// REQUEST BUILDER TESTS
// ============================================================================

func TestRequestProcessor_BuildHTTPRequest(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
		UserAgent:             "test-client/1.0",
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewRequestProcessor(config, memManager)

	tests := []struct {
		name        string
		request     *Request
		expectError bool
		validate    func(*testing.T, *http.Request)
	}{
		{
			name: "Simple GET request",
			request: &Request{
				Method:  "GET",
				URL:     "https://api.example.com/users",
				Context: context.Background(),
				Headers: map[string]string{
					"Accept": "application/json",
				},
			},
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Method != "GET" {
					t.Errorf("Expected method GET, got %s", req.Method)
				}
				if req.URL.String() != "https://api.example.com/users" {
					t.Errorf("Expected URL https://api.example.com/users, got %s", req.URL.String())
				}
				if req.Header.Get("Accept") != "application/json" {
					t.Errorf("Expected Accept header application/json, got %s", req.Header.Get("Accept"))
				}
				if req.Header.Get("User-Agent") != "test-client/1.0" {
					t.Errorf("Expected User-Agent test-client/1.0, got %s", req.Header.Get("User-Agent"))
				}
			},
		},
		{
			name: "POST with JSON body",
			request: &Request{
				Method:  "POST",
				URL:     "https://api.example.com/users",
				Context: context.Background(),
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: map[string]interface{}{
					"name":  "John Doe",
					"email": "john@example.com",
				},
			},
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Method != "POST" {
					t.Errorf("Expected method POST, got %s", req.Method)
				}
				if req.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", req.Header.Get("Content-Type"))
				}
				if req.Body == nil {
					t.Error("Expected request body, got nil")
				}
			},
		},
		{
			name: "Request with query parameters",
			request: &Request{
				Method:  "GET",
				URL:     "https://api.example.com/users",
				Context: context.Background(),
				QueryParams: map[string]any{
					"page":   1,
					"limit":  10,
					"filter": "active",
				},
			},
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				query := req.URL.Query()
				if query.Get("page") != "1" {
					t.Errorf("Expected page=1, got %s", query.Get("page"))
				}
				if query.Get("limit") != "10" {
					t.Errorf("Expected limit=10, got %s", query.Get("limit"))
				}
				if query.Get("filter") != "active" {
					t.Errorf("Expected filter=active, got %s", query.Get("filter"))
				}
			},
		},
		{
			name: "Request with cookies",
			request: &Request{
				Method:  "GET",
				URL:     "https://api.example.com/users",
				Context: context.Background(),
				Cookies: []*http.Cookie{
					{Name: "session_id", Value: "abc123"},
					{Name: "user_pref", Value: "dark_mode"},
				},
			},
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				cookies := req.Cookies()
				if len(cookies) != 2 {
					t.Errorf("Expected 2 cookies, got %d", len(cookies))
				}

				sessionFound := false
				prefFound := false
				for _, cookie := range cookies {
					if cookie.Name == "session_id" && cookie.Value == "abc123" {
						sessionFound = true
					}
					if cookie.Name == "user_pref" && cookie.Value == "dark_mode" {
						prefFound = true
					}
				}

				if !sessionFound {
					t.Error("session_id cookie not found")
				}
				if !prefFound {
					t.Error("user_pref cookie not found")
				}
			},
		},
		{
			name: "Request with timeout",
			request: &Request{
				Method:  "GET",
				URL:     "https://api.example.com/users",
				Context: context.Background(),
				Timeout: 15 * time.Second,
			},
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				// Check if context has timeout
				deadline, ok := req.Context().Deadline()
				if !ok {
					t.Error("Expected context with deadline, got none")
				} else {
					// Check if timeout is reasonable (allow some margin of error)
					expectedDeadline := time.Now().Add(15 * time.Second)
					if deadline.Before(expectedDeadline.Add(-1*time.Second)) ||
						deadline.After(expectedDeadline.Add(1*time.Second)) {
						t.Errorf("Unexpected deadline: %v", deadline)
					}
				}
			},
		},
		{
			name: "Invalid URL",
			request: &Request{
				Method:  "GET",
				URL:     "://invalid-url",
				Context: context.Background(),
			},
			expectError: true,
		},
		{
			name: "Empty method",
			request: &Request{
				Method:  "",
				URL:     "https://api.example.com/users",
				Context: context.Background(),
			},
			expectError: false, // Empty method defaults to GET
		},
		{
			name: "Nil context",
			request: &Request{
				Method:  "GET",
				URL:     "https://api.example.com/users",
				Context: nil,
			},
			expectError: false, // Nil context defaults to Background
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReq, err := processor.Build(tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if httpReq == nil {
				t.Fatal("Expected HTTP request, got nil")
			}

			if tt.validate != nil {
				tt.validate(t, httpReq)
			}
		})
	}
}

func TestRequestProcessor_BodySerializationComprehensive(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewRequestProcessor(config, memManager)

	tests := []struct {
		name        string
		body        interface{}
		contentType string
		expectError bool
		validate    func(*testing.T, *http.Request)
	}{
		{
			name:        "JSON object",
			body:        map[string]interface{}{"name": "John", "age": 30},
			contentType: "application/json",
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", req.Header.Get("Content-Type"))
				}
			},
		},
		{
			name:        "String body",
			body:        "Hello, World!",
			contentType: "text/plain",
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Body == nil {
					t.Error("Expected request body, got nil")
				}
			},
		},
		{
			name:        "Byte array body",
			body:        []byte("binary data"),
			contentType: "application/octet-stream",
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Body == nil {
					t.Error("Expected request body, got nil")
				}
			},
		},
		{
			name:        "URL values (form)",
			body:        url.Values{"key1": []string{"value1"}, "key2": []string{"value2"}},
			contentType: "application/x-www-form-urlencoded",
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Errorf("Expected Content-Type application/x-www-form-urlencoded, got %s", req.Header.Get("Content-Type"))
				}
			},
		},
		{
			name:        "Nil body",
			body:        nil,
			contentType: "",
			expectError: false,
			validate: func(t *testing.T, req *http.Request) {
				if req.Body != nil && req.Body != http.NoBody {
					t.Error("Expected nil or NoBody, got non-nil body")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &Request{
				Method:  "POST",
				URL:     "https://api.example.com/test",
				Context: context.Background(),
				Body:    tt.body,
				Headers: make(map[string]string),
			}

			if tt.contentType != "" {
				request.Headers["Content-Type"] = tt.contentType
			}

			httpReq, err := processor.Build(request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, httpReq)
			}
		})
	}
}

func TestRequestProcessor_HeaderHandlingComprehensive(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
		UserAgent:             "test-client/1.0",
		Headers: map[string]string{
			"X-Default-Header": "default-value",
		},
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewRequestProcessor(config, memManager)

	request := &Request{
		Method:  "GET",
		URL:     "https://api.example.com/test",
		Context: context.Background(),
		Headers: map[string]string{
			"X-Custom-Header":  "custom-value",
			"X-Default-Header": "overridden-value", // Should override default value
		},
	}

	httpReq, err := processor.Build(request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check custom headers
	if httpReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("Expected X-Custom-Header custom-value, got %s", httpReq.Header.Get("X-Custom-Header"))
	}

	// Check overridden default headers
	if httpReq.Header.Get("X-Default-Header") != "overridden-value" {
		t.Errorf("Expected X-Default-Header overridden-value, got %s", httpReq.Header.Get("X-Default-Header"))
	}

	// Check User-Agent
	if httpReq.Header.Get("User-Agent") != "test-client/1.0" {
		t.Errorf("Expected User-Agent test-client/1.0, got %s", httpReq.Header.Get("User-Agent"))
	}
}

func TestRequestProcessor_QueryParameterHandlingComprehensive(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewRequestProcessor(config, memManager)

	tests := []struct {
		name     string
		url      string
		params   map[string]any
		expected map[string]string
	}{
		{
			name: "URL without existing query",
			url:  "https://api.example.com/users",
			params: map[string]any{
				"page":  1,
				"limit": 10,
			},
			expected: map[string]string{
				"page":  "1",
				"limit": "10",
			},
		},
		{
			name: "URL with existing query",
			url:  "https://api.example.com/users?sort=name",
			params: map[string]any{
				"page":  1,
				"limit": 10,
			},
			expected: map[string]string{
				"sort":  "name",
				"page":  "1",
				"limit": "10",
			},
		},
		{
			name: "Mixed parameter types",
			url:  "https://api.example.com/search",
			params: map[string]any{
				"q":      "golang",
				"count":  25,
				"active": true,
				"score":  9.5,
			},
			expected: map[string]string{
				"q":      "golang",
				"count":  "25",
				"active": "true",
				"score":  "9.5",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &Request{
				Method:      "GET",
				URL:         tt.url,
				Context:     context.Background(),
				QueryParams: tt.params,
			}

			httpReq, err := processor.Build(request)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			query := httpReq.URL.Query()
			for key, expectedValue := range tt.expected {
				actualValue := query.Get(key)
				if actualValue != expectedValue {
					t.Errorf("Expected %s=%s, got %s=%s", key, expectedValue, key, actualValue)
				}
			}
		})
	}
}

func TestRequestProcessor_EdgeCases(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		MaxResponseBodySize:   50 * 1024 * 1024,
	}

	memManager := memory.NewManager(memory.DefaultConfig())
	defer memManager.Close()

	processor := NewRequestProcessor(config, memManager)

	t.Run("Very long URL", func(t *testing.T) {
		longPath := strings.Repeat("a", 2000)
		request := &Request{
			Method:  "GET",
			URL:     "https://api.example.com/" + longPath,
			Context: context.Background(),
		}

		_, err := processor.Build(request)
		// Should be able to handle long URLs (within reasonable limits)
		if err != nil {
			t.Errorf("Unexpected error for long URL: %v", err)
		}
	})

	t.Run("Many query parameters", func(t *testing.T) {
		params := make(map[string]any)
		for i := 0; i < 100; i++ {
			params[fmt.Sprintf("param%d", i)] = fmt.Sprintf("value%d", i)
		}

		request := &Request{
			Method:      "GET",
			URL:         "https://api.example.com/test",
			Context:     context.Background(),
			QueryParams: params,
		}

		httpReq, err := processor.Build(request)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		query := httpReq.URL.Query()
		if len(query) != 100 {
			t.Errorf("Expected 100 query parameters, got %d", len(query))
		}
	})

	t.Run("Many headers", func(t *testing.T) {
		headers := make(map[string]string)
		for i := 0; i < 50; i++ {
			headers[fmt.Sprintf("X-Header-%d", i)] = fmt.Sprintf("value-%d", i)
		}

		request := &Request{
			Method:  "GET",
			URL:     "https://api.example.com/test",
			Context: context.Background(),
			Headers: headers,
		}

		httpReq, err := processor.Build(request)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Check that all headers are set
		for key, expectedValue := range headers {
			actualValue := httpReq.Header.Get(key)
			if actualValue != expectedValue {
				t.Errorf("Expected header %s=%s, got %s", key, expectedValue, actualValue)
			}
		}
	})
}
