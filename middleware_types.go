package httpc

import (
	"github.com/cybergodev/httpc/internal/engine"
)

// Type aliases for engine types - provides a clean public API
// while delegating to the engine implementation.
type (
	// RequestMutator provides read-write access to request data for middleware.
	RequestMutator = engine.RequestMutator

	// ResponseAccessor provides read-only access to response data.
	ResponseAccessor = engine.ResponseAccessor

	// ResponseMutator provides read-write access to response data.
	ResponseMutator = engine.ResponseMutator

	// Handler processes an HTTP request and returns a response.
	Handler = engine.Handler

	// MutableHandler processes an HTTP request and returns a mutable response.
	MutableHandler = engine.MutableHandler

	// MiddlewareFunc wraps a Handler with additional functionality.
	MiddlewareFunc = engine.MiddlewareFunc

	// MutableMiddlewareFunc wraps a MutableHandler with additional functionality.
	MutableMiddlewareFunc = engine.MutableMiddlewareFunc
)

// AsMutable converts a Handler to a MutableHandler.
func AsMutable(h Handler) MutableHandler {
	return engine.AsMutable(h)
}

// AsHandler converts a MutableHandler back to a Handler.
func AsHandler(mh MutableHandler) Handler {
	return engine.AsHandler(mh)
}

// MutableResponseError indicates that a response could not be converted to mutable form.
type MutableResponseError = engine.MutableResponseError
