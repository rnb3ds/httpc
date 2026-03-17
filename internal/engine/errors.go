package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// containsFold reports whether substr is contained within s, case-insensitively.
// This is more efficient than strings.Contains(strings.ToLower(s), substr)
// because it performs ASCII-only case folding without allocating a new string.
func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}

	// Convert substr to lowercase once
	substrLower := strings.ToLower(substr)

	// Sliding window comparison
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c := s[i+j]
			// ASCII lowercase: 'A'-'Z' -> 'a'-'z'
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			if byte(c) != substrLower[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

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

	if e.Attempts > 0 {
		return fmt.Sprintf("%s (attempt %d)", baseMsg, e.Attempts)
	}

	return baseMsg
}

func sanitizeURL(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	if parsedURL.User == nil {
		return parsedURL.String()
	}

	_, hasPassword := parsedURL.User.Password()
	parsedURL.User = nil

	path := parsedURL.Path
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}
	if parsedURL.Fragment != "" {
		path += "#" + parsedURL.Fragment
	}

	if hasPassword {
		return fmt.Sprintf("%s://***:***@%s%s", parsedURL.Scheme, parsedURL.Host, path)
	}
	return fmt.Sprintf("%s://***@%s%s", parsedURL.Scheme, parsedURL.Host, path)
}

func (e *ClientError) Unwrap() error {
	return e.Cause
}

func (e *ClientError) IsRetryable() bool {
	switch e.Type {
	case ErrorTypeContextCanceled, ErrorTypeValidation, ErrorTypeTLS, ErrorTypeCertificate:
		return false
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeTransport, ErrorTypeDNS:
		return true
	case ErrorTypeResponseRead:
		if e.Cause != nil {
			var netErr *net.OpError
			if errors.As(e.Cause, &netErr) {
				return true
			}
			errMsg := e.Cause.Error()
			return strings.Contains(errMsg, "EOF") || strings.Contains(errMsg, "connection") || strings.Contains(errMsg, "timeout")
		}
		return true
	case ErrorTypeHTTP:
		msg := e.Message
		return strings.Contains(msg, "HTTP 429") || strings.Contains(msg, "HTTP 500") ||
			strings.Contains(msg, "HTTP 502") || strings.Contains(msg, "HTTP 503") ||
			strings.Contains(msg, "HTTP 504")
	default:
		return false
	}
}

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
	case ErrorTypeHTTP:
		return "HTTP_ERROR"
	default:
		return "UNKNOWN_ERROR"
	}
}

func ClassifyError(err error, reqURL, method string, attempts int) *ClientError {
	if err == nil {
		return nil
	}

	// Sanitize URL to prevent credential leakage in error storage
	sanitizedURL := sanitizeURL(reqURL)

	clientErr := &ClientError{
		Cause:    err,
		URL:      sanitizedURL,
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
		clientErr.Message = "request timeout"
		return clientErr
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		errMsg := urlErr.Error()
		if containsFold(errMsg, "http2") && containsFold(errMsg, "invalid") {
			clientErr.Type = ErrorTypeValidation
			clientErr.Message = "invalid HTTP/2 request header"
			return clientErr
		}
		if containsFold(errMsg, "parse") || containsFold(errMsg, "invalid url") || containsFold(errMsg, "missing protocol") {
			clientErr.Type = ErrorTypeValidation
			clientErr.Message = "URL validation failed"
			return clientErr
		}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		clientErr.Type = ErrorTypeDNS
		if dnsErr.IsTimeout {
			clientErr.Message = "DNS resolution timed out"
		} else {
			clientErr.Message = "DNS resolution failed"
		}
		return clientErr
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		clientErr.Type = ErrorTypeNetwork
		if opErr.Timeout() {
			clientErr.Message = "network operation timed out"
		} else {
			clientErr.Message = "network operation failed"
		}
		return clientErr
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			clientErr.Type = ErrorTypeTimeout
			clientErr.Message = "network timeout occurred"
		} else {
			clientErr.Type = ErrorTypeNetwork
			clientErr.Message = "network error occurred"
		}
		return clientErr
	}

	errMsg := err.Error()
	clientErr.Type = ErrorTypeUnknown
	clientErr.Message = errMsg

	switch {
	case containsFold(errMsg, "context canceled"):
		clientErr.Type = ErrorTypeContextCanceled
		clientErr.Message = "request context was canceled"
	case containsFold(errMsg, "context deadline exceeded"):
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "request context deadline exceeded"
	case containsFold(errMsg, "http2") && containsFold(errMsg, "invalid"):
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "invalid HTTP/2 request header"
	case containsFold(errMsg, "connection refused"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection refused by server"
	case containsFold(errMsg, "no such host"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "DNS resolution failed"
	case containsFold(errMsg, "connection reset"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection reset by peer"
	case containsFold(errMsg, "connection closed") || containsFold(errMsg, "peer closed"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection closed by peer"
	case containsFold(errMsg, "broken pipe"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "broken pipe"
	case containsFold(errMsg, "network unreachable"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "network unreachable"
	case containsFold(errMsg, "host unreachable"):
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "host unreachable"
	case (containsFold(errMsg, "tls") || containsFold(errMsg, "ssl")) && containsFold(errMsg, "handshake"):
		clientErr.Type = ErrorTypeTLS
		clientErr.Message = "TLS handshake error"
	case containsFold(errMsg, "certificate") || containsFold(errMsg, "x509"):
		clientErr.Type = ErrorTypeCertificate
		clientErr.Message = "certificate validation error"
	case containsFold(errMsg, "transport"):
		clientErr.Type = ErrorTypeTransport
		clientErr.Message = "HTTP transport error"
	case containsFold(errMsg, "protocol error"):
		clientErr.Type = ErrorTypeTransport
		clientErr.Message = "HTTP protocol error"
	case containsFold(errMsg, "failed to read response body"):
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "failed to read response body"
	case containsFold(errMsg, "unexpected eof"):
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "unexpected end of response"
	case containsFold(errMsg, "validation failed"):
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "request validation failed"
	case containsFold(errMsg, "invalid url") || containsFold(errMsg, "missing protocol scheme"):
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "URL validation failed"
	case containsFold(errMsg, "http 4") || containsFold(errMsg, "http 5"):
		clientErr.Type = ErrorTypeHTTP
	case containsFold(errMsg, "timeout") || containsFold(errMsg, "timed out"):
		if !containsFold(errMsg, "context") {
			clientErr.Type = ErrorTypeTimeout
			clientErr.Message = "operation timed out"
		}
	}

	return clientErr
}
