package validation

import (
	"fmt"
	"net"
	"strings"
)

// IsPrivateOrReservedIP checks if an IP address is private, reserved, or
// otherwise not suitable for public internet communication.
// This is used for SSRF (Server-Side Request Forgery) protection.
//
// SECURITY: This function handles IPv4-mapped IPv6 addresses (::ffff:x.x.x.x)
// to prevent bypass attempts using mixed notation.
func IsPrivateOrReservedIP(ip net.IP) bool {
	// Normalize: handle IPv4-mapped IPv6 addresses (::ffff:127.0.0.1)
	// This prevents SSRF bypass using mixed IPv4/IPv6 notation
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// IPv4 specific checks
	if ip4 := ip.To4(); ip4 != nil {
		// Check for reserved IP ranges
		if ip4[0] >= 240 || // Class E (240.0.0.0/4) - Reserved
			ip4[0] == 0 || // "This" Network (0.0.0.0/8)
			(ip4[0] == 100 && (ip4[1]&0xC0) == 64) || // Carrier-grade NAT (100.64.0.0/10)
			(ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)) { // Benchmarking (198.18.0.0/15)
			return true
		}
		return false
	}

	// IPv6 specific checks (only for true IPv6 addresses, not mapped IPv4)
	if len(ip) == 16 {
		// Documentation prefix 2001:db8::/32 (RFC 3849)
		if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
			return true
		}
		// Link-local IPv6: fe80::/10 (RFC 4291)
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return true
		}
		// Unique local IPv6: fc00::/7 (RFC 4193)
		if (ip[0] & 0xfe) == 0xfc {
			return true
		}
		// IPv6 loopback beyond just ::1 (full ::1/128 range is handled by IsLoopback)
		// IPv4-mapped IPv6 loopback: ::ffff:127.0.0.0/104
		if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 &&
			ip[4] == 0 && ip[5] == 0 && ip[6] == 0 && ip[7] == 0 &&
			ip[8] == 0 && ip[9] == 0 && ip[10] == 0xff && ip[11] == 0xff &&
			ip[12] == 127 {
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
