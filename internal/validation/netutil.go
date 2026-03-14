package validation

import (
	"fmt"
	"net"
	"strings"
)

// IsPrivateOrReservedIP checks if an IP address is private, reserved, or
// otherwise not suitable for public internet communication.
// This is used for SSRF (Server-Side Request Forgery) protection.
func IsPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		// Check for reserved IP ranges
		if ip4[0] >= 240 || // Class E (240.0.0.0/4) - Reserved
			ip4[0] == 0 || // "This" Network (0.0.0.0/8)
			(ip4[0] == 100 && (ip4[1]&0xC0) == 64) || // Carrier-grade NAT (100.64.0.0/10)
			(ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)) { // Benchmarking (198.18.0.0/15)
			return true
		}
	}

	return false
}

// ValidateIP checks if an IP is allowed for outbound requests.
// Returns an error if the IP is blocked by security policy.
func ValidateIP(ip net.IP) error {
	if IsPrivateOrReservedIP(ip) {
		return fmt.Errorf("blocked IP: %s", ip.String())
	}
	return nil
}

// IsLocalhost detects localhost variations including:
// - "localhost" (case-insensitive)
// - 127.0.0.1
// - ::1
// - 0.0.0.0
// - ::
// - 127.x.x.x range
// - localhost.* subdomains (case-insensitive)
func IsLocalhost(hostname string) bool {
	// Check exact matches (case-insensitive for localhost)
	switch strings.ToLower(hostname) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	}

	// Check 127.x.x.x range
	if len(hostname) > 4 && hostname[:4] == "127." {
		return true
	}

	// Check for localhost.* subdomains (case-insensitive)
	if strings.HasPrefix(strings.ToLower(hostname), "localhost.") {
		return true
	}

	return false
}
