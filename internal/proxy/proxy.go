// Package proxy detects system proxy configuration for the httpc library.
package proxy

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cybergodev/httpc/internal/validation"
)

// Detector provides automatic system proxy detection
type Detector struct{}

// NewDetector creates a new system proxy detector
func NewDetector() *Detector {
	return &Detector{}
}

// GetProxyFunc returns a proxy function that automatically detects system proxy settings.
// It returns nil if no proxy is configured, which means direct connection.
//
// Detection runs on every call — no cache. The only production caller
// (connection.NewPoolManager) calls this exactly once per Detector, so a
// double-checked-locking cache guarded concurrency that never occurs while
// freezing the first detection for the Detector's lifetime (surprising for
// anything named "auto-detect").
func (d *Detector) GetProxyFunc() func(*http.Request) (*url.URL, error) {
	return d.detect()
}

// detect performs platform-specific proxy detection
func (d *Detector) detect() func(*http.Request) (*url.URL, error) {
	// First try environment variables (works on all platforms)
	if envProxy := d.detectFromEnvironment(); envProxy != nil {
		return envProxy
	}

	// Platform-specific detection (Windows registry, macOS networksetup).
	// NO_PROXY applies to a platform-sourced proxy as well: without this the
	// two detection paths of the same Detector disagree — the common
	// enterprise combination of "proxy from the OS + NO_PROXY for the
	// intranet" silently sent intranet URLs to the corporate proxy, because
	// only the environment path honored NO_PROXY.
	platform := d.detectPlatform()
	if platform == nil {
		return nil
	}
	return applyNoProxy(platform, getEnvAny("NO_PROXY", "no_proxy"))
}

// detectFromEnvironment checks environment variables for proxy settings.
// Unlike http.ProxyFromEnvironment's process-wide sync.Once cache, each
// Detector re-reads the environment on its FIRST detection and caches the
// result for that detector's lifetime — later os.Setenv calls are not
// reflected by an already-created detector (a fresh Detector picks them up).
func (d *Detector) detectFromEnvironment() func(*http.Request) (*url.URL, error) {
	httpProxy := getEnvAny("HTTP_PROXY", "http_proxy")
	httpsProxy := getEnvAny("HTTPS_PROXY", "https_proxy")
	noProxy := getEnvAny("NO_PROXY", "no_proxy")
	// Capture the CGI indicator once: REQUEST_METHOD is fixed for the process
	// lifetime, and reading os.Getenv per request both costs a syscall and
	// races with any concurrent os.Setenv by the host application.
	inCGI := os.Getenv("REQUEST_METHOD") != ""

	if httpProxy == "" && httpsProxy == "" {
		return nil
	}

	// Pre-parse and normalize proxy URLs at detection time.
	httpURL, httpErr := parseProxyURL(httpProxy)
	httpsURL, httpsErr := parseProxyURL(httpsProxy)

	return func(req *http.Request) (*url.URL, error) {
		// Don't use proxies in a CGI environment. net/http refuses
		// HTTP_PROXY in CGI with an error (see golang.org/s/cgihttpproxy);
		// here both schemes direct-connect — simpler, and it never proxies
		// when the (potentially attacker-controlled) CGI environment would
		// be the one choosing the proxy.
		if inCGI {
			return nil, nil
		}

		host := req.URL.Hostname()

		// localhost and loopback IPs are always direct, matching net/http
		// behavior (x/net httpproxy exempts net.IP.IsLoopback in addition to
		// the literal hostname). Same check as isLoopbackRequest, kept
		// inlined-by-name via the helper rather than a hand copy.
		if isLoopbackRequest(req) {
			return nil, nil
		}

		// Check NO_PROXY exclusions. The port must follow net/http's
		// canonicalAddr semantics (see canonicalPort): httpproxy compares
		// entries against host:port with the scheme's default port filled in
		// ("https://example.com/" is "example.com:443"), so
		// NO_PROXY="example.com:443" must exempt it — the raw
		// req.URL.Port() is "" for implicit ports and never matches a
		// ported entry, diverging from net/http.
		if noProxy != "" && !shouldUseProxy(host, canonicalPort(req), noProxy) {
			return nil, nil
		}

		// Select the proxy URL with the same scheme-scoped resolution as
		// net/http's httpproxy.proxyForURL: https requests use HTTPS_PROXY
		// only and http requests use HTTP_PROXY only. net/http has NO
		// cross-scheme fallback in either direction — the previous fallback
		// branches routed plaintext http:// traffic into an HTTPS_PROXY-only
		// proxy (and https traffic into an HTTP_PROXY-only proxy), diverging
		// from every standard-library client.
		//
		// A proxy env var that is SET but INVALID must surface its parse
		// error for the schemes it governs rather than silently
		// direct-connecting — net/http's ProxyFromEnvironment does the same.
		// (The previous error checks sat inside the `!= nil` branches where
		// they were unreachable, so an invalid HTTPS_PROXY silently disabled
		// proxying for HTTPS traffic.)
		if req.URL.Scheme == "https" {
			if httpsURL != nil {
				return httpsURL, nil
			}
			if httpsErr != nil {
				return nil, httpsErr
			}
			return nil, nil
		}
		if httpURL != nil {
			return httpURL, nil
		}
		if httpErr != nil {
			return nil, httpErr
		}

		return nil, nil
	}
}

// getEnvAny returns the first non-empty value from the given env var names.
func getEnvAny(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// canonicalPort returns the request's port with the scheme's default filled
// in ("" → 443 for https, 80 otherwise), matching net/http's canonicalAddr:
// httpproxy compares NO_PROXY entries against host:port in that form, so
// NO_PROXY="example.com:443" must exempt https://example.com/ whose raw
// req.URL.Port() is "".
func canonicalPort(req *http.Request) string {
	if port := req.URL.Port(); port != "" {
		return port
	}
	if req.URL.Scheme == "https" {
		return "443"
	}
	return "80"
}

// applyNoProxy wraps a platform-detected proxy function (Windows registry,
// macOS networksetup) so NO_PROXY is honored exactly as on the
// environment-variable path: the shared shouldUseProxy rule with canonical
// port semantics. A noProxy of "" returns fn unchanged (zero per-request
// overhead). Platform bypass outcomes (loopback, ProxyOverride /
// bypass-domains) and detection errors pass through untouched — NO_PROXY can
// only make an outcome more direct, never less.
func applyNoProxy(fn func(*http.Request) (*url.URL, error), noProxy string) func(*http.Request) (*url.URL, error) {
	if noProxy == "" {
		return fn
	}
	return func(req *http.Request) (*url.URL, error) {
		u, err := fn(req)
		if err != nil || u == nil {
			return u, err
		}
		if !shouldUseProxy(req.URL.Hostname(), canonicalPort(req), noProxy) {
			return nil, nil
		}
		return u, nil
	}
}

// isLoopbackRequest reports whether the request targets localhost or a
// loopback IP. Platform-detected proxies (Windows registry, macOS
// networksetup) bypass such requests, matching net/http behavior and the
// environment-variable path above — without this, an enterprise system proxy
// would route http://127.0.0.1/health calls through the corporate proxy,
// breaking local calls and disclosing internal URLs to the proxy.
func isLoopbackRequest(req *http.Request) bool {
	host := req.URL.Hostname()
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// matchProxyBypassEntry reports whether a single system-proxy bypass entry
// matches host. Shared by the platform detectors (Windows ProxyOverride,
// macOS bypass domains): "<local>" matches dotless intranet hostnames (the
// Windows default rule), "*.suffix" matches strict subdomains (the apex
// itself is not included — list it separately), and any other entry matches
// the host exactly. Comparison is case-insensitive.
func matchProxyBypassEntry(host, entry string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	entry = strings.ToLower(strings.TrimSpace(entry))
	if host == "" || entry == "" {
		return false
	}
	switch {
	case entry == "<local>":
		return !strings.Contains(host, ".")
	case strings.HasPrefix(entry, "*."):
		return strings.HasSuffix(host, entry[1:])
	default:
		return host == entry
	}
}

// matchBypassCIDR (IP/CIDR bypass entries) lives in proxy_darwin.go: only
// macOS bypass lists support that form, so a Windows build would otherwise
// see an unused helper here.

// parseProxyURL parses an environment-sourced proxy URL. All system-source
// normalization (bare host:port, the bare "socks" scheme, case-insensitive
// schemes) and validation (allow-list, host requirement, credential
// redaction in errors) live in validation.ParseSystemProxyURL — the shared
// single point — so the environment and Windows registry detection paths
// cannot drift apart again.
func parseProxyURL(proxy string) (*url.URL, error) {
	return validation.ParseSystemProxyURL(proxy)
}

// shouldUseProxy reports whether requests to the given host should use a proxy,
// according to the NO_PROXY environment variable. It supports hostname suffix
// matching, IP addresses, CIDR notation, and the "*" wildcard, following the
// same conventions as net/http's httpproxy package.
func shouldUseProxy(host, port, noProxy string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return true
	}

	ip := net.ParseIP(host)

	// SplitSeq iterates the NO_PROXY list without allocating the pattern
	// slice. This runs on every request when a system proxy is configured and
	// NO_PROXY is set; iteration order and yielded entries (including empty
	// ones, skipped below) are identical to strings.Split.
	for pattern := range strings.SplitSeq(noProxy, ",") {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}

		if pattern == "*" {
			return false // bypass all hosts
		}

		// CIDR match (e.g., 10.0.0.0/8).
		if _, pnet, err := net.ParseCIDR(pattern); err == nil {
			if ip != nil && pnet.Contains(ip) {
				return false
			}
			continue
		}

		// Split optional host:port in pattern.
		phost, pport := pattern, ""
		if h, p, err := net.SplitHostPort(pattern); err == nil {
			phost, pport = h, p
		}

		// IP literal match.
		if net.ParseIP(phost) != nil {
			if host == phost && (pport == "" || pport == port) {
				return false
			}
			continue
		}

		if phost == "" {
			continue
		}

		// Domain suffix match (e.g., ".example.com" or "example.com").
		if strings.HasPrefix(phost, "*.") {
			phost = phost[1:] // "*.example.com" -> ".example.com"
		}
		matchHost := false
		if !strings.HasPrefix(phost, ".") {
			matchHost = true
			phost = "." + phost
		}

		if ip == nil {
			if strings.HasSuffix(host, phost) {
				if pport == "" || pport == port {
					return false
				}
			}
			if matchHost && host == phost[1:] {
				if pport == "" || pport == port {
					return false
				}
			}
		}
	}

	return true
}
