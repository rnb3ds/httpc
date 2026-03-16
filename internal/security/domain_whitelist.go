package security

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// DomainWhitelist manages a list of allowed domains for redirect validation.
// It supports exact matches and wildcard patterns (e.g., *.example.com).
type DomainWhitelist struct {
	mu        sync.RWMutex
	exact     map[string]bool
	wildcards []string
}

// NewDomainWhitelist creates a new DomainWhitelist from a list of domains.
// Domains can be:
//   - Exact matches: "example.com", "api.example.com"
//   - Wildcard patterns: "*.example.com" matches any subdomain of example.com
//
// Example:
//
//	whitelist := security.NewDomainWhitelist("example.com", "*.trusted.org")
func NewDomainWhitelist(domains ...string) *DomainWhitelist {
	dw := &DomainWhitelist{
		exact:     make(map[string]bool),
		wildcards: make([]string, 0),
	}

	for _, domain := range domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}

		if strings.HasPrefix(domain, "*.") {
			// Wildcard pattern: store without the "*." prefix for matching
			pattern := domain[2:]
			if pattern != "" {
				dw.wildcards = append(dw.wildcards, pattern)
			}
		} else {
			// Exact match
			dw.exact[domain] = true
		}
	}

	return dw
}

// IsAllowed checks if a hostname is in the whitelist.
// The hostname is checked against exact matches first, then wildcards.
// Returns true if the hostname is allowed, false otherwise.
func (w *DomainWhitelist) IsAllowed(hostname string) bool {
	if w == nil {
		return true // No whitelist means all domains are allowed
	}

	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return false
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	// Check exact match first
	if w.exact[hostname] {
		return true
	}

	// Check wildcard patterns
	for _, pattern := range w.wildcards {
		if w.matchWildcard(hostname, pattern) {
			return true
		}
	}

	return false
}

// matchWildcard checks if a hostname matches a wildcard pattern.
// pattern should be the domain part after "*." (e.g., "example.com")
func (w *DomainWhitelist) matchWildcard(hostname, pattern string) bool {
	// Exact match with wildcard domain
	if hostname == pattern {
		return true
	}

	// Subdomain match: hostname ends with .pattern
	if strings.HasSuffix(hostname, "."+pattern) {
		return true
	}

	return false
}

// Add adds a domain to the whitelist.
// Thread-safe: can be called while the whitelist is in use.
func (w *DomainWhitelist) Add(domain string) {
	if w == nil {
		return
	}

	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if strings.HasPrefix(domain, "*.") {
		pattern := domain[2:]
		if pattern != "" {
			w.wildcards = append(w.wildcards, pattern)
		}
	} else {
		w.exact[domain] = true
	}
}

// Remove removes a domain from the whitelist.
// Thread-safe: can be called while the whitelist is in use.
func (w *DomainWhitelist) Remove(domain string) {
	if w == nil {
		return
	}

	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if strings.HasPrefix(domain, "*.") {
		pattern := domain[2:]
		// Remove from wildcards
		newWildcards := make([]string, 0, len(w.wildcards))
		for _, p := range w.wildcards {
			if p != pattern {
				newWildcards = append(newWildcards, p)
			}
		}
		w.wildcards = newWildcards
	} else {
		delete(w.exact, domain)
	}
}

// Domains returns a copy of all domains in the whitelist.
// Returns two slices: exact matches and wildcard patterns.
func (w *DomainWhitelist) Domains() (exact []string, wildcards []string) {
	if w == nil {
		return nil, nil
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	// Copy exact matches
	exact = make([]string, 0, len(w.exact))
	for domain := range w.exact {
		exact = append(exact, domain)
	}

	// Copy wildcards
	wildcards = make([]string, len(w.wildcards))
	copy(wildcards, w.wildcards)

	return exact, wildcards
}

// ValidateRedirectWhitelist validates that a redirect target URL is allowed
// based on the provided whitelist.
// Returns an error if the redirect target hostname is not in the whitelist.
func ValidateRedirectWhitelist(targetURL *url.URL, whitelist *DomainWhitelist) error {
	if whitelist == nil {
		return nil // No whitelist means all redirects are allowed
	}

	if targetURL == nil {
		return fmt.Errorf("redirect URL is nil")
	}

	hostname := targetURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("redirect URL has empty hostname")
	}

	if !whitelist.IsAllowed(hostname) {
		return fmt.Errorf("redirect to '%s' is not in the whitelist", hostname)
	}

	return nil
}
