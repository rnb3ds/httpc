package fingerprint

import (
	"testing"

	ehttp "github.com/bogdanfinn/fhttp"
)

// TestWireHeaders pins the wire-map reconstruction used for
// Result.Request.Headers on the fingerprint path: the magic order keys are
// dropped, real fields are copied verbatim under the browser's casing, and
// values are owned (not aliased to the fork map).
func TestWireHeaders(t *testing.T) {
	t.Run("drops magic keys and keeps wire fields verbatim", func(t *testing.T) {
		in := ehttp.Header{
			"User-Agent":          {"Mozilla/5.0 ... Chrome/155.0"},
			"Accept-Encoding":     {"gzip, deflate, br, zstd"},
			"sec-ch-ua":           {`"Chromium";v="155"`},
			"Sec-Fetch-Mode":      {"navigate"},
			ehttp.HeaderOrderKey:  {"user-agent", "accept-encoding", "sec-ch-ua", "sec-fetch-mode"},
			ehttp.PHeaderOrderKey: {":method", ":authority", ":scheme", ":path"},
		}

		out := wireHeaders(in)

		if len(out) != 4 {
			t.Fatalf("len(out) = %d, want 4 (magic keys dropped): %v", len(out), out)
		}
		if got := out.Get("User-Agent"); got != "Mozilla/5.0 ... Chrome/155.0" {
			t.Errorf("User-Agent = %q", got)
		}
		// Get canonicalises the lookup, so the browser's lower-case casing is
		// checked against the raw key directly.
		if _, ok := out["sec-ch-ua"]; !ok {
			t.Errorf("sec-ch-ua missing under its browser casing: %v", out)
		}
		if _, ok := out[ehttp.HeaderOrderKey]; ok {
			t.Error("Header-Order: magic key must be dropped")
		}
		if _, ok := out[ehttp.PHeaderOrderKey]; ok {
			t.Error("PHeader-Order: magic key must be dropped")
		}
	})

	t.Run("values are owned, not aliased", func(t *testing.T) {
		in := ehttp.Header{"X-A": {"one"}}
		out := wireHeaders(in)
		in["X-A"][0] = "mutated"
		if got := out.Get("X-A"); got != "one" {
			t.Errorf("reported value = %q, want the snapshot %q", got, "one")
		}
	})

	t.Run("nil input is nil output", func(t *testing.T) {
		if out := wireHeaders(nil); out != nil {
			t.Errorf("wireHeaders(nil) = %v, want nil", out)
		}
	})

	t.Run("empty value slice becomes nil", func(t *testing.T) {
		in := ehttp.Header{"X-Empty": {}}
		out := wireHeaders(in)
		if vs, ok := out["X-Empty"]; !ok || vs != nil {
			t.Errorf("empty value slice should report as nil slice, got %v (ok=%v)", vs, ok)
		}
	})
}
