package security

import (
	"net"
	"testing"
)

// ============================================================================
// SECURITY COVERAGE IMPROVEMENT TESTS
// ============================================================================

func TestNewValidatorWithConfig(t *testing.T) {
	t.Run("CustomConfig", func(t *testing.T) {
		cfg := &Config{
			AllowPrivateIPs:     true,
			MaxResponseBodySize: 10 * 1024 * 1024,
			ValidateURL:         true,
			ValidateHeaders:     true,
		}

		validator := NewValidatorWithConfig(cfg)
		if validator == nil {
			t.Fatal("Expected non-nil validator")
		}
	})

	t.Run("NilConfig", func(t *testing.T) {
		validator := NewValidatorWithConfig(nil)
		if validator == nil {
			t.Fatal("Expected non-nil validator even with nil config")
		}
	})
}

// ----------------------------------------------------------------------------
// isPrivateOrReservedIP (0% coverage)
// ----------------------------------------------------------------------------

func TestIsPrivateOrReservedIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IPv4 ranges
		{"Private10", "10.0.0.1", true},
		{"Private172", "172.16.0.1", true},
		{"Private192", "192.168.1.1", true},

		// Loopback
		{"Loopback", "127.0.0.1", true},
		{"LoopbackIPv6", "::1", true},

		// Link-local
		{"LinkLocal", "169.254.1.1", true},
		{"LinkLocalIPv6", "fe80::1", true},

		// Multicast
		{"Multicast", "224.0.0.1", true},
		{"MulticastIPv6", "ff00::1", true},

		// Public IPs
		{"PublicIP", "8.8.8.8", false},
		{"PublicIP2", "1.1.1.1", false},

		// Edge cases
		{"Broadcast", "255.255.255.255", true},
		{"ZeroIP", "0.0.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			result := isPrivateOrReservedIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateOrReservedIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// validateHost Edge Cases
// ----------------------------------------------------------------------------

func TestValidateHost_EdgeCases(t *testing.T) {
	validator := NewValidatorWithConfig(&Config{
		ValidateURL:         true,
		ValidateHeaders:     true,
		MaxResponseBodySize: 50 * 1024 * 1024,
		AllowPrivateIPs:     false,
	})

	tests := []struct {
		name      string
		host      string
		shouldErr bool
	}{
		{"ValidDomain", "example.com", false},
		{"ValidSubdomain", "api.example.com", false},
		{"ValidIP", "8.8.8.8", false},
		{"Localhost", "localhost", true},
		{"PrivateIP", "192.168.1.1", true},
		{"MetadataService", "169.254.169.254", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateHost(tt.host)
			if (err != nil) != tt.shouldErr {
				t.Errorf("validateHost(%s) error = %v, shouldErr = %v", tt.host, err, tt.shouldErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// validateHeader Edge Cases
// ----------------------------------------------------------------------------

func TestValidateHeader_EdgeCases(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		key       string
		value     string
		shouldErr bool
	}{
		{"ValidHeader", "X-Custom", "value", false},
		{"EmptyKey", "", "value", true},
		{"EmptyValue", "X-Custom", "", false},
		{"CRLFInKey", "X-Custom\r\n", "value", true},
		{"CRLFInValue", "X-Custom", "value\r\nInjection", true},
		{"NullByteInKey", "X-Custom\x00", "value", true},
		{"NullByteInValue", "X-Custom", "value\x00", true},
		{"TabInValue", "X-Custom", "value\twith\ttabs", false},
		{"SpaceInValue", "X-Custom", "value with spaces", false},
		{"LongValue", "X-Custom", "very long value here", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateHeader(tt.key, tt.value)
			if (err != nil) != tt.shouldErr {
				t.Errorf("validateHeader(%q, %q) error = %v, shouldErr = %v", tt.key, tt.value, err, tt.shouldErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// validateURL with AllowPrivateIPs
// ----------------------------------------------------------------------------

func TestValidateURL_WithAllowPrivateIPs(t *testing.T) {
	cfg := &Config{
		AllowPrivateIPs: true,
	}
	validator := NewValidatorWithConfig(cfg)

	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{"PrivateIP", "http://192.168.1.1", false},
		{"Localhost", "http://localhost", false},
		{"PublicIP", "http://8.8.8.8", false},
		{"InvalidURL", "://invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateURL(tt.url)
			if (err != nil) != tt.shouldErr {
				t.Errorf("validateURL(%s) error = %v, shouldErr = %v", tt.url, err, tt.shouldErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Complex Request Validation
// ----------------------------------------------------------------------------

func TestValidateRequest_Complex(t *testing.T) {
	validator := NewValidator()

	t.Run("ValidComplexRequest", func(t *testing.T) {
		req := &Request{
			Method: "POST",
			URL:    "https://api.example.com/endpoint",
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer token123",
				"X-Custom":      "value",
			},
			Body: []byte(`{"key":"value"}`),
		}

		err := validator.ValidateRequest(req)
		if err != nil {
			t.Errorf("ValidateRequest failed: %v", err)
		}
	})

	t.Run("RequestWithInvalidHeader", func(t *testing.T) {
		req := &Request{
			Method: "GET",
			URL:    "https://example.com",
			Headers: map[string]string{
				"X-Custom\r\n": "value",
			},
		}

		err := validator.ValidateRequest(req)
		if err == nil {
			t.Error("Expected error for invalid header")
		}
	})

}
