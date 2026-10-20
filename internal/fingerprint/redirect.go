// Redirect-hop header realism: browsers recompute sec-fetch-site on every
// redirect hop (against the previous hop's URL, not the chain's origin),
// drop sec-fetch-user after the first hop (it marks user activation, which
// does not carry across redirects), and keep fetch/XHR header semantics
// when a 301/302/303 turns a non-GET request into a GET. The static
// profile tables model a first hop and cannot express any of this, so the
// adapter applies these corrections after the profile pipeline.
//
// Hop detection rides on net/http: every redirect request it hands to the
// RoundTripper carries Response set, whose Request is the previous hop.
// A first-hop request (Response nil) or a synthetic Response without
// Request leaves the profile's first-hop values untouched.

package fingerprint

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	ehttp "github.com/enetx/http"
)

// sec-fetch-site values a browser emits for one hop (Fetch Standard
// §headers-sfk; "none" is first-hop-only and comes from the profile tables).
const (
	fetchSiteSameOrigin = "same-origin"
	fetchSiteSameSite   = "same-site"
	fetchSiteCrossSite  = "cross-site"
)

// multipartPublicSuffixes lists the high-traffic public-suffix second
// levels for which the registrable domain is three labels, not two
// (publicsuffix.org rules like "*.co.uk"). It is an approximation of the
// PSL's dominant entries, deliberately not the full list: this package
// links into every httpc binary, and a miss only blurs same-site vs
// cross-site for an exotic suffix — a low-weight corner of an already
// heuristic header.
var multipartPublicSuffixes = map[string]bool{
	// *.uk
	"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true,
	// *.au
	"com.au": true, "net.au": true, "org.au": true, "edu.au": true,
	// *.jp
	"co.jp": true, "ne.jp": true, "or.jp": true, "ac.jp": true, "go.jp": true,
	// *.br, *.cn
	"com.br": true, "net.br": true, "org.br": true,
	"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true,
	// Americas
	"com.mx": true, "org.mx": true, "net.mx": true,
	"com.ar": true, "net.ar": true, "org.ar": true,
	// Asia-Pacific
	"co.nz": true, "net.nz": true, "org.nz": true,
	"co.za": true, "net.za": true, "org.za": true, "web.za": true,
	"com.sg": true, "net.sg": true, "org.sg": true,
	"co.in": true, "net.in": true, "org.in": true, "firm.in": true, "gen.in": true,
	"com.hk": true, "net.hk": true, "org.hk": true, "edu.hk": true,
	"com.tw": true, "org.tw": true, "net.tw": true,
	"co.kr": true, "ne.kr": true, "or.kr": true, "re.kr": true, "pe.kr": true,
	"co.il": true, "org.il": true, "net.il": true, "ac.il": true,
	"com.my": true, "net.my": true, "org.my": true,
	"com.ph": true, "net.ph": true, "org.ph": true,
	"com.pk": true, "net.pk": true, "org.pk": true,
	"co.th": true, "in.th": true, "or.th": true,
	"com.tr": true, "net.tr": true, "org.tr": true,
	"com.vn": true, "net.vn": true, "org.vn": true,
	"co.id": true, "or.id": true, "web.id": true,
	// Middle East
	"com.eg": true, "net.eg": true, "org.eg": true,
	"com.sa": true, "net.sa": true, "org.sa": true,
	"com.qa": true, "net.qa": true, "org.qa": true,
	"com.kw": true, "net.kw": true, "org.kw": true,
	// Americas (south)
	"com.co": true, "net.co": true, "org.co": true,
	"com.pe": true, "net.pe": true, "org.pe": true,
	"com.ve": true, "net.ve": true, "org.ve": true,
	"com.ec": true, "net.ec": true, "org.ec": true,
}

// redirectFacts describes the redirect-hop corrections for one wire
// request. The zero value means "first hop — profile values stand".
type redirectFacts struct {
	// hop reports that this request follows a redirect (net/http set
	// Response on it).
	hop bool

	// site is the browser's sec-fetch-site value for this hop: the
	// relationship between the previous hop's URL and this one.
	site string

	// fetchChain reports that the chain's first request was not GET, so
	// this hop keeps fetch/XHR header semantics even when the method was
	// downgraded to GET by a 301/302/303.
	fetchChain bool
}

// redirectFactsFor inspects a wire request for redirect-hop corrections. It
// walks the Response chain back to the first hop to classify the chain
// (navigation vs fetch); the chain is net/http's own linked list, so the
// walk is bounded by the engine's redirect cap.
func redirectFactsFor(req *http.Request) redirectFacts {
	var f redirectFacts
	if req.Response == nil || req.Response.Request == nil || req.Response.Request.URL == nil || req.URL == nil {
		return f
	}

	f.hop = true
	f.site = fetchSite(req.Response.Request.URL, req.URL)

	first := req.Response.Request
	for first.Response != nil && first.Response.Request != nil {
		first = first.Response.Request
	}
	f.fetchChain = first.Method != http.MethodGet

	return f
}

// tableMethodFor returns the profile-table method for a wire request: the
// request's own method, except that a redirect hop which began as a non-GET
// request keeps the fetch-style (POST) tables after a 301/302/303 downgraded
// it to GET — fetch() redirects keep fetch metadata class.
func tableMethodFor(req *http.Request, f redirectFacts) string {
	if f.hop && f.fetchChain && req.Method == http.MethodGet {
		return http.MethodPost
	}
	return req.Method
}

// applyRedirectOverrides applies the hop corrections to the fork header map
// after the profile pipeline: sec-fetch-site is recomputed against the
// previous hop and sec-fetch-user is dropped. Names removed from the map
// stay listed in HeaderOrderKey; the fork transports skip listed-but-absent
// headers the same way they skip the profile's empty placeholders.
func applyRedirectOverrides(dst *ehttp.Header, f redirectFacts) {
	if !f.hop {
		return
	}
	dst.Set("sec-fetch-site", f.site)
	dst.Del("sec-fetch-user")
}

// fetchSite returns the sec-fetch-site value a browser sends for a request
// to cur initiated from prev: "same-origin" when origins match (scheme,
// host, port — scheme-default ports normalized), "same-site" when the hosts
// share a registrable domain, "cross-site" otherwise. Either host empty
// (unreachable through the engine's URL validation) classifies cross-site.
func fetchSite(prev, cur *url.URL) string {
	if sameOriginURL(prev, cur) {
		return fetchSiteSameOrigin
	}
	ph, ch := prev.Hostname(), cur.Hostname()
	if ph == "" || ch == "" {
		return fetchSiteCrossSite
	}
	if registrableDomain(ph) == registrableDomain(ch) {
		return fetchSiteSameSite
	}
	return fetchSiteCrossSite
}

// sameOriginURL compares two URLs by RFC 6454 origin: scheme, host, and
// port with scheme-default ports normalized (example.com and
// example.com:443 under https are one origin).
func sameOriginURL(a, b *url.URL) bool {
	return strings.EqualFold(a.Hostname(), b.Hostname()) &&
		strings.EqualFold(a.Scheme, b.Scheme) &&
		effectivePort(a) == effectivePort(b)
}

// effectivePort returns the URL's transport port, substituting the scheme
// default when none is stated.
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "wss":
		return "443"
	default:
		return "80"
	}
}

// registrableDomain approximates a host's eTLD+1: the last two labels, or
// the last three when the last two match a known multi-part public suffix.
// IP literals and single-label hosts return themselves so they only match
// exactly.
func registrableDomain(host string) string {
	if host == "" || net.ParseIP(host) != nil {
		return strings.ToLower(host)
	}
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(host, ".")), ".")
	if len(labels) <= 2 {
		return strings.Join(labels, ".")
	}
	if multipartPublicSuffixes[strings.Join(labels[len(labels)-2:], ".")] {
		return strings.Join(labels[len(labels)-3:], ".")
	}
	return strings.Join(labels[len(labels)-2:], ".")
}
