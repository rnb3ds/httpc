package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/validation"
)

// redirectKey is the context key for storing per-request redirect settings
type redirectKey struct{}

// redirectSettings holds per-request redirect configuration
type redirectSettings struct {
	followRedirects bool
	maxRedirects    int
	chain           []string
}

// Transport manages HTTP transport with comprehensive security and optimal performance
type Transport struct {
	transport     *http.Transport
	httpClient    *http.Client
	config        *Config
	allowPrivateIPs bool // Cached for performance in redirect checks
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
		transport:       transport,
		config:          config,
		allowPrivateIPs: config.AllowPrivateIPs,
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
	settings, ok := req.Context().Value(redirectKey{}).(*redirectSettings)
	if !ok {
		// No settings in context, use defaults
		return nil
	}

	// Don't follow redirects if disabled
	if !settings.followRedirects {
		return http.ErrUseLastResponse
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
		settings.chain = append(settings.chain, via[len(via)-1].URL.String())
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

// SetRedirectPolicy updates the redirect policy for a specific request
// Returns a new context with the redirect settings
func (t *Transport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) context.Context {
	settings := &redirectSettings{
		followRedirects: followRedirects,
		maxRedirects:    maxRedirects,
		chain:           make([]string, 0, 10),
	}
	return context.WithValue(ctx, redirectKey{}, settings)
}

// GetRedirectChain returns the redirect chain from the context
func (t *Transport) GetRedirectChain(ctx context.Context) []string {
	settings, ok := ctx.Value(redirectKey{}).(*redirectSettings)
	if !ok || len(settings.chain) == 0 {
		return nil
	}
	chain := make([]string, len(settings.chain))
	copy(chain, settings.chain)
	return chain
}

// RoundTrip executes an HTTP round trip
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The http.Client with Jar handles cookies automatically
	// If there are manually set cookies, merge them with the jar
	if t.httpClient.Jar != nil {
		if requestCookies := req.Cookies(); len(requestCookies) > 0 {
			existingCookies := t.httpClient.Jar.Cookies(req.URL)
			cookieMap := make(map[string]*http.Cookie, len(existingCookies)+len(requestCookies))

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

			mergedCookies := make([]*http.Cookie, 0, len(cookieMap))
			for _, c := range cookieMap {
				mergedCookies = append(mergedCookies, c)
			}

			t.httpClient.Jar.SetCookies(req.URL, mergedCookies)
			req.Header.Del("Cookie")
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
