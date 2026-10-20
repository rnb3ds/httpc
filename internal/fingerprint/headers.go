// Per-request header assembly for the browser profiles: merge the profile's
// header set, sort it into the per-method wire order, apply it to the
// outgoing header map with Set semantics (the profile is authoritative for
// every header it defines), and publish the order through the fork
// transport's magic keys.
//
// Pipeline ported from github.com/enetx/surf (request.go
// updateRequestHeaderOrder + profiles/headers.go), Copyright enetx/surf
// authors, MIT License.
//
// Semantics pinned by tests (and by the Akamai H2 hash in the network e2e):
//   - The order-key lists are built from the FULL sorted key set including
//     empty placeholders (authorization/cookie/origin/referer/content-type/
//     content-length) — placeholders occupy wire slots but are not sent, so
//     the relative order of the headers that ARE sent is unaffected.
//   - Only the Set pass skips empty values and pseudo-headers; engine-set
//     Cookie/Referer/Authorization values land at their profile positions
//     because those names are listed.
//   - Names absent from the profile order (caller's custom headers) are left
//     untouched on the map; the fork transports sort them after all listed
//     headers, lexicographically.
//   - The magic keys are written only to the per-trip clone built by
//     prepareWireRequest; they never reach the engine's request object
//     (net/http's redirect copier would otherwise propagate them to every
//     redirect hop).

package fingerprint

import (
	"sort"
	"strings"

	ehttp "github.com/enetx/http"
)

// applyProfileHeaders applies p's header pipeline for one request onto dst,
// the header map of the per-trip request clone: profile values overwrite
// same-name caller headers, and the wire order is published via the
// HeaderOrderKey/PHeaderOrderKey magic keys consumed by the fork transports.
func applyProfileHeaders(dst *ehttp.Header, method string, p *browserProfile, os osKey) {
	merged := p.mergedHeaders(os, method)
	order := p.order(method)

	position := make(map[string]int, len(order))
	for i, name := range order {
		if _, dup := position[name]; !dup {
			position[name] = i
		}
	}

	keys := make([]string, 0, len(merged))
	for name := range merged {
		keys = append(keys, name)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, iok := position[keys[i]]
		pj, jok := position[keys[j]]
		switch {
		case iok && jok:
			return pi < pj
		case iok:
			return true // listed before unlisted
		case jok:
			return false
		default:
			return keys[i] < keys[j] // unlisted: lexicographic
		}
	})

	regular := make([]string, 0, len(keys))
	pseudo := make([]string, 0, 4)
	for _, name := range keys {
		if strings.HasPrefix(name, ":") {
			pseudo = append(pseudo, name)
			continue
		}
		regular = append(regular, name)
		if value := merged[name]; value != "" {
			dst.Set(name, value)
		}
	}

	if len(regular) > 0 {
		(*dst)[ehttp.HeaderOrderKey] = regular
	}
	if len(pseudo) > 0 {
		(*dst)[ehttp.PHeaderOrderKey] = pseudo
	}
}
