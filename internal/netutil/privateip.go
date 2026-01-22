// Package netutil provides common network utility functions used across
// the httpc library. It includes IP validation, localhost detection, and
// security-related network checks.
package netutil

import (
	"fmt"
	"net"
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
// - "localhost"
// - 127.0.0.1
// - ::1
// - 0.0.0.0
// - ::
// - 127.x.x.x range
func IsLocalhost(hostname string) bool {
	switch hostname {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	}

	// Check 127.x.x.x range
	if len(hostname) > 4 && hostname[:4] == "127." {
		return true
	}

	// Check for localhost. prefix
	if len(hostname) > 10 {
		hostnameLower := hostname
		for i := range hostnameLower {
			if hostnameLower[i] >= 'A' && hostnameLower[i] <= 'Z' {
				if i == 0 {
					// For case-insensitive comparison without allocating new string
					c := hostnameLower[i] + 32
					if c < 'a' || c > 'z' {
						break
					}
				}
			}
		}
		if len(hostnameLower) >= 10 &&
			(hostnameLower[0] == 'l' || hostnameLower[0] == 'L') &&
			(hostnameLower[1] == 'o' || hostnameLower[1] == 'O') &&
			(hostnameLower[2] == 'c' || hostnameLower[2] == 'C') &&
			(hostnameLower[3] == 'a' || hostnameLower[3] == 'A') &&
			(hostnameLower[4] == 'l' || hostnameLower[4] == 'L') &&
			(hostnameLower[5] == 'h' || hostnameLower[5] == 'H') &&
			(hostnameLower[6] == 'o' || hostnameLower[6] == 'O') &&
			(hostnameLower[7] == 's' || hostnameLower[7] == 'S') &&
			(hostnameLower[8] == 't' || hostnameLower[8] == 'T') &&
			(hostnameLower[9] == '.') {
			return true
		}
	}

	return false
}
