package connection

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestValidateProxiedTarget is the table-driven branch matrix for the
// proxied-target SSRF guard (ssrfGuardProxyFunc's enforcement core): IP
// literals validate without DNS, exemptions override blocked literals,
// hostnames that resolve only to blocked addresses are rejected, and
// unresolvable hostnames fail closed. All cases are offline-safe — no case
// needs a resolvable public name (that branch requires real DNS and is
// exercised by the e2e suite).
func TestValidateProxiedTarget(t *testing.T) {
	loopbackExempt := []*net.IPNet{mustParseCIDR(t, "127.0.0.0/8")}

	tests := []struct {
		name       string
		host       string
		exemptNets []*net.IPNet
		wantErr    bool
		wantSubstr string // empty: assert no error
	}{
		{
			name:    "empty host fails",
			host:    "",
			wantErr: true,
		},
		{
			name:    "private IP literal blocked",
			host:    "127.0.0.1",
			wantErr: true,
		},
		{
			name:    "link-local IP literal blocked",
			host:    "169.254.169.254",
			wantErr: true,
		},
		{
			name:       "private IPv6 literal blocked",
			host:       "[::1]",
			wantErr:    true,
			wantSubstr: "",
		},
		{
			name:       "public IP literal allowed",
			host:       "8.8.8.8",
			wantErr:    false,
			wantSubstr: "",
		},
		{
			name:       "private IP literal allowed by exemption",
			host:       "127.0.0.1",
			exemptNets: loopbackExempt,
			wantErr:    false,
		},
		{
			name:       "hostname resolving only to blocked addresses rejected",
			host:       "localhost",
			wantErr:    true,
			wantSubstr: "resolves only to blocked addresses",
		},
		{
			name:       "unresolvable hostname fails closed",
			host:       "nonexistent-7f3a-fix001.invalid",
			wantErr:    true,
			wantSubstr: "DNS resolution failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &url.URL{Scheme: "http", Host: tt.host, Path: "/"}
			err := validateProxiedTarget(context.Background(), target, tt.exemptNets, time.Second, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateProxiedTarget(%q) = nil error, want failure", tt.host)
				}
				if tt.wantSubstr != "" && !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Errorf("error %q does not contain %q", err, tt.wantSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateProxiedTarget(%q) = %v, want nil", tt.host, err)
			}
		})
	}
}

// mustParseCIDR parses a CIDR or fails the test — local helper for exemption
// fixtures (the package's other tests inline this via net.ParseCIDR + checks).
func mustParseCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("test setup: parse CIDR %q: %v", cidr, err)
	}
	return network
}

// TestSSRFGuardProxyFunc pinches the wrapper itself: when the wrapped selector
// returns no proxy (direct connection) or errors, the guard passes the result
// through untouched — the DNS work must only happen when a proxy will actually
// carry the request.
func TestSSRFGuardProxyFunc(t *testing.T) {
	called := 0
	selector := func(req *http.Request) (*url.URL, error) {
		called++
		if called == 2 {
			return nil, fmt.Errorf("selector boom")
		}
		return nil, nil
	}
	guard := (&PoolManager{}).ssrfGuardProxyFunc(selector)

	req, _ := http.NewRequest(http.MethodGet, "http://nonexistent-fix001.invalid/", nil)

	if u, err := guard(req); u != nil || err != nil {
		t.Errorf("direct-connection passthrough = (%v, %v), want (nil, nil)", u, err)
	}
	if u, err := guard(req); u != nil || err == nil {
		t.Errorf("selector error passthrough = (%v, %v), want (nil, error)", u, err)
	}

	// nil selector wraps to nil — no guard to install.
	if (&PoolManager{}).ssrfGuardProxyFunc(nil) != nil {
		t.Error("ssrfGuardProxyFunc(nil) should return nil")
	}
}

// TestValidateProxiedTarget_UsesProvidedResolver pins the resolver-consistency
// fix: the guard must run on the SAME resolver the dial path uses. With DoH
// enabled, the dial path resolves through DoH while the guard previously
// always used the system resolver — the two could disagree on what a host
// resolves to. A stub lookup standing in for the DoH resolver now governs
// the verdict (an .invalid host is unresolvable by the system resolver and
// must fail closed without the stub).
func TestValidateProxiedTarget_UsesProvidedResolver(t *testing.T) {
	ctx := context.Background()
	target := &url.URL{Scheme: "https", Host: "unresolvable.invalid", Path: "/x"}

	if err := validateProxiedTarget(ctx, target, nil, 500*time.Millisecond, nil); err == nil {
		t.Fatal("unresolvable host must fail closed with the system resolver")
	}
	stub := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	if err := validateProxiedTarget(ctx, target, nil, 500*time.Millisecond, stub); err != nil {
		t.Fatalf("provided (DoH-shaped) lookup should govern the verdict: %v", err)
	}
}
