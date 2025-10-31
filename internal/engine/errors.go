package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeNetwork
	ErrorTypeTimeout
	ErrorTypeContextCanceled
	ErrorTypeResponseRead
	ErrorTypeTransport
	ErrorTypeRetryExhausted
	ErrorTypeTLS
	ErrorTypeCertificate
	ErrorTypeDNS
	ErrorTypeValidation
	ErrorTypeCircuitBreaker
	ErrorTypeHTTP
)

type ClientError struct {
	Type       ErrorType
	Message    string
	Cause      error
	URL        string
	Method     string
	Attempts   int
	StatusCode int    // HTTP status code if applicable
	Host       string // Host for circuit breaker errors
}

func (e *ClientError) Error() string {
	var baseMsg string
	if e.URL != "" && e.Method != "" {
		sanitizedURL := sanitizeURL(e.URL)
		baseMsg = fmt.Sprintf("%s %s: %s", e.Method, sanitizedURL, e.Message)
	} else {
		baseMsg = e.Message
	}

	// Add attempt information if available
	if e.Attempts > 0 {
		return fmt.Sprintf("%s (attempt %d)", baseMsg, e.Attempts)
	}

	return baseMsg
}

// sanitizeURL removes sensitive information (username, password) from URL
func sanitizeURL(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		// If parsing fails, return a generic placeholder
		return "[invalid-url]"
	}

	// Remove user info (username and password)
	if parsedURL.User != nil {
		// Check if there's a password
		_, hasPassword := parsedURL.User.Password()

		// Clear the user info completely
		parsedURL.User = nil

		// Reconstruct URL string manually to show redaction
		scheme := parsedURL.Scheme
		host := parsedURL.Host
		path := parsedURL.Path
		if parsedURL.RawQuery != "" {
			path += "?" + parsedURL.RawQuery
		}
		if parsedURL.Fragment != "" {
			path += "#" + parsedURL.Fragment
		}

		// Show appropriate redaction based on whether password exists
		if hasPassword {
			return fmt.Sprintf("%s://***:***@%s%s", scheme, host, path)
		} else {
			return fmt.Sprintf("%s://***@%s%s", scheme, host, path)
		}
	}

	return parsedURL.String()
}

func (e *ClientError) Unwrap() error {
	return e.Cause
}

func (e *ClientError) IsRetryable() bool {
	switch e.Type {
	case ErrorTypeContextCanceled, ErrorTypeValidation, ErrorTypeCircuitBreaker:
		return false
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeTransport, ErrorTypeDNS:
		return true
	case ErrorTypeTLS, ErrorTypeCertificate:
		return false // TLS/cert errors are usually not transient
	case ErrorTypeResponseRead:
		// Response read errors are retryable if they have a network-related cause
		// or if they appear to be transient (like EOF, connection issues)
		if e.Cause != nil {
			var netErr *net.OpError
			if errors.As(e.Cause, &netErr) {
				return true
			}
			// Check if the error message suggests a transient issue
			errMsg := e.Cause.Error()
			if strings.Contains(errMsg, "EOF") ||
				strings.Contains(errMsg, "connection") ||
				strings.Contains(errMsg, "timeout") {
				return true
			}
			// Other response read errors (like parse errors) are not retryable
			return false
		}
		// If no specific cause, assume it's a transient response read issue
		return true
	case ErrorTypeHTTP:
		// Parse status code from error message to determine retryability
		errMsg := e.Message
		if strings.Contains(errMsg, "HTTP 429") || // Too Many Requests
			strings.Contains(errMsg, "HTTP 500") || // Internal Server Error
			strings.Contains(errMsg, "HTTP 502") || // Bad Gateway
			strings.Contains(errMsg, "HTTP 503") || // Service Unavailable
			strings.Contains(errMsg, "HTTP 504") { // Gateway Timeout
			return true
		}
		return false // Other HTTP errors (4xx client errors) are not retryable
	default:
		return false
	}
}

// Code returns a string error code for programmatic handling
func (e *ClientError) Code() string {
	switch e.Type {
	case ErrorTypeNetwork:
		return "NETWORK_ERROR"
	case ErrorTypeTimeout:
		return "TIMEOUT"
	case ErrorTypeContextCanceled:
		return "CONTEXT_CANCELED"
	case ErrorTypeResponseRead:
		return "RESPONSE_READ_ERROR"
	case ErrorTypeTransport:
		return "TRANSPORT_ERROR"
	case ErrorTypeRetryExhausted:
		return "RETRY_EXHAUSTED"
	case ErrorTypeTLS:
		return "TLS_ERROR"
	case ErrorTypeCertificate:
		return "CERTIFICATE_ERROR"
	case ErrorTypeDNS:
		return "DNS_ERROR"
	case ErrorTypeValidation:
		return "VALIDATION_ERROR"
	case ErrorTypeCircuitBreaker:
		return "CIRCUIT_BREAKER_OPEN"
	case ErrorTypeHTTP:
		return "HTTP_ERROR"
	default:
		return "UNKNOWN_ERROR"
	}
}

func ClassifyError(err error, url, method string, attempts int) *ClientError {
	if err == nil {
		return nil
	}

	clientErr := &ClientError{
		Cause:    err,
		URL:      url,
		Method:   method,
		Attempts: attempts,
	}

	// Check for context errors first (most specific)
	if errors.Is(err, context.Canceled) {
		clientErr.Type = ErrorTypeContextCanceled
		clientErr.Message = "request was canceled"
		return clientErr
	}

	if errors.Is(err, context.DeadlineExceeded) {
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "request timeout"
		return clientErr
	}

	errMsg := err.Error()

	// Check for context-related errors in message
	if strings.Contains(errMsg, "context canceled") {
		clientErr.Type = ErrorTypeContextCanceled
		clientErr.Message = "request context was canceled"
		return clientErr
	}

	if strings.Contains(errMsg, "context deadline exceeded") {
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "request context deadline exceeded"
		return clientErr
	}

	// Check for URL validation errors
	if strings.Contains(errMsg, "missing protocol scheme") ||
		strings.Contains(errMsg, "invalid URL") ||
		(strings.Contains(errMsg, "parse") && strings.Contains(errMsg, "://")) {
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "URL validation failed"
		return clientErr
	}

	// Check for DNS errors - classify as network errors for consistency
	if dnsErr, ok := err.(*net.DNSError); ok {
		clientErr.Type = ErrorTypeNetwork // Classify DNS errors as network errors
		if dnsErr.IsTimeout {
			clientErr.Message = "DNS resolution timed out"
		} else if dnsErr.IsTemporary {
			clientErr.Message = "temporary DNS resolution failure"
		} else {
			clientErr.Message = "DNS resolution failed"
		}
		return clientErr
	}

	// Check for OpError (network operations) - keep as network errors
	if opErr, ok := err.(*net.OpError); ok {
		clientErr.Type = ErrorTypeNetwork // Always classify OpError as network error
		if opErr.Timeout() {
			clientErr.Message = "network operation timed out"
		} else if opErr.Temporary() {
			clientErr.Message = "temporary network operation failed"
		} else {
			clientErr.Message = "network operation failed"
		}
		return clientErr
	}

	// Check for general network errors
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			clientErr.Type = ErrorTypeTimeout
			clientErr.Message = "network timeout occurred"
		} else if netErr.Temporary() {
			clientErr.Type = ErrorTypeNetwork
			clientErr.Message = "temporary network error occurred"
		} else {
			clientErr.Type = ErrorTypeNetwork
			clientErr.Message = "network error occurred"
		}
		return clientErr
	}

	// Pattern-based classification for wrapped errors
	switch {
	case strings.Contains(errMsg, "HTTP ") && (strings.Contains(errMsg, "HTTP 4") || strings.Contains(errMsg, "HTTP 5")):
		clientErr.Type = ErrorTypeHTTP
		clientErr.Message = errMsg
	case strings.Contains(errMsg, "tls:") || strings.Contains(errMsg, "TLS handshake"):
		clientErr.Type = ErrorTypeTLS
		clientErr.Message = "TLS handshake error"
	case strings.Contains(errMsg, "certificate") || strings.Contains(errMsg, "x509"):
		clientErr.Type = ErrorTypeCertificate
		clientErr.Message = "certificate validation error"
	case strings.Contains(errMsg, "transport") || strings.Contains(errMsg, "round trip"):
		clientErr.Type = ErrorTypeTransport
		clientErr.Message = "HTTP transport error"
	case strings.Contains(errMsg, "failed to read response body"):
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "failed to read response body"
	case strings.Contains(errMsg, "connection refused"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection refused by server"
	case strings.Contains(errMsg, "no such host"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "DNS resolution failed"
	case strings.Contains(errMsg, "timeout") && !strings.Contains(errMsg, "context"):
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "operation timed out"
	case strings.Contains(errMsg, "validation failed"):
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "request validation failed"
	case strings.Contains(errMsg, "circuit breaker"):
		clientErr.Type = ErrorTypeCircuitBreaker
		clientErr.Message = "circuit breaker is open"
	case strings.Contains(errMsg, "panic during request execution"):
		clientErr.Type = ErrorTypeUnknown
		clientErr.Message = "internal error during request execution"
	case strings.Contains(errMsg, "connection reset by peer"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection reset by peer"
	case strings.Contains(errMsg, "broken pipe"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "broken pipe"
	case strings.Contains(errMsg, "EOF"):
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "unexpected end of response"
	default:
		clientErr.Type = ErrorTypeUnknown
		clientErr.Message = fmt.Sprintf("unknown error: %s", errMsg)
	}

	return clientErr
}

func isNetworkRelated(err error) bool {
	if err == nil {
		return false
	}

	if _, ok := err.(net.Error); ok {
		return true
	}

	if _, ok := err.(*net.OpError); ok {
		return true
	}

	if _, ok := err.(*net.DNSError); ok {
		return true
	}

	errMsg := strings.ToLower(err.Error())
	networkKeywords := []string{
		"connection", "network", "timeout", "refused", "reset", "broken pipe",
		"no route", "unreachable", "dns", "resolve",
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}
