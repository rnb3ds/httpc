package fingerprint

import (
	"net/http"
	"strings"
	"testing"

	ehttp "github.com/enetx/http"
)

// applyHeaders runs the pipeline for one method over a header map seeded
// with caller headers, returning the resulting fork header map.
func applyHeaders(t *testing.T, p *browserProfile, os osKey, method string, caller map[string]string) ehttp.Header {
	t.Helper()
	h := ehttp.Header{}
	for k, v := range caller {
		h.Set(k, v)
	}
	applyProfileHeaders(&h, method, p, os)
	return h
}

// TestApplyProfileHeaders drives the whole per-request header pipeline
// (formerly per-case functions, folded into subtests with their assertions
// preserved): Set semantics, order-key contracts, per-browser pseudo-header
// layouts, method tables, OS-resolved Client Hints, and caller isolation.
func TestApplyProfileHeaders(t *testing.T) {
	t.Run("authoritative set semantics", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "GET", map[string]string{
			"User-Agent":      "httpc/1.0",
			"Accept-Encoding": "gzip, deflate",
		})

		if got := h.Get("User-Agent"); got != chromeUA[osWindows] {
			t.Errorf("User-Agent = %q, want the profile Chrome UA", got)
		}
		if got := h.Get("Accept-Encoding"); got != "gzip, deflate, br, zstd" {
			t.Errorf("Accept-Encoding = %q, want the profile value", got)
		}
		if got := h.Get("sec-ch-ua"); got != chromeSecCHUA {
			t.Errorf("sec-ch-ua = %q, want %q", got, chromeSecCHUA)
		}
	})

	t.Run("order keys", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "GET", map[string]string{
			"X-Custom": "keep-me",
			"Cookie":   "session=abc", // engine-set; a listed name
		})

		order, ok := h[ehttp.HeaderOrderKey]
		if !ok || len(order) == 0 {
			t.Fatal("HeaderOrderKey missing")
		}
		for _, name := range order {
			if name != strings.ToLower(name) {
				t.Errorf("order entry %q not lowercase", name)
			}
			if strings.HasPrefix(name, ":") {
				t.Errorf("pseudo-header %q leaked into HeaderOrderKey", name)
			}
		}

		// Placeholders absent from the map still occupy order slots.
		wantPresent := []string{"authorization", "cookie", "origin", "referer", "upgrade-insecure-requests", "priority"}
		inOrder := make(map[string]bool, len(order))
		for _, name := range order {
			inOrder[name] = true
		}
		for _, name := range wantPresent {
			if !inOrder[name] {
				t.Errorf("order list missing %q (placeholder names must be listed)", name)
			}
		}

		// Cookie carries the caller's value at its listed position; the empty
		// placeholders are not sent.
		if got := h.Get("Cookie"); got != "session=abc" {
			t.Errorf("Cookie = %q, want caller value", got)
		}
		for _, name := range []string{"Authorization", "Origin", "Referer"} {
			if vs := h.Values(name); len(vs) != 0 {
				t.Errorf("%s sent despite empty placeholder: %v", name, vs)
			}
		}

		// Custom header survives, unlisted.
		if got := h.Get("X-Custom"); got != "keep-me" {
			t.Errorf("X-Custom = %q, want it preserved", got)
		}
		for _, name := range order {
			if name == "x-custom" {
				t.Error("X-Custom must not be added to the order list")
			}
		}

		// Pseudo order in the Chrome layout.
		porder, ok := h[ehttp.PHeaderOrderKey]
		if !ok {
			t.Fatal("PHeaderOrderKey missing")
		}
		want := []string{":method", ":authority", ":scheme", ":path"}
		if len(porder) != len(want) {
			t.Fatalf("PHeaderOrderKey = %v, want %v", porder, want)
		}
		for i := range want {
			if porder[i] != want[i] {
				t.Fatalf("PHeaderOrderKey = %v, want %v", porder, want)
			}
		}
	})

	t.Run("firefox pseudo order without client hints", func(t *testing.T) {
		h := applyHeaders(t, firefox148Desktop, osLinux, "GET", nil)

		porder := h[ehttp.PHeaderOrderKey]
		want := []string{":method", ":path", ":authority", ":scheme"}
		if len(porder) != len(want) {
			t.Fatalf("PHeaderOrderKey = %v, want %v", porder, want)
		}
		for i := range want {
			if porder[i] != want[i] {
				t.Fatalf("PHeaderOrderKey = %v, want %v", porder, want)
			}
		}
		if vs := h.Values("sec-ch-ua"); len(vs) != 0 {
			t.Error("firefox profile emitted sec-ch-ua")
		}
		if got := h.Get("Accept-Language"); got != "en-US,en;q=0.5" {
			t.Errorf("Accept-Language = %q, want firefox value", got)
		}
	})

	t.Run("non-GET method falls back to GET tables", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "PUT", nil)

		// GET insert values present.
		if got := h.Get("sec-fetch-mode"); got != "navigate" {
			t.Errorf("sec-fetch-mode = %q, want navigate (GET inserts for PUT)", got)
		}
		if got := h.Get("upgrade-insecure-requests"); got != "1" {
			t.Errorf("upgrade-insecure-requests = %q, want 1", got)
		}

		// POST-only insert values absent.
		if vs := h.Values("Cache-Control"); len(vs) != 0 {
			t.Errorf("Cache-Control sent on PUT: %v", vs)
		}
	})

	t.Run("POST inserts", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "POST", nil)

		if got := h.Get("accept"); got != "*/*" {
			t.Errorf("accept = %q, want */*", got)
		}
		if got := h.Get("priority"); got != "u=1, i" {
			t.Errorf("priority = %q, want u=1, i", got)
		}
		if got := h.Get("sec-fetch-site"); got != "same-origin" {
			t.Errorf("sec-fetch-site = %q, want same-origin", got)
		}
		// content-length/content-type placeholders are not sent; the transport
		// injects the real values at their order slots.
		if vs := h.Values("Content-Type"); len(vs) != 0 {
			t.Errorf("content-type sent despite empty placeholder: %v", vs)
		}
	})

	t.Run("mobile OS client hints", func(t *testing.T) {
		h := applyHeaders(t, chrome152Mobile, osAndroid, "GET", nil)
		if got := h.Get("sec-ch-ua-mobile"); got != "?1" {
			t.Errorf("sec-ch-ua-mobile = %q, want ?1", got)
		}
		if got := h.Get("sec-ch-ua-platform"); got != `"Android"` {
			t.Errorf("sec-ch-ua-platform = %q, want \"Android\"", got)
		}

		h = applyHeaders(t, chrome152Desktop, osMacOS, "GET", nil)
		if got := h.Get("sec-ch-ua-mobile"); got != "?0" {
			t.Errorf("sec-ch-ua-mobile = %q, want ?0", got)
		}
	})

	t.Run("does not touch caller request", func(t *testing.T) {
		caller := http.Header{}
		caller.Set("User-Agent", "httpc/1.0")
		fork := ehttp.Header{}
		for k, v := range caller {
			fork[k] = v
		}

		applyProfileHeaders(&fork, "GET", chrome152Desktop, osWindows)

		if got := caller.Get("User-Agent"); got != "httpc/1.0" {
			t.Errorf("caller UA mutated: %q", got)
		}
		if len(caller[ehttp.HeaderOrderKey]) != 0 {
			t.Error("order key leaked onto caller map")
		}
	})

	t.Run("merged unlisted names sort last", func(t *testing.T) {
		// Synthetic profile whose merged header set contains names absent
		// from the order table (shipped profiles list every merged name, so
		// the unlisted comparator arms are data-unreachable there): listed
		// names keep their table position, unlisted merged names sort
		// lexicographically after ALL listed ones.
		synth := &browserProfile{
			name:     "synthetic",
			orderGet: []string{"alpha", "beta"},
			baseHeaders: func(*browserProfile, osKey) []headerField {
				return []headerField{
					{Name: "alpha", Value: "1"},
					{Name: "beta", Value: "2"},
					{Name: "Xray", Value: "x"},
					{Name: "Yankee", Value: "y"},
				}
			},
			insertGet: []headerField{{Name: "Gamma", Value: "g"}},
		}

		h := applyHeaders(t, synth, osWindows, "GET", nil)

		// Listed names appear in table (lowercase) form at their positions;
		// unlisted merged names keep their declared case and sort
		// lexicographically after every listed one.
		want := []string{"alpha", "beta", "Gamma", "Xray", "Yankee"}
		got := h[ehttp.HeaderOrderKey]
		if len(got) != len(want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order = %v, want %v (listed first in table order, unlisted lexicographic after)", got, want)
			}
		}
		for _, name := range []string{"alpha", "beta", "Gamma", "Xray", "Yankee"} {
			if h.Get(name) == "" {
				t.Errorf("merged name %q missing from the map", name)
			}
		}
	})
}
