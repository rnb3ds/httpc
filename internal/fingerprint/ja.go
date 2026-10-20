// TLS ClientHello (JA3/JA4) fingerprint mechanics: spec selection,
// per-connection mutation, and ALPN rewriting.
//
// The per-connection mutations run on a private deep copy of the profile's
// spec (cloneSpec): Chromium's extension-order shuffle is in-place, and the
// GREASE signature-algorithm substitution writes into the
// SignatureAlgorithmsExtension, so dialing from the shared spec without
// copying would race concurrent dials and corrupt the package-level profile.
//
// Reference: https://lwthiker.com/networks/2022/06/17/tls-fingerprinting.html
// Shuffle/GREASE mechanics ported from github.com/enetx/surf (ja.go), MIT.

package fingerprint

import (
	"crypto/rand"
	"fmt"
	"math/big"

	utls "github.com/refraction-networking/utls"
)

// specForDial returns the ClientHelloSpec to use for one connection: a deep
// copy of the profile's spec (or the utls built-in spec for HelloID-based
// profiles, which UTLSIdToSpec constructs fresh on every call), with the
// per-connection mutations applied.
func specForDial(p *browserProfile) (*utls.ClientHelloSpec, error) {
	var spec *utls.ClientHelloSpec

	if p.helloSpec != nil {
		spec = cloneSpec(p.helloSpec)
	} else {
		built, err := utls.UTLSIdToSpec(p.helloID)
		if err != nil {
			return nil, fmt.Errorf("fingerprint: build spec from %s: %w", p.helloID.Str(), err)
		}
		spec = &built
	}

	if p.shuffleExtensions {
		spec.Extensions = utls.ShuffleChromeTLSExtensions(spec.Extensions)
	}

	greaseSignatureAlgorithms(spec)

	return spec, nil
}

// greaseSignatureAlgorithms replaces every GREASE placeholder in the spec's
// signature_algorithms list with a fresh random GREASE value. Chrome sends a
// GREASE signature scheme at the head of that list and picks a new value per
// connection; uTLS substitutes GREASE placeholders for cipher suites, groups,
// key shares, versions and extension IDs but leaves signature schemes
// untouched, so it is done here.
func greaseSignatureAlgorithms(spec *utls.ClientHelloSpec) {
	for _, ext := range spec.Extensions {
		sigalgs, ok := ext.(*utls.SignatureAlgorithmsExtension)
		if !ok {
			continue
		}
		for i, scheme := range sigalgs.SupportedSignatureAlgorithms {
			if scheme == utls.GREASE_PLACEHOLDER {
				sigalgs.SupportedSignatureAlgorithms[i] = utls.SignatureScheme(randomGREASE())
			}
		}
	}
}

// randomGREASE returns one of the 16 GREASE code points (0x0a0a, 0x1a1a, …
// 0xfafa). A crypto/rand failure falls back to the placeholder, which uTLS
// substitutes with its own GREASE value.
func randomGREASE() uint16 {
	n, err := rand.Int(rand.Reader, big.NewInt(16))
	if err != nil {
		return utls.GREASE_PLACEHOLDER
	}
	return uint16(0x0a0a + 0x1010*n.Int64())
}

// setAlpnProtocolToHTTP1 rewrites the spec's ALPN extension to drop "h2" and
// keep (or add) "http/1.1", for connections that must not negotiate HTTP/2.
func setAlpnProtocolToHTTP1(spec *utls.ClientHelloSpec) {
	for _, ext := range spec.Extensions {
		alpns, ok := ext.(*utls.ALPNExtension)
		if !ok {
			continue
		}

		filtered := alpns.AlpnProtocols[:0]
		hasHTTP1 := false

		for _, proto := range alpns.AlpnProtocols {
			if proto == "h2" {
				continue
			}

			if proto == "http/1.1" {
				hasHTTP1 = true
			}

			filtered = append(filtered, proto)
		}

		alpns.AlpnProtocols = filtered

		if !hasHTTP1 {
			alpns.AlpnProtocols = append(alpns.AlpnProtocols, "http/1.1")
		}

		return
	}

	spec.Extensions = append(spec.Extensions, &utls.ALPNExtension{
		AlpnProtocols: []string{"http/1.1"},
	})
}

// supportsResumption reports whether a ClientHelloSpec carries the extensions
// TLS session resumption requires. Kept for the future session-resumption
// knob; the transport currently disables resumption (matching the historical
// behavior of this path), so it is not wired into the handshake config.
//
//nolint:unused // reserved for the session-resumption knob; see above
func supportsResumption(spec utls.ClientHelloSpec) bool {
	var (
		hasSessionTicket bool
		hasPskModes      bool
		hasPreSharedKey  bool
	)

	for _, ext := range spec.Extensions {
		switch ext.(type) {
		case *utls.SessionTicketExtension:
			hasSessionTicket = true
		case *utls.PSKKeyExchangeModesExtension:
			hasPskModes = true
		case *utls.UtlsPreSharedKeyExtension, *utls.FakePreSharedKeyExtension:
			hasPreSharedKey = true
		}

		if hasSessionTicket && hasPskModes && hasPreSharedKey {
			return true
		}
	}

	// If any TLS 1.3 PSK-related extensions are present, resumption is valid
	// only when all TLS 1.3 resumption indicators are present simultaneously.
	if hasPskModes || hasPreSharedKey {
		return false
	}

	// Otherwise fall back to TLS 1.2 semantics where the session ticket
	// extension alone indicates resumption support.
	return hasSessionTicket
}
