// Reconstruction of the request header map the fingerprint transport actually
// writes to the wire, for the engine's Result.Request.Headers report.
//
// The adapter builds the wire request in a fork (*ehttp.Request) whose header
// map carries the browser profile's values under the browser's exact casing,
// plus the transport's two magic order keys (Header-Order: / PHeader-Order:).
// Those keys are transport metadata, not request fields: they order how the
// writer emits headers but are never sent themselves. Reconstructing the wire
// map means copying the real header fields verbatim and dropping the two magic
// keys, so the engine reports what a server would observe, not the engine's
// own request object (which still carries httpc/1.0 — the default UA the
// profile replaced on the wire).
//
// Wire ordering is deliberately not reproduced: http.Header is a map with no
// stable iteration order, so only the field set and values can be reported.

package fingerprint

import (
	"net/http"

	ehttp "github.com/bogdanfinn/fhttp"
)

// wireHeaders returns the header map the fingerprint transport writes for one
// request: every non-magic header from the fork request map, with the two
// magic order keys (Header-Order: / PHeader-Order:) dropped. The copy is
// defensive: the engine's Result retains the reported map independently of
// the per-trip fork map (prepareWireRequest clones the engine request — and
// therefore the fork map — fresh for every round trip, retries included).
func wireHeaders(h ehttp.Header) http.Header {
	if h == nil {
		return nil
	}

	out := make(http.Header, len(h))
	for k, v := range h {
		if k == ehttp.HeaderOrderKey || k == ehttp.PHeaderOrderKey {
			continue
		}
		out[k] = cloneHeaderValues(v)
	}
	return out
}

// cloneHeaderValues copies one header's value slice so the reconstructed map
// owns its backing arrays.
func cloneHeaderValues(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return append([]string(nil), v...)
}
