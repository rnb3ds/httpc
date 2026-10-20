package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

func generateTestCertificate(t *testing.T) ([]byte, *x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	return certDER, cert, privateKey
}

// mustnewPublicKeyPinner creates a pinner or fails the test.
func mustnewPublicKeyPinner(t *testing.T, publicKeys ...[]byte) *publicKeyPinner {
	t.Helper()
	p, err := newPublicKeyPinner(publicKeys...)
	if err != nil {
		t.Fatalf("newPublicKeyPinner failed: %v", err)
	}
	return p
}

// TestExportedConstructors verifies the exported constructor wrappers
// (NewSPKIHashPinner, NewPublicKeyPinner, NewCertificatePinnerChain) delegate
// correctly to their unexported counterparts. These are the public API surface
// used by the root httpc package.
func TestExportedConstructors(t *testing.T) {
	_, cert, _ := generateTestCertificate(t)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}
	hash := sha256.Sum256(pubKeyBytes)
	validHash := base64.StdEncoding.EncodeToString(hash[:])
	secondDigest := sha256.Sum256([]byte("different"))
	hash2 := base64.StdEncoding.EncodeToString(secondDigest[:])

	tests := []struct {
		name    string
		build   func() (CertificatePinner, error)
		wantErr bool
	}{
		{"NewSPKIHashPinner valid", func() (CertificatePinner, error) {
			return NewSPKIHashPinner(validHash)
		}, false},
		{"NewSPKIHashPinner multiple for rotation", func() (CertificatePinner, error) {
			return NewSPKIHashPinner(validHash, hash2)
		}, false},
		{"NewSPKIHashPinner empty returns error", func() (CertificatePinner, error) {
			return NewSPKIHashPinner()
		}, true},
		{"NewSPKIHashPinner invalid base64 returns error", func() (CertificatePinner, error) {
			return NewSPKIHashPinner("not-valid-base64!!!")
		}, true},
		{"NewPublicKeyPinner valid", func() (CertificatePinner, error) {
			return NewPublicKeyPinner(pubKeyBytes)
		}, false},
		{"NewPublicKeyPinner empty returns error", func() (CertificatePinner, error) {
			return NewPublicKeyPinner()
		}, true},
		{"NewCertificatePinnerChain", func() (CertificatePinner, error) {
			a, err := NewSPKIHashPinner(validHash)
			if err != nil {
				return nil, err
			}
			b, err := NewSPKIHashPinner(hash2)
			if err != nil {
				return nil, err
			}
			return NewCertificatePinnerChain(a, b), nil
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinner, err := tt.build()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pinner == nil {
				t.Fatal("expected non-nil pinner")
			}
			if pinner.Pin() == "" {
				t.Error("expected non-empty Pin() description")
			}
		})
	}
}

func TestPublicKeyPinner(t *testing.T) {
	certDER, cert, _ := generateTestCertificate(t)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	t.Run("newPublicKeyPinner", func(t *testing.T) {
		pinner := mustnewPublicKeyPinner(t, pubKeyBytes)
		if pinner == nil {
			t.Fatal("expected non-nil pinner")
		}
		if pinner.Pin() == "" {
			t.Error("expected non-empty pin description")
		}
	})

	t.Run("newPublicKeyPinner empty", func(t *testing.T) {
		_, err := newPublicKeyPinner()
		if err == nil {
			t.Error("expected error for no public keys")
		}
	})

	t.Run("VerifyPeerCertificate matching", func(t *testing.T) {
		pinner := mustnewPublicKeyPinner(t, pubKeyBytes)
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching public key, got %v", err)
		}
	})

	t.Run("VerifyPeerCertificate non-matching", func(t *testing.T) {
		_, cert2, _ := generateTestCertificate(t)
		pubKeyBytes2, err := x509.MarshalPKIXPublicKey(cert2.PublicKey)
		if err != nil {
			t.Fatalf("failed to marshal public key: %v", err)
		}

		pinner := mustnewPublicKeyPinner(t, pubKeyBytes2)
		err = pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error for non-matching public key")
		}
	})

	t.Run("no certificates", func(t *testing.T) {
		pinner := mustnewPublicKeyPinner(t, pubKeyBytes)
		err := pinner.VerifyPeerCertificate(nil, nil)
		if err == nil {
			t.Error("expected error for no certificates")
		}
	})

	t.Run("invalid certificate", func(t *testing.T) {
		pinner := mustnewPublicKeyPinner(t, pubKeyBytes)
		err := pinner.VerifyPeerCertificate([][]byte{{0x01, 0x02, 0x03}}, nil)
		if err == nil {
			t.Error("expected error for invalid certificate that doesn't match the pinned key")
		}
	})
}

func TestSPKIHashPinner(t *testing.T) {
	certDER, cert, _ := generateTestCertificate(t)
	spkiBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	// newSPKIHashPinner construction (valid/invalid-base64/empty) is covered
	// by TestExportedConstructors (exported wrappers) and
	// TestSPKIHashPinnerConstruction (real certificate keys) — no third copy here.

	t.Run("Pin description", func(t *testing.T) {
		hash := base64.StdEncoding.EncodeToString([]byte("test-hash-32-bytes-long-enough!!"))
		pinner, _ := newSPKIHashPinner(hash)
		if pinner.Pin() == "" {
			t.Error("expected non-empty pin description")
		}
	})

	t.Run("VerifyPeerCertificate matching", func(t *testing.T) {
		spkiHash := sha256.Sum256(spkiBytes)
		spkiHashBase64 := base64.StdEncoding.EncodeToString(spkiHash[:])
		pinner, err := newSPKIHashPinner(spkiHashBase64)
		if err != nil {
			t.Fatalf("failed to create pinner: %v", err)
		}

		err = pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching SPKI hash, got %v", err)
		}
	})

	t.Run("VerifyPeerCertificate non-matching", func(t *testing.T) {
		wrong := sha256.Sum256([]byte("not-the-correct-hash"))
		hash := base64.StdEncoding.EncodeToString(wrong[:])
		pinner, _ := newSPKIHashPinner(hash)
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error for non-matching SPKI hash")
		}
	})

	t.Run("nil pinner fails closed", func(t *testing.T) {
		var pinner *spkiHashPinner
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("nil pinner must reject (fail closed), got nil error")
		}
	})

	t.Run("pin stuffing: unverified chain cert must not satisfy pin", func(t *testing.T) {
		// A MITM presents [attacker-leaf, victim-real-cert] (the victim cert
		// is publicly downloadable). The pin targets the victim's key.
		victimDER, victimCert, _ := generateTestCertificate(t)
		victimSPKI := sha256.Sum256(victimCert.RawSubjectPublicKeyInfo)
		pin := base64.StdEncoding.EncodeToString(victimSPKI[:])
		pinner, err := newSPKIHashPinner(pin)
		if err != nil {
			t.Fatalf("newSPKIHashPinner failed: %v", err)
		}

		// InsecureSkipVerify mode: only the leaf is eligible, so a pin on the
		// stuffed victim cert must fail even though rawCerts contains it.
		err = pinner.VerifyPeerCertificate([][]byte{certDER, victimDER}, nil)
		if err == nil {
			t.Error("expected error: stuffed chain cert must not satisfy pin in leaf-only mode")
		}

		// Verification-on mode: attacker leaf is verified (self-signed test
		// chain), victim cert is NOT part of any verified chain — pin must fail.
		err = pinner.VerifyPeerCertificate(
			[][]byte{certDER, victimDER},
			[][]*x509.Certificate{{cert}},
		)
		if err == nil {
			t.Error("expected error: cert outside verified chains must not satisfy pin")
		}
	})

	t.Run("verified-chain pin: intermediate in verified chain satisfies pin", func(t *testing.T) {
		// Legitimate CA/intermediate pinning: the pinned cert appears in the
		// verified chain, so the pin must succeed.
		spki := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
		pin := base64.StdEncoding.EncodeToString(spki[:])
		pinner, err := newSPKIHashPinner(pin)
		if err != nil {
			t.Fatalf("newSPKIHashPinner failed: %v", err)
		}
		err = pinner.VerifyPeerCertificate(
			[][]byte{certDER},
			[][]*x509.Certificate{{cert}},
		)
		if err != nil {
			t.Errorf("expected verified-chain pin match to succeed, got %v", err)
		}
	})
}

func TestCertificatePinnerChain(t *testing.T) {
	certDER, cert, _ := generateTestCertificate(t)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	t.Run("nil chain fails closed", func(t *testing.T) {
		var chain *certificatePinnerChain
		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("nil chain must reject (fail closed), got nil error")
		}
	})

	t.Run("matching pinner in chain", func(t *testing.T) {
		pinner := mustnewPublicKeyPinner(t, pubKeyBytes)
		chain := newCertificatePinnerChain(pinner)
		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching chain, got %v", err)
		}
	})

	t.Run("any pinner match in chain", func(t *testing.T) {
		_, cert2, _ := generateTestCertificate(t)
		pubKeyBytes2, _ := x509.MarshalPKIXPublicKey(cert2.PublicKey)

		pinner1 := mustnewPublicKeyPinner(t, pubKeyBytes2)
		pinner2 := mustnewPublicKeyPinner(t, pubKeyBytes)
		chain := newCertificatePinnerChain(pinner1, pinner2)

		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error when any pinner matches, got %v", err)
		}
	})

	t.Run("no pinner matches in chain", func(t *testing.T) {
		_, cert2, _ := generateTestCertificate(t)
		pubKeyBytes2, _ := x509.MarshalPKIXPublicKey(cert2.PublicKey)

		pinner := mustnewPublicKeyPinner(t, pubKeyBytes2)
		chain := newCertificatePinnerChain(pinner)

		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error when no pinner matches")
		}
	})

	t.Run("Pin description", func(t *testing.T) {
		pinner := mustnewPublicKeyPinner(t, pubKeyBytes)
		chain := newCertificatePinnerChain(pinner)
		if chain.Pin() == "" {
			t.Error("expected non-empty pin description")
		}
	})
}

// TestSPKIHashPinnerConstruction covers base64 SPKI-hash construction as a
// table (valid/invalid hash formats, trimming, pin counts) plus the
// raw-SPKI-hash verification round-trip (replaces the former
// newPublicKeyPinnerFromBase64 tests; the hash is computed from the raw SPKI
// bytes, matching the verify path since the M23 fix).
func TestSPKIHashPinnerConstruction(t *testing.T) {
	certDER, cert, _ := generateTestCertificate(t)
	spkiHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	spkiHashBase64 := base64.StdEncoding.EncodeToString(spkiHash[:])
	secondDigest := sha256.Sum256([]byte("another-spki-hash"))
	hash2 := base64.StdEncoding.EncodeToString(secondDigest[:])
	notSHA256 := base64.StdEncoding.EncodeToString([]byte("wrong-hash-32-bytes-long-enough!!")) // 33 bytes

	tests := []struct {
		name     string
		hashes   []string
		wantErr  bool
		contains string // substring of the expected error message
		wantPins int    // pin count in the Pin() description on success
	}{
		{"valid hash", []string{spkiHashBase64}, false, "", 1},
		{"multiple hashes", []string{spkiHashBase64, hash2}, false, "", 2},
		{"no hashes at all", nil, true, "at least one valid SPKI hash is required", 0},
		{"empty hash returns error", []string{""}, true, "at least one valid SPKI hash is required", 0},
		{"whitespace hash is trimmed", []string{"  " + spkiHashBase64 + "  "}, false, "", 1},
		{"invalid base64 returns error", []string{"not-valid-base64!!!"}, true, "invalid base64 hash", 0},
		{"wrong-length hash rejected at construction", []string{notSHA256}, true, "invalid SPKI hash", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinner, err := newSPKIHashPinner(tt.hashes...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.contains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pinner == nil {
				t.Fatal("expected non-nil pinner")
			}
			if pinner.Pin() == "" {
				t.Error("expected non-empty pin description")
			}
			if want := fmt.Sprintf("spki-pins:%d", tt.wantPins); pinner.Pin() != want {
				t.Errorf("Pin() = %q, want %q", pinner.Pin(), want)
			}
		})
	}

	t.Run("verify matching certificate", func(t *testing.T) {
		pinner, err := newSPKIHashPinner(spkiHashBase64)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching hash, got %v", err)
		}
	})

	t.Run("verify non-matching certificate", func(t *testing.T) {
		wrong := sha256.Sum256([]byte("wrong-hash"))
		wrongHash := base64.StdEncoding.EncodeToString(wrong[:])
		pinner, err := newSPKIHashPinner(wrongHash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error for non-matching hash")
		}
	})

	t.Run("nil pinner fails closed", func(t *testing.T) {
		var pinner *publicKeyPinner
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("nil pinner must reject (fail closed), got nil error")
		}
	})
}

// TestSPKIHashPinner_MixedValidInvalidHashOrdering pins construction behavior
// when valid and invalid hashes are mixed: a valid entry never rescues an
// invalid one (validation is not short-circuited on first success), and the
// error names the FIRST invalid hash in argument order. Blank entries remain
// skippable even in mixed company.
func TestSPKIHashPinner_MixedValidInvalidHashOrdering(t *testing.T) {
	_, cert, _ := generateTestCertificate(t)
	validDigest := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	validHash := base64.StdEncoding.EncodeToString(validDigest[:])
	invalidBase64 := "not-valid-base64!!!"
	wrongLength := base64.StdEncoding.EncodeToString([]byte("wrong-hash-32-bytes-long-enough!!")) // 33 bytes

	tests := []struct {
		name     string
		hashes   []string
		wantErr  bool
		contains string
	}{
		{"valid then invalid base64 still rejected", []string{validHash, invalidBase64}, true, "invalid base64 hash"},
		{"invalid base64 then valid", []string{invalidBase64, validHash}, true, "invalid base64 hash"},
		{"valid then wrong length still rejected", []string{validHash, wrongLength}, true, "invalid SPKI hash"},
		{"wrong length before invalid base64 wins", []string{wrongLength, invalidBase64}, true, "invalid SPKI hash"},
		{"invalid base64 before wrong length wins", []string{invalidBase64, wrongLength}, true, "invalid base64 hash"},
		{"blank entries skipped around valid", []string{"", "   ", validHash}, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinner, err := newSPKIHashPinner(tt.hashes...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error for mixed hashes, got nil")
				}
				if !strings.Contains(err.Error(), tt.contains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.contains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pinner == nil {
				t.Fatal("expected non-nil pinner")
			}
		})
	}
}

// TestCertificatePinnerChain_ZeroPinners pins the fail-closed behavior of a
// chain built with no pinners: verification must reject every certificate
// ("no pinners configured") rather than silently disabling pinning.
func TestCertificatePinnerChain_ZeroPinners(t *testing.T) {
	certDER, _, _ := generateTestCertificate(t)

	chain := newCertificatePinnerChain()
	if chain == nil {
		t.Fatal("expected non-nil chain struct")
	}
	err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
	if err == nil {
		t.Fatal("zero-pinner chain must fail closed, got nil error")
	}
	if !strings.Contains(err.Error(), "no pinners configured") {
		t.Errorf("error %q should say 'no pinners configured'", err.Error())
	}

	// The exported constructor behaves identically.
	exported := NewCertificatePinnerChain()
	if err := exported.VerifyPeerCertificate([][]byte{certDER}, nil); err == nil {
		t.Error("exported zero-pinner chain must also fail closed")
	}
}

// TestPinCacheEviction verifies that the certificate pin cache evicts entries
// when it exceeds pinCacheMaxSize, preventing unbounded memory growth.
func TestPinCacheEviction(t *testing.T) {
	digest := sha256.Sum256([]byte("test-hash"))
	hash := base64.StdEncoding.EncodeToString(digest[:])
	pinner, err := newSPKIHashPinner(hash)
	if err != nil {
		t.Fatalf("newSPKIHashPinner failed: %v", err)
	}

	// Generate many unique rawCerts to fill the cache beyond pinCacheMaxSize
	for i := 0; i < pinCacheMaxSize+100; i++ {
		// Each call adds unique fingerprint entries to pinCache
		rawCerts := [][]byte{[]byte(fmt.Sprintf("cert-data-%d", i))}
		_ = pinner.VerifyPeerCertificate(rawCerts, nil)
	}

	pinner.mu.RLock()
	cacheSize := len(pinner.pinCache)
	pinner.mu.RUnlock()

	if cacheSize > pinCacheMaxSize {
		t.Errorf("pinCache grew to %d entries, expected max ~%d", cacheSize, pinCacheMaxSize)
	}
}

// TestPinnerPin_NoPins pins the nil/empty-contract of the three Pin()
// implementations: an unusable pinner must describe itself as "no-pins"
// rather than panic or report a pin count.
func TestPinnerPin_NoPins(t *testing.T) {
	t.Run("nil spki hash pinner", func(t *testing.T) {
		var p *spkiHashPinner
		if got := p.Pin(); got != "no-pins" {
			t.Errorf("nil spkiHashPinner.Pin() = %q, want %q", got, "no-pins")
		}
	})

	t.Run("spki hash pinner without hashes", func(t *testing.T) {
		p := &spkiHashPinner{hashes: map[string]bool{}}
		if got := p.Pin(); got != "no-pins" {
			t.Errorf("empty spkiHashPinner.Pin() = %q, want %q", got, "no-pins")
		}
	})

	t.Run("nil public key pinner", func(t *testing.T) {
		var p *publicKeyPinner
		if got := p.Pin(); got != "no-pins" {
			t.Errorf("nil publicKeyPinner.Pin() = %q, want %q", got, "no-pins")
		}
	})

	t.Run("public key pinner without inner", func(t *testing.T) {
		p := &publicKeyPinner{}
		if got := p.Pin(); got != "no-pins" {
			t.Errorf("innerless publicKeyPinner.Pin() = %q, want %q", got, "no-pins")
		}
	})

	t.Run("nil pinner chain", func(t *testing.T) {
		var c *certificatePinnerChain
		if got := c.Pin(); got != "no-pins" {
			t.Errorf("nil certificatePinnerChain.Pin() = %q, want %q", got, "no-pins")
		}
	})

	t.Run("empty pinner chain", func(t *testing.T) {
		c := &certificatePinnerChain{}
		if got := c.Pin(); got != "no-pins" {
			t.Errorf("empty certificatePinnerChain.Pin() = %q, want %q", got, "no-pins")
		}
	})
}
