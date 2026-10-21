package security

import "testing"

// A Unicode whitelist entry must match the punycode (xn--) form servers emit
// in Location headers; before IDNA normalization the two spellings never
// matched and allowed redirects were denied with no hint why.
func TestDomainWhitelist_IDNAMatching(t *testing.T) {
	// münchen.de ⇄ xn--mnchen-3ya.de (a well-known punycode pair).
	w := NewDomainWhitelist("münchen.de")

	if !w.IsAllowed("xn--mnchen-3ya.de") {
		t.Error("punycode form of a Unicode entry must be allowed")
	}
	if !w.IsAllowed("münchen.de") {
		t.Error("Unicode form of its own entry must be allowed")
	}
	if w.IsAllowed("other.de") {
		t.Error("unrelated domain must stay denied")
	}

	// Wildcards normalize their label too: *.bücher.example matches
	// subdomains of xn--bcher-kva.example, but not the apex.
	ww := NewDomainWhitelist("*.bücher.example")
	if !ww.IsAllowed("shop.xn--bcher-kva.example") {
		t.Error("punycode subdomain of Unicode wildcard entry must be allowed")
	}
	if ww.IsAllowed("xn--bcher-kva.example") {
		t.Error("apex must not match a *.wildcard (unchanged semantics)")
	}

	// All-ASCII behavior is byte-identical to before.
	aw := NewDomainWhitelist("example.com", "*.trusted.org")
	if !aw.IsAllowed("example.com") || !aw.IsAllowed("a.trusted.org") {
		t.Error("ASCII entries must keep matching")
	}
	if aw.IsAllowed("trusted.org") || aw.IsAllowed("evil.com") {
		t.Error("ASCII denials must keep denying")
	}
}
