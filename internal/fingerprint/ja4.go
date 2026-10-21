// Offline JA4 derivation from a ClientHelloSpec: marshal the spec through
// uTLS exactly as one handshake would write it, then read the fingerprint
// parts off the resulting wire bytes. This is the same computation a
// TLS-observing endpoint (tls.peet.ws, ja4db) performs, so a JA4 pinned here
// transfers one-to-one to the network gates in e2e_network_test.go and can
// be verified without touching the network.
//
// The result is invariant under specForDial's per-connection mutations by
// construction: JA4 sorts the cipher and extension lists and strips GREASE
// values, so neither the Chromium extension shuffle nor the GREASE
// signature-algorithm substitution moves it.
//
// JA4 format (FoxIO): t13d1517h2_<cipher-hash>_<ext+sigalgs-hash>, where the
// readable head carries transport, TLS version, SNI presence, cipher and
// extension counts and the first ALPN protocol, and each hash is the first
// 12 hex characters of SHA-256 over the sorted, comma-joined value lists.
//
// Reference: https://github.com/FoxIO-LLC/ja4 (technical_details.md).

package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	utls "github.com/bogdanfinn/utls"
)

// ja4OfSpec returns the JA4 fingerprint of one ClientHelloSpec.
func ja4OfSpec(spec utls.ClientHelloSpec) (string, error) {
	conn := utls.UClient(nil, &utls.Config{ServerName: "ja4.httpc.test", OmitEmptyPsk: true}, utls.HelloCustom, false, false, true)
	if err := conn.ApplyPreset(&spec); err != nil {
		return "", fmt.Errorf("fingerprint: apply spec preset: %w", err)
	}
	if err := conn.BuildHandshakeStateWithoutSession(); err != nil {
		return "", fmt.Errorf("fingerprint: build handshake state: %w", err)
	}
	if err := conn.MarshalClientHello(); err != nil {
		return "", fmt.Errorf("fingerprint: marshal ClientHello: %w", err)
	}

	hello := conn.HandshakeState.Hello
	extIDs, err := helloExtensionIDs(hello.Raw)
	if err != nil {
		return "", err
	}
	return ja4FromWire(hello, extIDs), nil
}

// ja4FromWire assembles the JA4 string from the parsed hello and its wire
// extension IDs, following the FoxIO technical spec: cipher and extension
// values are GREASE-stripped; SNI and ALPN count toward the extension count
// but are omitted from the extension hash; signature algorithms keep their
// wire order. The pre_shared_key extension (41) is excluded from the count as
// well: real Chrome's reconnect JA4 keeps the same extension count as its
// initial one, so observed behavior treats the resumption-only extension as
// invisible.
func ja4FromWire(hello *utls.PubClientHelloMsg, extIDs []uint16) string {
	ciphers := dropGREASE16(hello.CipherSuites)
	slices.Sort(ciphers)

	counted := make([]uint16, 0, len(extIDs))
	hashed := make([]uint16, 0, len(extIDs))
	for _, id := range extIDs {
		if isGREASE16(id) || id == extensionPreSharedKey {
			continue
		}
		counted = append(counted, id)
		if id != extensionServerName && id != extensionALPN {
			hashed = append(hashed, id)
		}
	}
	slices.Sort(hashed)

	sigalgs := dropGREASE16u(hello.SupportedSignatureAlgorithms)

	head := fmt.Sprintf("t%02d%c%02d%02d%s",
		tlsVersionDigits(hello.SupportedVersions),
		sniMark(hello.ServerName),
		len(ciphers), len(counted), alpnMark(hello.AlpnProtocols))
	return head + "_" + hash12Lists(ciphers) + "_" + hash12Lists(hashed, sigalgs)
}

const (
	extensionServerName uint16 = 0  // SNI: counted, never hashed (JA4 spec)
	extensionALPN       uint16 = 16 // ALPN: counted, never hashed (JA4 spec)
	// extensionPreSharedKey is pre_shared_key; see ja4FromWire for why it is
	// excluded from the count and the hash.
	extensionPreSharedKey uint16 = 41
)

func tlsVersionDigits(versions []uint16) int {
	max := uint16(0x0303)
	for _, v := range versions {
		if isGREASE16(v) {
			continue
		}
		if v > max && v <= 0x0304 {
			max = v
		}
	}
	switch max {
	case 0x0304:
		return 13
	default: // 0x0303 and below
		return 12
	}
}

func sniMark(serverName string) rune {
	if serverName == "" {
		return 'i'
	}
	return 'd'
}

// alpnMark returns the first and last ASCII alphanumeric characters of the
// first ALPN value ("h2" → "h2", "http/1.1" → "h1"), "00" when absent.
func alpnMark(protos []string) string {
	if len(protos) == 0 || protos[0] == "" {
		return "00"
	}
	first, last := protos[0][0], protos[0][len(protos[0])-1]
	alnum := func(b byte) bool {
		return b >= 0x30 && b <= 0x39 || b >= 0x41 && b <= 0x5A || b >= 0x61 && b <= 0x7A
	}
	if !alnum(first) || !alnum(last) {
		return fmt.Sprintf("%x%x", protos[0][:1], protos[0][len(protos[0])-1:])
	}
	return string(first) + string(last)
}

func isGREASE16(v uint16) bool {
	return v&0x0f0f == 0x0a0a && byte(v>>8) == byte(v)
}

func dropGREASE16(values []uint16) []uint16 {
	out := make([]uint16, 0, len(values))
	for _, v := range values {
		if !isGREASE16(v) {
			out = append(out, v)
		}
	}
	return out
}

func dropGREASE16u(values []utls.SignatureScheme) []uint16 {
	out := make([]uint16, 0, len(values))
	for _, v := range values {
		if !isGREASE16(uint16(v)) {
			out = append(out, uint16(v))
		}
	}
	return out
}

// hash12Lists hashes one or more value lists: values inside a list are
// comma-joined after formatting, lists are underscore-joined — the exact
// composition JA4 uses for its cipher (one list) and extension+sigalgs (two
// lists) components.
func hash12Lists(lists ...[]uint16) string {
	var b strings.Builder
	for li, list := range lists {
		if li > 0 {
			b.WriteByte('_')
		}
		for i, v := range list {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "%04x", v)
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:12]
}

// helloExtensionIDs walks a marshaled ClientHello handshake message and
// returns its extension type IDs in wire order.
func helloExtensionIDs(raw []byte) ([]uint16, error) {
	// handshake header: type(1) + length(3); body: version(2) + random(32) +
	// session_id(1+n) + cipher_suites(2+n) + compression(1+n) + extensions
	// (2+n).
	if len(raw) < 4+2+32+1 {
		return nil, fmt.Errorf("fingerprint: ClientHello too short: %d bytes", len(raw))
	}
	if raw[0] != 0x01 {
		return nil, fmt.Errorf("fingerprint: not a ClientHello message: type 0x%02x", raw[0])
	}

	p := 4 + 2 + 32
	if p+1 > len(raw) {
		return nil, fmt.Errorf("fingerprint: ClientHello truncated at session id")
	}
	p += 1 + int(raw[p])

	if p+2 > len(raw) {
		return nil, fmt.Errorf("fingerprint: ClientHello truncated at cipher list")
	}
	p += 2 + int(raw[p])<<8 + int(raw[p+1])

	if p+1 > len(raw) {
		return nil, fmt.Errorf("fingerprint: ClientHello truncated at compression list")
	}
	p += 1 + int(raw[p])

	if p+2 > len(raw) {
		return nil, fmt.Errorf("fingerprint: ClientHello truncated at extensions length")
	}
	end := p + 2 + int(raw[p])<<8 + int(raw[p+1])
	p += 2
	if end > len(raw) {
		return nil, fmt.Errorf("fingerprint: ClientHello extensions overflow: need %d have %d", end, len(raw))
	}

	var ids []uint16
	for p+4 <= end {
		id := uint16(raw[p])<<8 | uint16(raw[p+1])
		length := int(raw[p+2])<<8 | int(raw[p+3])
		ids = append(ids, id)
		p += 4 + length
	}
	if p != end {
		return nil, fmt.Errorf("fingerprint: ClientHello extensions misaligned: at %d want %d", p, end)
	}
	return ids, nil
}
