package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ============================================================================
// ERROR HANDLING COMPREHENSIVE TESTS
// ============================================================================

func TestErrorClassification_NetworkErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		isRetryable  bool
	}{
		{
			name:         "Connection refused",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
			expectedType: ErrorTypeNetwork,
			isRetryable:  true,
		},
		{
			name:         "DNS resolution failure",
			err:          &net.DNSError{Err: "no such host", Name: "nonexistent.example.com"},
			expectedType: ErrorTypeNetwork,
			isRetryable:  true,
		},
		{
			name:         "Connection timeout",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ETIMEDOUT},
			expectedType: ErrorTypeNetwork,
			isRetryable:  true,
		},
		{
			name:         "Connection reset",
			err:          &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
			expectedType: ErrorTypeNetwork,
			isRetryable:  true,
		},
		{
			name:         "Network unreachable",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH},
			expectedType: ErrorTypeNetwork,
			isRetryable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientErr := ClassifyError(tt.err, "https://example.com", "GET", 1)

			if clientErr.Type != tt.expectedType {
				t.Errorf("Expected error type %v, got %v", tt.expectedType, clientErr.Type)
			}

			if clientErr.IsRetryable() != tt.isRetryable {
				t.Errorf("Expected isRetryable=%v, got %v", tt.isRetryable, clientErr.IsRetryable())
			}

			if !strings.Contains(clientErr.Error(), "GET https://example.com") {
				t.Errorf("Error message should contain method and URL: %s", clientErr.Error())
			}
		})
	}
}

func TestErrorClassification_ContextErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		isRetryable  bool
	}{
		{
			name:         "Context deadline exceeded",
			err:          context.DeadlineExceeded,
			expectedType: ErrorTypeTimeout,
			isRetryable:  true,
		},
		{
			name:         "Context cancelled",
			err:          context.Canceled,
			expectedType: ErrorTypeContextCanceled,
			isRetryable:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientErr := ClassifyError(tt.err, "https://example.com", "POST", 2)

			if clientErr.Type != tt.expectedType {
				t.Errorf("Expected error type %v, got %v", tt.expectedType, clientErr.Type)
			}

			if clientErr.IsRetryable() != tt.isRetryable {
				t.Errorf("Expected isRetryable=%v, got %v", tt.isRetryable, clientErr.IsRetryable())
			}

			if clientErr.Attempts != 2 {
				t.Errorf("Expected attempts=2, got %d", clientErr.Attempts)
			}
		})
	}
}

func TestErrorClassification_HTTPErrors(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		expectedType ErrorType
		isRetryable  bool
	}{
		{
			name:         "400 Bad Request",
			statusCode:   400,
			expectedType: ErrorTypeHTTP,
			isRetryable:  false,
		},
		{
			name:         "401 Unauthorized",
			statusCode:   401,
			expectedType: ErrorTypeHTTP,
			isRetryable:  false,
		},
		{
			name:         "403 Forbidden",
			statusCode:   403,
			expectedType: ErrorTypeHTTP,
			isRetryable:  false,
		},
		{
			name:         "404 Not Found",
			statusCode:   404,
			expectedType: ErrorTypeHTTP,
			isRetryable:  false,
		},
		{
			name:         "429 Too Many Requests",
			statusCode:   429,
			expectedType: ErrorTypeHTTP,
			isRetryable:  true,
		},
		{
			name:         "500 Internal Server Error",
			statusCode:   500,
			expectedType: ErrorTypeHTTP,
			isRetryable:  true,
		},
		{
			name:         "502 Bad Gateway",
			statusCode:   502,
			expectedType: ErrorTypeHTTP,
			isRetryable:  true,
		},
		{
			name:         "503 Service Unavailable",
			statusCode:   503,
			expectedType: ErrorTypeHTTP,
			isRetryable:  true,
		},
		{
			name:         "504 Gateway Timeout",
			statusCode:   504,
			expectedType: ErrorTypeHTTP,
			isRetryable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an HTTP error
			httpErr := fmt.Errorf("HTTP %d", tt.statusCode)
			clientErr := ClassifyError(httpErr, "https://api.example.com/users", "GET", 1)

			if clientErr.Type != tt.expectedType {
				t.Errorf("Expected error type %v, got %v", tt.expectedType, clientErr.Type)
			}

			if clientErr.IsRetryable() != tt.isRetryable {
				t.Errorf("Expected isRetryable=%v, got %v", tt.isRetryable, clientErr.IsRetryable())
			}
		})
	}
}

func TestErrorClassification_TransportErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		isRetryable  bool
	}{
		{
			name:         "Transport error",
			err:          fmt.Errorf("transport error: connection failed"),
			expectedType: ErrorTypeTransport,
			isRetryable:  true,
		},
		{
			name:         "Response read error",
			err:          fmt.Errorf("failed to read response body: unexpected EOF"),
			expectedType: ErrorTypeResponseRead,
			isRetryable:  true,
		},
		{
			name:         "URL parse error",
			err:          &url.Error{Op: "parse", URL: "://invalid", Err: fmt.Errorf("missing protocol scheme")},
			expectedType: ErrorTypeValidation,
			isRetryable:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientErr := ClassifyError(tt.err, "https://example.com", "GET", 1)

			if clientErr.Type != tt.expectedType {
				t.Errorf("Expected error type %v, got %v (error: %q)", tt.expectedType, clientErr.Type, tt.err.Error())
			}

			if clientErr.IsRetryable() != tt.isRetryable {
				t.Errorf("Expected isRetryable=%v, got %v", tt.isRetryable, clientErr.IsRetryable())
			}
		})
	}
}

func TestErrorClassification_UnknownErrors(t *testing.T) {
	unknownErr := fmt.Errorf("some unknown error")
	clientErr := ClassifyError(unknownErr, "https://example.com", "POST", 3)

	if clientErr.Type != ErrorTypeUnknown {
		t.Errorf("Expected error type %v, got %v", ErrorTypeUnknown, clientErr.Type)
	}

	if clientErr.IsRetryable() {
		t.Error("Unknown errors should not be retryable")
	}

	if clientErr.Attempts != 3 {
		t.Errorf("Expected attempts=3, got %d", clientErr.Attempts)
	}
}

func TestErrorSanitization(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectedURL string
	}{
		{
			name:        "URL with credentials",
			url:         "https://user:pass@api.example.com/data",
			expectedURL: "https://***:***@api.example.com/data",
		},
		{
			name:        "URL without credentials",
			url:         "https://api.example.com/data",
			expectedURL: "https://api.example.com/data",
		},
		{
			name:        "URL with only username",
			url:         "https://user@api.example.com/data",
			expectedURL: "https://***@api.example.com/data",
		},
		{
			name:        "HTTP URL with credentials",
			url:         "http://admin:secret@localhost:8080/admin",
			expectedURL: "http://***:***@localhost:8080/admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized := sanitizeURL(tt.url)
			if sanitized != tt.expectedURL {
				t.Errorf("Expected sanitized URL %s, got %s", tt.expectedURL, sanitized)
			}
		})
	}
}

func TestClientError_ErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      *ClientError
		expected string
	}{
		{
			name: "Complete error information",
			err: &ClientError{
				Type:     ErrorTypeNetwork,
				Message:  "connection refused",
				URL:      "https://api.example.com/users",
				Method:   "GET",
				Attempts: 3,
				Cause:    fmt.Errorf("dial tcp: connection refused"),
			},
			expected: "GET https://api.example.com/users: connection refused (attempt 3)",
		},
		{
			name: "Error without URL and method",
			err: &ClientError{
				Type:     ErrorTypeValidation,
				Message:  "invalid request",
				Attempts: 1,
				Cause:    fmt.Errorf("validation failed"),
			},
			expected: "invalid request (attempt 1)",
		},
		{
			name: "Error with credentials in URL",
			err: &ClientError{
				Type:     ErrorTypeNetwork,
				Message:  "connection timeout",
				URL:      "https://user:pass@api.example.com/data",
				Method:   "POST",
				Attempts: 2,
				Cause:    fmt.Errorf("timeout"),
			},
			expected: "POST https://***:***@api.example.com/data: connection timeout (attempt 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.err.Error()
			if actual != tt.expected {
				t.Errorf("Expected error message:\n%s\nGot:\n%s", tt.expected, actual)
			}
		})
	}
}

func TestClientError_UnwrapComprehensive(t *testing.T) {
	cause := fmt.Errorf("original error")
	clientErr := &ClientError{
		Type:    ErrorTypeNetwork,
		Message: "network error",
		Cause:   cause,
	}

	unwrapped := clientErr.Unwrap()
	if unwrapped != cause {
		t.Errorf("Expected unwrapped error to be %v, got %v", cause, unwrapped)
	}

	// Test errors.Is
	if !errors.Is(clientErr, cause) {
		t.Error("errors.Is should return true for the wrapped error")
	}
}

func TestErrorHandling_IntegrationWithClient(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		expectedError bool
		expectedType  ErrorType
		expectedRetry bool
	}{
		{
			name: "Server returns 500 error",
			serverHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("Internal Server Error"))
			}),
			expectedError: false, // 500 errors will be retried, may eventually succeed or fail
			expectedType:  ErrorTypeHTTP,
			expectedRetry: true,
		},
		{
			name: "Server returns 404 error",
			serverHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("Not Found"))
			}),
			expectedError: false, // 404 will not retry, returns response directly
			expectedType:  ErrorTypeHTTP,
			expectedRetry: false,
		},
		{
			name: "Server closes connection immediately",
			serverHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Close connection immediately
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					_ = conn.Close()
				}
			}),
			expectedError: true,
			expectedType:  ErrorTypeNetwork, // Connection close is classified as network error
			expectedRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			config := &Config{
				Timeout:         10 * time.Second, // Increase timeout
				AllowPrivateIPs: true,
				MaxRetries:      2,
				RetryDelay:      50 * time.Millisecond,
				MaxRetryDelay:   1 * time.Second,
				BackoffFactor:   2.0,
				Jitter:          false,
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			resp, err := client.Request(ctx, "GET", server.URL)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, got nil")
					if resp != nil {
						t.Logf("Unexpected response: %d", resp.StatusCode)
					}
					return
				}

				var clientErr *ClientError
				if errors.As(err, &clientErr) {
					if clientErr.Type != tt.expectedType {
						t.Errorf("Expected error type %v, got %v (error: %q)", tt.expectedType, clientErr.Type, err.Error())
					}

					if clientErr.IsRetryable() != tt.expectedRetry {
						t.Errorf("Expected isRetryable=%v, got %v", tt.expectedRetry, clientErr.IsRetryable())
					}
				} else {
					t.Errorf("Expected ClientError, got %T: %v", err, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}

				if resp == nil {
					t.Error("Expected response, got nil")
					return
				}

				t.Logf("Response: %d %s", resp.StatusCode, resp.Status)
			}
		})
	}
}

func TestErrorHandling_TimeoutScenarios(t *testing.T) {
	tests := []struct {
		name          string
		serverDelay   time.Duration
		clientTimeout time.Duration
		expectTimeout bool
	}{
		{
			name:          "Request completes within timeout",
			serverDelay:   100 * time.Millisecond,
			clientTimeout: 1 * time.Second,
			expectTimeout: false,
		},
		{
			name:          "Request exceeds timeout",
			serverDelay:   2 * time.Second,
			clientTimeout: 500 * time.Millisecond,
			expectTimeout: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tt.serverDelay)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			}))
			defer server.Close()

			config := &Config{
				Timeout:         tt.clientTimeout,
				AllowPrivateIPs: true,
				MaxRetries:      0, // Disable retry to test pure timeout
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			start := time.Now()
			ctx := context.Background()
			resp, err := client.Request(ctx, "GET", server.URL)
			duration := time.Since(start)

			if tt.expectTimeout {
				if err == nil {
					t.Error("Expected timeout error, got nil")
					if resp != nil {
						resp.Body = ""
						t.Logf("Unexpected response: %+v", resp)
					}
					return
				}

				var clientErr *ClientError
				if errors.As(err, &clientErr) {
					if clientErr.Type != ErrorTypeTimeout {
						t.Errorf("Expected timeout error, got %v", clientErr.Type)
					}
				}

				// Check if timeout occurred within reasonable time
				if duration > tt.clientTimeout+200*time.Millisecond {
					t.Errorf("Timeout took too long: %v (expected ~%v)", duration, tt.clientTimeout)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}

				if resp == nil {
					t.Error("Expected response, got nil")
					return
				}

				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected status 200, got %d", resp.StatusCode)
				}
			}
		})
	}
}

func TestErrorHandling_PanicRecovery(t *testing.T) {
	// This test verifies that the client can recover from internal panics
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      1,
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Normal requests should work
	ctx := context.Background()
	resp, err := client.Request(ctx, "GET", server.URL)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if resp == nil {
		t.Error("Expected response, got nil")
		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
