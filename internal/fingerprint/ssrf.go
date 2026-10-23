package fingerprint

import (
	"fmt"
	"net"
	"net/http"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/validation"
)

// preflightSSRF validates the request target before handing it to the backend.
// For DIRECT connections this is now defense-in-depth: the validating dialer
// (directdial.go) resolve-validates-pins every dial, so the pre-flight only
// fails the request earlier and never widens what can connect. For PROXIED
// requests it remains the enforcement — the proxy resolves the target itself,
// so pinning is impossible and this check is the same rule, with the same
// trust-the-proxy residual, as the standard path's validateProxiedTarget.
//
// A per-request AllowPrivateIPs override on the context
// (WithAllowPrivateIPs) takes precedence, matching both the standard dialer
// and the validating dialer's behavior.
func (a *Adapter) preflightSSRF(req *http.Request) error {
	allowPrivateIPs := a.cfg.AllowPrivateIPs
	if override, ok := connection.AllowPrivateIPsOverrideFromContext(req.Context()); ok {
		allowPrivateIPs = override
	}
	if allowPrivateIPs {
		return nil
	}

	host := req.URL.Hostname()
	if host == "" {
		return fmt.Errorf("fingerprint: SSRF pre-flight: empty request host")
	}

	// Literal IPs validate directly; no DNS, no window at all.
	if ip := net.ParseIP(host); ip != nil {
		if err := validation.ValidateIPWithExemptions(ip, a.cfg.ExemptNets); err != nil {
			return fmt.Errorf("fingerprint: SSRF protection: %w", err)
		}
		return nil
	}

	// Shared SSRF resolve-and-filter rule (validation.ResolveFilteredIPs) —
	// the same implementation the standard path's dialer and proxied-target
	// check and the validating dialer below use; dialTimeout 0 keeps the
	// 10s cap this pre-flight always applied. DNSLookup (nil = system
	// resolver) keeps the check on the same resolver the dials use.
	allowed, err := validation.ResolveFilteredIPs(req.Context(), host, a.cfg.ExemptNets, 0, a.cfg.DNSLookup)
	if err != nil {
		return fmt.Errorf("fingerprint: SSRF pre-flight: DNS resolution failed for %s: %w", host, err)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("fingerprint: SSRF protection: domain %s resolves only to blocked addresses", host)
	}
	return nil
}
