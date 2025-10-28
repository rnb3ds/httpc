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
	if e.URL != "" && e.Method != "" {
		sanitizedURL := sanitizeURL(e.URL)
		return fmt.Sprintf("%s %s: %s", e.Method, sanitizedURL, e.Message)
	}
	return e.Message
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
		return fmt.Sprintf("%s://***:***@%s%s", scheme, host, path)
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
		return isNetworkRelated(e.Cause)
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

	if errors.Is(err, context.Canceled) {
		clientErr.Type = ErrorTypeContextCanceled
		clientErr.Message = "request was canceled"
		return clientErr
	}

	if errors.Is(err, context.DeadlineExceeded) {
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "request timed out"
		return clientErr
	}

	errMsg := err.Error()

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

	if _, ok := err.(*net.OpError); ok {
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "network operation failed"
		return clientErr
	}

	if _, ok := err.(*net.DNSError); ok {
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "DNS resolution failed"
		return clientErr
	}

	switch {
	case strings.Contains(errMsg, "tls:") || strings.Contains(errMsg, "TLS"):
		clientErr.Type = ErrorTypeTLS
		clientErr.Message = "TLS handshake error"
	case strings.Contains(errMsg, "certificate") || strings.Contains(errMsg, "x509"):
		clientErr.Type = ErrorTypeCertificate
		clientErr.Message = "certificate validation error"
	case strings.Contains(errMsg, "transport"):
		clientErr.Type = ErrorTypeTransport
		clientErr.Message = "HTTP transport error"
	case strings.Contains(errMsg, "failed to read response body"):
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "failed to read response body"
	case strings.Contains(errMsg, "connection refused"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection refused by server"
	case strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "dns"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "DNS resolution failed"
	case strings.Contains(errMsg, "timeout"):
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "operation timed out"
	case strings.Contains(errMsg, "validation"):
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "request validation failed"
	case strings.Contains(errMsg, "circuit breaker"):
		clientErr.Type = ErrorTypeCircuitBreaker
		clientErr.Message = "circuit breaker is open"
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
