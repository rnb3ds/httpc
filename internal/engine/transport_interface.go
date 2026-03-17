package engine

import (
	"context"
	"net/http"
)

// RoundTripper is an interface for executing HTTP requests.
// This abstraction allows injecting custom transport implementations
// for testing, custom protocols, or specialized networking requirements.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type RoundTripper interface {
	// RoundTrip executes a single HTTP transaction and returns a Response.
	// RoundTrip should not attempt to interpret the response.
	// RoundTrip must always close the body, including on errors.
	RoundTrip(req *http.Request) (*http.Response, error)
}

// TransportManager extends RoundTripper with redirect and lifecycle management.
type TransportManager interface {
	RoundTripper

	// SetRedirectPolicy configures redirect behavior for a specific request.
	// Returns a new context with the redirect settings.
	SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) context.Context

	// GetRedirectChain returns the list of URLs followed during redirects.
	GetRedirectChain(ctx context.Context) []string

	// CleanupRedirectSettings releases redirect settings back to the pool.
	// This MUST be called after the request completes to prevent memory leaks.
	CleanupRedirectSettings(ctx context.Context)

	// Close releases resources held by the transport.
	Close() error
}
