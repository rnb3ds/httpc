// Per-request header assembly for the browser profiles: merge the profile's
// header set, sort it into the per-method wire order, apply it to the
// outgoing header map with Set semantics (the profile is authoritative for
// every header it defines), and publish the order through the transport's
// magic keys.
//
// Casing is a fingerprint axis on HTTP/1.1: the browser writes lower-case
// sec-ch-ua and Title-Case Sec-Fetch-*, and the transport writes map keys
// verbatim instead of canonicalising them. Three rules follow, each pinned by a
// subtest of TestApplyProfileHeaders:
//   - header values land under the profile's exact spelling, written by direct
//     map assignment (Header.Set would canonicalise the key);
//   - the order-key list carries lower-case names, because the transport's
//     order lookup lower-cases each map key before matching;
//   - a caller header whose name matches a profile header that carries a
//     value is deleted before the profile value is written, or both spellings
//     would go out and the browser would emit the header twice; empty
//     placeholders never own their name, so engine-set Cookie/Referer/
//     Authorization values survive under their own key.
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
//     untouched on the map; the transport sorts them after all listed
//     headers, lexicographically.
//   - The magic keys are written only to the per-trip clone built by
//     prepareWireRequest; they never reach the engine's request object
//     (net/http's redirect copier would otherwise propagate them to every
//     redirect hop).
//
// Pipeline ported from github.com/enetx/surf (request.go
// updateRequestHeaderOrder + profiles/headers.go), Copyright enetx/surf
// authors, MIT License.

package fingerprint

import (
	"sort"
	"strings"

	ehttp "github.com/bogdanfinn/fhttp"
)

// applyProfileHeaders applies p's header pipeline for one request onto dst,
// the header map of the per-trip request clone: profile values overwrite
// same-name caller headers, and the wire order is published via the
// HeaderOrderKey/PHeaderOrderKey magic keys consumed by the transport.
func applyProfileHeaders(dst *ehttp.Header, method string, p *browserProfile, os osKey) {
	merged := p.mergedHeaders(os, method)
	order := p.order(method)

	position := make(map[string]int, len(order))
	for i, name := range order {
		if _, dup := position[name]; !dup {
			position[name] = i
		}
	}

	// Sort by the lower-cased name so a header's position does not depend on
	// the casing the profile happens to use.
	ranked := make([]headerField, len(merged))
	copy(ranked, merged)
	sort.SliceStable(ranked, func(i, j int) bool {
		ki, kj := strings.ToLower(ranked[i].Name), strings.ToLower(ranked[j].Name)
		pi, iok := position[ki]
		pj, jok := position[kj]
		switch {
		case iok && jok:
			return pi < pj
		case iok:
			return true // listed before unlisted
		case jok:
			return false
		default:
			return ki < kj // unlisted: lexicographic
		}
	})

	// Only a name the profile actually writes a value for owns the wire
	// exclusively, so caller keys can be matched against it without regard
	// to their spelling. Empty placeholders (Cookie, Authorization, Origin,
	// Referer, Content-Type, Content-Length) reserve order slots for engine-
	// and caller-set values; marking them owned would delete those values
	// and then skip rewriting them — the header would never reach the wire.
	owned := make(map[string]bool, len(ranked))
	for _, f := range ranked {
		if f.Value != "" {
			owned[strings.ToLower(f.Name)] = true
		}
	}

	// Drop caller entries the profile is about to write. Without this pass a
	// caller header spelled "Sec-Ch-Ua" would survive alongside the profile's
	// "sec-ch-ua" and the request would carry the field twice — a shape no
	// browser produces, and one that defeats the Set semantics this pipeline
	// promises. Deleting before writing also keeps the map from holding two
	// keys for one field while the order list names only one of them.
	for name := range *dst {
		if owned[strings.ToLower(name)] {
			delete(*dst, name)
		}
	}

	regular := make([]string, 0, len(ranked))
	pseudo := make([]string, 0, 4)
	for _, f := range ranked {
		name := strings.ToLower(f.Name)
		if strings.HasPrefix(name, ":") {
			pseudo = append(pseudo, name)
			continue
		}
		regular = append(regular, name)
		if f.Value != "" {
			// Direct assignment: Header.Set canonicalises the key, and the
			// key's exact spelling is what reaches the wire on HTTP/1.1.
			(*dst)[f.Name] = []string{f.Value}
		}
	}

	if len(regular) > 0 {
		(*dst)[ehttp.HeaderOrderKey] = regular
	}
	if len(pseudo) > 0 {
		(*dst)[ehttp.PHeaderOrderKey] = pseudo
	}
}
