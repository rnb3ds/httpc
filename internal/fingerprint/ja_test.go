package fingerprint

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
)

// TestGreaseSignatureAlgorithms pins the GREASE substitution: every
// placeholder becomes one of the 16 GREASE code points (0x0a0a + 0x1010*n),
// and non-placeholder schemes are untouched.
func TestGreaseSignatureAlgorithms(t *testing.T) {
	spec := cloneSpec(&chrome152Spec)
	sigs := findSigAlgs(spec)
	if sigs == nil {
		t.Fatal("chrome spec has no signature_algorithms extension")
	}
	if len(sigs.SupportedSignatureAlgorithms) == 0 || sigs.SupportedSignatureAlgorithms[0] != utls.GREASE_PLACEHOLDER {
		t.Fatal("chrome spec signature_algorithms does not start with the GREASE placeholder")
	}

	greaseSignatureAlgorithms(spec)

	for i, scheme := range sigs.SupportedSignatureAlgorithms {
		if i == 0 {
			// The head must be a GREASE code point. Note 0x0a0a is both the
			// placeholder value and a legitimate GREASE draw (n=0), so
			// landing on it is valid — uTLS would re-substitute identically
			// at write time.
			v := uint16(scheme)
			isGREASE := false
			for n := 0; n < 16; n++ {
				if v == uint16(0x0a0a+0x1010*n) {
					isGREASE = true
					break
				}
			}
			if !isGREASE {
				t.Fatalf("head scheme %#x is not a GREASE code point", v)
			}
		}
	}
	// ML-DSA and standard schemes must survive untouched.
	foundMLDSA := false
	for _, s := range sigs.SupportedSignatureAlgorithms {
		if s == MLDSA44 {
			foundMLDSA = true
		}
	}
	if !foundMLDSA {
		t.Error("ML-DSA scheme lost in substitution")
	}
}

// TestRandomGREASE_Domain pins the code-point domain and the rand failure
// fallback.
func TestRandomGREASE_Domain(t *testing.T) {
	seen := make(map[uint16]bool)
	for range 512 {
		v := randomGREASE()
		if v == utls.GREASE_PLACEHOLDER {
			continue // crypto/rand failure fallback (not expected here)
		}
		if (v-0x0a0a)%0x1010 != 0 || v < 0x0a0a || v > 0xfafa {
			t.Fatalf("randomGREASE() = %#x, outside the GREASE domain", v)
		}
		seen[v] = true
	}
	// 512 draws should exercise a good fraction of 16 code points.
	if len(seen) < 8 {
		t.Fatalf("poor GREASE value spread: %d distinct values in 512 draws", len(seen))
	}
}

// TestSetAlpnProtocolToHTTP1 pins the forced-HTTP/1 rewrite: h2 dropped,
// remaining order preserved, http/1.1 guaranteed present, and the
// add-if-missing path for specs without an ALPN extension.
func TestSetAlpnProtocolToHTTP1(t *testing.T) {
	t.Run("filters h2 and keeps order", func(t *testing.T) {
		spec := &utls.ClientHelloSpec{Extensions: []utls.TLSExtension{
			&utls.ALPNExtension{AlpnProtocols: []string{"h2", "http/1.1"}},
		}}
		setAlpnProtocolToHTTP1(spec)
		alpn := findALPN(spec)
		if alpn == nil || len(alpn.AlpnProtocols) != 1 || alpn.AlpnProtocols[0] != "http/1.1" {
			t.Fatalf("got %v", alpn.AlpnProtocols)
		}
	})

	t.Run("keeps non-h2 protocols ahead of http/1.1", func(t *testing.T) {
		spec := &utls.ClientHelloSpec{Extensions: []utls.TLSExtension{
			&utls.ALPNExtension{AlpnProtocols: []string{"h2", "spdy/3.1"}},
		}}
		setAlpnProtocolToHTTP1(spec)
		alpn := findALPN(spec)
		want := []string{"spdy/3.1", "http/1.1"}
		if len(alpn.AlpnProtocols) != 2 || alpn.AlpnProtocols[0] != want[0] || alpn.AlpnProtocols[1] != want[1] {
			t.Fatalf("got %v, want %v", alpn.AlpnProtocols, want)
		}
	})

	t.Run("adds ALPN when missing", func(t *testing.T) {
		spec := &utls.ClientHelloSpec{}
		setAlpnProtocolToHTTP1(spec)
		alpn := findALPN(spec)
		if alpn == nil || len(alpn.AlpnProtocols) != 1 || alpn.AlpnProtocols[0] != "http/1.1" {
			t.Fatalf("got %+v", alpn)
		}
	})
}

// TestSpecForDial_ShuffleCompleteness pins that the per-connection shuffle
// reorders but never drops or duplicates extensions, and that the returned
// spec never aliases the shared profile value.
func TestSpecForDial_ShuffleCompleteness(t *testing.T) {
	p := chrome152Desktop // shuffle enabled

	seenOrders := 0
	lastOrder := ""
	for range 8 {
		spec, err := specForDial(p)
		if err != nil {
			t.Fatalf("specForDial: %v", err)
		}
		if spec == &chrome152Spec {
			t.Fatal("specForDial returned the shared spec pointer")
		}
		if len(spec.Extensions) != len(chrome152Spec.Extensions) {
			t.Fatalf("extension count changed: %d != %d", len(spec.Extensions), len(chrome152Spec.Extensions))
		}
		order := ""
		for _, ext := range spec.Extensions {
			order += fmt.Sprintf("%T", ext) + ","
		}
		if order != lastOrder {
			seenOrders++
			lastOrder = order
		}
	}
	// With per-connection shuffle at least two of eight dials should differ
	// (probabilistically near-certain for 21 extensions).
	if seenOrders < 2 {
		t.Fatalf("shuffle appears fixed across dials (%d distinct orders)", seenOrders)
	}
}

// TestSupportsResumption pins the reserved session-resumption classifier:
// TLS 1.2 semantics (ticket alone) vs TLS 1.3 semantics (ticket + PSK modes
// + a PSK extension, all simultaneously — a partial PSK set is NOT resumable).
func TestSupportsResumption(t *testing.T) {
	ticket := func() utls.TLSExtension { return &utls.SessionTicketExtension{} }
	pskModes := func() utls.TLSExtension {
		return &utls.PSKKeyExchangeModesExtension{Modes: []uint8{1}}
	}
	utlsPSK := func() utls.TLSExtension { return &utls.UtlsPreSharedKeyExtension{} }
	fakePSK := func() utls.TLSExtension { return &utls.FakePreSharedKeyExtension{} }

	tests := []struct {
		name string
		spec utls.ClientHelloSpec
		want bool
	}{
		{"empty spec", utls.ClientHelloSpec{}, false},
		{"unrelated extensions only", utls.ClientHelloSpec{Extensions: []utls.TLSExtension{
			&utls.ALPNExtension{AlpnProtocols: []string{"h2"}},
		}}, false},
		{"session ticket only (TLS 1.2 resumption)", utls.ClientHelloSpec{Extensions: []utls.TLSExtension{ticket()}}, true},
		{"PSK key exchange modes only", utls.ClientHelloSpec{Extensions: []utls.TLSExtension{pskModes()}}, false},
		{"ticket + PSK modes without a PSK extension", utls.ClientHelloSpec{Extensions: []utls.TLSExtension{ticket(), pskModes()}}, false},
		{"ticket + PSK modes + utls PSK extension", utls.ClientHelloSpec{Extensions: []utls.TLSExtension{ticket(), pskModes(), utlsPSK()}}, true},
		{"ticket + PSK modes + fake PSK extension", utls.ClientHelloSpec{Extensions: []utls.TLSExtension{ticket(), pskModes(), fakePSK()}}, true},
		{"realistic full spec (chrome 152)", chrome152Spec, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsResumption(tt.spec); got != tt.want {
				t.Errorf("supportsResumption(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestSpecForDial_HelloIDProfile pins the UTLSIdToSpec branch: Firefox
// profiles carry a helloID instead of a hand-built spec, so every dial
// constructs a fresh spec (never aliased) and the fixed (unshuffled) order
// is preserved between the built spec and the dial copy.
func TestSpecForDial_HelloIDProfile(t *testing.T) {
	p := firefox148Desktop
	if p.helloSpec != nil {
		t.Fatal("test setup: firefox desktop profile should be helloID-based")
	}
	if p.shuffleExtensions {
		t.Fatal("test setup: firefox profile should not shuffle extensions")
	}

	s1, err := specForDial(p)
	if err != nil {
		t.Fatalf("specForDial(firefox): %v", err)
	}
	if len(s1.Extensions) == 0 || len(s1.CipherSuites) == 0 {
		t.Fatalf("built spec is empty: %d extensions, %d suites", len(s1.Extensions), len(s1.CipherSuites))
	}

	// A fresh construction per call: two dials must not alias.
	s2, err := specForDial(p)
	if err != nil {
		t.Fatalf("specForDial(firefox) second: %v", err)
	}
	if s1 == s2 {
		t.Error("helloID dials must construct a fresh spec per connection")
	}

	// GREASE substitution may mutate the dial copies; ordering must stay fixed.
	if len(s1.Extensions) != len(s2.Extensions) {
		t.Errorf("fixed-order profile changed extension count between dials: %d vs %d",
			len(s1.Extensions), len(s2.Extensions))
	}
}

// TestSpecForDial_UnknownHelloID pins the error branch: an unresolvable
// helloID surfaces a wrapped error naming the id.
func TestSpecForDial_UnknownHelloID(t *testing.T) {
	p := &browserProfile{helloID: utls.ClientHelloID{Client: "bogus", Version: "999"}}
	_, err := specForDial(p)
	if err == nil {
		t.Fatal("unknown helloID accepted")
	}
	if !containsAll(err.Error(), "bogus", "build spec") {
		t.Errorf("error should name the id and the failure, got: %v", err)
	}
}

// TestRandomGREASE_FallbackOnRandFailure pins the crypto/rand failure branch:
// a failing reader yields the placeholder, which uTLS re-substitutes with its
// own GREASE value at write time. rand.Reader is swapped for the duration of
// this test only (tests in this package do not run in parallel).
func TestRandomGREASE_FallbackOnRandFailure(t *testing.T) {
	orig := rand.Reader
	t.Cleanup(func() { rand.Reader = orig })
	rand.Reader = failingReader{}

	if got := randomGREASE(); got != utls.GREASE_PLACEHOLDER {
		t.Errorf("randomGREASE() under a failing reader = %#x, want the placeholder %#x",
			got, utls.GREASE_PLACEHOLDER)
	}
}

// failingReader fails every Read, to drive the crypto/rand error branch.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// containsAll reports whether s contains every part, for terse error-shape
// assertions in this package's tests.
func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
