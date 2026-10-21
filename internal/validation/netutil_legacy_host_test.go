package validation

import "testing"

// Legal public hostnames whose labels merely LOOK like legacy IP notation
// must not be blocked: inet_aton-family resolvers reinterpret a dotted name
// as an address only when every label is numeric (or 0x-hex). A single
// alphabetic label makes re-interpretation impossible, so flagging them
// rejects real sites (0x.org, 007.com) on the default validation path.
func TestLooksLikeLegacyIPLiteral_LegalHostnames(t *testing.T) {
	legal := []string{
		"0x.org",           // whole-string "0x" prefix, alphabetic TLD
		"007.com",          // leading-zero label, alphabetic TLD
		"0day.example.com", // leading-zero label mid-name
		"0800.example.com", // multi-digit leading-zero label
		"0.0.0.0.e.164.cn", // numeric labels + alphabetic labels (real TLD shape)
		"www.0x.org",       // hex-looking label inside a hostname
	}
	for _, host := range legal {
		if looksLikeLegacyIPLiteral(host) {
			t.Errorf("looksLikeLegacyIPLiteral(%q) = true, want false (legal hostname)", host)
		}
	}

	// True positives stay blocked.
	blocked := []string{
		"2130706433", // pure decimal integer
		"0x7f000001", // whole-string hex
		"0x7f.0.0.1", // per-octet hex
		"0177.0.0.1", // octal leading zero
		"10.1",       // short form
		"192.168.1",  // short form
		"1.2.3.256",  // out-of-range octet
		"1.2.3.4.5",  // all-numeric, wrong arity
		"0x.1.2.3",   // bare 0x octet
	}
	for _, host := range blocked {
		if !looksLikeLegacyIPLiteral(host) {
			t.Errorf("looksLikeLegacyIPLiteral(%q) = false, want true (legacy IP literal)", host)
		}
	}
}

// The hostname-level gate must accept the legal forms end to end.
func TestValidateSSRFHost_LegalNumericPrefixedHostnames(t *testing.T) {
	for _, host := range []string{"0x.org", "007.com", "0day.example.com"} {
		if err := ValidateSSRFHost(host, nil); err != nil {
			t.Errorf("ValidateSSRFHost(%q) = %v, want nil", host, err)
		}
	}
}
