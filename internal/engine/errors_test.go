package engine

import (
	"context"
	"errors"
	"net"
	"testing"
)

// ============================================================================
// ERROR CLASSIFICATION UNIT TESTS
// ============================================================================

func TestClientError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ClientError
		expected string
	}{
		{
			name: "With URL and Method",
			err: &ClientError{
				Type:    ErrorTypeNetwork,
				Message: "connection failed",
				URL:     "https://example.com",
				Method:  "GET",
			},
			expected: "GET https://example.com: connection failed",
		},
		{
			name: "Without URL and Method",
			err: &ClientError{
				Type:    ErrorTypeTimeout,
				Message: "timeout occurred",
			},
			expected: "timeout occurred",
		},
		{
			name: "With URL containing credentials",
			err: &ClientError{
				Type:    ErrorTypeNetwork,
				Message: "connection failed",
				URL:     "https://user:password@example.com/path",
				Method:  "GET",
			},
			expected: "GET https://***:***@example.com/path: connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL without credentials",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "URL with username and password",
			input:    "https://user:password@example.com/path",
			expected: "https://***:***@example.com/path",
		},
		{
			name:     "URL with only username",
			input:    "https://user@example.com/path",
			expected: "https://***@example.com/path",
		},
		{
			name:     "URL with query parameters",
			input:    "https://user:pass@example.com/path?key=value",
			expected: "https://***:***@example.com/path?key=value",
		},
		{
			name:     "URL with fragment",
			input:    "https://user:pass@example.com/path#section",
			expected: "https://***:***@example.com/path#section",
		},
		{
			name:     "URL without scheme (relative path)",
			input:    "not a valid url",
			expected: "not%20a%20valid%20url",
		},
		{
			name:     "HTTP URL with credentials",
			input:    "http://admin:secret@localhost:8080/api",
			expected: "http://***:***@localhost:8080/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClientError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &ClientError{
		Type:    ErrorTypeNetwork,
		Message: "network error",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Expected unwrapped error to be %v, got %v", cause, unwrapped)
	}
}

func TestClientError_IsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		cause     error
		expected  bool
	}{
		{
			name:      "Network error is retryable",
			errorType: ErrorTypeNetwork,
			expected:  true,
		},
		{
			name:      "Timeout error is retryable",
			errorType: ErrorTypeTimeout,
			expected:  true,
		},
		{
			name:      "Transport error is retryable",
			errorType: ErrorTypeTransport,
			expected:  true,
		},
		{
			name:      "Context canceled is not retryable",
			errorType: ErrorTypeContextCanceled,
			expected:  false,
		},
		{
			name:      "Unknown error is not retryable",
			errorType: ErrorTypeUnknown,
			expected:  false,
		},
		{
			name:      "Response read with network cause is retryable",
			errorType: ErrorTypeResponseRead,
			cause:     &net.OpError{Op: "read"},
			expected:  true,
		},
		{
			name:      "Response read without network cause is not retryable",
			errorType: ErrorTypeResponseRead,
			cause:     errors.New("parse error"),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ClientError{
				Type:  tt.errorType,
				Cause: tt.cause,
			}

			result := err.IsRetryable()
			if result != tt.expected {
				t.Errorf("Expected IsRetryable() = %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestClassifyError_ContextErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		expectedMsg  string
	}{
		{
			name:         "Context canceled",
			err:          context.Canceled,
			expectedType: ErrorTypeContextCanceled,
			expectedMsg:  "request was canceled",
		},
		{
			name:         "Context deadline exceeded",
			err:          context.DeadlineExceeded,
			expectedType: ErrorTypeTimeout,
			expectedMsg:  "request timeout",
		},
		{
			name:         "Context canceled in message",
			err:          errors.New("context canceled"),
			expectedType: ErrorTypeContextCanceled,
			expectedMsg:  "request context was canceled",
		},
		{
			name:         "Context deadline in message",
			err:          errors.New("context deadline exceeded"),
			expectedType: ErrorTypeTimeout,
			expectedMsg:  "request context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, "https://example.com", "GET", 1)

			if result.Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, result.Type)
			}

			if result.Message != tt.expectedMsg {
				t.Errorf("Expected message %q, got %q", tt.expectedMsg, result.Message)
			}

			if result.URL != "https://example.com" {
				t.Errorf("Expected URL to be set")
			}

			if result.Method != "GET" {
				t.Errorf("Expected Method to be set")
			}

			if result.Attempts != 1 {
				t.Errorf("Expected Attempts to be 1, got %d", result.Attempts)
			}
		})
	}
}

func TestClassifyError_NetworkErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
	}{
		{
			name:         "OpError",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			expectedType: ErrorTypeNetwork,
		},
		{
			name:         "DNSError",
			err:          &net.DNSError{Name: "example.com", Err: "no such host"},
			expectedType: ErrorTypeNetwork,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, "", "", 0)

			if result.Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, result.Type)
			}
		})
	}
}

func TestClassifyError_MessagePatterns(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		expectedType ErrorType
	}{
		{
			name:         "Transport error",
			errMsg:       "transport connection broken",
			expectedType: ErrorTypeTransport,
		},
		{
			name:         "Response read error",
			errMsg:       "failed to read response body",
			expectedType: ErrorTypeResponseRead,
		},
		{
			name:         "Connection refused",
			errMsg:       "connection refused",
			expectedType: ErrorTypeNetwork,
		},
		{
			name:         "No such host",
			errMsg:       "no such host",
			expectedType: ErrorTypeNetwork,
		},
		{
			name:         "Timeout",
			errMsg:       "timeout waiting for response",
			expectedType: ErrorTypeTimeout,
		},
		{
			name:         "Unknown error",
			errMsg:       "something went wrong",
			expectedType: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			result := ClassifyError(err, "", "", 0)

			if result.Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, result.Type)
			}
		})
	}
}

func TestClassifyError_NilError(t *testing.T) {
	result := ClassifyError(nil, "", "", 0)

	if result != nil {
		t.Errorf("Expected nil for nil error, got %v", result)
	}
}

func TestIsNetworkRelated(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "OpError",
			err:      &net.OpError{Op: "dial"},
			expected: true,
		},
		{
			name:     "DNSError",
			err:      &net.DNSError{Name: "example.com"},
			expected: true,
		},
		{
			name:     "Connection keyword",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "Network keyword",
			err:      errors.New("network unreachable"),
			expected: true,
		},
		{
			name:     "Timeout keyword",
			err:      errors.New("operation timeout"),
			expected: true,
		},
		{
			name:     "DNS keyword",
			err:      errors.New("dns lookup failed"),
			expected: true,
		},
		{
			name:     "Non-network error",
			err:      errors.New("invalid JSON"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkRelated(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// mockNetError implements net.Error for testing
type mockNetError struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

func TestClassifyError_NetError(t *testing.T) {
	tests := []struct {
		name         string
		err          net.Error
		expectedType ErrorType
		expectedMsg  string
	}{
		{
			name: "Timeout error",
			err: &mockNetError{
				timeout: true,
				msg:     "i/o timeout",
			},
			expectedType: ErrorTypeTimeout,
			expectedMsg:  "network timeout occurred",
		},
		{
			name: "Temporary error",
			err: &mockNetError{
				temporary: true,
				msg:       "temporary failure",
			},
			expectedType: ErrorTypeNetwork,
			expectedMsg:  "temporary network error occurred",
		},
		{
			name: "Permanent network error",
			err: &mockNetError{
				timeout:   false,
				temporary: false,
				msg:       "permanent failure",
			},
			expectedType: ErrorTypeNetwork,
			expectedMsg:  "network error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, "", "", 0)

			if result.Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, result.Type)
			}

			if result.Message != tt.expectedMsg {
				t.Errorf("Expected message %q, got %q", tt.expectedMsg, result.Message)
			}
		})
	}
}

func TestErrorType_String(t *testing.T) {
	// Test that ErrorType values are distinct
	types := []ErrorType{
		ErrorTypeUnknown,
		ErrorTypeNetwork,
		ErrorTypeTimeout,
		ErrorTypeContextCanceled,
		ErrorTypeResponseRead,
		ErrorTypeTransport,
		ErrorTypeRetryExhausted,
	}

	seen := make(map[ErrorType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("Duplicate ErrorType value: %v", et)
		}
		seen[et] = true
	}
}
