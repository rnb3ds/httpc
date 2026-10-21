package security

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// A pin supplied in a non-canonical base64 spelling (a final character whose
// low bits differ but decode to identical bytes) must be stored under its
// canonical form — otherwise it never equals the canonical string the
// verifier compares against, and every connection fails with a confusing
// "no matching SPKI hash".
func TestSPKIHashPinner_CanonicalizesBase64(t *testing.T) {
	sum := sha256.Sum256([]byte("canonicalization fixture"))
	canonical := base64.StdEncoding.EncodeToString(sum[:])

	// Build a non-canonical spelling: bump the final character by one code
	// point. Only its low 2 bits are meaningful, so decoding yields the same
	// 32 bytes (Go's decoder accepts it — which is exactly the trap).
	last := len(canonical) - 2 // before the '=' padding
	bumped := canonical[:last] + string(rune(canonical[last]+1)) + "="

	decodedCanon, errC := base64.StdEncoding.DecodeString(canonical)
	decodedBump, errB := base64.StdEncoding.DecodeString(bumped)
	if errC != nil || errB != nil || string(decodedCanon) != string(decodedBump) {
		t.Skipf("fixture not non-canonical on this input (canonErr=%v bumpErr=%v)", errC, errB)
	}

	p, err := newSPKIHashPinner(bumped)
	if err != nil {
		t.Fatalf("non-canonical pin rejected: %v", err)
	}
	if !p.hashes[canonical] {
		t.Error("pin not stored under its canonical re-encoding; it would never match")
	}

	// The canonical spelling keeps working.
	p2, err := newSPKIHashPinner(canonical)
	if err != nil {
		t.Fatalf("canonical pin rejected: %v", err)
	}
	if !p2.hashes[canonical] {
		t.Error("canonical pin not stored under its own form")
	}
}
