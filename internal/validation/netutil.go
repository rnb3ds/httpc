package validation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// isPrivateOrReservedIP checks if an IP address is private, reserved, or
// otherwise not suitable for public internet communication.
// This is used for SSRF (Server-Side Request Forgery) protection.
//
// SECURITY: This function handles IPv4-mapped IPv6 addresses (::ffff:x.x.x.x)
// to prevent bypass attempts using mixed notation.
func isPrivateOrReservedIP(ip net.IP) bool {
	// Normalize: handle IPv4-mapped IPv6 addresses (::ffff:127.0.0.1)
	// This prevents SSRF bypass using mixed IPv4/IPv6 notation
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// IPv4 specific checks
	if ip4 := ip.To4(); ip4 != nil {
		// Check for reserved IP ranges
		if ip4[0] >= 240 || // Class E (240.0.0.0/4) - Reserved
			ip4[0] == 0 || // "This" Network (0.0.0.0/8)
			(ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127) || // Carrier-Grade NAT (RFC 6598) 100.64.0.0/10
			(ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0) || // IETF Protocol Assignments (192.0.0.0/24)
			(ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) || // Documentation TEST-NET-1 (192.0.2.0/24)
			(ip4[0] == 192 && ip4[1] == 88 && ip4[2] == 99) || // 6to4 Relay Anycast (192.88.99.0/24)
			(ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) || // Documentation TEST-NET-2 (198.51.100.0/24)
			(ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) { // Documentation TEST-NET-3 (203.0.113.0/24)
			return true
		}
		// NOTE: 198.18.0.0/15 (RFC 2544 benchmarking) is deliberately allowed:
		// Clash/Surge-style proxy tools allocate their fake-IP pool there, and
		// blocking it would break those setups (see netutil_test).
		return false
	}

	// IPv6 specific checks (only for true IPv6 addresses, not mapped IPv4)
	if len(ip) == 16 {
		// Documentation prefix 2001:db8::/32 (RFC 3849)
		if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
			return true
		}
		// Teredo 2001::/32 (RFC 4380): the last 4 bytes carry the client's
		// IPv4 address in obfuscated (complemented) form, so ANY target —
		// including 127.0.0.1 — can be hidden there. Checking the embedded
		// address is unreliable (the complement of an arbitrary public IP
		// quasi-randomly lands in reserved space), so the whole prefix is
		// blocked, matching common SSRF filter practice. Teredo is obsolete
		// (removed from Windows 10+), making the false-positive cost negligible.
		if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
			return true
		}
		// Site-local fec0::/10 (deprecated but still routable in some nets)
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0xc0 {
			return true
		}
		// NAT64 well-known prefix 64:ff9b::/96 (RFC 6052)
		// Validates embedded IPv4 to prevent SSRF bypass via IPv6.
		if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b &&
			ip[4] == 0 && ip[5] == 0 && ip[6] == 0 && ip[7] == 0 &&
			ip[8] == 0 && ip[9] == 0 && ip[10] == 0 && ip[11] == 0 {
			embeddedIP := net.IPv4(ip[12], ip[13], ip[14], ip[15])
			return isPrivateOrReservedIP(embeddedIP)
		}
		// Local-Use NAT64 prefix 64:ff9b:1::/48 (RFC 8215): unlike the
		// well-known /96 above, this prefix is explicitly allowed to carry
		// translations of RFC 1918 and other non-global addresses, and its
		// embedded IPv4 does not sit at a fixed offset — block the whole
		// prefix, the same treatment fec0::/10 gets.
		if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b &&
			ip[4] == 0x00 && ip[5] == 0x01 {
			return true
		}
		// 6to4 2002::/16 (RFC 7526, deprecated): bytes 2-5 embed the public
		// IPv4 target of the relay, so a private target (e.g. 2002:7f00:1::
		// = 127.0.0.1) hides inside a globally-routed prefix. Validate the
		// embedded address, mirroring the NAT64 handling above — the same
		// embedded-IPv4 SSRF bypass the Teredo/NAT64 comments describe.
		if ip[0] == 0x20 && ip[1] == 0x02 {
			embeddedIP := net.IPv4(ip[2], ip[3], ip[4], ip[5])
			return isPrivateOrReservedIP(embeddedIP)
		}
		// IPv4-compatible ::/96 (::a.b.c.d) and IPv4-translated
		// ::ffff:0:0/96 (::ffff:0:a.b.c.d), both deprecated, embed an IPv4
		// target the To4 normalization above does not unwrap (To4 only
		// recognizes the ::ffff:a.b.c.d mapped form) — the same
		// embedded-target bypass the NAT64/6to4 blocks guard against, and
		// inconsistent to leave open while those are closed. Unwrap and
		// re-validate the embedded IPv4. (::1 and the unspecified address
		// were already caught by the checks above.)
		if ip[10] == 0 && ip[11] == 0 && allZero(ip[0:10]) {
			return isPrivateOrReservedIP(net.IPv4(ip[12], ip[13], ip[14], ip[15]))
		}
		if ip[8] == 0xff && ip[9] == 0xff && ip[10] == 0 && ip[11] == 0 && allZero(ip[0:8]) {
			return isPrivateOrReservedIP(net.IPv4(ip[12], ip[13], ip[14], ip[15]))
		}
	}

	return false
}

// allZero reports whether every byte in b is zero.
func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// isIPExempted checks if an IP address matches any of the exempt CIDR ranges.
func isIPExempted(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateIPWithExemptions checks if an IP is allowed, considering exempt CIDR ranges.
// IPs that are private/reserved but match an exempt CIDR are allowed.
func ValidateIPWithExemptions(ip net.IP, exemptNets []*net.IPNet) error {
	if isPrivateOrReservedIP(ip) && !isIPExempted(ip, exemptNets) {
		return fmt.Errorf("blocked IP address")
	}
	return nil
}

// ResolveFilteredIPs is the single implementation of the SSRF
// resolve-and-filter rule, shared by every enforcement layer: the standard
// dial path and the proxied-target pre-check (internal/connection/pool.go),
// the fingerprint pre-flight (internal/fingerprint/ssrf.go), and the
// fingerprint validating dialer (internal/fingerprint/directdial.go). The
// four hand-maintained copies it replaced had already drifted in timeout
// derivation and error text; a filter-rule change now applies everywhere at
// once.
//
// It resolves host via lookup (nil means net.DefaultResolver; tests
// substitute a seam) under the shared DNS timeout policy — capped at 10s,
// tightened to dialTimeout when dialTimeout is positive and smaller — and
// returns the resolved IPs that survive FilterAllowedIPs. Resolution errors
// are returned unwrapped so each layer keeps its own error prefix; an empty
// allowed set is NOT an error here (each layer reports it with its own
// message, some after further processing the IPs).
func ResolveFilteredIPs(ctx context.Context, host string, exemptNets []*net.IPNet, dialTimeout time.Duration, lookup func(context.Context, string) ([]net.IPAddr, error)) ([]net.IP, error) {
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	dnsTimeout := 10 * time.Second
	if dialTimeout > 0 && dialTimeout < dnsTimeout {
		dnsTimeout = dialTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dnsCtx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()
	ipAddrs, err := lookup(dnsCtx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(ipAddrs))
	for i, addr := range ipAddrs {
		ips[i] = addr.IP
	}
	return FilterAllowedIPs(ips, exemptNets), nil
}

// ValidatedCandidates turns "host:port" into SSRF-validated "ip:port" dial
// candidates: an IP literal validates directly (no DNS, no rebinding
// window); a hostname resolves once through the shared resolve-and-filter
// rule (ResolveFilteredIPs) and every allowed address becomes a candidate so
// callers can fail over IPv6→IPv4. Single owner of the split → validate →
// candidate-join sequence the standard dialer, its DoH variant, and the
// fingerprint validating dialer previously maintained as three hand-synced
// copies — copies that had already drifted (the pool assumed port 443 for
// addresses without a port; the fingerprint path failed closed).
//
// Fail-closed: an address without a port is an error. Guessing a port would
// dial the wrong service; both httpc dial paths receive host:port from the
// transport, so a portless address is a caller bug worth an error, not a
// silent 443 that dials the wrong port for any non-TLS target.
//
// Error shapes (callers add their own prefixes): a policy-rejected IP
// returns ValidateIPWithExemptions's error verbatim; a DNS failure is
// wrapped as "DNS resolution failed for <host>"; an empty allowed set
// reports the shared "resolves only to blocked addresses" message.
func ValidatedCandidates(ctx context.Context, addr string, exemptNets []*net.IPNet, dialTimeout time.Duration, lookup func(context.Context, string) ([]net.IPAddr, error)) ([]string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("SSRF dial validation: address %q must be host:port: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := ValidateIPWithExemptions(ip, exemptNets); err != nil {
			return nil, err
		}
		return []string{addr}, nil
	}
	allowed, err := ResolveFilteredIPs(ctx, host, exemptNets, dialTimeout, lookup)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("domain %s resolves only to blocked addresses", host)
	}
	candidates := make([]string, 0, len(allowed))
	for _, ip := range allowed {
		candidates = append(candidates, net.JoinHostPort(ip.String(), port))
	}
	return candidates, nil
}

// FilterAllowedIPs filters a list of IPs to only those allowed under SSRF rules.
// Private/reserved IPs are excluded unless they match an exempt CIDR range.
// Returns nil slice when no IPs are allowed.
// This supports Split-Horizon DNS environments where a domain may resolve to
// both public and private IPs — only public (or exempted) IPs are used for dialing.
func FilterAllowedIPs(ips []net.IP, exemptNets []*net.IPNet) []net.IP {
	allowed := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if !isPrivateOrReservedIP(ip) || isIPExempted(ip, exemptNets) {
			allowed = append(allowed, ip)
		}
	}
	return allowed
}

// isLocalhost detects localhost variations by hostname string:
//   - "localhost" (case-insensitive)
//   - 127.0.0.1, ::1, 0.0.0.0, ::
//   - full dotted-quad 127.x.x.x loopback addresses (but NOT domains that
//     merely start with "127." — 127.net and 127.com are real public domains)
//   - "localhost.localdomain" and "localhost.localdomain." (case-insensitive)
//
// Arbitrary localhost.* subdomains are intentionally NOT matched here: names
// such as localhost.example.com may be legitimate public domains. Hosts that
// resolve to loopback IPs are still blocked at the dialer layer (see
// ValidateSSRFHost and isPrivateOrReservedIP), so excluding them here only
// affects this early hostname-string fast path, not overall SSRF protection.
//
// Optimized to avoid repeated string allocations from strings.ToLower.
func isLocalhost(hostname string) bool {
	// Fast path: check length before any string operations
	hlen := len(hostname)
	if hlen == 0 {
		return false
	}

	// Check for dotted-quad 127.x.x.x loopback forms first (most common
	// localhost pattern). The bare "127." prefix check this replaces
	// misclassified legitimate public domains such as 127.net / 127.com as
	// localhost; the remainder must now parse as a full dotted-quad IPv4
	// loopback address. Legacy short forms that net.ParseIP rejects (127.1,
	// 127.0.0.1.2) are still blocked by looksLikeLegacyIPLiteral in
	// ValidateSSRFHost.
	if hlen >= 4 && hostname[0] == '1' && hostname[1] == '2' && hostname[2] == '7' && hostname[3] == '.' {
		if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
			return true
		}
	}

	// Exact IP-literal loopback matches. The "localhost"/"LOCALHOST" cases
	// this switch used to carry are subsumed by the EqualFold branch below
	// (both are length 9 and start with l/L).
	switch hostname {
	case "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	}

	// Check "Localhost" and other case variations without allocation
	// Only do case-insensitive check if first char is 'l' or 'L'
	if hlen == 9 && (hostname[0] == 'l' || hostname[0] == 'L') {
		// Check "localhost" case-insensitively
		if EqualFold(hostname, "localhost") {
			return true
		}
	}

	// Check for known local DNS names derived from localhost
	// Only match specific local DNS suffixes, not arbitrary localhost.* subdomains
	// which may be legitimate public domains (e.g., localhost.example.com).
	// Domains resolving to loopback IPs are still blocked at the dialer layer.
	if hlen == 21 && (hostname[0] == 'l' || hostname[0] == 'L') {
		if EqualFold(hostname, "localhost.localdomain") {
			return true
		}
	}
	if hlen == 22 && (hostname[0] == 'l' || hostname[0] == 'L') {
		if EqualFold(hostname, "localhost.localdomain.") {
			return true
		}
	}

	return false
}

// EqualFold checks if s equals t case-insensitively (ASCII only).
// Avoids allocation from strings.ToLower.
func EqualFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c1 := s[i]
		c2 := t[i]
		if c1 == c2 {
			continue
		}
		// Convert to lowercase for comparison
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

// ContainsFold reports whether substr is contained within s, ASCII case-insensitively.
// Zero-allocation alternative to strings.Contains(strings.ToLower(s), substr).
func ContainsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	end := len(s) - len(substr)
	for i := 0; i <= end; i++ {
		if EqualFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// ValidateAndParseURL validates a URL and returns the parsed result.
// This avoids callers needing to parse the URL again after validation.
func ValidateAndParseURL(urlStr string) (*url.URL, error) {
	if urlStr == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}
	if len(urlStr) > maxURLLen {
		return nil, fmt.Errorf("URL too long (max %d)", maxURLLen)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		// SECURITY: url.Parse errors embed the raw input verbatim
		// (`parse "…": …`), which may carry userinfo credentials. Redact so a
		// malformed URL with an embedded password cannot leak it up the error
		// chain into logs — the same treatment ValidateProxyURL already applies.
		return nil, fmt.Errorf("invalid URL: %w", redactErrorURL(err, urlStr))
	}
	if parsedURL.Scheme == "" {
		return nil, fmt.Errorf("URL scheme is required")
	}
	// Hostname(), not Host: "http://:8080/x" parses with Host=":8080" but an
	// empty hostname, and an empty host sails through every check in
	// ValidateSSRFHost — the request then surfaces as a confusing dial error,
	// or (with AllowPrivateIPs) dials localhost, because net.Dial("tcp",
	// ":8080") connects to 127.0.0.1:8080.
	if parsedURL.Hostname() == "" {
		return nil, fmt.Errorf("URL host is required")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}
	return parsedURL, nil
}

// ValidateProxyURL parses and validates a proxy URL string, returning the parsed
// URL so callers do not need to re-parse.
//
// Accepted schemes: http, https, socks5, socks5h. The socks5 schemes are
// supported natively by net/http.Transport when set via transport.Proxy =
// http.ProxyURL(u), so they must not be rejected here.
//
// This is the single source of truth for proxy URL validation, shared by the
// public Config validator (ValidateConfig) and the internal connection pool
// (NewPoolManager). Centralizing it prevents the two layers from drifting —
// which is what previously let the public layer accept socks5 while the pool
// rejected it with a different error.
func ValidateProxyURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("proxy URL cannot be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		// SECURITY: url.Parse errors embed the raw input verbatim
		// (`parse "…": …`), which may contain proxy credentials. Redact both
		// the echoed URL and the wrapped error text so the password cannot
		// reach application logs through the error chain.
		return nil, fmt.Errorf("invalid proxy URL %q: %w", SanitizeURL(rawURL), redactErrorURL(err, rawURL))
	}

	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		if u.Scheme == "" {
			return nil, fmt.Errorf("invalid proxy URL %q: missing scheme", SanitizeURL(rawURL))
		}
		return nil, fmt.Errorf("unsupported proxy URL scheme %q (want http, https, socks5, or socks5h)", u.Scheme)
	}

	if u.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q: missing host", SanitizeURL(rawURL))
	}

	return u, nil
}

// redactErrorURL replaces any occurrence of rawURL inside err's message with
// its sanitized form, so a wrapped url.Parse error ("parse \"raw\": …") does
// not carry credentials up the error chain. Escaped (%q) occurrences are
// matched too — see RedactURLInMessage. When rawURL has nothing to sanitize
// the original error is returned unchanged, preserving its type for
// errors.Is/As; otherwise a new error is built from the scrubbed message.
func redactErrorURL(err error, rawURL string) error {
	msg, changed := RedactURLInMessage(err.Error(), rawURL)
	if !changed {
		return err
	}
	return errors.New(msg)
}

// ParseSystemProxyURL parses a proxy address obtained from system sources —
// environment variables (HTTP_PROXY/HTTPS_PROXY) and platform settings
// (Windows registry, macOS networksetup). It is the system-detection
// counterpart of ValidateProxyURL and the single normalization point for
// those sources: the environment and registry paths previously carried two
// hand-maintained variants of these rules, and their drift made an
// uppercase-scheme registry value silently disable the system proxy.
//
// Normalization applied before validation:
//   - "" (or whitespace) returns (nil, nil): "no value configured" is not an
//     error at the system-detection layer.
//   - A bare "socks" scheme is mapped to "socks5" — net/http recognizes
//     socks5/socks5h but not "socks://".
//   - A bare host:port ("proxy:3128", the dominant registry/env spelling,
//     which parses with the hostname in the scheme position) gets an
//     "http://" prefix.
//
// Everything else — including uppercase schemes, which url.Parse lowercases —
// is validated by ValidateProxyURL unchanged, inheriting its scheme
// allow-list, host requirement, and credential redaction.
func ParseSystemProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if after, ok := strings.CutPrefix(raw, "socks://"); ok {
		raw = "socks5://" + after
	} else if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return ValidateProxyURL(raw)
}

// schemeDefaultPorts maps proxy URL schemes to the port net/http appends when
// the proxy URL omits one (mirrors net/http transport.schemePort, which drives
// the address the transport actually dials).
var schemeDefaultPorts = map[string]string{
	"http":    "80",
	"https":   "443",
	"socks5":  "1080",
	"socks5h": "1080",
}

// CanonicalProxyAddr returns the host:port form of a proxy URL as net/http
// will actually DIAL it. net/http's transport dials canonicalAddr(proxyURL) —
// the URL hostname with the scheme's default port appended when the URL omits
// one — so any comparison against the dial address (proxy SSRF exemptions,
// proxypool health-report keys) must use this canonical form, not url.Host.
// Before this existed, a portless entry like "socks5://internal-proxy" was
// keyed as "internal-proxy" while the dialer saw "internal-proxy:1080": the
// proxy SSRF exemption silently never applied and circuit-breaking failure
// counts never incremented.
//
// Non-ASCII (IDN) hostnames are punycoded exactly as net/http does at dial
// time (idna.Lookup.ToASCII, left unchanged on conversion failure), so an
// IDN proxy URL matches the dialed form with or without an explicit port.
func CanonicalProxyAddr(u *url.URL) string {
	if u == nil {
		return ""
	}
	// Mirror stdlib exactly: canonicalAddr applies idnaASCIIFromURL to the
	// hostname (ASCII passes through; non-ASCII is punycoded, kept verbatim
	// when conversion fails) and url.Port() — which returns "" both for a
	// missing port AND for a syntactically present but EMPTY port
	// ("http://proxy:" → Host "proxy:", Port() "") — then appends the scheme
	// default. Testing SplitHostPort instead would keep the literal "proxy:"
	// and never match the dialed "proxy:80".
	host := u.Hostname()
	if !IsASCII(host) {
		if ascii, err := idna.Lookup.ToASCII(host); err == nil {
			host = ascii
		}
	}
	port := u.Port()
	if port == "" {
		port = schemeDefaultPorts[u.Scheme]
	}
	return net.JoinHostPort(host, port)
}

// SameOrigin reports whether two URLs share an origin per RFC 6454: scheme,
// host, and effective port, with scheme-default ports normalized ("example.com"
// and "example.com:80" under http are one origin; "example.com:8080" is not).
// It is the single canonical implementation shared by the engine's redirect
// credential-stripping and the fingerprint path's sec-fetch-site computation —
// the two previously carried private copies whose semantics had drifted apart.
func SameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		EffectivePort(a) == EffectivePort(b)
}

// EffectivePort returns u's explicit port, or the scheme's default when the
// URL omits one. Schemes are matched case-insensitively (url.Parse already
// lowercases the scheme; the fold is belt-and-braces for hand-built URLs).
// A URL with neither port nor a recognized scheme has no effective port and
// matches only the same degenerate form — engines only route http/https
// (ws/wss included for the transports that speak them), so the "" default is
// unreachable through validated requests and exists to fail closed on the
// exotic forms rather than lump them onto port 80.
func EffectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "wss":
		return "443"
	case "http", "ws":
		return "80"
	default:
		return ""
	}
}

// IsASCII reports whether s consists solely of ASCII bytes. Exported next to
// ASCIILower so the security package's IDNA pre-checks can share this
// predicate instead of carrying an identical private copy: the two gate the
// same normalization family (proxy addresses, redirect-whitelist domains) and
// must never diverge.
func IsASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// ValidateSSRFHost checks whether a hostname (which may include a port) should be
// blocked under SSRF protection rules. It checks localhost, direct IP addresses,
// and legacy IP-literal notation.
// Returns nil if the host is allowed, or an error describing why it was blocked.
//
// DNS resolution of hostnames is deliberately NOT performed here: the
// connection pool dialer (internal/connection) resolves once, validates every
// IP, and dials the validated IP directly, which also prevents DNS-rebinding
// TOCTOU attacks a pre-check could not.
func ValidateSSRFHost(host string, exemptNets []*net.IPNet) error {
	// Extract hostname from host:port format
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// Strip the square brackets net/url keeps on IPv6 literals without a port
	// ("[::1]"): SplitHostPort only removes them together with the port, and
	// net.ParseIP / isLocalhost reject bracketed strings. Without this, a bare
	// bracketed literal sailed through every check below — and when a proxy is
	// configured, the dialer-level SSRF validation is bypassed entirely
	// (pool.go: proxy connections dial the proxy address, not the target), so
	// this pre-check is then the only line of defense.
	if len(hostname) >= 2 && hostname[0] == '[' && hostname[len(hostname)-1] == ']' {
		hostname = hostname[1 : len(hostname)-1]
	}

	// Normalize the fully-qualified trailing dot ("localhost." is a legal FQDN
	// spelling of "localhost") so hostname checks cannot be bypassed by it.
	hostname = strings.TrimSuffix(hostname, ".")

	// Fail closed on an empty hostname ("http://:8080/x"): every check below
	// treats "" as nothing-to-block, letting the request slip past host
	// validation entirely. Defense-in-depth — ValidateAndParseURL already
	// rejects empty hostnames earlier in the request path.
	if hostname == "" {
		return fmt.Errorf("empty host")
	}

	if isLocalhost(hostname) {
		return fmt.Errorf("localhost access blocked for security")
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if err := ValidateIPWithExemptions(ip, exemptNets); err != nil {
			return fmt.Errorf("private/reserved IP address blocked")
		}
		return nil
	}

	// Block legacy integer/hex/octal IPv4 notation (e.g. "2130706433",
	// "0x7f000001", "0177.0.0.1", "10.1"). net.ParseIP rejects these, so
	// without this guard they fall through to the dialer, where libc-family
	// resolvers (directly or via proxies) may map them to private IPs
	// (e.g. 127.0.0.1), bypassing SSRF protection.
	if looksLikeLegacyIPLiteral(hostname) {
		return fmt.Errorf("legacy IP literal notation blocked for security: %s", hostname)
	}

	return nil
}

// looksLikeLegacyIPLiteral reports whether s uses a legacy integer, hex, or octal
// IPv4 notation that net.ParseIP does not recognize but some platform resolvers
// (cgo getaddrinfo) accept. Used to defensively block SSRF bypass attempts.
//
// Recognized legacy forms:
//   - Pure decimal integer without dots: "2130706433"
//   - Hex with 0x prefix (dotted or not): "0x7f000001", "0x7f.0.0.1"
//   - Octal via leading-zero octets: "0177.0.0.1"
func looksLikeLegacyIPLiteral(s string) bool {
	if s == "" {
		return false
	}

	if !strings.Contains(s, ".") {
		// Pure decimal integer host (no dots): 2130706433. A bare all-numeric
		// label is not a legitimate DNS hostname. The whole-string hex form
		// 0x7f000001 belongs here too.
		return isAllDigits(s) || strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")
	}

	// Dotted form: inet_aton-family resolvers reinterpret a dotted name as
	// an address only when EVERY label is numeric (decimal/octal) or 0x-hex.
	// A single alphabetic label ("0x.org", "007.com") makes it an ordinary
	// hostname no resolver will re-read as an address — flagging those would
	// block legal public sites on the default validation path.
	inetCandidate := true
	for part := range strings.SplitSeq(s, ".") {
		if part == "" {
			continue
		}
		if !isAllDigits(part) && !strings.HasPrefix(part, "0x") && !strings.HasPrefix(part, "0X") {
			inetCandidate = false
			break
		}
	}
	if !inetCandidate {
		return false
	}

	// Within a candidate: flag hex octets and leading-zero (octal) octets.
	for part := range strings.SplitSeq(s, ".") {
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "0x") || strings.HasPrefix(part, "0X") {
			return true
		}
		// Leading zero on a multi-char octet => octal (e.g. "0177"). "0" is fine.
		if len(part) > 1 && part[0] == '0' {
			return true
		}
	}

	// Short-form inet_aton literals ("10.1" → 10.0.0.1, "192.168.1" →
	// 192.168.0.1) and out-of-range octets ("1.2.3.256"): net.ParseIP has
	// already rejected this string, and a legal DNS hostname cannot have an
	// all-numeric final label (RFC 1123 §2.1), so treat it as a legacy IP
	// literal. Proxied requests resolve the host remotely, where libc-family
	// resolvers still accept these forms — this pre-check is then the only
	// line of SSRF defense. Full dotted quads with every octet in range are
	// excluded: net.ParseIP accepts those before this helper is consulted.
	parts := strings.Split(s, ".")
	if last := parts[len(parts)-1]; isAllDigits(last) {
		if len(parts) != 4 {
			return true
		}
		for _, part := range parts {
			if !isAllDigits(part) {
				// A non-numeric label (e.g. "a.1.2.3") makes this a hostname,
				// not an IP literal.
				return false
			}
			if v := octetValue(part); v < 0 || v > 255 {
				return true
			}
		}
	}
	return false
}

// octetValue parses a digits-only label into its decimal value. Returns -1
// for labels longer than 3 digits, which no valid octet can be.
func octetValue(s string) int {
	if len(s) > 3 {
		return -1
	}
	v := 0
	for i := 0; i < len(s); i++ {
		v = v*10 + int(s[i]-'0')
	}
	return v
}

// isAllDigits reports whether s is non-empty and consists only of ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
