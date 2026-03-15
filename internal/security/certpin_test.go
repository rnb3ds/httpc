package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
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

func TestPublicKeyPinner(t *testing.T) {
	certDER, cert, _ := generateTestCertificate(t)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	t.Run("NewPublicKeyPinner", func(t *testing.T) {
		pinner := NewPublicKeyPinner(pubKeyBytes)
		if pinner == nil {
			t.Fatal("expected non-nil pinner")
		}
		if pinner.Pin() == "" {
			t.Error("expected non-empty pin description")
		}
	})

	t.Run("VerifyPeerCertificate matching", func(t *testing.T) {
		pinner := NewPublicKeyPinner(pubKeyBytes)
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching public key, got %v", err)
		}
	})

	t.Run("VerifyPeerCertificate non-matching", func(t *testing.T) {
		// Create a different key
		_, cert2, _ := generateTestCertificate(t)
		pubKeyBytes2, err := x509.MarshalPKIXPublicKey(cert2.PublicKey)
		if err != nil {
			t.Fatalf("failed to marshal public key: %v", err)
		}

		pinner := NewPublicKeyPinner(pubKeyBytes2)
		err = pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error for non-matching public key")
		}
	})

	t.Run("no certificates", func(t *testing.T) {
		pinner := NewPublicKeyPinner(pubKeyBytes)
		err := pinner.VerifyPeerCertificate(nil, nil)
		if err == nil {
			t.Error("expected error for no certificates")
		}
	})

	t.Run("invalid certificate", func(t *testing.T) {
		pinner := NewPublicKeyPinner(pubKeyBytes)
		err := pinner.VerifyPeerCertificate([][]byte{{0x01, 0x02, 0x03}}, nil)
		// Invalid certificates don't match, so we should get an error
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

	t.Run("NewSPKIHashPinner valid", func(t *testing.T) {
		// Use a valid base64 string
		hash := base64.StdEncoding.EncodeToString([]byte("test-hash-32-bytes-long-enough!!"))
		pinner, err := NewSPKIHashPinner(hash)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if pinner == nil {
			t.Fatal("expected non-nil pinner")
		}
	})

	t.Run("NewSPKIHashPinner invalid base64", func(t *testing.T) {
		_, err := NewSPKIHashPinner("not-valid-base64!!!")
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})

	t.Run("NewSPKIHashPinner empty", func(t *testing.T) {
		_, err := NewSPKIHashPinner()
		if err == nil {
			t.Error("expected error for empty hashes")
		}
	})

	t.Run("Pin description", func(t *testing.T) {
		hash := base64.StdEncoding.EncodeToString([]byte("test-hash-32-bytes-long-enough!!"))
		pinner, _ := NewSPKIHashPinner(hash)
		if pinner.Pin() == "" {
			t.Error("expected non-empty pin description")
		}
	})

	t.Run("VerifyPeerCertificate matching", func(t *testing.T) {
		// Create pinner with the actual certificate's SPKI hash
		spkiHash := sha256.Sum256(spkiBytes)
		spkiHashBase64 := base64.StdEncoding.EncodeToString(spkiHash[:])
		pinner, err := NewSPKIHashPinner(spkiHashBase64)
		if err != nil {
			t.Fatalf("failed to create pinner: %v", err)
		}

		err = pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching SPKI hash, got %v", err)
		}
	})

	t.Run("VerifyPeerCertificate non-matching", func(t *testing.T) {
		hash := base64.StdEncoding.EncodeToString([]byte("not-the-correct-hash-32-bytes!"))
		pinner, _ := NewSPKIHashPinner(hash)
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error for non-matching SPKI hash")
		}
	})

	t.Run("nil pinner", func(t *testing.T) {
		var pinner *SPKIHashPinner
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("nil pinner should allow all, got error: %v", err)
		}
	})
}

func TestCertificatePinnerChain(t *testing.T) {
	certDER, cert, _ := generateTestCertificate(t)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	t.Run("nil chain", func(t *testing.T) {
		var chain *CertificatePinnerChain
		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("nil chain should allow all, got error: %v", err)
		}
	})

	t.Run("matching pinner in chain", func(t *testing.T) {
		pinner := NewPublicKeyPinner(pubKeyBytes)
		chain := NewCertificatePinnerChain(pinner)
		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error for matching chain, got %v", err)
		}
	})

	t.Run("any pinner match in chain", func(t *testing.T) {
		// Create a different key
		_, cert2, _ := generateTestCertificate(t)
		pubKeyBytes2, _ := x509.MarshalPKIXPublicKey(cert2.PublicKey)

		pinner1 := NewPublicKeyPinner(pubKeyBytes2) // Non-matching
		pinner2 := NewPublicKeyPinner(pubKeyBytes)  // Matching
		chain := NewCertificatePinnerChain(pinner1, pinner2)

		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("expected no error when any pinner matches, got %v", err)
		}
	})

	t.Run("no pinner matches in chain", func(t *testing.T) {
		// Create a different key
		_, cert2, _ := generateTestCertificate(t)
		pubKeyBytes2, _ := x509.MarshalPKIXPublicKey(cert2.PublicKey)

		pinner := NewPublicKeyPinner(pubKeyBytes2)
		chain := NewCertificatePinnerChain(pinner)

		err := chain.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err == nil {
			t.Error("expected error when no pinner matches")
		}
	})

	t.Run("Pin description", func(t *testing.T) {
		pinner := NewPublicKeyPinner(pubKeyBytes)
		chain := NewCertificatePinnerChain(pinner)
		if chain.Pin() == "" {
			t.Error("expected non-empty pin description")
		}
	})
}

func TestNoOpPinner(t *testing.T) {
	certDER, _, _ := generateTestCertificate(t)

	pinner := &NoOpPinner{}

	t.Run("Pin", func(t *testing.T) {
		if pinner.Pin() != "no-op" {
			t.Errorf("Pin() = %q, want %q", pinner.Pin(), "no-op")
		}
	})

	t.Run("VerifyPeerCertificate", func(t *testing.T) {
		err := pinner.VerifyPeerCertificate([][]byte{certDER}, nil)
		if err != nil {
			t.Errorf("NoOpPinner should allow all, got error: %v", err)
		}
	})

	t.Run("VerifyPeerCertificate nil", func(t *testing.T) {
		err := pinner.VerifyPeerCertificate(nil, nil)
		if err != nil {
			t.Errorf("NoOpPinner should allow all even nil, got error: %v", err)
		}
	})
}
