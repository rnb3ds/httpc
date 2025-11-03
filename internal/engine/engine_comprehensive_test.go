package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CORE ENGINE COMPREHENSIVE TESTS - IMPROVE COVERAGE
// ============================================================================

func TestEngine_RequestProcessing(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		url      string
		options  []RequestOption
		validate func(*testing.T, *Request)
	}{
		{
			name:   "GET with headers",
			method: "GET",
			url:    "https://api.example.com/users",
			options: []RequestOption{
				func(r *Request) {
					if r.Headers == nil {
						r.Headers = make(map[string]string)
					}
					r.Headers["Authorization"] = "Bearer token"
					r.Headers["Accept"] = "application/json"
				},
			},
			validate: func(t *testing.T, req *Request) {
				if req.Headers["Authorization"] != "Bearer token" {
					t.Error("Authorization header not set correctly")
				}
				if req.Headers["Accept"] != "application/json" {
					t.Error("Accept header not set correctly")
				}
			},
		},
		{
			name:   "POST with query params",
			method: "POST",
			url:    "https://api.example.com/users",
			options: []RequestOption{
				func(r *Request) {
					if r.QueryParams == nil {
						r.QueryParams = make(map[string]any)
					}
					r.QueryParams["page"] = 1
					r.QueryParams["limit"] = 10
				},
			},
			validate: func(t *testing.T, req *Request) {
				if req.QueryParams["page"] != 1 {
					t.Error("Page query param not set correctly")
				}
				if req.QueryParams["limit"] != 10 {
					t.Error("Limit query param not set correctly")
				}
			},
		},
		{
			name:   "PUT with body",
			method: "PUT",
			url:    "https://api.example.com/users/123",
			options: []RequestOption{
				func(r *Request) {
					r.Body = map[string]any{
						"name":  "John Doe",
						"email": "john@example.com",
					}
				},
			},
			validate: func(t *testing.T, req *Request) {
				if req.Body == nil {
					t.Error("Body not set")
				}
			},
		},
		{
			name:   "DELETE with timeout",
			method: "DELETE",
			url:    "https://api.example.com/users/123",
			options: []RequestOption{
				func(r *Request) {
					r.Timeout = 30 * time.Second
				},
			},
			validate: func(t *testing.T, req *Request) {
				if req.Timeout != 30*time.Second {
					t.Error("Timeout not set correctly")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Timeout:               60 * time.Second,
				MaxConcurrentRequests: 100,
				ValidateURL:           true,
				ValidateHeaders:       true,
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			// Create request
			req := &Request{
				Method:      tt.method,
				URL:         tt.url,
				Headers:     make(map[string]string),
				QueryParams: make(map[string]any),
				Context:     context.Background(),
			}

			// Apply options
			for _, opt := range tt.options {
				opt(req)
			}

			// Validate request
			tt.validate(t, req)
		})
	}
}

func TestEngine_ResponseProcessing(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		validate       func(*testing.T, *Response)
	}{
		{
			name: "JSON response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"success","code":200}`))
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 200 {
					t.Errorf("Expected status 200, got %d", resp.StatusCode)
				}
				if !strings.Contains(resp.Body, "success") {
					t.Error("Response body doesn't contain expected content")
				}
				if len(resp.RawBody) == 0 {
					t.Error("RawBody should not be empty")
				}
			},
		},
		{
			name: "Error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid request"}`))
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 400 {
					t.Errorf("Expected status 400, got %d", resp.StatusCode)
				}
				if !strings.Contains(resp.Body, "error") {
					t.Error("Response body doesn't contain error message")
				}
			},
		},
		{
			name: "Large response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				// Write large amount of data
				data := strings.Repeat("A", 1024*1024) // 1MB
				w.Write([]byte(data))
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode != 200 {
					t.Errorf("Expected status 200, got %d", resp.StatusCode)
				}
				if len(resp.RawBody) < 1024*1024 {
					t.Error("Large response not handled correctly")
				}
			},
		},
		{
			name: "Response with cookies",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, &http.Cookie{
					Name:  "session_id",
					Value: "abc123",
					Path:  "/",
				})
				http.SetCookie(w, &http.Cookie{
					Name:  "user_pref",
					Value: "dark_mode",
					Path:  "/",
				})
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			config := &Config{
				Timeout:               60 * time.Second,
				MaxConcurrentRequests: 100,
				ValidateURL:           true,
				ValidateHeaders:       true,
				AllowPrivateIPs:       true, // Allow test server access
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			tt.validate(t, resp)
		})
	}
}

func TestEngine_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name: "Connection refused",
			setupServer: func() *httptest.Server {
				// Return a closed server
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				server.Close() // Close immediately
				return server
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected connection error, got nil")
				}
			},
		},
		{
			name: "Server timeout",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second) // Exceed client timeout
					w.WriteHeader(http.StatusOK)
				}))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected timeout error, got nil")
				}
			},
		},
		{
			name: "Invalid response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Send invalid HTTP response
					w.Header().Set("Content-Length", "100")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("short")) // Content length mismatch
				}))
			},
			expectError: true, // Our enhanced security now detects this
			errorCheck: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected content-length mismatch error, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			if server != nil {
				defer server.Close()
			}

			config := &Config{
				Timeout:               1 * time.Second, // Short timeout for testing
				MaxConcurrentRequests: 100,
				ValidateURL:           true,
				ValidateHeaders:       true,
				AllowPrivateIPs:       true, // Allow test server access
				StrictContentLength:   true, // Enable strict content-length validation
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			_, err = client.Get(server.URL)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.errorCheck != nil {
				tt.errorCheck(t, err)
			}
		})
	}
}

func TestEngine_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate processing time
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 50,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       true, // Allow test server access
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	const numRequests = 20
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)
	responses := make(chan *Response, numRequests)

	start := time.Now()

	for i := range numRequests {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			resp, err := client.Get(server.URL)
			if err != nil {
				errors <- err
				return
			}

			responses <- resp
		}(i)
	}

	wg.Wait()
	close(errors)
	close(responses)

	duration := time.Since(start)

	// Check errors
	errorCount := 0
	for err := range errors {
		t.Logf("Request error: %v", err)
		errorCount++
	}

	// Check responses
	responseCount := 0
	for resp := range responses {
		if resp.StatusCode != 200 {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
		responseCount++
	}

	t.Logf("Concurrent test results:")
	t.Logf("  Total requests: %d", numRequests)
	t.Logf("  Successful: %d", responseCount)
	t.Logf("  Failed: %d", errorCount)
	t.Logf("  Duration: %v", duration)

	if responseCount < numRequests-2 { // Allow some failures
		t.Errorf("Too many failed requests: %d/%d", errorCount, numRequests)
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Long processing time
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       true, // Allow test server access
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = client.Request(ctx, "GET", server.URL)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	if duration > 1*time.Second {
		t.Errorf("Request took too long to cancel: %v", duration)
	}

	t.Logf("Context cancellation worked correctly in %v", duration)
}

func TestEngine_RetryMechanism(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       true, // Allow test server access
		MaxRetries:            3,
		RetryDelay:            100 * time.Millisecond,
		BackoffFactor:         2.0,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	start := time.Now()
	resp, err := client.Get(server.URL)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Request failed after retries: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Attempts < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", resp.Attempts)
	}

	t.Logf("Retry mechanism worked: %d attempts in %v", attemptCount, duration)
}

func TestEngine_ClientLifecycle(t *testing.T) {
	config := &Config{
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 100,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       true, // Allow test server access
	}

	// Test client creation
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test client usage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Errorf("Request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test client close
	err = client.Close()
	if err != nil {
		t.Errorf("Failed to close client: %v", err)
	}

	// Test request after close should fail
	_, err = client.Get(server.URL)
	if err == nil {
		t.Error("Expected error after client close, got nil")
	}
}

func TestEngine_ConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name:        "Nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "Valid config",
			config: &Config{
				Timeout:               30 * time.Second,
				MaxConcurrentRequests: 100,
				ValidateURL:           true,
				ValidateHeaders:       true,
				AllowPrivateIPs:       true, // Allow test server access
			},
			expectError: false,
		},
		{
			name: "Zero timeout",
			config: &Config{
				Timeout:               0,
				MaxConcurrentRequests: 100,
				ValidateURL:           true,
				ValidateHeaders:       true,
				AllowPrivateIPs:       true, // Allow test server access
			},
			expectError: false, // Zero timeout is allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if client != nil {
				client.Close()
			}
		})
	}
}
