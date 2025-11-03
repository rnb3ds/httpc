package httpc

import (
	"context"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client, err := newTestClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Client should not be nil")
	}
}

func TestNewClientWithConfig(t *testing.T) {
	config := DefaultConfig()
	config.Timeout = 10 * time.Second
	config.MaxRetries = 2

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client with config: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Client should not be nil")
	}
}

func TestSecureClient(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 1
	config.FollowRedirects = false
	config.EnableCookies = false
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create secure client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Secure client should not be nil")
	}
}

func TestRequestOptions(t *testing.T) {
	// Test that request options can be created without errors
	options := []RequestOption{
		WithHeader("X-Test", "value"),
		WithUserAgent("test-agent"),
		WithTimeout(5 * time.Second),
		WithJSON(map[string]string{"key": "value"}),
		WithQuery("param", "value"),
		WithBasicAuth("user", "pass"),
		WithBearerToken("token123"),
	}

	// Create a dummy request to test options
	req := &Request{
		Method:  "GET",
		URL:     "https://example.com",
		Headers: make(map[string]string),
		Context: context.Background(),
	}

	// Apply all options
	for _, opt := range options {
		opt(req)
	}

	// Verify some options were applied
	if req.Headers["X-Test"] != "value" {
		t.Error("Header option not applied correctly")
	}

	if req.Headers["User-Agent"] != "test-agent" {
		t.Error("User-Agent option not applied correctly")
	}

	if req.Timeout != 5*time.Second {
		t.Error("Timeout option not applied correctly")
	}
}

func TestResponseMethods(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		isSuccess  bool
		isRedirect bool
		isClient   bool
		isServer   bool
	}{
		{"200 OK", 200, true, false, false, false},
		{"201 Created", 201, true, false, false, false},
		{"301 Moved", 301, false, true, false, false},
		{"404 Not Found", 404, false, false, true, false},
		{"500 Server Error", 500, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{StatusCode: tt.statusCode}

			if resp.IsSuccess() != tt.isSuccess {
				t.Errorf("IsSuccess() = %v, want %v", resp.IsSuccess(), tt.isSuccess)
			}

			if resp.IsRedirect() != tt.isRedirect {
				t.Errorf("IsRedirect() = %v, want %v", resp.IsRedirect(), tt.isRedirect)
			}

			if resp.IsClientError() != tt.isClient {
				t.Errorf("IsClientError() = %v, want %v", resp.IsClientError(), tt.isClient)
			}

			if resp.IsServerError() != tt.isServer {
				t.Errorf("IsServerError() = %v, want %v", resp.IsServerError(), tt.isServer)
			}
		})
	}
}

// TestDefaultConfig removed - covered by config_validation_test.go

// TestPackageLevelFunctions removed - covered by package_level_test.go

func TestHTTPError(t *testing.T) {
	err := &HTTPError{
		StatusCode: 404,
		Status:     "Not Found",
		Method:     "GET",
		URL:        "https://example.com",
	}

	expected := "HTTP 404: GET https://example.com"
	if err.Error() != expected {
		t.Errorf("HTTPError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestFormData(t *testing.T) {
	formData := &FormData{
		Fields: map[string]string{
			"field1": "value1",
			"field2": "value2",
		},
		Files: map[string]*FileData{
			"file1": {
				Filename: "test.txt",
				Content:  []byte("test content"),
			},
		},
	}

	if len(formData.Fields) != 2 {
		t.Error("FormData should have 2 fields")
	}

	if len(formData.Files) != 1 {
		t.Error("FormData should have 1 file")
	}

	file := formData.Files["file1"]
	if file.Filename != "test.txt" {
		t.Error("File filename should be test.txt")
	}

	if string(file.Content) != "test content" {
		t.Error("File content should match")
	}
}

// TestSecurityFeatures removed - covered by security_test.go and comprehensive_test.go

func TestConcurrencyLimits(t *testing.T) {
	client, err := newTestClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test that client can be closed safely
	if err := client.Close(); err != nil {
		t.Errorf("Client close should not error: %v", err)
	}

	// Test that closed client returns error
	_, err = client.Get("https://example.com")
	if err == nil {
		t.Error("Expected error when using closed client")
	}
}
