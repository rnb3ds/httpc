// Package types provides shared type definitions used across the httpc library.
// This package eliminates interface duplication between public and internal layers,
// enabling compile-time type checking without runtime type assertions.
package types

import (
	"context"
	"net/http"
	"time"
)

// The request and response interfaces are split along the read/write boundary
// so that consumers can declare the narrow contract they actually need rather
// than the full mutator. The exemplar is RetryPolicy.ShouldRetry, which accepts
// a ResponseReader (read-only) even though the engine hands around
// ResponseMutator values — a policy that only inspects a response never needs
// its setters, and expressing that in the signature makes the intent explicit.
//
// RequestMutator and ResponseMutator remain the aggregate interfaces used by
// the middleware chain (Handler) and by RequestOption, where a value must be
// both inspected and modified.

// RequestReader provides read-only access to request data.
// Use it when a consumer only inspects request properties without modifying
// them (mirroring how ResponseReader is used by RetryPolicy).
type RequestReader interface {
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
	StreamBody() bool
}

// RequestWriter provides write-only access to request data.
// Use it when a consumer only modifies request properties without reading them.
type RequestWriter interface {
	SetMethod(string)
	SetURL(string)
	SetHeaders(map[string]string)
	SetHeader(key, value string)
	SetQueryParams(map[string]any)
	SetBody(any)
	SetTimeout(time.Duration)
	SetMaxRetries(int)
	SetContext(context.Context)
	SetCookies([]http.Cookie)
	SetFollowRedirects(*bool)
	SetMaxRedirects(*int)
	SetStreamBody(bool)
}

// RequestMutator provides read-write access to request data for middleware and
// request options. It embeds RequestReader and RequestWriter for full access to
// request properties. Middleware can inspect and modify request properties
// before the request is sent.
//
// LIFETIME CONTRACT: the maps returned by Headers() and QueryParams() are
// pooled internal storage, valid only for the duration of the request. Do not
// retain references beyond the middleware call (e.g. in async logging or
// audit pipelines) — after the request completes the maps are cleared and
// recycled for unrelated requests. Copy the data if you need to keep it.
//
// OWNERSHIP CONTRACT (setter side): SetHeaders and SetQueryParams COPY the
// supplied maps into engine-owned pooled storage. Passing a caller-owned or
// shared map is safe — the engine never retains the argument, and later
// mutations to it do not affect the in-flight request.
type RequestMutator interface {
	RequestReader
	RequestWriter
}

// ResponseReader provides read-only access to response data for middleware.
// Use this interface when middleware only needs to inspect response properties
// without modifying them. It is also the interface consumed by RetryPolicy.
type ResponseReader interface {
	StatusCode() int
	Status() string
	Proto() string
	Headers() http.Header
	Body() string
	RawBody() []byte
	ContentLength() int64
	Duration() time.Duration
	Attempts() int
	Cookies() []*http.Cookie
	RedirectChain() []string
	RedirectCount() int
	RequestHeaders() http.Header
	RequestURL() string
	RequestMethod() string
}

// ResponseWriter provides write-only access to response data.
// Use it when a consumer only modifies response properties without reading them.
type ResponseWriter interface {
	SetStatusCode(int)
	SetStatus(string)
	SetProto(string)
	SetHeaders(http.Header)
	SetBody(string)
	SetRawBody([]byte)
	SetContentLength(int64)
	SetDuration(time.Duration)
	SetAttempts(int)
	SetCookies([]*http.Cookie)
	SetRedirectChain([]string)
	SetRedirectCount(int)
	SetRequestHeaders(http.Header)
	SetRequestURL(string)
	SetRequestMethod(string)
	SetHeader(key string, values ...string)
}

// ResponseMutator provides read-write access to response data for middleware.
// It embeds ResponseReader and ResponseWriter for full access to response
// properties. Middleware can inspect and modify response properties after the
// request completes. This is useful for:
//   - Response caching middleware
//   - Response transformation (e.g., JSON pretty-printing)
//   - Content encoding/decoding
//   - Response filtering
type ResponseMutator interface {
	ResponseReader
	ResponseWriter
}

// Handler processes an HTTP request and returns a response.
// This is the core function signature for request processing in the middleware chain.
type Handler func(ctx context.Context, req RequestMutator) (ResponseMutator, error)

// MiddlewareFunc wraps a Handler with additional functionality.
// Middleware can inspect/modify requests, handle responses, add logging, etc.
type MiddlewareFunc func(Handler) Handler

// RetryPolicy defines the interface for custom retry behavior.
// Implementations can provide custom retry strategies beyond the default
// exponential backoff with jitter.
//
// Example implementation:
//
//	type CustomRetryPolicy struct {
//	    maxRetries int
//	}
//
//	func (p *CustomRetryPolicy) ShouldRetry(resp ResponseReader, err error, attempt int) bool {
//	    if err != nil {
//	        return true // Retry on errors
//	    }
//	    if resp.StatusCode() >= 500 && resp.StatusCode() < 600 {
//	        return attempt < p.maxRetries
//	    }
//	    return false
//	}
//
//	func (p *CustomRetryPolicy) GetDelay(attempt int) time.Duration {
//	    return time.Second * time.Duration(attempt+1)
//	}
//
//	func (p *CustomRetryPolicy) MaxRetries() int {
//	    return p.maxRetries
//	}
type RetryPolicy interface {
	// ShouldRetry determines if a request should be retried based on the
	// response, error, and current attempt number (0-indexed).
	// Return true to retry, false to stop.
	ShouldRetry(resp ResponseReader, err error, attempt int) bool

	// GetDelay returns the delay duration before the next retry attempt.
	GetDelay(attempt int) time.Duration

	// MaxRetries returns the maximum number of retry attempts.
	MaxRetries() int
}

// The narrow capability interfaces below let the httpc facade reach request
// fields that are intentionally NOT on the public RequestMutator surface:
// per-request SSRF/retry/no-timeout overrides, the lazy query-param
// initializer, and the engine's pre/post callbacks. They mirror the
// sanitizedURLer pattern in package httpc: the facade asserts one of these
// small contracts instead of downcasting to a concrete engine type, so a
// third-party RequestMutator implementation degrades gracefully (no-op or a
// clear error) rather than panicking. The engine's request type satisfies each
// one by construction, so no engine change is required for these.
//
// These are separate interfaces rather than extra methods on RequestWriter:
// RequestWriter is public and intended for external implementation, and adding
// methods to it would be a breaking change for every external implementor.
type PrivateIPSetter interface {
	SetAllowPrivateIPs(v *bool)
}

// RetryNonIdempotentSetter sets the per-request non-idempotent retry override.
type RetryNonIdempotentSetter interface {
	SetRetryNonIdempotent(v *bool)
}

// NoTimeoutSetter suppresses the client-level request timeout for one request.
type NoTimeoutSetter interface {
	SetNoTimeout(v bool)
}

// QueryParamsEnsurer lazily initializes and returns the request's query-param
// map. It backs WithQuery/WithQueryMap, which mutate the returned map in place.
type QueryParamsEnsurer interface {
	EnsureQueryParams() map[string]any
}

// RequestCallbackSetter reads and installs the engine's pre-request and
// post-response callbacks. The callback signatures use the public mutator
// interfaces (not concrete engine types) so this contract stays in the leaf
// types package and never imports the engine. The *Mutator method names avoid
// colliding with the engine's concrete OnRequest/SetOnRequest accessors, whose
// signatures carry the concrete request type.
type RequestCallbackSetter interface {
	OnRequestMutator() func(RequestMutator) error
	SetOnRequestMutator(cb func(RequestMutator) error)
	OnResponseMutator() func(ResponseMutator) error
	SetOnResponseMutator(cb func(ResponseMutator) error)
}
