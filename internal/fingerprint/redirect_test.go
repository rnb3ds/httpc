package fingerprint

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	ehttp "github.com/bogdanfinn/fhttp"
)

// TestRegistrableDomain pins the eTLD+1 approximation: two-label hosts are
// themselves, three-label hosts under a known multi-part suffix keep three
// labels, IP literals and single labels match only exactly.
func TestRegistrableDomain(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"example.com", "example.com"},
		{"www.example.com", "example.com"},
		{"a.b.example.com", "example.com"},
		{"example.co.uk", "example.co.uk"},
		{"shop.example.co.uk", "example.co.uk"},
		{"example.com.au", "example.com.au"},
		{"EXAMPLE.COM.", "example.com"}, // case + trailing dot
		{"localhost", "localhost"},
		{"127.0.0.1", "127.0.0.1"}, // IP literals never strip labels
		{"::1", "::1"},
	}
	for _, tc := range cases {
		if got := registrableDomain(tc.host); got != tc.want {
			t.Errorf("registrableDomain(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

// TestFetchSite pins the sec-fetch-site classification between one hop's
// initiator URL and the next target: origin equality (with scheme-default
// port normalization), registrable-domain sharing, and everything else.
func TestFetchSite(t *testing.T) {
	mustURL := func(t *testing.T, raw string) *url.URL {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		return u
	}

	cases := []struct {
		prev, cur string
		want      string
	}{
		// same-origin: scheme, host, port identical
		{"https://example.com/a", "https://example.com/b", fetchSiteSameOrigin},
		// default ports normalize (443 == unstated under https)
		{"https://example.com/a", "https://example.com:443/b", fetchSiteSameOrigin},
		{"http://example.com/a", "http://example.com:80/b", fetchSiteSameOrigin},
		// scheme or port change: same host, different origin → same-site
		{"http://example.com/a", "https://example.com/b", fetchSiteSameSite},
		{"https://example.com:8443/a", "https://example.com/b", fetchSiteSameSite},
		// subdomains share the registrable domain → same-site
		{"https://www.example.com/", "https://api.example.com/v1", fetchSiteSameSite},
		{"https://example.com/", "https://shop.example.co.uk/", fetchSiteCrossSite},
		{"https://shop.example.co.uk/", "https://api.example.co.uk/", fetchSiteSameSite},
		// multi-part suffixes: different registrable domains → cross-site
		{"https://foo.co.uk/", "https://bar.co.uk/", fetchSiteCrossSite},
		// different registrable domains → cross-site
		{"https://example.com/", "https://example.org/", fetchSiteCrossSite},
		{"https://accounts.google.com/", "https://example.com/", fetchSiteCrossSite},
		// scheme case-insensitive
		{"HTTPS://example.com/a", "https://example.com/b", fetchSiteSameOrigin},
	}
	for _, tc := range cases {
		got := fetchSite(mustURL(t, tc.prev), mustURL(t, tc.cur))
		if got != tc.want {
			t.Errorf("fetchSite(%q → %q) = %q, want %q", tc.prev, tc.cur, got, tc.want)
		}
	}

	// An empty host on either side classifies cross-site (unreachable through
	// the engine's URL validation; pinned as defense in depth).
	if got := fetchSite(&url.URL{}, mustURL(t, "https://example.com/")); got != fetchSiteCrossSite {
		t.Errorf("fetchSite(empty host → example.com) = %q, want %q", got, fetchSiteCrossSite)
	}
	if got := fetchSite(mustURL(t, "https://example.com/"), &url.URL{}); got != fetchSiteCrossSite {
		t.Errorf("fetchSite(example.com → empty host) = %q, want %q", got, fetchSiteCrossSite)
	}
}

// hopRequest builds a redirect-hop request linked to its previous hop the
// way net/http does: the hop request carries Response, whose Request is the
// request that produced the redirect.
func hopRequest(t *testing.T, method, target string, prev *http.Request) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Response = &http.Response{Request: prev, StatusCode: http.StatusFound}
	return req
}

// TestRedirectFactsFor pins hop detection and chain classification: no
// Response means first hop; the fetch/navigation classification comes from
// the chain's first request, not the current method.
func TestRedirectFactsFor(t *testing.T) {
	t.Run("first hop", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		f := redirectFactsFor(req)
		if f.hop || f.site != "" || f.fetchChain {
			t.Errorf("redirectFactsFor(first hop) = %+v, want zero facts", f)
		}
	})

	t.Run("navigation chain", func(t *testing.T) {
		hop1 := httptest.NewRequest(http.MethodGet, "https://a.com/", nil)
		hop2 := hopRequest(t, http.MethodGet, "https://b.com/", hop1)
		f := redirectFactsFor(hop2)
		if !f.hop {
			t.Fatal("hop not detected")
		}
		if f.site != fetchSiteCrossSite {
			t.Errorf("site = %q, want %q", f.site, fetchSiteCrossSite)
		}
		if f.fetchChain {
			t.Error("GET→GET navigation chain classified as fetch")
		}
	})

	t.Run("fetch chain survives method downgrade", func(t *testing.T) {
		hop1 := httptest.NewRequest(http.MethodPost, "https://a.com/login", nil)
		hop2 := hopRequest(t, http.MethodGet, "https://a.com/welcome", hop1)
		f := redirectFactsFor(hop2)
		if !f.fetchChain {
			t.Error("POST→GET hop must keep fetch classification")
		}

		// Deep chain: the classification sticks to the chain's first hop.
		hop3 := hopRequest(t, http.MethodGet, "https://cdn.a.com/asset", hop2)
		if f := redirectFactsFor(hop3); !f.fetchChain {
			t.Error("POST→GET→GET chain must stay fetch-classified")
		}
	})

	t.Run("defensive against synthetic response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.Response = &http.Response{Request: nil} // not a net/http redirect
		if f := redirectFactsFor(req); f.hop {
			t.Error("Response without Request must not count as a hop")
		}
	})
}

// TestTableMethodFor pins the table selection: the request's own method,
// except GET hops of a fetch chain (a 301/302/303 downgrade) keep the
// fetch-style POST tables.
func TestTableMethodFor(t *testing.T) {
	cases := []struct {
		method string
		facts  redirectFacts
		want   string
	}{
		{http.MethodGet, redirectFacts{}, http.MethodGet},
		{http.MethodPost, redirectFacts{}, http.MethodPost},
		{http.MethodPut, redirectFacts{}, http.MethodPut},
		{http.MethodGet, redirectFacts{hop: true}, http.MethodGet}, // navigation hop
		{http.MethodGet, redirectFacts{fetchChain: true}, http.MethodGet},
		{http.MethodGet, redirectFacts{hop: true, fetchChain: true}, http.MethodPost},
		{http.MethodPost, redirectFacts{hop: true, fetchChain: true}, http.MethodPost},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "https://example.com/", nil)
		if got := tableMethodFor(req, tc.facts); got != tc.want {
			t.Errorf("tableMethodFor(%s, %+v) = %q, want %q", tc.method, tc.facts, got, tc.want)
		}
	}
}

// TestApplyRedirectOverrides_Hop pins the hop corrections over a fully
// assembled profile header block: sec-fetch-site is replaced with the
// recomputed value, sec-fetch-user is dropped from the wire while staying
// listed in HeaderOrderKey (listed-but-absent, like the placeholders).
func TestApplyRedirectOverrides_Hop(t *testing.T) {
	h := ehttp.Header{}
	applyProfileHeaders(&h, http.MethodGet, chrome152Desktop, osWindows)

	applyRedirectOverrides(&h, redirectFacts{hop: true, site: fetchSiteCrossSite})

	if got := h.Get("sec-fetch-site"); got != fetchSiteCrossSite {
		t.Errorf("sec-fetch-site = %q, want %q", got, fetchSiteCrossSite)
	}
	if vs := h.Values("sec-fetch-user"); len(vs) != 0 {
		t.Errorf("sec-fetch-user sent on a hop: %v", vs)
	}
	inOrder := false
	for _, name := range h[ehttp.HeaderOrderKey] {
		if name == "sec-fetch-user" {
			inOrder = true
		}
	}
	if !inOrder {
		t.Error("sec-fetch-user lost from HeaderOrderKey (order slot must be kept)")
	}
}

// TestApplyRedirectOverrides_FirstHop pins the zero-facts no-op: a
// first-hop request keeps the profile's own sec-fetch values.
func TestApplyRedirectOverrides_FirstHop(t *testing.T) {
	h := ehttp.Header{}
	applyProfileHeaders(&h, http.MethodGet, chrome152Desktop, osWindows)

	applyRedirectOverrides(&h, redirectFacts{})

	if got := h.Get("sec-fetch-site"); got != "none" {
		t.Errorf("sec-fetch-site = %q, want the profile value none", got)
	}
	if got := h.Get("sec-fetch-user"); got != "?1" {
		t.Errorf("sec-fetch-user = %q, want the profile value ?1", got)
	}
}

// TestRedirectFetchChainHeaderShape assembles a downgraded fetch hop the
// way the adapter does (profile pipeline under tableMethodFor, then the
// overrides) and pins the wire shape: fetch-style insert values, no
// navigation-only headers, recomputed sec-fetch-site.
func TestRedirectFetchChainHeaderShape(t *testing.T) {
	hop1 := httptest.NewRequest(http.MethodPost, "https://api.example.com/login", nil)
	hop2 := hopRequest(t, http.MethodGet, "https://cdn.example.net/asset", hop1)

	facts := redirectFactsFor(hop2)
	h := ehttp.Header{}
	applyProfileHeaders(&h, tableMethodFor(hop2, facts), chrome152Desktop, osWindows)
	applyRedirectOverrides(&h, facts)

	if got := h.Get("accept"); got != "*/*" {
		t.Errorf("accept = %q, want the fetch-style */*", got)
	}
	if got := h.Get("sec-fetch-dest"); got != "empty" {
		t.Errorf("sec-fetch-dest = %q, want empty", got)
	}
	if got := h.Get("sec-fetch-mode"); got != "cors" {
		t.Errorf("sec-fetch-mode = %q, want cors", got)
	}
	if got := h.Get("sec-fetch-site"); got != fetchSiteCrossSite {
		t.Errorf("sec-fetch-site = %q, want %q", got, fetchSiteCrossSite)
	}
	if vs := h.Values("Upgrade-Insecure-Requests"); len(vs) != 0 {
		t.Errorf("navigation-only upgrade-insecure-requests sent on a fetch hop: %v", vs)
	}
	if vs := h.Values("sec-fetch-user"); len(vs) != 0 {
		t.Errorf("sec-fetch-user sent on a hop: %v", vs)
	}
}
