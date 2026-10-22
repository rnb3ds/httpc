// Direct-dial SSRF validation for the fingerprint transport.
//
// tls-client selects its transport dialer in buildFromConfig: when a
// ProxyDialerFactory is set it becomes THE dialer — unconditionally, whether
// or not a proxy URL is configured. This package already relies on that hook
// for proxies (proxydial.go); direct connections now install a validating
// dialer through the same hook, replicating the standard pool's posture
// (connection/pool.go resolveAndValidateAddress): resolve the hostname once,
// filter the records to SSRF-allowed IPs, and dial the validated IP literals
// in turn — the hostname itself is never dialed.
//
// That closes the two pre-flight-only gaps documented in ssrf.go for direct
// connections: there is no second resolution (no TOCTOU window), and a host
// with a MIX of public and private records can only ever be dialed on one of
// its allowed records (the deterministic mixed-record bypass is gone). The
// TLS layer is unaffected: ServerName, certificate verification, SNI and the
// transport's connection pooling key all derive from the request URL's
// hostname, which dialing an IP literal does not change.
//
// Proxied connections cannot pin (the proxy resolves the target itself), so
// they keep the request-time pre-flight — the same rule and the same
// trust-the-proxy residual the standard path's validateProxiedTarget has.

package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	ehttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"golang.org/x/net/proxy"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/validation"
)

// The validating DNS lookup is capped at 10s by the shared
// validation.ResolveFilteredIPs rule (the same cap the standard pool's
// resolveAndValidateAddress applies). The request context's own deadline
// still applies underneath (WithTimeout only tightens).

// newValidatingDialerFactory returns a tls_client.ProxyDialerFactory whose
// dialer resolve-validates-pins every direct target (see the file comment).
// The factory ignores tls-client's arguments: the proxy URL is always "" on
// this path (the package never sets WithProxyUrl), the timeout is owned by
// the request context, and the transport supplies no local address unless
// one was configured.
func newValidatingDialerFactory(cfg Config) tls_client.ProxyDialerFactory {
	return func(_ string, _ time.Duration, localAddr *net.TCPAddr, _ ehttp.Header, _ tls_client.Logger) (proxy.ContextDialer, error) {
		return newValidatingDialer(cfg, localAddr), nil
	}
}

// validatingDialer dials direct targets only after SSRF validation, and only
// by validated IP literal. Immutable after construction; safe for concurrent
// use (net.Dialer is, and the config fields are read-only snapshots).
type validatingDialer struct {
	exemptNets      []*net.IPNet
	allowPrivateIPs bool
	tcp             *net.Dialer
	// lookupIPAddr is the resolution seam: net.DefaultResolver.LookupIPAddr
	// in production, overridable in tests. net.Resolver offers no pluggable
	// interface, so the seam is the function itself.
	lookupIPAddr func(ctx context.Context, host string) ([]net.IPAddr, error)
}

// newValidatingDialer builds the dialer from the owning adapter's config.
// No net.Dialer.Timeout: the engine owns the deadline budget through the
// request context (the same choice as WithTimeoutSeconds(0) in transport.go).
func newValidatingDialer(cfg Config, localAddr *net.TCPAddr) *validatingDialer {
	return &validatingDialer{
		exemptNets:      cfg.ExemptNets,
		allowPrivateIPs: cfg.AllowPrivateIPs,
		tcp:             &net.Dialer{LocalAddr: localAddr},
		lookupIPAddr:    net.DefaultResolver.LookupIPAddr,
	}
}

// Dial implements the non-context half of proxy.ContextDialer.
func (d *validatingDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// DialContext validates addr and dials a validated IP literal. The context
// is the fork request's context, so a per-request AllowPrivateIPs override
// (WithAllowPrivateIPs) reaches here exactly as it reaches the pre-flight.
func (d *validatingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	allowPrivateIPs := d.allowPrivateIPs
	if override, ok := connection.AllowPrivateIPsOverrideFromContext(ctx); ok {
		allowPrivateIPs = override
	}
	if allowPrivateIPs {
		// Opted out entirely — dial the address as given, matching the
		// standard pool's AllowPrivateIPs branch and the pre-flight's.
		return d.tcp.DialContext(ctx, network, addr)
	}

	candidates, err := d.resolveAndValidate(ctx, addr)
	if err != nil {
		return nil, err
	}
	return d.dialCandidates(ctx, network, candidates)
}

// resolveAndValidate turns "host:port" into validated "ip:port" candidates.
// IP literals validate directly (no DNS, no window). Hostnames resolve once
// under a capped timeout and filter to allowed records — split-horizon DNS
// keeps working: a domain with both public and private records yields its
// public records only, and a domain whose records are all blocked fails.
func (d *validatingDialer) resolveAndValidate(ctx context.Context, addr string) ([]string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: SSRF dial validation: %w", err)
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := validation.ValidateIPWithExemptions(ip, d.exemptNets); err != nil {
			return nil, fmt.Errorf("fingerprint: SSRF protection: %w", err)
		}
		return []string{addr}, nil
	}

	// Resolve-and-filter via the shared SSRF rule (validation.ResolveFilteredIPs,
	// the same implementation the standard path and the pre-flight use);
	// dialTimeout 0 keeps the 10s cap, and the lookup seam preserves the
	// test override.
	allowed, err := validation.ResolveFilteredIPs(ctx, host, d.exemptNets, 0, d.lookupIPAddr)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: SSRF dial validation: DNS resolution failed for %s: %w", host, err)
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("fingerprint: SSRF protection: domain %s resolves only to blocked addresses", host)
	}

	candidates := make([]string, len(allowed))
	for i, ip := range allowed {
		candidates[i] = net.JoinHostPort(ip.String(), port)
	}
	return candidates, nil
}

// dialCandidates tries each validated address in turn (IPv6→IPv4 failover,
// mirroring the standard pool's dialCandidates). Every candidate passed
// validation seconds earlier in this same call, so any of them is
// rebinding-safe by construction.
func (d *validatingDialer) dialCandidates(ctx context.Context, network string, candidates []string) (net.Conn, error) {
	var conn net.Conn
	// Seeded so an empty candidate list cannot render as %!w(<nil>).
	lastErr := errors.New("fingerprint: no validated addresses to dial")
	for _, addr := range candidates {
		conn, lastErr = d.tcp.DialContext(ctx, network, addr)
		if conn != nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("fingerprint: connection failed after trying %d validated address(es): %w", len(candidates), lastErr)
}
