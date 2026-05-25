package dns

import (
	"context"
	"net"
)

// resolver defines the interface for DNS resolution.
// Used only as a compile-time contract check — not consumed as a parameter
// or field type anywhere. Retained for documentation and future abstraction.
type resolver interface {
	// LookupIPAddr resolves a host name to IP addresses.
	// It returns a slice of net.IPAddr and any error encountered.
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)

	// ClearCache clears any cached DNS entries.
	ClearCache()
}
