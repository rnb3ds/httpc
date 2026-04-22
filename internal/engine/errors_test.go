package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/validation"
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
		{
			name: "With cause",
			err: &ClientError{
				Type:    ErrorTypeNetwork,
				Message: "network operation failed",
				URL:     "https://example.com",
				Method:  "GET",
				Cause:   errors.New("dial tcp 192.168.1.1:443: connection refused"),
			},
			expected: "GET https://example.com: network operation failed: dial tcp 192.168.1.1:443: connection refused",
		},
		{
			name: "With cause and attempts",
			err: &ClientError{
				Type:     ErrorTypeNetwork,
				Message:  "network operation failed",
				URL:      "https://example.com",
				Method:   "GET",
				Cause:    &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")},
				Attempts: 3,
			},
			expected: "GET https://example.com: network operation failed: dial tcp: refused (attempt 3)",
		},
		{
			name: "Without URL and Method but with cause",
			err: &ClientError{
				Type:    ErrorTypeTimeout,
				Message: "timeout occurred",
				Cause:   context.DeadlineExceeded,
			},
			expected: "timeout occurred: context deadline exceeded",
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
			result := validation.SanitizeURL(tt.input)
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
			name:      "Network error with timeout is retryable",
			errorType: ErrorTypeNetwork,
			cause:     &net.OpError{Op: "dial", Net: "tcp", Err: &mockNetError{timeout: true}},
			expected:  true,
		},
		{
			name:      "Network error without cause is not retryable",
			errorType: ErrorTypeNetwork,
			cause:     nil,
			expected:  false,
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
			expectedType: ErrorTypeDNS,
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
			expectedType: ErrorTypeDNS,
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
			expectedMsg:  "network error occurred",
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

func TestClientError_Code(t *testing.T) {
	tests := []struct {
		name       string
		errorType  ErrorType
		expectCode string
	}{
		{"Network", ErrorTypeNetwork, "NETWORK_ERROR"},
		{"Timeout", ErrorTypeTimeout, "TIMEOUT"},
		{"ContextCanceled", ErrorTypeContextCanceled, "CONTEXT_CANCELED"},
		{"ResponseRead", ErrorTypeResponseRead, "RESPONSE_READ_ERROR"},
		{"Transport", ErrorTypeTransport, "TRANSPORT_ERROR"},
		{"RetryExhausted", ErrorTypeRetryExhausted, "RETRY_EXHAUSTED"},
		{"TLS", ErrorTypeTLS, "TLS_ERROR"},
		{"Certificate", ErrorTypeCertificate, "CERTIFICATE_ERROR"},
		{"DNS", ErrorTypeDNS, "DNS_ERROR"},
		{"Validation", ErrorTypeValidation, "VALIDATION_ERROR"},
		{"HTTP", ErrorTypeHTTP, "HTTP_ERROR"},
		{"Unknown", ErrorTypeUnknown, "UNKNOWN_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ClientError{Type: tt.errorType}
			if got := err.Code(); got != tt.expectCode {
				t.Errorf("Code() = %q, want %q", got, tt.expectCode)
			}
		})
	}
}

func TestContainsFold(t *testing.T) {
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"Hello World", "world", true},
		{"hello world", "WORLD", true},
		{"Hello", "ello", true},
		{"Hello", "xyz", false},
		{"", "", true},
		{"abc", "", true},
		{"", "a", false},
		{"ab", "abc", false},
		{"HTTP/2 invalid", "INVALID", true},
		{"connection refused", "REFUSED", true},
	}

	for _, tt := range tests {
		got := containsFold(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestClassifyError_AdditionalPatterns(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		expectedType ErrorType
	}{
		{"ConnectionReset", "connection reset by peer", ErrorTypeNetwork},
		{"ConnectionClosed", "connection closed by peer", ErrorTypeNetwork},
		{"PeerClosed", "peer closed connection", ErrorTypeNetwork},
		{"BrokenPipe", "broken pipe error", ErrorTypeNetwork},
		{"NetworkUnreachable", "network unreachable", ErrorTypeNetwork},
		{"HostUnreachable", "host unreachable", ErrorTypeNetwork},
		{"TLSHandshake", "TLS handshake failure", ErrorTypeTLS},
		{"SSLHandshake", "SSL handshake error", ErrorTypeTLS},
		{"Certificate", "certificate verify failed", ErrorTypeCertificate},
		{"X509", "x509 certificate error", ErrorTypeCertificate},
		{"Transport", "transport error occurred", ErrorTypeTransport},
		{"ProtocolError", "protocol error in response", ErrorTypeTransport},
		{"ResponseReadBody", "failed to read response body", ErrorTypeResponseRead},
		{"UnexpectedEOF", "unexpected eof in response", ErrorTypeResponseRead},
		{"ValidationFailed", "validation failed for input", ErrorTypeValidation},
		{"InvalidURL", "invalid url format", ErrorTypeValidation},
		{"MissingProtocol", "missing protocol scheme", ErrorTypeValidation},
		{"HTTP4xx", "HTTP 403 forbidden", ErrorTypeHTTP},
		{"HTTP5xx", "HTTP 503 unavailable", ErrorTypeHTTP},
		{"Timeout", "connection timeout", ErrorTypeTimeout},
		{"TimedOut", "request timed out", ErrorTypeTimeout},
		{"ContextCanceled", "request context canceled", ErrorTypeContextCanceled},
		{"ContextDeadlineExceeded", "context deadline exceeded", ErrorTypeTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(errors.New(tt.errMsg), "https://example.com", "GET", 1)
			if result.Type != tt.expectedType {
				t.Errorf("ClassifyError(%q) type = %v, want %v", tt.errMsg, result.Type, tt.expectedType)
			}
		})
	}
}

func TestClientError_IsRetryable_Additional(t *testing.T) {
	tests := []struct {
		name      string
		err       *ClientError
		wantRetry bool
	}{
		{
			name:      "DNS temp error is retryable",
			err:       &ClientError{Type: ErrorTypeDNS, Cause: &net.DNSError{IsTemporary: true}},
			wantRetry: true,
		},
		{
			name:      "DNS timeout is retryable",
			err:       &ClientError{Type: ErrorTypeDNS, Cause: &net.DNSError{IsTimeout: true}},
			wantRetry: true,
		},
		{
			name:      "DNS permanent is not retryable",
			err:       &ClientError{Type: ErrorTypeDNS, Cause: &net.DNSError{}},
			wantRetry: false,
		},
		{
			name:      "HTTP 429 is retryable",
			err:       &ClientError{Type: ErrorTypeHTTP, StatusCode: 429},
			wantRetry: true,
		},
		{
			name:      "HTTP 500 is retryable",
			err:       &ClientError{Type: ErrorTypeHTTP, StatusCode: 500},
			wantRetry: true,
		},
		{
			name:      "HTTP 503 is retryable",
			err:       &ClientError{Type: ErrorTypeHTTP, StatusCode: 503},
			wantRetry: true,
		},
		{
			name:      "HTTP 404 is not retryable",
			err:       &ClientError{Type: ErrorTypeHTTP, StatusCode: 404},
			wantRetry: false,
		},
		{
			name:      "HTTP status 0 with retryable message",
			err:       &ClientError{Type: ErrorTypeHTTP, StatusCode: 0, Message: "HTTP 503 unavailable"},
			wantRetry: true,
		},
		{
			name:      "TLS error is not retryable",
			err:       &ClientError{Type: ErrorTypeTLS},
			wantRetry: false,
		},
		{
			name:      "Certificate error is not retryable",
			err:       &ClientError{Type: ErrorTypeCertificate},
			wantRetry: false,
		},
		{
			name:      "Validation error is not retryable",
			err:       &ClientError{Type: ErrorTypeValidation},
			wantRetry: false,
		},
		{
			name:      "Network error with connection reset message",
			err:       &ClientError{Type: ErrorTypeNetwork, Cause: errors.New("connection reset by peer")},
			wantRetry: true,
		},
		{
			name:      "Network error with EOF message",
			err:       &ClientError{Type: ErrorTypeNetwork, Cause: errors.New("unexpected EOF")},
			wantRetry: true,
		},
		{
			name:      "Response read nil cause is retryable",
			err:       &ClientError{Type: ErrorTypeResponseRead, Cause: nil},
			wantRetry: true,
		},
		{
			name:      "Context canceled via cause is not retryable",
			err:       &ClientError{Type: ErrorTypeNetwork, Cause: context.Canceled},
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.IsRetryable()
			if got != tt.wantRetry {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.wantRetry)
			}
		})
	}
}

func TestClientError_ErrorWithAttempts(t *testing.T) {
	tests := []struct {
		name     string
		err      *ClientError
		contains string
	}{
		{
			name:     "With attempts",
			err:      &ClientError{Type: ErrorTypeNetwork, Message: "failed", Method: "GET", URL: "https://example.com", Attempts: 3},
			contains: "attempt 3",
		},
		{
			name:     "Zero attempts no suffix",
			err:      &ClientError{Type: ErrorTypeNetwork, Message: "failed", Method: "GET", URL: "https://example.com"},
			contains: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !containsFold(got, tt.contains) {
				t.Errorf("Error() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

// TestErrorHandling_IntegrationWithClient validates error handling with a real client and server.
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
						t.Logf("Unexpected response: %d", resp.StatusCode())
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

				t.Logf("Response: %d %s", resp.StatusCode(), resp.Status())
			}
		})
	}
}

// TestClassifyError_UrlErrorWrapping verifies that errors wrapped in *url.Error
// are classified based on the actual underlying error, not the outer *url.Error.
// *url.Error implements net.Error, which would cause misclassification as
// "network error occurred" without proper unwrapping.
func TestClassifyError_UrlErrorWrapping(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		expectedMsg  string
	}{
		{
			name:         "url.Error wrapping OpError connection refused",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}},
			expectedType: ErrorTypeNetwork,
			expectedMsg:  "network operation failed",
		},
		{
			name:         "url.Error wrapping OpError timeout",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: &net.OpError{Op: "dial", Net: "tcp", Err: &mockNetError{timeout: true}}},
			expectedType: ErrorTypeNetwork,
			expectedMsg:  "network operation timed out",
		},
		{
			name:         "url.Error wrapping DNSError",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: &net.DNSError{Name: "example.com", Err: "no such host"}},
			expectedType: ErrorTypeDNS,
			expectedMsg:  "DNS resolution failed",
		},
		{
			name:         "url.Error wrapping TLS handshake error",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("tls: handshake failure")},
			expectedType: ErrorTypeTLS,
			expectedMsg:  "TLS handshake error",
		},
		{
			name:         "url.Error wrapping certificate error",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("x509: certificate signed by unknown authority")},
			expectedType: ErrorTypeCertificate,
			expectedMsg:  "certificate validation error",
		},
		{
			name:         "url.Error wrapping fmt.Errorf wrapping OpError",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: fmt.Errorf("connection failed: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")})},
			expectedType: ErrorTypeNetwork,
			expectedMsg:  "network operation failed",
		},
		{
			name:         "url.Error with HTTP/2 invalid header",
			err:          &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("http2: invalid header field")},
			expectedType: ErrorTypeValidation,
			expectedMsg:  "invalid HTTP/2 request header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, "https://example.com", "GET", 1)
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, result.Type)
			}
			if result.Message != tt.expectedMsg {
				t.Errorf("Expected message %q, got %q", tt.expectedMsg, result.Message)
			}
		})
	}
}

// TestClassifyError_DoubleClassificationPrevention verifies that passing an
// already-classified *ClientError into ClassifyError returns it directly
// instead of creating a new wrapper.
func TestClassifyError_DoubleClassificationPrevention(t *testing.T) {
	originalCause := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	original := &ClientError{
		Type:     ErrorTypeNetwork,
		Message:  "network operation failed",
		Cause:    originalCause,
		URL:      "https://example.com",
		Method:   "GET",
		Attempts: 1,
	}

	result := ClassifyError(original, "https://example.com/api", "POST", 3)

	if result != original {
		t.Fatal("Expected same *ClientError instance to be returned")
	}
	if result.Type != ErrorTypeNetwork {
		t.Errorf("Expected type %v, got %v", ErrorTypeNetwork, result.Type)
	}
	if result.Message != "network operation failed" {
		t.Errorf("Expected original message preserved, got %q", result.Message)
	}
	if result.URL != "https://example.com/api" {
		t.Errorf("Expected URL updated, got %q", result.URL)
	}
	if result.Method != "POST" {
		t.Errorf("Expected Method updated, got %q", result.Method)
	}
	if result.Attempts != 3 {
		t.Errorf("Expected Attempts updated to 3, got %d", result.Attempts)
	}
	if result.Cause != originalCause {
		t.Error("Expected Cause preserved")
	}
}

// TestClassifyError_UrlErrorWrappingRetryability verifies that errors classified
// through *url.Error unwrapping have correct retryability.
func TestClassifyError_UrlErrorWrappingRetryability(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantRetry bool
	}{
		{
			name:      "url.Error wrapping OpError with reset is retryable",
			err:       &url.Error{Op: "Get", URL: "https://example.com", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection reset by peer")}},
			wantRetry: true,
		},
		{
			name:      "url.Error wrapping OpError with refused is not retryable",
			err:       &url.Error{Op: "Get", URL: "https://example.com", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}},
			wantRetry: false,
		},
		{
			name:      "url.Error wrapping TLS error is not retryable",
			err:       &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("tls: handshake failure")},
			wantRetry: false,
		},
		{
			name:      "url.Error wrapping certificate error is not retryable",
			err:       &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("x509: certificate verify failed")},
			wantRetry: false,
		},
		{
			name:      "url.Error wrapping DNS timeout is retryable",
			err:       &url.Error{Op: "Get", URL: "https://example.com", Err: &net.DNSError{Name: "example.com", IsTimeout: true}},
			wantRetry: true,
		},
		{
			name:      "url.Error wrapping DNS permanent is not retryable",
			err:       &url.Error{Op: "Get", URL: "https://example.com", Err: &net.DNSError{Name: "example.com"}},
			wantRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, "https://example.com", "GET", 1)
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.IsRetryable() != tt.wantRetry {
				t.Errorf("IsRetryable() = %v, want %v (type=%v, msg=%q)", result.IsRetryable(), tt.wantRetry, result.Type, result.Message)
			}
		})
	}
}

// TestErrorHandling_TimeoutScenarios validates timeout behavior under various conditions.
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

				if resp.StatusCode() != http.StatusOK {
					t.Errorf("Expected status 200, got %d", resp.StatusCode())
				}
			}
		})
	}
}
