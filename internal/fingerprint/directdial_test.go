package fingerprint

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybergodev/httpc/internal/connection"
)

// loopbackNet is the CIDR used by tests that need loopback dialing to pass
// validation WITHOUT AllowPrivateIPs — exercising the validating path rather
// than the passthrough branch.
func loopbackNet(t *testing.T) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR("127.0.0.0/8")
	if err != nil {
		t.Fatalf("parse loopback CIDR: %v", err)
	}
	return n
}

// TestValidatingDialer_RejectsBlockedLiterals pins that blocked IP literals
// fail validation before any dial is attempted.
func TestValidatingDialer_RejectsBlockedLiterals(t *testing.T) {
	d := newValidatingDialer(Config{}, nil)
	for _, addr := range []string{
		"127.0.0.1:80",
		"[::1]:443",
		"10.0.0.5:80",
		"192.168.1.1:80",
		"169.254.169.254:80", // cloud metadata
		"[fd00::1]:443",
	} {
		_, err := d.DialContext(context.Background(), "tcp", addr)
		if err == nil {
			t.Errorf("DialContext(%q) = nil error, want SSRF rejection", addr)
			continue
		}
		if !strings.Contains(err.Error(), "SSRF") {
			t.Errorf("DialContext(%q) = %v, want an SSRF-protection error", addr, err)
		}
	}
}

// TestValidatingDialer_PublicLiteralReachesDial proves a public literal passes
// validation and proceeds to the dial itself: with a canceled context the
// failure must be the context error, not an SSRF error.
func TestValidatingDialer_PublicLiteralReachesDial(t *testing.T) {
	d := newValidatingDialer(Config{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.DialContext(ctx, "tcp", "93.184.216.34:443")
	if err == nil {
		t.Fatal("DialContext with canceled ctx = nil error, want dial failure")
	}
	if strings.Contains(err.Error(), "SSRF") {
		t.Errorf("public literal must pass validation, got SSRF error: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled at the dial stage, got: %v", err)
	}
}

// TestValidatingDialer_AllowPrivateIPsPaths pins every opt-out path: the
// config-level flag, the per-request context override winning in BOTH
// directions, and real loopback dialing through the passthrough branch.
func TestValidatingDialer_AllowPrivateIPsPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	addr := net.JoinHostPort("127.0.0.1", port)

	t.Run("config flag dials loopback", func(t *testing.T) {
		d := newValidatingDialer(Config{AllowPrivateIPs: true}, nil)
		conn, err := d.DialContext(context.Background(), "tcp", addr)
		if err != nil {
			t.Fatalf("dial loopback with AllowPrivateIPs: %v", err)
		}
		_ = conn.Close() // best-effort test cleanup
	})

	t.Run("context override enables", func(t *testing.T) {
		d := newValidatingDialer(Config{}, nil)
		ctx := connection.WithAllowPrivateIPsOverride(context.Background(), true)
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			t.Fatalf("dial loopback with override: %v", err)
		}
		_ = conn.Close() // best-effort test cleanup
	})

	t.Run("context override false beats config true", func(t *testing.T) {
		d := newValidatingDialer(Config{AllowPrivateIPs: true}, nil)
		ctx := connection.WithAllowPrivateIPsOverride(context.Background(), false)
		if _, err := d.DialContext(ctx, "tcp", addr); err == nil {
			t.Fatal("override false must re-enable validation even with AllowPrivateIPs config")
		}
	})
}

// TestValidatingDialer_MixedRecordsPinAllowedIP is the regression pin for the
// deterministic mixed-record bypass: a host with one public and one private
// record must yield ONLY the public literal as a dial candidate — the
// hostname is never dialed, so no second resolution can land on the private
// record.
func TestValidatingDialer_MixedRecordsPinAllowedIP(t *testing.T) {
	public := "93.184.216.34"
	d := newValidatingDialer(Config{}, nil)
	d.lookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP(public)},
			{IP: net.ParseIP("10.0.0.8")}, // private — must never be dialed
			{IP: net.ParseIP("fd12:3456::1")},
		}, nil
	}

	cands, err := d.resolveAndValidate(context.Background(), "mixed.example:443")
	if err != nil {
		t.Fatalf("mixed records must pass via the public record: %v", err)
	}
	if len(cands) != 1 || cands[0] != public+":443" {
		t.Errorf("candidates = %v, want exactly [%s:443] — private records must be filtered out", cands, public)
	}
}

// TestValidatingDialer_BlockedHostnames pins the fail-closed hostname cases.
func TestValidatingDialer_BlockedHostnames(t *testing.T) {
	cases := []struct {
		name   string
		lookup func(ctx context.Context, host string) ([]net.IPAddr, error)
		want   string // substring of the expected error
	}{
		{
			name: "all records private",
			lookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("10.0.0.8")}, {IP: net.ParseIP("172.16.0.9")}}, nil
			},
			want: "resolves only to blocked addresses",
		},
		{
			name: "resolver failure",
			lookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
				return nil, errors.New("no such host")
			},
			want: "DNS resolution failed",
		},
		{
			name: "empty answer",
			lookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
				return nil, nil
			},
			want: "resolves only to blocked addresses",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newValidatingDialer(Config{}, nil)
			d.lookupIPAddr = tc.lookup
			_, err := d.DialContext(context.Background(), "tcp", "blocked.example:443")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("DialContext error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// TestValidatingDialer_ExemptHostnamePinsAndDials proves the full
// resolve→pin→dial chain end to end: a hostname (via the stub resolver)
// resolving to an EXEMPTED loopback IP is dialed by its literal and reaches a
// real listener.
func TestValidatingDialer_ExemptHostnamePinsAndDials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))

	d := newValidatingDialer(Config{ExemptNets: []*net.IPNet{loopbackNet(t)}}, nil)
	d.lookupIPAddr = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host != "local.test" {
			t.Errorf("resolved unexpected host %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}

	conn, err := d.DialContext(context.Background(), "tcp", "local.test:"+port)
	if err != nil {
		t.Fatalf("dial via pinned exempt literal: %v", err)
	}
	_ = conn.Close() // best-effort test cleanup
}

// TestValidatingDialer_BracketedIPv6Split pins host:port extraction for
// bracketed IPv6 literals.
func TestValidatingDialer_BracketedIPv6Split(t *testing.T) {
	_, v6loop, err := net.ParseCIDR("::1/128")
	if err != nil {
		t.Fatalf("parse ::1/128: %v", err)
	}
	d := newValidatingDialer(Config{ExemptNets: []*net.IPNet{v6loop}}, nil)
	// ::1 is IPv6 loopback; the 127.0.0.0/8 exemption does NOT cover it, so
	// exempt its own /128 — the point here is bracket handling, not policy.
	cands, err := d.resolveAndValidate(context.Background(), "[::1]:443")
	if err != nil {
		t.Fatalf("bracketed exempt literal: %v", err)
	}
	if len(cands) != 1 || cands[0] != "[::1]:443" {
		t.Errorf("candidates = %v, want [\"[::1]:443\"]", cands)
	}
}

// TestDirectAdapterValidatingDialerEndToEnd runs the whole direct stack —
// pre-flight, validating dialer, uTLS handshake — against a loopback TLS
// server admitted via SSRFExemptCIDRs (not AllowPrivateIPs), so the request
// exercises the validating branch of the dialer inside the real transport.
func TestDirectAdapterValidatingDialerEndToEnd(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok")) // best-effort test response
	}))
	defer srv.Close()

	a := NewAdapter(Config{
		PresetName:         "chrome",
		InsecureSkipVerify: true, // httptest self-signed cert
		ExemptNets:         []*net.IPNet{loopbackNet(t)},
	})
	defer func() { _ = a.Close() }()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := a.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip through validating dialer: %v", err)
	}
	defer func() { _ = resp.Body.Close() }() // best-effort test cleanup
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
