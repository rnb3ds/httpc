package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/cybergodev/httpc/internal/connection"
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
// Returns a copy to prevent mutation.
func (s *redirectSettings) getChain() []string {
	if s.chainLen == 0 {
		return nil
	}
	chain := make([]string, s.chainLen)
	if s.overflowChain != nil {
		copy(chain, s.overflowChain)
	} else {
		copy(chain, s.inlineChain[:s.chainLen])
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

// cookieMapPool reduces allocations for cookie merging maps.
// Used in RoundTrip when merging request cookies with jar cookies.
var cookieMapPool = sync.Pool{
	New: func() any {
		m := make(map[string]*http.Cookie, 8)
		return &m
	},
}

// cookieSlicePool reduces allocations for cookie slices.
var cookieSlicePool = sync.Pool{
	New: func() any {
		s := make([]*http.Cookie, 0, 8)
		return &s
	},
}

// getRedirectSettings retrieves a redirectSettings from the pool.
func getRedirectSettings() *redirectSettings {
	s, ok := redirectSettingsPool.Get().(*redirectSettings)
	if !ok || s == nil {
		return &redirectSettings{}
	}
	return s
}

// putRedirectSettings returns a redirectSettings to the pool after resetting it.
// SECURITY: Clears all redirect URLs to prevent sensitive URL leakage.
func putRedirectSettings(s *redirectSettings) {
	if s == nil {
		return
	}
	// Reset all fields
	s.followRedirects = false
	s.maxRedirects = 0
	s.chainLen = 0
	// Clear inline chain to allow GC of strings
	for i := range s.inlineChain {
		s.inlineChain[i] = ""
	}
	// SECURITY: Clear overflow chain data to prevent memory leaks
	// Each URL string reference must be cleared to allow GC
	if s.overflowChain != nil {
		for i := range s.overflowChain {
			s.overflowChain[i] = ""
		}
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
		allowPrivateIPs:   config.AllowPrivateIPs,
		exemptNets:        config.ExemptNets,
		redirectWhitelist: config.RedirectWhitelist,
	}

	// Create http.Client with optional cookie jar
	httpClient := &http.Client{
		Transport: httpTransport,
	}

	// Set cookie jar if enabled and provided
	if config.EnableCookies && config.CookieJar != nil {
		httpClient.Jar = config.CookieJar
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
		// No settings in context, use defaults
		return nil
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

	// SECURITY: Validate redirect target for SSRF protection
	// This prevents redirects to private/reserved IP addresses when SSRF protection is enabled
	if !t.allowPrivateIPs {
		if err := t.validateRedirectTarget(req.URL); err != nil {
			return fmt.Errorf("redirect blocked: %w", err)
		}
	}

	// Track redirect chain
	if len(via) > 0 {
		settings.addRedirect(via[len(via)-1].URL.String())
	}

	// SECURITY: Strip sensitive headers on cross-origin redirects to prevent
	// credential leakage. When the redirect target host differs from the original
	// request host, remove Authorization, Cookie, and Proxy-Authorization headers.
	if len(via) > 0 && req.URL.Hostname() != via[0].URL.Hostname() {
		req.Header.Del("Authorization")
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Cookie")
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
				return fmt.Errorf("circular redirect detected: %s", targetURL)
			}
		}
	}

	// Check redirect limit (0 means unlimited)
	if settings.maxRedirects > 0 && len(via) > settings.maxRedirects {
		return fmt.Errorf("stopped after %d redirects", settings.maxRedirects)
	}

	// Default Go limit is 10, we respect that if maxRedirects is 0
	if settings.maxRedirects == 0 && len(via) >= 10 {
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

	scheme := strings.ToLower(targetURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported redirect scheme: %s", targetURL.Scheme)
	}

	host := targetURL.Hostname()
	if host == "" {
		return fmt.Errorf("empty host in redirect URL")
	}

	// Resolve DNS for redirect targets to prevent SSRF via DNS-rebinding.
	// The connection pool dialer provides a second layer of defense.
	return validation.ValidateSSRFHost(host, t.exemptNets, true)
}

// redirectContextKey is a typed context key for redirect settings.
// Using a typed key avoids collisions with other context keys.
type redirectContextKey struct{}

// SetRedirectPolicy updates the redirect policy for a specific request.
// Returns a new context with the redirect settings and a cleanup function.
//
// IMPORTANT: The returned cleanup function MUST be called after the request completes
// to return settings to the pool. Use defer to ensure cleanup:
//
//	ctx, cleanup := transport.SetRedirectPolicy(ctx, true, 5)
//	defer cleanup()
//
// SECURITY: Failure to call cleanup will cause memory leaks and pool exhaustion.
func (t *transport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, *redirectSettings) {
	settings := getRedirectSettings()
	settings.followRedirects = followRedirects
	settings.maxRedirects = maxRedirects
	newCtx := context.WithValue(ctx, redirectContextKey{}, settings)
	return newCtx, settings
}

// GetRedirectChain returns the redirect chain from the context
func (t *transport) GetRedirectChain(ctx context.Context) []string {
	settings, ok := ctx.Value(redirectContextKey{}).(*redirectSettings)
	if !ok || settings.chainLen == 0 {
		return nil
	}
	return settings.getChain()
}

// RoundTrip executes an HTTP round trip
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The http.Client with Jar handles cookies automatically
	// If there are manually set cookies, merge them with the jar
	if t.httpClient.Jar != nil {
		if requestCookies := req.Cookies(); len(requestCookies) > 0 {
			existingCookies := t.httpClient.Jar.Cookies(req.URL)

			// Use pooled cookie map to reduce allocations
			cookieMapPtr, ok := cookieMapPool.Get().(*map[string]*http.Cookie)
			if !ok || cookieMapPtr == nil {
				m := make(map[string]*http.Cookie, len(existingCookies)+len(requestCookies))
				cookieMapPtr = &m
			}
			cookieMap := *cookieMapPtr

			// Clear the map for reuse
			for k := range cookieMap {
				delete(cookieMap, k)
			}

			for _, c := range existingCookies {
				cookieMap[c.Name] = c
			}

			for _, c := range requestCookies {
				cookieCopy := *c
				if cookieCopy.Domain == "" {
					cookieCopy.Domain = req.URL.Hostname()
				}
				if cookieCopy.Path == "" {
					cookieCopy.Path = "/"
				}
				cookieMap[cookieCopy.Name] = &cookieCopy
			}

			// Use pooled slice for merged cookies
			mergedPtr, ok := cookieSlicePool.Get().(*[]*http.Cookie)
			if !ok || mergedPtr == nil {
				s := make([]*http.Cookie, 0, len(cookieMap))
				mergedPtr = &s
			}
			mergedCookies := (*mergedPtr)[:0]

			for _, c := range cookieMap {
				mergedCookies = append(mergedCookies, c)
			}

			t.httpClient.Jar.SetCookies(req.URL, mergedCookies)
			req.Header.Del("Cookie")

			// SECURITY: Clear sensitive cookie data before returning to pool
			// Use defer to ensure cleanup even if subsequent operations panic
			defer func() {
				// Clear the map to prevent data leakage between requests
				for k := range cookieMap {
					delete(cookieMap, k)
				}
				// SECURITY: Clear each Cookie's sensitive fields to prevent cross-request data leakage
				// The http.Cookie objects are retained in memory until overwritten, so we must
				// explicitly clear their Value, Domain, and Path fields.
				for i := range *mergedPtr {
					if (*mergedPtr)[i] != nil {
						(*mergedPtr)[i].Value = ""
						(*mergedPtr)[i].Domain = ""
						(*mergedPtr)[i].Path = ""
						(*mergedPtr)[i].RawExpires = ""
						(*mergedPtr)[i].Raw = ""
					}
				}
				// Clear the slice but keep capacity for reuse
				*mergedPtr = (*mergedPtr)[:0]

				// Return slices to pool (now cleared of sensitive data)
				cookieMapPool.Put(cookieMapPtr)
				cookieSlicePool.Put(mergedPtr)
			}()
		}
	}

	return t.httpClient.Do(req)
}

// Close closes the transport and cleans up resources
func (t *transport) Close() error {
	if t.transport != nil {
		t.transport.CloseIdleConnections()
	}
	return nil
}

// clearPools clears all sync.Pool instances used by the transport package.
// This is primarily useful for testing and debugging to ensure a clean state.
// Note: sync.Pool is automatically managed by the GC, so this is typically not needed
// in production code. The pools will be repopulated on next use.
func clearPools() {
	// Clear redirect settings pool by creating fresh pool entries
	// The old entries will be garbage collected
	redirectSettingsPool = sync.Pool{
		New: func() any {
			return &redirectSettings{}
		},
	}
	// Clear cookie map pool
	cookieMapPool = sync.Pool{
		New: func() any {
			m := make(map[string]*http.Cookie, 8)
			return &m
		},
	}
	// Clear cookie slice pool
	cookieSlicePool = sync.Pool{
		New: func() any {
			s := make([]*http.Cookie, 0, 8)
			return &s
		},
	}
}
