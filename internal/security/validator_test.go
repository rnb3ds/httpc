package security

import (
	"strings"
	"testing"
)

// ============================================================================
// SECURITY VALIDATOR UNIT TESTS
// ============================================================================

func TestValidator_NewValidator(t *testing.T) {
	validator := NewValidator()
	if validator == nil {
		t.Fatal("Expected validator to be created")
	}

	if validator.config == nil {
		t.Fatal("Expected config to be set")
	}

	if !validator.config.ValidateURL {
		t.Error("Expected ValidateURL to be true by default")
	}

	if !validator.config.ValidateHeaders {
		t.Error("Expected ValidateHeaders to be true by default")
	}
}

func TestValidator_ValidateURL(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		url       string
		shouldErr bool
		errMsg    string
	}{
		{
			name:      "Valid HTTP URL",
			url:       "http://example.com",
			shouldErr: false,
		},
		{
			name:      "Valid HTTPS URL",
			url:       "https://example.com/path",
			shouldErr: false,
		},
		{
			name:      "Valid URL with port",
			url:       "http://example.com:8080",
			shouldErr: false,
		},
		{
			name:      "Valid URL with query",
			url:       "https://example.com/path?key=value",
			shouldErr: false,
		},
		{
			name:      "Empty URL",
			url:       "",
			shouldErr: true,
			errMsg:    "URL cannot be empty",
		},
		{
			name:      "Missing scheme",
			url:       "example.com",
			shouldErr: true,
			errMsg:    "URL scheme is required",
		},
		{
			name:      "Invalid scheme - FTP",
			url:       "ftp://example.com",
			shouldErr: true,
			errMsg:    "unsupported URL scheme",
		},
		{
			name:      "Invalid scheme - JavaScript",
			url:       "javascript:alert(1)",
			shouldErr: true,
			errMsg:    "URL host is required", // Host is checked before scheme
		},
		{
			name:      "Invalid scheme - Data URL",
			url:       "data:text/html,<script>alert(1)</script>",
			shouldErr: true,
			errMsg:    "URL host is required", // Host is checked before scheme
		},
		{
			name:      "Invalid scheme - File",
			url:       "file:///etc/passwd",
			shouldErr: true,
			errMsg:    "URL host is required", // File URLs have empty host, so host check fails first
		},
		{
			name:      "Missing host",
			url:       "http://",
			shouldErr: true,
			errMsg:    "URL host is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Method: "GET",
				URL:    tt.url,
			}

			err := validator.ValidateRequest(req)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for URL: %s", tt.url)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message to contain '%s', got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid URL %s: %v", tt.url, err)
				}
			}
		})
	}
}

func TestValidator_ValidateHeaders(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		headers   map[string]string
		shouldErr bool
		errMsg    string
	}{
		{
			name: "Valid headers",
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
			shouldErr: false,
		},
		{
			name: "CRLF injection in key",
			headers: map[string]string{
				"X-Test\r\nX-Injected": "value",
			},
			shouldErr: true,
			errMsg:    "invalid characters",
		},
		{
			name: "CRLF injection in value",
			headers: map[string]string{
				"X-Test": "value\r\nX-Injected: bad",
			},
			shouldErr: true,
			errMsg:    "invalid characters",
		},
		{
			name: "Null byte in key",
			headers: map[string]string{
				"X-Test\x00": "value",
			},
			shouldErr: true,
			errMsg:    "invalid characters",
		},
		{
			name: "Null byte in value",
			headers: map[string]string{
				"X-Test": "value\x00",
			},
			shouldErr: true,
			errMsg:    "invalid characters",
		},
		{
			name: "Very long header value",
			headers: map[string]string{
				"X-Test": strings.Repeat("a", 10000),
			},
			shouldErr: true,
			errMsg:    "too long",
		},
		{
			name: "Empty header key",
			headers: map[string]string{
				"": "value",
			},
			shouldErr: true,
			errMsg:    "cannot be empty",
		},
		{
			name: "Whitespace-only header key",
			headers: map[string]string{
				"   ": "value",
			},
			shouldErr: true,
			errMsg:    "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Method:  "GET",
				URL:     "http://example.com",
				Headers: tt.headers,
			}

			err := validator.ValidateRequest(req)
			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error for invalid headers")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message to contain '%s', got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid headers: %v", err)
				}
			}
		})
	}
}

func TestValidator_ValidateRequestSize(t *testing.T) {
	validator := NewValidator()
	// Override max size for testing
	validator.config.MaxResponseBodySize = 1024 // 1KB limit

	tests := []struct {
		name      string
		body      interface{}
		shouldErr bool
	}{
		{
			name:      "Small string body",
			body:      "small content",
			shouldErr: false,
		},
		{
			name:      "Large string body",
			body:      strings.Repeat("a", 2000),
			shouldErr: true,
		},
		{
			name:      "Small byte array",
			body:      []byte("small content"),
			shouldErr: false,
		},
		{
			name:      "Large byte array",
			body:      make([]byte, 2000),
			shouldErr: true,
		},
		{
			name:      "Nil body",
			body:      nil,
			shouldErr: false,
		},
		{
			name:      "Struct body (not validated)",
			body:      struct{ Data string }{Data: strings.Repeat("a", 2000)},
			shouldErr: false, // Structs are not validated for size
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Method: "POST",
				URL:    "http://example.com",
				Body:   tt.body,
			}

			err := validator.ValidateRequest(req)
			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error for large request body")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidator_DisabledValidation(t *testing.T) {
	validator := NewValidator()
	// Disable validation for testing
	validator.config.ValidateURL = false
	validator.config.ValidateHeaders = false

	// Even with invalid data, validation should pass when disabled
	req := &Request{
		Method: "GET",
		URL:    "javascript:alert(1)", // Invalid scheme
		Headers: map[string]string{
			"X-Test\r\n": "value\r\n", // CRLF injection
		},
	}

	err := validator.ValidateRequest(req)
	if err != nil {
		t.Errorf("Expected no error when validation is disabled, got: %v", err)
	}
}

func TestValidator_ComplexScenarios(t *testing.T) {
	validator := NewValidator()

	t.Run("Valid complex request", func(t *testing.T) {
		req := &Request{
			Method: "POST",
			URL:    "https://api.example.com/v1/users?page=1&limit=10",
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				"X-Request-ID":  "123e4567-e89b-12d3-a456-426614174000",
				"User-Agent":    "Mozilla/5.0",
			},
			Body: `{"name":"John Doe","email":"john@example.com"}`,
		}

		err := validator.ValidateRequest(req)
		if err != nil {
			t.Errorf("Unexpected error for valid complex request: %v", err)
		}
	})

	t.Run("Multiple validation failures", func(t *testing.T) {
		req := &Request{
			Method: "POST",
			URL:    "", // Invalid: empty URL
			Headers: map[string]string{
				"X-Test\r\n": "value", // Invalid: CRLF in key
			},
			Body: strings.Repeat("a", 20000), // Invalid: too large
		}

		err := validator.ValidateRequest(req)
		if err == nil {
			t.Error("Expected error for request with multiple validation failures")
		}
	})
}

func TestValidator_SpecialCharacters(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "URL with Unicode",
			url:     "https://example.com/path/chinese-text",
			wantErr: false,
		},
		{
			name:    "URL with encoded characters",
			url:     "https://example.com/path?q=%E4%B8%AD%E6%96%87",
			wantErr: false,
		},
		{
			name:    "URL with special characters",
			url:     "https://example.com/path?q=hello+world&foo=bar",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{
				Method: "GET",
				URL:    tt.url,
			}

			err := validator.ValidateRequest(req)
			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
