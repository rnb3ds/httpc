package engine

import (
	"context"
	"net/http"
	"time"
)

// RequestMutator provides read-write access to request data for middleware.
// Middleware can inspect and modify request properties before the request is sent.
type RequestMutator interface {
	// Read accessors
	Method() string
	URL() string
	Headers() map[string]string
	QueryParams() map[string]any
	Body() any
	Timeout() time.Duration
	MaxRetries() int
	Context() context.Context
	Cookies() []http.Cookie
	FollowRedirects() *bool
	MaxRedirects() *int

	// Write mutators
	SetMethod(string)
	SetURL(string)
	SetHeader(key, value string)
	SetQueryParams(map[string]any)
	SetBody(any)
	SetTimeout(time.Duration)
	SetMaxRetries(int)
	SetContext(context.Context)
	SetCookies([]http.Cookie)
	SetFollowRedirects(*bool)
	SetMaxRedirects(*int)
}

// ResponseAccessor provides read-only access to response data.
// Middleware can inspect response properties after the request completes.
type ResponseAccessor interface {
	StatusCode() int
	Status() string
	Headers() http.Header
	Body() string
	RawBody() []byte
	ContentLength() int64
	Duration() time.Duration
	Attempts() int
	Cookies() []*http.Cookie
	RedirectChain() []string
	RedirectCount() int
}

// ResponseMutator provides read-write access to response data.
// Middleware can modify response properties after the request completes.
// This is useful for:
//   - Response caching middleware
//   - Response transformation (e.g., JSON pretty-printing)
//   - Content encoding/decoding
//   - Response filtering
type ResponseMutator interface {
	ResponseAccessor
	SetStatusCode(int)
	SetHeader(key string, values ...string)
	SetBody(string)
	SetDuration(time.Duration)
}

// Handler processes an HTTP request and returns a response.
// This is the core function signature for request processing in the middleware chain.
type Handler func(ctx context.Context, req RequestMutator) (ResponseAccessor, error)

// MutableHandler processes an HTTP request and returns a mutable response.
// This allows middleware to modify the response (e.g., for caching, transformation).
// Use MutableHandler when you need to modify response content or headers.
type MutableHandler func(ctx context.Context, req RequestMutator) (ResponseMutator, error)

// MiddlewareFunc wraps a Handler with additional functionality.
// Middleware can inspect/modify requests, handle responses, add logging, etc.
type MiddlewareFunc func(Handler) Handler

// MutableMiddlewareFunc wraps a MutableHandler with additional functionality.
// This variant allows middleware to modify responses in addition to requests.
type MutableMiddlewareFunc func(MutableHandler) MutableHandler

// AsMutable converts a Handler to a MutableHandler.
// This is useful when you need to work with mutable responses in middleware.
func AsMutable(h Handler) MutableHandler {
	return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		resp, err := h(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, nil
		}
		// If the response already implements ResponseMutator, return it directly
		if mut, ok := resp.(ResponseMutator); ok {
			return mut, nil
		}
		// Otherwise, we can't mutate it - this shouldn't happen with our internal types
		// Return an error to indicate the incompatibility
		return nil, &MutableResponseError{Response: resp}
	}
}

// AsHandler converts a MutableHandler back to a Handler.
// This allows mutable middleware to be used in the standard middleware chain.
func AsHandler(mh MutableHandler) Handler {
	return func(ctx context.Context, req RequestMutator) (ResponseAccessor, error) {
		return mh(ctx, req)
	}
}

// MutableResponseError indicates that a response could not be converted to mutable form.
type MutableResponseError struct {
	Response ResponseAccessor
}

func (e *MutableResponseError) Error() string {
	return "response does not implement ResponseMutator"
}

// RetryPolicy defines the interface for retry behavior.
// Implementations can provide custom retry strategies.
type RetryPolicy interface {
	// ShouldRetry determines if a request should be retried based on the
	// response, error, and current attempt number.
	ShouldRetry(resp *Response, err error, attempt int) bool

	// GetDelay returns the delay duration before the next retry attempt.
	GetDelay(attempt int) time.Duration

	// MaxRetries returns the maximum number of retry attempts.
	MaxRetries() int
}

// RequestValidator defines the interface for request validation.
// Implementations can provide different validation strategies.
type RequestValidator interface {
	// ValidateRequest validates the given request and returns an error
	// if the request is invalid.
	ValidateRequest(req *Request) error
}
