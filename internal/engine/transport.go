package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/fingerprint"
	"github.com/cybergodev/httpc/internal/security"
	"github.com/cybergodev/httpc/internal/validation"
)

// maxInlineRedirects is the number of redirects we can track inline without heap allocation.
// Most redirects are < 5, so 8 provides a good balance.
const maxInlineRedirects = 8

// redirectSettings holds per-request redirect configuration.
// Uses a fixed-size array for the first few redirects to avoid heap allocation
// in the common case. Falls back to slice allocation only if needed.
type redirectSettings struct {
	followRedirects bool
	maxRedirects    int
	chainLen        int
	inlineChain     [maxInlineRedirects]string
	overflowChain   []string
}

// addRedirect adds a URL to the redirect chain.
// Uses inline array first, then overflows to slice.
func (s *redirectSettings) addRedirect(url string) {
	if s.chainLen < maxInlineRedirects {
		s.inlineChain[s.chainLen] = url
	} else {
		// Lazily allocate overflow slice only when needed
		if s.overflowChain == nil {
			s.overflowChain = make([]string, 0, maxInlineRedirects)
			// Copy inline entries to overflow for consistent iteration
			s.overflowChain = append(s.overflowChain, s.inlineChain[:s.chainLen]...)
		}
		s.overflowChain = append(s.overflowChain, url)
	}
	s.chainLen++
}

// getChain returns the redirect chain as a slice.
// Returns a freshly allocated copy to prevent mutation.
func (s *redirectSettings) getChain() []string {
	if s.chainLen == 0 {
		return nil
	}
	if s.chainLen <= maxInlineRedirects {
		// Fast path: use inline data directly without pool overhead
		chain := make([]string, s.chainLen)
		copy(chain, s.inlineChain[:s.chainLen])
		return chain
	}
	chain := make([]string, s.chainLen)
	if s.overflowChain != nil {
		copy(chain, s.overflowChain)
	}
	return chain
}

// redirectSettingsPool reduces allocations for redirectSettings objects.
// Safe to use because settings are only accessed during a single request's lifetime.
var redirectSettingsPool = sync.Pool{
	New: func() any {
		return &redirectSettings{}
	},
}

// getRedirectSettings retrieves a redirectSettings from the pool.
func getRedirectSettings() *redirectSettings {
	s, ok := redirectSettingsPool.Get().(*redirectSettings)
	if !ok || s == nil {
		return &redirectSettings{}
	}
	// No re-clear here: putRedirectSettings is the single reset point, so a
	// recycled object is already zeroed. Clearing in exactly one place keeps
	// the reset contract obvious.
	return s
}

// putRedirectSettings returns a redirectSettings to the pool after resetting it.
// This is the single reset point for recycled settings objects (see
// getRedirectSettings).
// SECURITY: Clears all redirect URLs to prevent sensitive URL leakage.
func putRedirectSettings(s *redirectSettings) {
	if s == nil {
		return
	}
	// Reset all fields
	s.followRedirects = false
	s.maxRedirects = 0
	s.chainLen = 0
	// Clear inline chain to allow GC of strings. clear() zeroes the backing
	// array in one memclr, cheaper than a per-element loop on the hot path
	// (a redirectSettings is acquired per request).
	clear(s.inlineChain[:])
	// SECURITY: Clear overflow chain data to prevent memory leaks
	// Each URL string reference must be cleared to allow GC
	if s.overflowChain != nil {
		clear(s.overflowChain[:])
		s.overflowChain = nil
	}
	redirectSettingsPool.Put(s)
}

// transport manages HTTP transport with comprehensive security and optimal performance
type transport struct {
	transport         *http.Transport
	httpClient        *http.Client
	config            *Config
	allowPrivateIPs   bool                      // Cached for performance in redirect checks
	exemptNets        []*net.IPNet              // SSRF exempt CIDR ranges
	redirectWhitelist *security.DomainWhitelist // Whitelist for redirect domains
	// sensitiveHeaders is the effective cross-origin strip set: the built-in
	// redirectSensitiveHeaders plus Config.SensitiveHeaders (canonicalized,
	// deduped). Built in newTransport; read-only afterwards.
	sensitiveHeaders []string
	// fingerprintAdapter is non-nil when Security.TLSFingerprint is set; it
	// replaces httpClient.Transport for all requests. The pooled transport
	// above stays for CloseIdleConnections compatibility.
	fingerprintAdapter *fingerprint.Adapter
}

// Compile-time interface check
var _ transportManager = (*transport)(nil)

// newTransport creates a new transport manager with connection pool
func newTransport(config *Config, pool *connection.PoolManager) (*transport, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if pool == nil {
		return nil, fmt.Errorf("connection pool cannot be nil")
	}

	// Use the optimized transport from the connection pool
	httpTransport := pool.GetTransport()

	t := &transport{
		transport:         httpTransport,
		config:            config,
		allowPrivateIPs:   config.Connection.AllowPrivateIPs,
		exemptNets:        config.Connection.ExemptNets,
		redirectWhitelist: config.RedirectWhitelist,
		sensitiveHeaders:  mergeSensitiveHeaders(config.SensitiveHeaders),
	}

	// Create http.Client with optional cookie jar. GetRoundTripper installs
	// the per-proxy dispatcher when EnableProxyPoolHTTP2 is active (one
	// transport — and HTTP/2 pool — per proxy); otherwise it is the shared
	// transport above. t.transport keeps pointing at the shared transport for
	// CloseIdleConnections compatibility and tests.
	//
	// INVARIANT — never set Timeout on this Client (pinned by
	// TestHTTPClientNoTimeoutInvariant): a non-zero deadline makes the
	// stdlib's Client.send FORK the request (shallow copy sharing our pooled
	// http.Header map), and the fork flows back as resp.Request; the engine
	// then transfers that shared map into the Result while the deferred
	// putHTTPHeader recycles the original — the same pooled map is handed to
	// two owners and the next Build poisons the previous Result's headers.
	// The request deadline is owned by the engine's timeout budget
	// (wrapCtxIfFarther), not by the stdlib Client. The other stdlib fork
	// trigger (req.URL.User) is neutralized in Build (parsed.User = nil).
	httpClient := &http.Client{
		Transport: pool.GetRoundTripper(),
	}

	// Browser fingerprinting replaces the underlying transport: TLS
	// ClientHello, HTTP/2 frames, and the header block all come from the
	// preset. Everything above (this client's redirect policy, the cookie
	// overlay, retries) stays in charge — the adapter is a plain
	// http.RoundTripper beneath it. Certificate pinning rides along: the
	// adapter owns the utls.Config and runs the pinner as
	// VerifyPeerCertificate on every fingerprinted handshake.
	if config.TLSFingerprint != "" {
		preset, ok := fingerprint.ResolvePresetName(config.TLSFingerprint)
		if !ok {
			preset = config.TLSFingerprint // validated upstream; pass through
		}
		t.fingerprintAdapter = fingerprint.NewAdapter(fingerprint.Config{
			PresetName:            preset,
			InsecureSkipVerify:    config.Connection.InsecureSkipVerify,
			AllowPrivateIPs:       config.Connection.AllowPrivateIPs,
			EnableHTTP2:           config.Connection.EnableHTTP2,
			ExemptNets:            config.Connection.ExemptNets,
			CertPinner:            config.CertificatePinner,
			DisableSessionTickets: config.TLSFingerprintDisableSessionTickets,
			Pool:                  pool,
			DNSLookup:             pool.DoHLookup(),
		})
		httpClient.Transport = t.fingerprintAdapter
	}

	// Set cookie jar if enabled and provided
	if config.EnableCookies && config.Connection.CookieJar != nil {
		httpClient.Jar = config.Connection.CookieJar
	}

	// Set a single redirect policy that reads from context
	httpClient.CheckRedirect = t.checkRedirect

	t.httpClient = httpClient

	return t, nil
}

// checkRedirect is the single redirect policy that handles all requests
// It reads per-request settings from the context and validates redirect targets for SSRF
func (t *transport) checkRedirect(req *http.Request, via []*http.Request) error {
	// Get redirect settings from context
	settings, ok := req.Context().Value(redirectContextKey{}).(*redirectSettings)
	if !ok {
		// SECURITY: fail closed. Installing CheckRedirect REPLACES net/http's
		// default policy, so returning nil here would mean "follow, with no
		// redirect-count cap, no SSRF validation, no whitelist check, and no
		// cross-origin header stripping" — an unbounded SSRF bypass. Settings
		// are always injected by SetRedirectPolicy before RoundTrip, so this
		// branch is unreachable on current call paths; it exists to catch any
		// future path that rebuilds the context without them (a previous
		// version silently allowed the redirect here).
		return fmt.Errorf("redirect blocked: no redirect policy in request context (internal error)")
	}

	// Don't follow redirects if disabled
	if !settings.followRedirects {
		return http.ErrUseLastResponse
	}

	// SECURITY: Check redirect whitelist first
	if t.redirectWhitelist != nil {
		if !t.redirectWhitelist.IsAllowed(req.URL.Hostname()) {
			return fmt.Errorf("redirect blocked by whitelist: target '%s' is not allowed", req.URL.Hostname())
		}
	}

	// SECURITY: Validate redirect target for SSRF protection.
	// This prevents redirects to private/reserved IP addresses when SSRF protection is enabled.
	// A per-request AllowPrivateIPs override (from the WithAllowPrivateIPs option), carried on
	// the request context, takes precedence over the client-level policy so an explicitly
	// permitted local/internal request can still follow local redirects.
	allowPrivateIPs := t.allowPrivateIPs
	if override, ok := connection.AllowPrivateIPsOverrideFromContext(req.Context()); ok {
		allowPrivateIPs = override
	}
	if !allowPrivateIPs {
		if err := t.validateRedirectTarget(req.URL); err != nil {
			return fmt.Errorf("redirect blocked: %w", err)
		}
	}

	// Track redirect chain
	if len(via) > 0 {
		settings.addRedirect(via[len(via)-1].URL.String())
	}

	// SECURITY: Strip sensitive headers on cross-origin redirects to prevent
	// credential leakage. Origin follows RFC 6454: scheme, host, AND port
	// (scheme-default ports normalized, so example.com and example.com:80 are
	// one origin under http, while example.com:443 is not). Any scheme change
	// — including the https → http downgrade, where credentials would cross a
	// plaintext hop — is cross-origin; treating the http → https upgrade the
	// same way matches net/http's own redirect header-copying behavior.
	//
	// Authorization, Proxy-Authorization, and Cookie are also stripped by
	// net/http's own cross-origin redirect handling; stripping them here is
	// defense-in-depth and keeps the full sensitive set (including custom
	// credential headers like X-Api-Key) in one place.
	if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
		for _, h := range t.sensitiveHeaders {
			req.Header.Del(h)
		}
	}

	// SECURITY: Detect circular redirects to prevent infinite loops.
	// A circular redirect occurs when the target URL appeared earlier in the chain
	// but was reached from a DIFFERENT URL (true cycle). Same-URL repeats (A→A→A)
	// are excluded because the server may return different responses per visit.
	targetURL := req.URL.String()
	if len(via) >= 2 {
		for i := 0; i < len(via); i++ {
			if via[i].URL.String() == targetURL {
				// Exclude consecutive same-URL redirects (A→A)
				prevIdx := i - 1
				if prevIdx >= 0 && via[prevIdx].URL.String() == targetURL {
					continue
				}
				// Also exclude if the immediate predecessor (last via entry) is the target
				if via[len(via)-1].URL.String() == targetURL {
					continue
				}
				// SECURITY (SEC-001 M-1): the redirect target can carry
				// sensitive query parameters from the original URL; redact
				// before the message can reach logs.
				return fmt.Errorf("circular redirect detected: %s", validation.SanitizeURL(targetURL))
			}
		}
	}

	// Redirect limit. WithMaxRedirects requires 1..50 and the config default is
	// 10. Any value <= 0 — the "not explicitly set" sentinel, or a negative
	// value handed in by an internal caller — falls back to the default cap
	// of 10; there is no unlimited mode.
	if limit := settings.maxRedirects; limit > 0 {
		if len(via) >= limit {
			return fmt.Errorf("stopped after %d redirects", limit)
		}
	} else if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}

	return nil
}

// validateRedirectTarget checks if the redirect target URL is allowed under SSRF protection rules.
// This prevents attackers from using HTTP redirects to bypass initial SSRF validation.
//
// The connection pool's dialer (pool.go createDialer) also performs SSRF validation
// with DNS rebinding protection (resolves once, validates, dials IP directly). This
// redirect check provides an additional early-validation layer that blocks malicious
// redirects before the connection is even attempted.
func (t *transport) validateRedirectTarget(targetURL *url.URL) error {
	if targetURL == nil {
		return fmt.Errorf("nil redirect URL")
	}

	// EqualFold instead of lowering: URL schemes are ASCII, and this avoids
	// a per-redirect string allocation.
	if !strings.EqualFold(targetURL.Scheme, "http") && !strings.EqualFold(targetURL.Scheme, "https") {
		return fmt.Errorf("unsupported redirect scheme: %s", targetURL.Scheme)
	}

	host := targetURL.Hostname()
	if host == "" {
		return fmt.Errorf("empty host in redirect URL")
	}

	// Check SSRF without DNS resolution. The connection pool dialer (pool.go
	// createDialer → resolveAndValidateAddress) performs full SSRF validation
	// with DNS rebinding prevention (resolves once, validates, dials IP directly).
	// Skipping DNS here avoids a redundant resolution and a TOCTOU window.
	return validation.ValidateSSRFHost(host, t.exemptNets)
}

// redirectSensitiveHeaders are the request header names stripped on a
// cross-origin redirect. They mirror the facade's sensitive header set
// (httpc.result.go sensitiveHeaders): Authorization, Cookie, and
// Proxy-Authorization are stripped by net/http itself on cross-origin hops,
// but custom credential headers such as X-Api-Key and X-Auth-Token are not —
// the open-redirect + header-forwarding combination would otherwise leak them
// to a third-party origin. Keys are canonical (http.CanonicalHeaderKey) form.
//
// Config.SensitiveHeaders extends this set per client (see
// mergeSensitiveHeaders) so caller-declared credential headers get the same
// treatment.
var redirectSensitiveHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"X-Api-Key",
	"X-Auth-Token",
}

// mergeSensitiveHeaders returns the effective cross-origin strip set: the
// built-in redirectSensitiveHeaders plus extra (Config.SensitiveHeaders),
// canonicalized so Header.Del's canonical-form matching lines up, and deduped.
// extra == nil yields the built-in slice unchanged (zero allocation).
func mergeSensitiveHeaders(extra []string) []string {
	if len(extra) == 0 {
		return redirectSensitiveHeaders
	}
	merged := make([]string, 0, len(redirectSensitiveHeaders)+len(extra))
	merged = append(merged, redirectSensitiveHeaders...)
	for _, h := range extra {
		c := http.CanonicalHeaderKey(h)
		if !slices.Contains(merged, c) {
			merged = append(merged, c)
		}
	}
	return merged
}

// sameOrigin reports whether the two URLs share an RFC 6454 origin; the
// canonical implementation lives in validation.SameOrigin (shared with the
// fingerprint path). checkRedirect uses it to decide when
// Authorization/Cookie/Proxy-Authorization must be stripped; comparing
// hostname alone would treat "example.com:443 → example.com:8080" as
// same-origin and leak credentials across what is really a server change.
func sameOrigin(a, b *url.URL) bool {
	return validation.SameOrigin(a, b)
}

// redirectContextKey is a typed context key for redirect settings.
// Using a typed key avoids collisions with other context keys.
type redirectContextKey struct{}

// SetRedirectPolicy updates the redirect policy for a specific request.
// Returns a new context with the redirect settings.
//
// IMPORTANT: The returned settings MUST be returned to the pool after
// the request completes. Use defer to ensure cleanup:
//
//	ctx, settings := transport.SetRedirectPolicy(ctx, true, 5)
//	defer putRedirectSettings(settings)
//
// SECURITY: Failure to call putRedirectSettings will cause memory leaks and pool exhaustion.
func (t *transport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, *redirectSettings) {
	settings := getRedirectSettings()
	settings.followRedirects = followRedirects
	settings.maxRedirects = maxRedirects
	newCtx := context.WithValue(ctx, redirectContextKey{}, settings)
	return newCtx, settings
}

// RoundTrip executes an HTTP round trip
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Manually set cookies are layered onto the jar via a per-request
	// overlay instead of being written into the shared client jar: a
	// one-off WithCookies value must not persist across requests. The
	// overlay stays active for the whole round trip including redirects.
	client := t.httpClient
	if client.Jar != nil {
		// Parse the Cookie header once and share the result with the overlay
		// constructor — req.Cookies() re-parses on every call, so the old
		// len-check + construct sequence paid the full parse twice.
		if cookies := req.Cookies(); len(cookies) > 0 {
			overlay := newCookieOverlayJar(client.Jar, req, cookies)
			req.Header.Del("Cookie") // supplied per-hop by the overlay jar
			scoped := *client        // shallow copy; Transport and friends are shared
			scoped.Jar = overlay
			client = &scoped
		}
	}
	return client.Do(req)
}

// cookieOverlayJar layers request-scoped manual cookies on top of a shared
// base jar for the duration of one round trip. Manual cookies are matched by
// name against the original request's hostname and win over jar cookies with
// the same name. SetCookies forwards to the base jar only, so manual cookies
// are never persisted client-wide.
type cookieOverlayJar struct {
	base   http.CookieJar
	manual []*http.Cookie
	origin string // hostname the manual cookies were set for
}

// newCookieOverlayJar builds the overlay from an already-parsed cookie list
// (callers parse the Cookie header once and pass the result in).
func newCookieOverlayJar(base http.CookieJar, req *http.Request, requestCookies []*http.Cookie) *cookieOverlayJar {
	manual := make([]*http.Cookie, 0, len(requestCookies))
	for _, c := range requestCookies {
		cc := *c
		manual = append(manual, &cc)
	}
	return &cookieOverlayJar{
		base:   base,
		manual: manual,
		origin: req.URL.Hostname(),
	}
}

// Cookies returns the base jar's cookies with manual cookies layered on top
// (manual wins by name) when the target hostname matches the overlay's
// origin. Redirects to other hosts do not receive the manual cookies.
func (j *cookieOverlayJar) Cookies(u *url.URL) []*http.Cookie {
	merged := j.base.Cookies(u)
	if !strings.EqualFold(u.Hostname(), j.origin) {
		return merged
	}
	out := make([]*http.Cookie, 0, len(merged)+len(j.manual))
	out = append(out, merged...)
	for _, m := range j.manual {
		replaced := false
		for i := range out {
			if out[i].Name == m.Name {
				out[i] = m
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, m)
		}
	}
	return out
}

// SetCookies forwards to the base jar only; the overlay itself is read-only.
func (j *cookieOverlayJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.base.SetCookies(u, cookies)
}

// Close closes the transport and cleans up resources
func (t *transport) Close() error {
	if t.fingerprintAdapter != nil {
		_ = t.fingerprintAdapter.Close() // best-effort backend cleanup
	}
	if t.transport != nil {
		t.transport.CloseIdleConnections()
	}
	return nil
}
