package httpc

import (
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"
)

// Valid base64-encoded SHA-256 SPKI hash (Let's Encrypt ISRG Root X1, from docs).
// The constructor only validates base64 format, so this is sufficient for unit tests.
const testSPKIHash = "YLh1dUR9y6Kja30RrAn7JKnbQG/uEtLMkBgFF2fuihg="

// testSPKIHashSecondary is a second valid, distinct SPKI hash (GitHub docs),
// shared by tests that need a different-but-valid pin value (e.g. the config
// validation tests in config_test.go) without repeating inline literals.
const testSPKIHashSecondary = "9n7mFq5luXPuMTvPp1LfMNJPtRnnlZqpn6vW3tqS5xE="

// TestNewSPKIHashPinner table-drives the constructor contract. (Consolidates
// the former one-row Valid / MultipleSupportsRotation / InvalidBase64 / Empty
// funcs; each original assertion is preserved as a row.)
func TestNewSPKIHashPinner(t *testing.T) {
	tests := []struct {
		name    string
		hashes  []string
		wantPin string // asserted when non-empty; otherwise an error is expected
		wantErr bool
	}{
		{"valid single hash", []string{testSPKIHash}, "spki-pins:1", false},
		// Multiple hashes are accepted (key rotation) and all counted.
		{"multiple hashes support rotation", []string{testSPKIHash, "C5+lpZ7tcVwmwQIMcRtPbsQtWLABXhQzejna0wHFr8M="}, "spki-pins:2", false},
		{"invalid base64 rejected", []string{"not-valid-base64!!!"}, "", true},
		{"no hashes rejected", nil, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinner, err := NewSPKIHashPinner(tt.hashes...)
			if tt.wantErr {
				if err == nil {
					t.Error("NewSPKIHashPinner accepted invalid input; want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewSPKIHashPinner returned error: %v", err)
			}
			if pinner == nil {
				t.Fatal("NewSPKIHashPinner returned nil pinner")
			}
			if got := pinner.Pin(); got != tt.wantPin {
				t.Errorf("Pin() = %q, want %q", got, tt.wantPin)
			}
		})
	}
}

func TestNewPublicKeyPinner_Empty(t *testing.T) {
	if _, err := NewPublicKeyPinner(); err == nil {
		t.Error("NewPublicKeyPinner accepted no keys; want error")
	}
}

// TestNewPublicKeyPinner_Variants covers the remaining constructor arms: raw
// public keys are hashed into SPKI pins (counted in Pin()), and empty key
// entries are skipped — an all-empty input fails like no input.
func TestNewPublicKeyPinner_Variants(t *testing.T) {
	t.Run("valid keys counted", func(t *testing.T) {
		pinner, err := NewPublicKeyPinner([]byte("raw-pk-1"), []byte("raw-pk-2"))
		if err != nil {
			t.Fatalf("NewPublicKeyPinner returned error: %v", err)
		}
		if got := pinner.Pin(); got != "public-key-pins:2" {
			t.Errorf("Pin() = %q, want %q", got, "public-key-pins:2")
		}
	})

	t.Run("empty entries skipped, one valid key accepted", func(t *testing.T) {
		pinner, err := NewPublicKeyPinner(nil, []byte{}, []byte("raw-pk"))
		if err != nil {
			t.Fatalf("NewPublicKeyPinner returned error: %v", err)
		}
		if got := pinner.Pin(); got != "public-key-pins:1" {
			t.Errorf("Pin() = %q, want %q", got, "public-key-pins:1")
		}
	})

	t.Run("all-empty keys rejected", func(t *testing.T) {
		if _, err := NewPublicKeyPinner(nil, []byte{}); err == nil {
			t.Error("NewPublicKeyPinner accepted only empty keys; want error")
		}
	})
}

// TestCertificatePinnerChain_ZeroPinners pins the fail-closed contract of a
// zero-pinner chain: it accepts NOTHING (a nil return would silently disable
// pinning), and Pin() reports "no-pins".
func TestCertificatePinnerChain_ZeroPinners(t *testing.T) {
	chain := NewCertificatePinnerChain()
	if got := chain.Pin(); got != "no-pins" {
		t.Errorf("zero-pinner chain Pin() = %q, want %q", got, "no-pins")
	}
	if err := chain.VerifyPeerCertificate(nil, nil); err == nil {
		t.Error("zero-pinner chain accepted a handshake; want fail-closed error")
	}
}

// selfSignedCertDER generates a throwaway self-signed certificate and returns
// its DER encoding plus the SPKI hash input (RawSubjectPublicKeyInfo) —
// offline, deterministic, stdlib-only (ed25519 keys make generation fast).
func selfSignedCertDER(t *testing.T) (der, spki []byte) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(cryptorand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err = x509.CreateCertificate(cryptorand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	return der, parsed.RawSubjectPublicKeyInfo
}

// TestSPKIHashPinner_VerifyPeerCertificate drives the verification arms with
// a real (self-generated) certificate: a matching pin accepts, a mismatching
// pin rejects, and a missing peer chain rejects.
func TestSPKIHashPinner_VerifyPeerCertificate(t *testing.T) {
	der, spki := selfSignedCertDER(t)

	correctHash := sha256.Sum256(spki)
	pinner, err := NewSPKIHashPinner(base64.StdEncoding.EncodeToString(correctHash[:]))
	if err != nil {
		t.Fatalf("NewSPKIHashPinner: %v", err)
	}

	t.Run("matching pin accepted (leaf, skip-verify mode)", func(t *testing.T) {
		if err := pinner.VerifyPeerCertificate([][]byte{der}, nil); err != nil {
			t.Errorf("matching pin rejected: %v", err)
		}
	})

	t.Run("mismatching pin rejected", func(t *testing.T) {
		wrong := sha256.Sum256([]byte("not the certificate's key"))
		badPinner, err := NewSPKIHashPinner(base64.StdEncoding.EncodeToString(wrong[:]))
		if err != nil {
			t.Fatalf("NewSPKIHashPinner: %v", err)
		}
		err = badPinner.VerifyPeerCertificate([][]byte{der}, nil)
		if err == nil {
			t.Fatal("mismatching pin accepted; want rejection")
		}
		if !strings.Contains(err.Error(), "no matching SPKI hash") {
			t.Errorf("want 'no matching SPKI hash' diagnosis, got: %v", err)
		}
	})

	t.Run("no peer certificates rejected", func(t *testing.T) {
		if err := pinner.VerifyPeerCertificate(nil, nil); err == nil {
			t.Error("empty peer chain accepted; want rejection")
		}
	})
}

func TestNewCertificatePinnerChain(t *testing.T) {
	a, err := NewSPKIHashPinner(testSPKIHash)
	if err != nil {
		t.Fatalf("NewSPKIHashPinner: %v", err)
	}
	b, err := NewSPKIHashPinner("C5+lpZ7tcVwmwQIMcRtPbsQtWLABXhQzejna0wHFr8M=")
	if err != nil {
		t.Fatalf("NewSPKIHashPinner: %v", err)
	}
	chain := NewCertificatePinnerChain(a, b)
	if got := chain.Pin(); !strings.HasPrefix(got, "chain:[") {
		t.Errorf("chain Pin() = %q, want chain:[...] prefix", got)
	}
}

// TestConfigCertificatePinner_AcceptedByNew ensures a Config carrying a pinner is
// accepted by New() and reflected in String(). This guards the convertToEngineConfig
// wiring end-to-end (public field -> engine.Config.CertificatePinner).
func TestConfigCertificatePinner_AcceptedByNew(t *testing.T) {
	pinner, err := NewSPKIHashPinner(testSPKIHash)
	if err != nil {
		t.Fatalf("NewSPKIHashPinner: %v", err)
	}

	t.Run("disabled by default", func(t *testing.T) {
		cfg := DefaultConfig()
		if s := cfg.String(); !strings.Contains(s, "CertPinning: <disabled>") {
			t.Errorf("DefaultConfig String() = %q, want CertPinning: <disabled>", s)
		}
	})

	t.Run("configured reflected in String", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Security.CertificatePinner = pinner
		if s := cfg.String(); !strings.Contains(s, "CertPinning: <configured>") {
			t.Errorf("Config String() = %q, want CertPinning: <configured>", s)
		}
	})

	t.Run("accepted by New", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Security.CertificatePinner = pinner
		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New with pinner returned error: %v", err)
		}
		if client == nil {
			t.Fatal("New returned nil client")
		}
		if err := client.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}
