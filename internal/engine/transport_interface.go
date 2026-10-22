package engine

import (
	"context"
	"net/http"
)

// transportManager extends http.RoundTripper with redirect and lifecycle management.
//
// Header-map ownership contract for RoundTrip implementors: the returned
// response's Request (when non-nil) must either be the identical
// *http.Request that was passed in, or carry a header map that is NOT shared
// with the input request's Header. captureRequestHeaders transfers the
// returned Request's header map into the engine Response, while the caller's
// deferred putHTTPHeader(httpReq.Header) recycles the input's map whenever it
// is still non-nil — a shallow-copied Request sharing the input's map would
// have that live, Result-owned map recycled into the pool, poisoning later
// requests. The stdlib transport satisfies this on every path (the original
// request is returned identity-intact; redirect hops build fresh maps), as
// does the fingerprint adapter (it clones the request and assigns a fresh
// wire-header map). Custom transports injected through the test seam must do
// the same.
type transportManager interface {
	http.RoundTripper

	// SetRedirectPolicy configures redirect behavior for a specific request.
	// Returns a new context with the redirect settings and the settings pointer.
	// The settings object carries the redirect chain for the request; callers
	// read it directly via getChain. The caller MUST call
	// putRedirectSettings(settings) (typically via defer) after the request
	// completes to prevent memory leaks from pool exhaustion.
	SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, *redirectSettings)

	// Close releases resources held by the transport.
	Close() error
}
