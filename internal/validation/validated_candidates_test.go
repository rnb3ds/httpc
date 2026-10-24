package validation

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func stubLookup(ips ...string) func(context.Context, string) ([]net.IPAddr, error) {
	return func(_ context.Context, host string) ([]net.IPAddr, error) {
		out := make([]net.IPAddr, len(ips))
		for i, s := range ips {
			out[i] = net.IPAddr{IP: net.ParseIP(s)}
		}
		return out, nil
	}
}

// TestValidatedCandidates pins the shared SSRF split→validate→candidate
// sequence consumed by the standard dialer, its DoH variant, and the
// fingerprint validating dialer: IP literals validate without DNS, mixed
// records yield only the allowed literal, portless addresses fail closed
// (the old pool copy silently assumed port 443), and the error message
// shapes callers rely on stay stable.
func TestValidatedCandidates(t *testing.T) {
	t.Run("public IP literal passes through", func(t *testing.T) {
		got, err := ValidatedCandidates(context.Background(), "93.184.216.34:443", nil, 0, nil)
		if err != nil || len(got) != 1 || got[0] != "93.184.216.34:443" {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("blocked IP literal rejected", func(t *testing.T) {
		_, err := ValidatedCandidates(context.Background(), "10.0.0.8:443", nil, 0, nil)
		if err == nil || !strings.Contains(err.Error(), "blocked IP address") {
			t.Errorf("err = %v, want blocked-IP message", err)
		}
	})

	t.Run("exempt CIDR admits private literal", func(t *testing.T) {
		_, exempt, err := net.ParseCIDR("10.0.0.0/8")
		if err != nil {
			t.Fatalf("parse CIDR: %v", err)
		}
		got, err := ValidatedCandidates(context.Background(), "10.0.0.8:9443", []*net.IPNet{exempt}, 0, nil)
		if err != nil || len(got) != 1 || got[0] != "10.0.0.8:9443" {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("mixed records yield only the allowed literal", func(t *testing.T) {
		got, err := ValidatedCandidates(context.Background(), "mixed.example:8080", nil, 0,
			stubLookup("93.184.216.34", "10.0.0.8"))
		if err != nil || len(got) != 1 || got[0] != "93.184.216.34:8080" {
			t.Fatalf("got %v, %v; want only the public literal with the caller's port", got, err)
		}
	})

	t.Run("all records blocked", func(t *testing.T) {
		_, err := ValidatedCandidates(context.Background(), "blocked.example:443", nil, 0,
			stubLookup("10.0.0.8"))
		if err == nil || !strings.Contains(err.Error(), "resolves only to blocked addresses") {
			t.Errorf("err = %v, want blocked-domain message", err)
		}
	})

	t.Run("DNS failure", func(t *testing.T) {
		lookup := func(context.Context, string) ([]net.IPAddr, error) {
			return nil, &net.DNSError{Err: "no such host", Name: "nx.example"}
		}
		_, err := ValidatedCandidates(context.Background(), "nx.example:443", nil, 0, lookup)
		if err == nil || !strings.Contains(err.Error(), "DNS resolution failed") {
			t.Errorf("err = %v, want DNS-failure message", err)
		}
	})

	t.Run("portless address fails closed", func(t *testing.T) {
		_, err := ValidatedCandidates(context.Background(), "localhost", nil, time.Second, nil)
		if err == nil || !strings.Contains(err.Error(), "must be host:port") {
			t.Errorf("err = %v, want host:port failure (not a silent 443)", err)
		}
	})
}
