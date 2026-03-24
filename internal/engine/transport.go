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
		s.overflowChain = s.overflowChain[:0]
	}
	redirectSettingsPool.Put(s)
}

// Transport manages HTTP transport with comprehensive security and optimal performance
type Transport struct {
	transport         *http.Transport
	httpClient        *http.Client
	config            *Config
	allowPrivateIPs   bool                      // Cached for performance in redirect checks
	redirectWhitelist *security.DomainWhitelist // Whitelist for redirect domains
}

// Compile-time interface check
var _ TransportManager = (*Transport)(nil)

// NewTransport creates a new transport manager with connection pool
func NewTransport(config *Config, pool *connection.PoolManager) (*Transport, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if pool == nil {
		return nil, fmt.Errorf("connection pool cannot be nil")
	}

	// Use the optimized transport from the connection pool
	transport := pool.GetTransport()

	t := &Transport{
		transport:         transport,
		config:            config,
		allowPrivateIPs:   config.AllowPrivateIPs,
		redirectWhitelist: config.RedirectWhitelist,
	}

	// Create http.Client with optional cookie jar
	httpClient := &http.Client{
		Transport: transport,
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
func (t *Transport) checkRedirect(req *http.Request, via []*http.Request) error {
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

	// Check redirect limit (0 means unlimited)
	if settings.maxRedirects > 0 && len(via) >= settings.maxRedirects {
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
func (t *Transport) validateRedirectTarget(targetURL *url.URL) error {
	if targetURL == nil {
		return fmt.Errorf("nil redirect URL")
	}

	host := targetURL.Hostname()
	if host == "" {
		return fmt.Errorf("empty host in redirect URL")
	}

	// Check for localhost variations
	if validation.IsLocalhost(host) {
		return fmt.Errorf("localhost access blocked")
	}

	// If host is an IP address, validate it directly
	if ip := net.ParseIP(host); ip != nil {
		if err := validation.ValidateIP(ip); err != nil {
			return fmt.Errorf("private/reserved IP blocked: %s", ip.String())
		}
		return nil
	}

	// For domain names, resolve and check all IPs
	// This provides protection against DNS rebinding attacks
	ips, err := net.LookupIP(host)
	if err != nil {
		// SECURITY: Block on DNS resolution failure to prevent DNS-based bypasses
		return fmt.Errorf("DNS resolution failed for redirect target: %w", err)
	}

	// Check all resolved IPs - if any point to a private/reserved address, block it
	for _, ip := range ips {
		if err := validation.ValidateIP(ip); err != nil {
			return fmt.Errorf("redirect domain resolves to blocked address: %w", err)
		}
	}

	// SECURITY: Check URL scheme - only allow http and https
	scheme := strings.ToLower(targetURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported redirect scheme: %s", targetURL.Scheme)
	}

	return nil
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
func (t *Transport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, func()) {
	settings := getRedirectSettings()
	settings.followRedirects = followRedirects
	settings.maxRedirects = maxRedirects
	newCtx := context.WithValue(ctx, redirectContextKey{}, settings)

	// Return cleanup function that captures the settings reference
	cleanup := func() {
		putRedirectSettings(settings)
	}

	return newCtx, cleanup
}

// GetRedirectChain returns the redirect chain from the context
func (t *Transport) GetRedirectChain(ctx context.Context) []string {
	settings, ok := ctx.Value(redirectContextKey{}).(*redirectSettings)
	if !ok || settings.chainLen == 0 {
		return nil
	}
	return settings.getChain()
}

// RoundTrip executes an HTTP round trip
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The http.Client with Jar handles cookies automatically
	// If there are manually set cookies, merge them with the jar
	if t.httpClient.Jar != nil {
		if requestCookies := req.Cookies(); len(requestCookies) > 0 {
			existingCookies := t.httpClient.Jar.Cookies(req.URL)

			// Use pooled cookie map to reduce allocations
			cookieMapPtr, _ := cookieMapPool.Get().(*map[string]*http.Cookie)
			if cookieMapPtr == nil {
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
			mergedPtr, _ := cookieSlicePool.Get().(*[]*http.Cookie)
			if mergedPtr == nil {
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
func (t *Transport) Close() error {
	if t.transport != nil {
		t.transport.CloseIdleConnections()
	}
	return nil
}

// ClearPools clears all sync.Pool instances used by the transport package.
// This is primarily useful for testing and debugging to ensure a clean state.
// Note: sync.Pool is automatically managed by the GC, so this is typically not needed
// in production code. The pools will be repopulated on next use.
func ClearPools() {
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
