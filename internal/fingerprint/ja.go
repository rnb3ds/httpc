// TLS ClientHello (JA3/JA4) fingerprint mechanics: per-connection spec
// construction and its mutations.
//
// Every handshake asks the profile for a fresh ClientHelloSpec (browserProfile
// .hello). Chrome profiles build theirs from the local spec data in
// profile_chrome.go — utls upstream ships no hello for Chrome 152 — and the
// mutations run on that fresh value: the extension-order shuffle and the
// GREASE signature-algorithm substitution. Firefox profiles delegate to the
// utls built-in hello, which UTLSIdToSpec constructs fresh on every call.
//
// Neither mutation touches package-level state: no connection can observe
// another's spec, so concurrent dials need no lock and no deep-clone step.
//
// Reference: https://lwthiker.com/networks/2022/06/17/tls-fingerprinting.html
// Shuffle/GREASE mechanics ported from github.com/enetx/surf (ja.go), MIT.

package fingerprint

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"

	utls "github.com/bogdanfinn/utls"
)

// specForDial returns the ClientHelloSpec to use for one connection: a fresh
// value from the profile, with the per-connection mutations applied.
func specForDial(p *browserProfile) (utls.ClientHelloSpec, error) {
	spec, err := p.hello()
	if err != nil {
		return utls.ClientHelloSpec{}, fmt.Errorf("fingerprint: build ClientHello spec for %s: %w", p.name, err)
	}

	if p.shuffleExtensions {
		spec.Extensions = utls.ShuffleChromeTLSExtensions(spec.Extensions)
	}
	greaseSignatureAlgorithms(&spec)

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
// 0xfafa).
func randomGREASE() uint16 {
	return randomGREASEFrom(rand.Reader)
}

// randomGREASEFrom is randomGREASE with an injectable entropy source, so the
// crypto/rand failure arm is testable without swapping the process-wide
// rand.Reader — a global write that races against every other goroutine using
// it (the test suite dials concurrently).
//
// A failure falls back to the placeholder, which uTLS substitutes with its own
// GREASE value at write time.
func randomGREASEFrom(source io.Reader) uint16 {
	n, err := rand.Int(source, big.NewInt(16))
	if err != nil {
		return utls.GREASE_PLACEHOLDER
	}
	return uint16(0x0a0a + 0x1010*n.Int64())
}

// helloFromSpec returns a per-handshake factory over a fixed spec value.
//
// It hands out a DEEP copy, not a shallow one, and that is load-bearing: the
// transport library rewrites fields inside extension structs it was given —
// the forced-HTTP/1.1 path replaces ALPNExtension.AlpnProtocols in place, and
// the h3-removal path edits ApplicationSettingsExtensionNew — so a shallow
// copy would let one EnableHTTP2=false request permanently pin the shared
// profile's ALPN list to http/1.1 and silently downgrade every later
// connection. TestTransport_NegotiationModes is the regression guard.
func helloFromSpec(spec utls.ClientHelloSpec) func() (utls.ClientHelloSpec, error) {
	return func() (utls.ClientHelloSpec, error) {
		return *cloneSpec(&spec), nil
	}
}

// helloFromID returns a per-handshake factory over a utls built-in hello.
// UTLSIdToSpec constructs a fresh spec on every call, so the factory's value
// is already private to the handshake; the clone is kept anyway so both
// factories share one invariant (nothing a handshake mutates can outlive it)
// and so a future change to UTLSIdToSpec's allocation strategy cannot
// reintroduce the shared-state hazard above.
func helloFromID(id utls.ClientHelloID) func() (utls.ClientHelloSpec, error) {
	return func() (utls.ClientHelloSpec, error) {
		spec, err := utls.UTLSIdToSpec(id)
		if err != nil {
			return utls.ClientHelloSpec{}, err
		}
		return *cloneSpec(&spec), nil
	}
}
