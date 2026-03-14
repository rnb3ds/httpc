package httpc

import (
	"context"
	"fmt"
	"time"
)

// Note: engine.Request now directly implements the RequestMutator interface,
// and engine.Response now directly implements the ResponseMutator interface.
// The adapters have been removed to eliminate the GC overhead.

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
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(start)

			status := 0
			if resp != nil {
				status = resp.StatusCode()
			}

			if err != nil {
				log("%s %s -> error: %v (%v)", req.Method(), req.URL(), err, duration)
			} else {
				log("%s %s -> %d (%v)", req.Method(), req.URL(), status, duration)
			}

			return resp, err
		}
	}
}

// RecoveryMiddleware creates a middleware that recovers from panics in the request handler.
// If a panic occurs, it is converted to an error and returned.
func RecoveryMiddleware() MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (resp ResponseMutator, err error) {
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
			// Use single timestamp call and combine with counter for uniqueness
			ts := time.Now().UnixNano()
			return fmt.Sprintf("%d-%04d", ts, ts&0xFFFF)
		}
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			headers := req.Headers()
			if _, exists := headers[headerName]; !exists {
				req.SetHeader(headerName, generator())
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
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			if timeout <= 0 {
				return next(ctx, req)
			}

			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Also set the request timeout so the engine respects it
			req.SetTimeout(timeout)

			return next(timeoutCtx, req)
		}
	}
}

// HeaderMiddleware creates a middleware that adds static headers to every request.
// Existing headers with the same keys will be overwritten.
func HeaderMiddleware(headers map[string]string) MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			for key, value := range headers {
				req.SetHeader(key, value)
			}

			return next(ctx, req)
		}
	}
}

// MetricsMiddleware creates a middleware that collects request metrics.
// The onMetrics callback is invoked with metrics after each request completes.
func MetricsMiddleware(onMetrics func(method, url string, statusCode int, duration time.Duration, err error)) MiddlewareFunc {
	return func(next Handler) Handler {
		return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(start)

			statusCode := 0
			if resp != nil {
				statusCode = resp.StatusCode()
			}

			onMetrics(req.Method(), req.URL(), statusCode, duration, err)

			return resp, err
		}
	}
}
