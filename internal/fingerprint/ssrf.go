package fingerprint

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/validation"
)

// ssrfResolveTimeout caps the pre-flight DNS lookup so a slow resolver cannot
// stall the request beyond the engine's own budget.
const ssrfResolveTimeout = 10 * time.Second

// preflightSSRF validates the request target before handing it to the backend.
// The backend dials on its own, so this mirrors the connection-layer check the
// standard path performs (resolve once, filter to allowed IPs) at request
// granularity instead of connection granularity. A per-request AllowPrivateIPs
// override on the context (WithAllowPrivateIPs) takes precedence, matching the
// standard dialer's behavior.
//
// KNOWN LIMITATION: the backend performs its own DNS resolution after this
// check, so an attacker controlling DNS retains a narrow TOCTOU window —
// weaker than the standard path's resolve-once-dial-IP guarantee.
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

	ctx, cancel := context.WithTimeout(req.Context(), ssrfResolveTimeout)
	defer cancel()
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("fingerprint: SSRF pre-flight: DNS resolution failed for %s: %w", host, err)
	}
	ips := make([]net.IP, len(ipAddrs))
	for i, addr := range ipAddrs {
		ips[i] = addr.IP
	}
	if len(validation.FilterAllowedIPs(ips, a.cfg.ExemptNets)) == 0 {
		return fmt.Errorf("fingerprint: SSRF protection: domain %s resolves only to blocked addresses", host)
	}
	return nil
}
