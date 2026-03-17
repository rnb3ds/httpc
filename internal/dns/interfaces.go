package dns

import (
	"context"
	"net"
)

// Resolver defines the interface for DNS resolution.
// Implementations can provide different resolution strategies (DoH, system DNS, etc.)
type Resolver interface {
	// LookupIPAddr resolves a host name to IP addresses.
	// It returns a slice of net.IPAddr and any error encountered.
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)

	// ClearCache clears any cached DNS entries.
	ClearCache()
}
