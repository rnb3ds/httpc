package httpc

import (
	"context"
	"fmt"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// Handler processes an HTTP request and returns a response.
// This is the core function signature for request processing in the middleware chain.
type Handler func(ctx context.Context, req *engine.Request) (*engine.Response, error)

// MiddlewareFunc wraps a Handler with additional functionality.
// Middleware can inspect/modify requests, handle responses, add logging, etc.
type MiddlewareFunc func(Handler) Handler

// Chain combines multiple middlewares into a single middleware.
// Middlewares are executed in the order they are provided (first to last).
// The final handler is executed after all middlewares have processed the request.
func Chain(middlewares ...MiddlewareFunc) MiddlewareFunc {
	return func(final Handler) Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// LoggingMiddleware creates a middleware that logs request and response information.
// The log function receives formatted log messages (similar to log.Printf).
func LoggingMiddleware(log func(format string, args ...any)) MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(start)

			status := 0
			if resp != nil {
				status = resp.StatusCode
			}

			if err != nil {
				log("%s %s -> error: %v (%v)", req.Method, req.URL, err, duration)
			} else {
				log("%s %s -> %d (%v)", req.Method, req.URL, status, duration)
			}

			return resp, err
		}
	}
}

// RecoveryMiddleware creates a middleware that recovers from panics in the request handler.
// If a panic occurs, it is converted to an error and returned.
func RecoveryMiddleware() MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (resp *engine.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					if e, ok := r.(error); ok {
						err = fmt.Errorf("panic recovered: %w", e)
					} else {
						err = fmt.Errorf("panic recovered: %v", r)
					}
				}
			}()
			return next(ctx, req)
		}
	}
}

// RequestIDMiddleware creates a middleware that adds a unique request ID to each request.
// The request ID is added to the request headers with the specified header name.
// If generator is nil, a default time-based ID generator is used.
func RequestIDMiddleware(headerName string, generator func() string) MiddlewareFunc {
	if generator == nil {
		generator = func() string {
			return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
		}
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}

			if _, exists := req.Headers[headerName]; !exists {
				req.Headers[headerName] = generator()
			}

			return next(ctx, req)
		}
	}
}

// TimeoutMiddleware creates a middleware that enforces a maximum duration for requests.
// If the request exceeds the timeout, the context is canceled and an error is returned.
// This timeout applies at the middleware level, before the client's built-in timeout.
func TimeoutMiddleware(timeout time.Duration) MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
			if timeout <= 0 {
				return next(ctx, req)
			}

			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			return next(timeoutCtx, req)
		}
	}
}

// HeaderMiddleware creates a middleware that adds static headers to every request.
// Existing headers with the same keys will be overwritten.
func HeaderMiddleware(headers map[string]string) MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}

			for key, value := range headers {
				req.Headers[key] = value
			}

			return next(ctx, req)
		}
	}
}

// MetricsMiddleware creates a middleware that collects request metrics.
// The onMetrics callback is invoked with metrics after each request completes.
func MetricsMiddleware(onMetrics func(method, url string, statusCode int, duration time.Duration, err error)) MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req *engine.Request) (*engine.Response, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(start)

			statusCode := 0
			if resp != nil {
				statusCode = resp.StatusCode
			}

			onMetrics(req.Method, req.URL, statusCode, duration, err)

			return resp, err
		}
	}
}
