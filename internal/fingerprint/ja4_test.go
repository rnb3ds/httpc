package fingerprint

import (
	"testing"

	utls "github.com/bogdanfinn/utls"
)

// TestJA4_OfflineDerivation pins the exact JA4 fingerprints of the shipped
// specs, derived offline (ja4.go marshals the spec the way a handshake would
// and reads the fingerprint off the wire bytes). The desktop-Chrome and
// Firefox values are the same strings the network gates observe live on
// tls.peet.ws; the Safari value matches the dominant live iOS observation
// (2,015-observation row in public HTTP/2/JA4 databases, October 2026). A
// failure here means a spec edit changed the wire shape — fix the spec or
// re-baseline on purpose, never the other way around.
func TestJA4_OfflineDerivation(t *testing.T) {
	tests := []struct {
		name string
		spec utls.ClientHelloSpec
		want string
	}{
		{"chrome 152-155 desktop", chrome152Spec, "t13d1517h2_8daaf6152771_cb7bf5808d99"},
		{"firefox 148/158 (HelloFirefox_148)", func() utls.ClientHelloSpec {
			spec, err := utls.UTLSIdToSpec(utls.HelloFirefox_148)
			if err != nil {
				t.Fatal(err)
			}
			return spec
		}(), "t13d1715h2_5b57614c22b0_a54fffd0eb61"},
		{"iOS WebKit (HelloSafari_26_3)", func() utls.ClientHelloSpec {
			spec, err := utls.UTLSIdToSpec(utls.HelloSafari_26_3)
			if err != nil {
				t.Fatal(err)
			}
			return spec
		}(), "t13d2013h2_a09f3c656075_7f0f34a4126d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ja4OfSpec(tt.spec)
			if err != nil {
				t.Fatalf("ja4OfSpec: %v", err)
			}
			if got != tt.want {
				t.Errorf("JA4 = %s, want %s", got, tt.want)
			}
		})
	}
}
