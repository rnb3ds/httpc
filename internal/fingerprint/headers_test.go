package fingerprint

import (
	"net/http"
	"strings"
	"testing"

	ehttp "github.com/bogdanfinn/fhttp"
)

// applyHeaders runs the profile pipeline over a fresh map seeded with the
// caller's headers and returns it.
func applyHeaders(t *testing.T, p *browserProfile, os osKey, method string, seed map[string]string) ehttp.Header {
	t.Helper()
	h := ehttp.Header{}
	for k, v := range seed {
		h[k] = []string{v}
	}
	applyProfileHeaders(&h, method, p, os)
	return h
}

// wire returns the single value stored under the exact key, bypassing
// Header.Get's canonicalisation — the whole point of this pipeline is that
// the stored key's spelling is what reaches the wire.
func wire(h ehttp.Header, key string) string {
	if vs := h[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// TestApplyProfileHeaders pins the pipeline's contract: profile values
// overwrite caller values, order keys carry lower-case names, wire casing is
// the profile's exact spelling, empty placeholders are not sent, and the
// caller's map is never touched.
func TestApplyProfileHeaders(t *testing.T) {
	t.Run("authoritative set semantics", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "GET", map[string]string{
			"User-Agent":      "httpc/1.0",
			"Accept-Encoding": "gzip, deflate",
		})

		if got := wire(h, "User-Agent"); got != chrome152UA[osWindows] {
			t.Errorf("User-Agent = %q, want the profile Chrome UA", got)
		}
		if got := wire(h, "Accept-Encoding"); got != "gzip, deflate, br, zstd" {
			t.Errorf("Accept-Encoding = %q, want the profile value", got)
		}
		if got := wire(h, "sec-ch-ua"); got != chrome152SecCHUA {
			t.Errorf("sec-ch-ua = %q, want %q", got, chrome152SecCHUA)
		}
	})

	// The regression this pipeline exists to prevent on HTTP/1.1: the
	// Chromium client-hint family is lower case on the wire, and every other
	// header the browser sends is Title-Case. A map key is the only place
	// that spelling can live.
	t.Run("wire casing is the browser's", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "GET", nil)

		lower := []string{"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"}
		for _, name := range lower {
			if wire(h, name) == "" {
				t.Errorf("%q missing under its lower-case key", name)
			}
			if canonical := [2]string{strings.ToUpper(name[:1]), name}; wire(h, canonical[0]+canonical[1][1:]) != "" {
				t.Errorf("%q was stored under a canonicalised key", name)
			}
		}

		title := []string{
			"User-Agent", "Accept", "Accept-Encoding", "Accept-Language",
			"Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-User", "Sec-Fetch-Dest",
			"Upgrade-Insecure-Requests", "Priority",
		}
		for _, name := range title {
			if wire(h, name) == "" {
				t.Errorf("%q missing under its Title-Case key", name)
			}
			if wire(h, strings.ToLower(name)) != "" {
				t.Errorf("%q was stored lower-case", name)
			}
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
		// The transport lower-cases each map key before matching it against
		// this list, so every entry must already be lower case.
		for _, name := range order {
			if name != strings.ToLower(name) {
				t.Errorf("order entry %q is not lower case", name)
			}
		}
		if !containsName(order, "cookie") {
			t.Error("engine-set Cookie must occupy its profile order slot")
		}
		if !containsName(order, "sec-ch-ua-platform") {
			t.Error("listed header missing from the order list")
		}

		// Regression: the engine-set value itself must survive the pipeline.
		// The ownership pass once treated empty placeholders (Cookie et al.)
		// as owning their names, deleted the caller value, and never wrote
		// anything back — the header never reached the wire.
		if got := wire(h, "Cookie"); got != "session=abc" {
			t.Errorf("engine-set Cookie = %q, want session=abc", got)
		}
		if got := wire(h, "X-Custom"); got != "keep-me" {
			t.Errorf("X-Custom = %q, want keep-me", got)
		}

		pseudo, ok := h[ehttp.PHeaderOrderKey]
		if !ok || len(pseudo) != 4 {
			t.Fatalf("PHeaderOrderKey = %v, want the four pseudo-headers", pseudo)
		}
		want := []string{":method", ":authority", ":scheme", ":path"}
		for i := range want {
			if pseudo[i] != want[i] {
				t.Fatalf("pseudo order = %v, want %v", pseudo, want)
			}
		}
	})

	t.Run("firefox emits no client hints", func(t *testing.T) {
		h := applyHeaders(t, firefox148Desktop, osWindows, "GET", nil)
		for _, name := range []string{"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"} {
			if vs := h[name]; len(vs) != 0 {
				t.Errorf("firefox profile emitted %q", name)
			}
		}
		if wire(h, "User-Agent") != firefox148UA[osWindows] {
			t.Errorf("User-Agent = %q", wire(h, "User-Agent"))
		}
	})

	t.Run("post sends no navigation headers", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "POST", nil)

		if got := wire(h, "Accept"); got != "*/*" {
			t.Errorf("Accept = %q, want the fetch value */*", got)
		}
		if got := wire(h, "Sec-Fetch-Dest"); got != "empty" {
			t.Errorf("Sec-Fetch-Dest = %q, want empty", got)
		}
		if got := wire(h, "Sec-Fetch-Site"); got != "same-origin" {
			t.Errorf("Sec-Fetch-Site = %q, want same-origin", got)
		}
		// content-length/content-type placeholders are not sent; the transport
		// injects the real values at their order slots.
		if vs := h["Content-Type"]; len(vs) != 0 {
			t.Errorf("content-type sent despite empty placeholder: %v", vs)
		}
	})

	t.Run("mobile OS client hints", func(t *testing.T) {
		h := applyHeaders(t, chrome152Mobile, osAndroid, "GET", nil)
		if got := wire(h, "sec-ch-ua-mobile"); got != "?1" {
			t.Errorf("sec-ch-ua-mobile = %q, want ?1", got)
		}
		if got := wire(h, "sec-ch-ua-platform"); got != `"Android"` {
			t.Errorf("sec-ch-ua-platform = %q, want Android", got)
		}

		h = applyHeaders(t, chrome152Desktop, osMacOS, "GET", nil)
		if got := wire(h, "sec-ch-ua-mobile"); got != "?0" {
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
		// lexicographically after ALL listed ones. Order entries are lower
		// case regardless of the wire casing the profile declares.
		synth := &browserProfile{
			name:     "synthetic",
			orderGet: []string{"alpha", "beta"},
			baseHeaders: func(*browserProfile, osKey) []headerField {
				return []headerField{
					{Name: "Alpha", Value: "1"},
					{Name: "Beta", Value: "2"},
					{Name: "Xray", Value: "x"},
					{Name: "Yankee", Value: "y"},
				}
			},
			insertGet: []headerField{{Name: "Gamma", Value: "g"}},
		}

		h := applyHeaders(t, synth, osWindows, "GET", nil)

		want := []string{"alpha", "beta", "gamma", "xray", "yankee"}
		got := h[ehttp.HeaderOrderKey]
		if len(got) != len(want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order = %v, want %v (listed first in table order, unlisted lexicographic after)", got, want)
			}
		}
		for _, name := range []string{"Alpha", "Beta", "Gamma", "Xray", "Yankee"} {
			if wire(h, name) == "" {
				t.Errorf("merged name %q missing from the map", name)
			}
		}
	})

	// Caller headers spelled differently from the profile must not produce a
	// second copy of the field: two keys for one header would put it on the
	// wire twice, a shape no browser emits.
	t.Run("caller casing does not duplicate a profile header", func(t *testing.T) {
		seed := map[string]string{
			"Sec-Ch-Ua":      "caller-brand", // canonical spelling
			"SEC-FETCH-SITE": "cross-site",   // all caps
			"User-Agent":     "caller-agent",
			"X-Keep":         "kept",
		}
		h := applyHeaders(t, chrome152Desktop, osWindows, "GET", seed)

		for _, field := range []string{"sec-ch-ua", "Sec-Fetch-Site", "User-Agent"} {
			count := 0
			for k := range h {
				if strings.EqualFold(k, field) {
					count++
				}
			}
			if count != 1 {
				t.Errorf("%q present under %d keys, want 1: %v", field, count, keysOf(h))
			}
		}

		// The profile's value won, not the caller's.
		if got := wire(h, "sec-ch-ua"); got != chrome152SecCHUA {
			t.Errorf("sec-ch-ua = %q, want the profile value", got)
		}
		if got := wire(h, "Sec-Fetch-Site"); got != "none" {
			t.Errorf("Sec-Fetch-Site = %q, want the profile value none", got)
		}
		if got := wire(h, "User-Agent"); got != chrome152UA[osWindows] {
			t.Errorf("User-Agent = %q, want the profile UA", got)
		}

		// A caller header the profile does not define survives untouched.
		if got := wire(h, "X-Keep"); got != "kept" {
			t.Errorf("X-Keep = %q, want it preserved", got)
		}

		// Each surviving field appears exactly once in the order list too.
		order := h[ehttp.HeaderOrderKey]
		seen := map[string]int{}
		for _, name := range order {
			seen[name]++
		}
		for name, n := range seen {
			if n != 1 {
				t.Errorf("order entry %q appears %d times", name, n)
			}
		}
	})

	// Regression: placeholder-named caller values must survive. Cookie,
	// Authorization, Origin, Referer, and Content-Type exist in the profiles
	// only as empty order-slot placeholders; the ownership pass must not
	// claim their names.
	t.Run("placeholder-named caller values survive", func(t *testing.T) {
		h := applyHeaders(t, chrome152Desktop, osWindows, "POST", map[string]string{
			"Cookie":        "session=abc",
			"authorization": "Bearer tok", // caller spelling differs from profile's
			"Content-Type":  "application/json",
			"Referer":       "https://example.com/",
		})

		if got := wire(h, "Cookie"); got != "session=abc" {
			t.Errorf("Cookie = %q, want session=abc", got)
		}
		if got := wire(h, "authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		if got := wire(h, "Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if got := wire(h, "Referer"); got != "https://example.com/" {
			t.Errorf("Referer = %q", got)
		}
		// Each name exactly once on the map.
		for _, field := range []string{"Cookie", "Authorization", "Content-Type", "Referer"} {
			count := 0
			for k := range h {
				if strings.EqualFold(k, field) {
					count++
				}
			}
			if count != 1 {
				t.Errorf("%q present under %d keys, want 1: %v", field, count, keysOf(h))
			}
		}
	})

	t.Run("insert overrides base under either casing", func(t *testing.T) {
		// A base placeholder and a method insert naming the same field must
		// collapse into one entry, not two keys differing in case.
		synth := &browserProfile{
			name:     "synthetic",
			orderGet: []string{"priority"},
			baseHeaders: func(*browserProfile, osKey) []headerField {
				return []headerField{{Name: "Priority", Value: ""}}
			},
			insertGet: []headerField{{Name: "priority", Value: "u=0, i"}},
		}
		h := applyHeaders(t, synth, osWindows, "GET", nil)

		count := 0
		for k := range h {
			if strings.EqualFold(k, "priority") {
				count++
			}
		}
		if count != 1 {
			t.Errorf("priority stored under %d keys, want 1: %v", count, h)
		}
		if got := wire(h, "priority"); got != "u=0, i" {
			t.Errorf("priority = %q, want the insert value", got)
		}
	})
}

// keysOf returns the map's keys, for failure messages.
func keysOf(h ehttp.Header) []string {
	out := make([]string, 0, len(h))
	for k := range h {
		out = append(out, k)
	}
	return out
}

// containsName reports whether names contains target.
func containsName(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}
