package fingerprint

import (
	"net/http"
	"net/url"

	"github.com/cybergodev/httpc/internal/validation"
)

// selectProxy resolves the proxy for one round trip through the owning
// PoolManager, honoring the per-attempt index and ProxyRecorder carried on the
// request context. Returns (nil, nil) for a direct connection.
func (a *Adapter) selectProxy(req *http.Request) (*url.URL, error) {
	if a.cfg.Pool == nil {
		return nil, nil
	}
	return a.cfg.Pool.SelectProxy(req.Context(), req.URL)
}

// reportProxy feeds the proxy circuit breaker from the adapter path. addr is
// the canonical proxy dial address (host:port), matching what the pool seeded.
func (a *Adapter) reportProxy(proxy *url.URL, ok bool) {
	if a.cfg.Pool == nil || proxy == nil {
		return
	}
	addr := validation.CanonicalProxyAddr(proxy)
	if ok {
		a.cfg.Pool.ReportProxySuccess(addr)
	} else {
		a.cfg.Pool.ReportProxyFailure(addr)
	}
}
