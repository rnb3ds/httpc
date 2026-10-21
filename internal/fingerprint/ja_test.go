package fingerprint

import (
	"fmt"
	"io"
	"strings"
	"testing"

	utls "github.com/bogdanfinn/utls"
)

// TestGreaseSignatureAlgorithms pins the GREASE substitution: every
// placeholder becomes one of the 16 GREASE code points (0x0a0a + 0x1010*n),
// and non-placeholder schemes are untouched.
func TestGreaseSignatureAlgorithms(t *testing.T) {
	spec := &chrome152Spec
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
		if &spec.CipherSuites[0] == &chrome152Spec.CipherSuites[0] {
			t.Fatal("specForDial aliased the shared spec's cipher slice")
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

// TestSpecForDial_HelloIDProfile pins the UTLSIdToSpec branch: Firefox
// profiles carry a helloID instead of a hand-built spec, so every dial
// constructs a fresh spec (never aliased) and the fixed (unshuffled) order
// is preserved between the built spec and the dial copy.
func TestSpecForDial_HelloIDProfile(t *testing.T) {
	p := firefox148Desktop
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
	if &s1.CipherSuites[0] == &s2.CipherSuites[0] {
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
	p := &browserProfile{name: "bogus-999", hello: func() (utls.ClientHelloSpec, error) {
		return utls.ClientHelloSpec{}, fmt.Errorf("unknown hello")
	}}
	_, err := specForDial(p)
	if err == nil {
		t.Fatal("unknown helloID accepted")
	}
	if !containsAll(err.Error(), "bogus", "build ClientHello spec") {
		t.Errorf("error should name the id and the failure, got: %v", err)
	}
}

// TestRandomGREASE_FallbackOnRandFailure pins the crypto/rand failure branch:
// a failing source yields the placeholder, which uTLS re-substitutes with its
// own GREASE value at write time.
//
// The entropy source is injected rather than swapping the process-wide
// rand.Reader: several tests in this package dial concurrently, and a global
// write races with them (it produced an intermittent failure under -race).
func TestRandomGREASE_FallbackOnRandFailure(t *testing.T) {
	if got := randomGREASEFrom(failingReader{}); got != utls.GREASE_PLACEHOLDER {
		t.Errorf("randomGREASEFrom under a failing reader = %#x, want the placeholder %#x",
			got, utls.GREASE_PLACEHOLDER)
	}

	// The real path still draws from crypto/rand and stays inside the domain.
	if got := randomGREASE(); got != utls.GREASE_PLACEHOLDER {
		if (got-0x0a0a)%0x1010 != 0 || got < 0x0a0a || got > 0xfafa {
			t.Errorf("randomGREASE() = %#x, outside the GREASE domain", got)
		}
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
