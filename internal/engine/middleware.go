package engine

import (
	"context"
)

// Handler processes an HTTP request and returns a response.
type Handler func(ctx context.Context, req *Request) (*Response, error)

// MiddlewareFunc wraps a Handler with additional functionality.
type MiddlewareFunc func(Handler) Handler

// BuildChain constructs a middleware chain from a slice of middlewares.
// The final handler is executed after all middlewares have processed the request.
// If the middlewares slice is empty, the final handler is returned directly (zero overhead).
func BuildChain(middlewares []MiddlewareFunc, final Handler) Handler {
	if len(middlewares) == 0 {
		return final
	}

	chain := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		chain = middlewares[i](chain)
	}
	return chain
}
