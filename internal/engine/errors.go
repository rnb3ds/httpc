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
		return "[invalid-url]"
	}

	if parsedURL.User == nil {
		return parsedURL.String()
	}

	_, hasPassword := parsedURL.User.Password()
	parsedURL.User = nil

	creds := "***"
	if hasPassword {
		creds = "***:***"
	}

	path := parsedURL.Path
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}
	if parsedURL.Fragment != "" {
		path += "#" + parsedURL.Fragment
	}

	return fmt.Sprintf("%s://%s@%s%s", parsedURL.Scheme, creds, parsedURL.Host, path)
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

	clientErr := &ClientError{
		Cause:    err,
		URL:      reqURL,
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
		errMsg := strings.ToLower(urlErr.Error())
		if strings.Contains(errMsg, "parse") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "missing protocol") {
			clientErr.Type = ErrorTypeValidation
			clientErr.Message = "URL validation failed"
			return clientErr
		}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		clientErr.Type = ErrorTypeNetwork
		if dnsErr.IsTimeout {
			clientErr.Message = "DNS resolution timed out"
		} else if dnsErr.IsTemporary {
			clientErr.Message = "temporary DNS resolution failure"
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

	errMsg := strings.ToLower(err.Error())
	clientErr.Type = ErrorTypeUnknown
	clientErr.Message = err.Error()

	if strings.Contains(errMsg, "context canceled") {
		clientErr.Type = ErrorTypeContextCanceled
		clientErr.Message = "request context was canceled"
	} else if strings.Contains(errMsg, "context deadline exceeded") {
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "request context deadline exceeded"
	} else if strings.Contains(errMsg, "connection refused") {
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection refused by server"
	} else if strings.Contains(errMsg, "no such host") {
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "DNS resolution failed"
	} else if strings.Contains(errMsg, "connection reset") {
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "connection reset by peer"
	} else if strings.Contains(errMsg, "broken pipe") {
		clientErr.Type = ErrorTypeNetwork
		clientErr.Message = "broken pipe"
	} else if strings.Contains(errMsg, "tls") || strings.Contains(errMsg, "handshake") {
		clientErr.Type = ErrorTypeTLS
		clientErr.Message = "TLS handshake error"
	} else if strings.Contains(errMsg, "certificate") || strings.Contains(errMsg, "x509") {
		clientErr.Type = ErrorTypeCertificate
		clientErr.Message = "certificate validation error"
	} else if strings.Contains(errMsg, "transport") || strings.Contains(errMsg, "round trip") {
		clientErr.Type = ErrorTypeTransport
		clientErr.Message = "HTTP transport error"
	} else if strings.Contains(errMsg, "failed to read response body") {
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "failed to read response body"
	} else if strings.Contains(errMsg, "eof") {
		clientErr.Type = ErrorTypeResponseRead
		clientErr.Message = "unexpected end of response"
	} else if strings.Contains(errMsg, "validation failed") {
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "request validation failed"
	} else if (strings.Contains(errMsg, "parse") && strings.Contains(errMsg, "url")) || strings.Contains(errMsg, "invalid url") || strings.Contains(errMsg, "missing protocol scheme") {
		clientErr.Type = ErrorTypeValidation
		clientErr.Message = "URL validation failed"
	} else if strings.Contains(errMsg, "http ") && (strings.Contains(errMsg, "http 4") || strings.Contains(errMsg, "http 5")) {
		clientErr.Type = ErrorTypeHTTP
	} else if strings.Contains(errMsg, "timeout") && !strings.Contains(errMsg, "context") {
		clientErr.Type = ErrorTypeTimeout
		clientErr.Message = "operation timed out"
	}

	return clientErr
}
