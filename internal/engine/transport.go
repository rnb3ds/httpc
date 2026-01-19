package engine

import (
	"fmt"
	"net/http"

	"github.com/cybergodev/httpc/internal/connection"
)

// Transport manages HTTP transport with comprehensive security and optimal performance
type Transport struct {
	transport     *http.Transport
	httpClient    *http.Client
	config        *Config
	redirectChain []string
}

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
		transport:     transport,
		config:        config,
		redirectChain: make([]string, 0, 10),
	}

	// Create http.Client with optional cookie jar
	httpClient := &http.Client{
		Transport: transport,
	}

	// Set cookie jar if enabled and provided
	if config.EnableCookies && config.CookieJar != nil {
		if jar, ok := config.CookieJar.(http.CookieJar); ok {
			httpClient.Jar = jar
		}
	}

	// Configure redirect policy with tracking
	httpClient.CheckRedirect = t.createRedirectPolicy(config.FollowRedirects, config.MaxRedirects)

	t.httpClient = httpClient

	return t, nil
}

// createRedirectPolicy creates a redirect policy function with tracking
func (t *Transport) createRedirectPolicy(followRedirects bool, maxRedirects int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		// Don't follow redirects if disabled
		if !followRedirects {
			return http.ErrUseLastResponse
		}

		// Track redirect chain
		if len(via) > 0 {
			t.redirectChain = append(t.redirectChain, via[len(via)-1].URL.String())
		}

		// Check redirect limit (0 means unlimited)
		if maxRedirects > 0 && len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}

		// Default Go limit is 10, we respect that if maxRedirects is 0
		if maxRedirects == 0 && len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}

		return nil
	}
}

// SetRedirectPolicy updates the redirect policy for a specific request
func (t *Transport) SetRedirectPolicy(followRedirects bool, maxRedirects int) {
	t.redirectChain = make([]string, 0, 10)
	t.httpClient.CheckRedirect = t.createRedirectPolicy(followRedirects, maxRedirects)
}

// GetRedirectChain returns the redirect chain and resets it
func (t *Transport) GetRedirectChain() []string {
	chain := make([]string, len(t.redirectChain))
	copy(chain, t.redirectChain)
	t.redirectChain = t.redirectChain[:0]
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
