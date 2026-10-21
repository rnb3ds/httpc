package security

import (
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/net/idna"

	"github.com/cybergodev/httpc/internal/validation"
)

// DomainWhitelist manages a list of allowed domains for redirect validation.
// It supports exact matches and wildcard patterns (e.g., *.example.com).
type DomainWhitelist struct {
	mu        sync.RWMutex
	exact     map[string]bool
	wildcards []string
}

// normalizeDomain normalizes a domain for consistent comparison.
// Performs single-pass ASCII whitespace trimming and lowercase conversion
// to avoid intermediate string allocations from chained TrimSpace+ToLower.
func normalizeDomain(domain string) string {
	// Fast path: empty string
	if len(domain) == 0 {
		return ""
	}

	// Find leading/trailing whitespace and check if lowercase needed
	start := 0
	end := len(domain)
	needsLower := false

	// Skip leading whitespace (ASCII only for performance)
	for start < end && (domain[start] == ' ' || domain[start] == '\t' || domain[start] == '\n' || domain[start] == '\r') {
		start++
	}

	// Skip trailing whitespace
	for end > start && (domain[end-1] == ' ' || domain[end-1] == '\t' || domain[end-1] == '\n' || domain[end-1] == '\r') {
		end--
	}

	// Strip the fully-qualified trailing dot: "example.com." is a legal FQDN
	// spelling of "example.com" and must match a whitelist entry without the
	// dot (the root "." alone is left untouched).
	if end-start > 1 && domain[end-1] == '.' {
		end--
	}

	// Check if any uppercase letters exist
	for i := start; i < end; i++ {
		if domain[i] >= 'A' && domain[i] <= 'Z' {
			needsLower = true
			break
		}
	}

	// Fast path: no changes needed
	if start == 0 && end == len(domain) && !needsLower {
		return domain
	}

	// Build normalized string
	result := domain[start:end]
	if needsLower {
		return validation.ASCIILower(result)
	}
	return result
}

// normalizeDomainForCompare maps a configured or observed domain to its
// ASCII comparison form: normalizeDomain (trim, lowercase, trailing-dot
// strip) plus IDNA mapping, so a Unicode entry ("münchen.de") matches the
// xn-- punycode form servers emit in Location headers. A wildcard prefix is
// preserved. Conversion failure falls back to the trimmed form — an
// unconvertible name simply never matches (fail-closed).
func normalizeDomainForCompare(domain string) string {
	prefix := ""
	if rest, ok := strings.CutPrefix(domain, "*."); ok {
		prefix, domain = "*.", rest
	}
	domain = normalizeDomain(domain)
	if domain == "" {
		return ""
	}
	if !isASCII(domain) {
		if ascii, err := idna.Lookup.ToASCII(domain); err == nil {
			domain = ascii
		}
	}
	return prefix + domain
}

// isASCII reports whether s consists entirely of ASCII bytes.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// NewDomainWhitelist creates a new DomainWhitelist from a list of domains.
// Domains can be:
//   - Exact matches: "example.com", "api.example.com"
//   - Wildcard patterns: "*.example.com" matches any strict subdomain of
//     example.com (e.g. api.example.com), but NOT the bare apex example.com.
//     To allow both the apex and its subdomains, list both explicitly:
//     "example.com", "*.example.com".
//
// INVARIANT (deliberate, fail-closed): calling with NO domains returns a
// whitelist that DENIES every host — an empty whitelist means "nothing is
// allowed", not "everything is allowed". This differs from a nil
// *DomainWhitelist receiver, whose IsAllowed returns true for all hosts
// ("no whitelist configured = no restriction"). The engine only installs a
// whitelist when the config lists at least one domain (config_convert.go),
// preserving both semantics.
//
// Example:
//
//	whitelist := security.NewDomainWhitelist("example.com", "*.trusted.org")
func NewDomainWhitelist(domains ...string) *DomainWhitelist {
	n := len(domains)

	// Count wildcards for accurate pre-allocation
	wildcardCount := 0
	for _, domain := range domains {
		if strings.HasPrefix(domain, "*.") {
			wildcardCount++
		}
	}
	exactCap := n - wildcardCount
	wildcardCap := wildcardCount
	if wildcardCap == 0 {
		wildcardCap = 1 // minimum capacity 1
	}

	dw := &DomainWhitelist{
		exact:     make(map[string]bool, exactCap),
		wildcards: make([]string, 0, wildcardCap),
	}

	for _, domain := range domains {
		domain = normalizeDomainForCompare(domain)
		if domain == "" {
			continue
		}

		if strings.HasPrefix(domain, "*.") {
			// Wildcard pattern: store with "." prefix for zero-alloc HasSuffix matching
			pattern := domain[1:] // "*." becomes "."
			if len(pattern) > 1 {
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

	hostname = normalizeDomainForCompare(hostname)
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
// pattern includes the "." prefix (e.g., ".example.com") for zero-alloc matching.
// The wildcard matches strict subdomains only: "*.example.com" matches
// "api.example.com" but not the bare apex "example.com". The leading "." in
// pattern guarantees the suffix aligns to a label boundary, so "notexample.com"
// cannot match ".example.com".
func (w *DomainWhitelist) matchWildcard(hostname, pattern string) bool {
	// Subdomain match: hostname ends with .pattern (e.g. "api.example.com" / ".example.com")
	return strings.HasSuffix(hostname, pattern)
}

// Add adds a domain to the whitelist.
// Thread-safe: can be called while the whitelist is in use.
// Currently exercised only by tests; retained for a future public API.
func (w *DomainWhitelist) Add(domain string) {
	if w == nil {
		return
	}

	domain = normalizeDomainForCompare(domain)
	if domain == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if strings.HasPrefix(domain, "*.") {
		pattern := domain[1:] // "*." becomes "."
		if len(pattern) > 1 {
			for _, existing := range w.wildcards {
				if existing == pattern {
					return
				}
			}
			w.wildcards = append(w.wildcards, pattern)
		}
	} else {
		w.exact[domain] = true
	}
}

// Remove removes a domain from the whitelist.
// Thread-safe: can be called while the whitelist is in use.
// Currently exercised only by tests; retained for a future public API.
func (w *DomainWhitelist) Remove(domain string) {
	if w == nil {
		return
	}

	domain = normalizeDomainForCompare(domain)
	if domain == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if strings.HasPrefix(domain, "*.") {
		pattern := domain[1:] // "*." becomes "."
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
// Currently exercised only by tests; retained for a future public API.
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
