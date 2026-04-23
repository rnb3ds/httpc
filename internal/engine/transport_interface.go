package engine

import (
	"context"
	"net/http"
)

// roundTripper is an interface for executing HTTP requests.
// This abstraction allows injecting custom transport implementations
// for testing, custom protocols, or specialized networking requirements.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type roundTripper interface {
	// RoundTrip executes a single HTTP transaction and returns a Response.
	// RoundTrip should not attempt to interpret the response.
	// RoundTrip must always close the body, including on errors.
	RoundTrip(req *http.Request) (*http.Response, error)
}

// transportManager extends roundTripper with redirect and lifecycle management.
type transportManager interface {
	roundTripper

	// SetRedirectPolicy configures redirect behavior for a specific request.
	// Returns a new context with the redirect settings and a cleanup function.
	// The cleanup function MUST be called (typically via defer) after the request
	// completes to prevent memory leaks from pool exhaustion.
	SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, func())

	// GetRedirectChain returns the list of URLs followed during redirects.
	GetRedirectChain(ctx context.Context) []string

	// Close releases resources held by the transport.
	Close() error
}
