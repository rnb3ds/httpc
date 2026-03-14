package engine

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// RedirectKey is the context key for redirect policy configuration.
	RedirectKey contextKey = "redirect"
)
