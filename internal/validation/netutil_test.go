package validation

import (
	"net"
	"testing"
)

func TestIsPrivateOrReservedIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// IPv4 private addresses
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 private A", "10.0.0.1", true},
		{"IPv4 private B", "172.16.0.1", true},
		{"IPv4 private C", "192.168.1.1", true},
		{"IPv4 link-local", "169.254.1.1", true},
		{"IPv4 multicast", "224.0.0.1", true},
		{"IPv4 unspecified", "0.0.0.0", true},

		// IPv4 reserved ranges
		{"IPv4 Class E", "240.0.0.1", true},
		{"IPv4 This network", "0.1.2.3", true},
		{"IPv4 CGNAT", "100.64.0.1", true},
		{"IPv4 Benchmarking", "198.18.0.1", true},

		// IPv4 public addresses
		{"IPv4 public Google DNS", "8.8.8.8", false},
		{"IPv4 public Cloudflare", "1.1.1.1", false},
		{"IPv4 public", "93.184.216.34", false},

		// IPv6 addresses
		{"IPv6 loopback", "::1", true},
		{"IPv6 unspecified", "::", true},
		{"IPv6 private fc00", "fc00::1", true},
		{"IPv6 private fd00", "fd00::1", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv6 documentation", "2001:db8::1", true},

		// IPv6 public addresses
		{"IPv6 public Google", "2001:4860:4860::8888", false},
		{"IPv6 public Cloudflare", "2606:4700:4700::1111", false},

		// IPv4-mapped IPv6 addresses (SSRF bypass prevention)
		{"IPv4-mapped loopback", "::ffff:127.0.0.1", true},
		{"IPv4-mapped private A", "::ffff:10.0.0.1", true},
		{"IPv4-mapped private B", "::ffff:172.16.0.1", true},
		{"IPv4-mapped private C", "::ffff:192.168.1.1", true},
		{"IPv4-mapped public", "::ffff:8.8.8.8", false},
		{"IPv4-mapped link-local", "::ffff:169.254.1.1", true},
		{"IPv4-mapped CGNAT", "::ffff:100.64.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			result := IsPrivateOrReservedIP(ip)
			if result != tt.expected {
				t.Errorf("IsPrivateOrReservedIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"public IP no error", "8.8.8.8", false},
		{"private IP error", "192.168.1.1", true},
		{"loopback error", "127.0.0.1", true},
		{"IPv4-mapped private error", "::ffff:192.168.1.1", true},
		{"IPv6 private error", "fc00::1", true},
		{"IPv6 link-local error", "fe80::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			err := ValidateIP(ip)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateIP(%s) expected error, got nil", tt.ip)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateIP(%s) unexpected error: %v", tt.ip, err)
			}
		})
	}
}

func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		// Standard localhost
		{"localhost lowercase", "localhost", true},
		{"localhost uppercase", "LOCALHOST", true},
		{"localhost mixed case", "LocalHost", true},

		// IP addresses
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.2", "127.0.0.2", true},
		{"127.1.1.1", "127.1.1.1", true},
		{"::1", "::1", true},
		{"0.0.0.0", "0.0.0.0", true},
		{"::", "::", true},

		// Localhost subdomains
		{"localhost.local", "localhost.local", true},
		{"localhost.example.com", "localhost.example.com", true},
		{"LOCALHOST.LOCAL", "LOCALHOST.LOCAL", true},

		// Non-localhost
		{"example.com", "example.com", false},
		{"192.168.1.1", "192.168.1.1", false},
		{"mylocalhost.com", "mylocalhost.com", false}, // "localhost" not at start
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLocalhost(tt.hostname)
			if result != tt.expected {
				t.Errorf("IsLocalhost(%s) = %v, want %v", tt.hostname, result, tt.expected)
			}
		})
	}
}

func TestSSRFBypassPrevention(t *testing.T) {
	// This test specifically validates SSRF bypass prevention techniques
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		// Common SSRF bypass attempts
		{"IPv4-mapped IPv6 localhost", "::ffff:127.0.0.1", true},
		{"IPv4-mapped IPv6 private", "::ffff:10.0.0.1", true},
		{"IPv4-mapped IPv6 loopback variant", "::ffff:127.0.0.2", true},
		{"IPv4-mapped IPv6 0.0.0.0", "::ffff:0.0.0.0", true},
		{"IPv4-mapped IPv6 169.254", "::ffff:169.254.1.1", true},

		// IPv6 local ranges
		{"IPv6 link-local fe80", "fe80::1", true},
		{"IPv6 unique local fc00", "fc00::1", true},
		{"IPv6 unique local fd00", "fdff:ffff:ffff:ffff::1", true},

		// Should NOT be blocked
		{"IPv4-mapped public", "::ffff:1.1.1.1", false},
		{"IPv6 public", "2606:4700:4700::1111", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}

			blocked := IsPrivateOrReservedIP(ip)
			if blocked != tt.blocked {
				t.Errorf("SSRF bypass check for %s: blocked=%v, want=%v", tt.ip, blocked, tt.blocked)
			}
		})
	}
}
