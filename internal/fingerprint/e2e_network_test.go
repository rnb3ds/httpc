//go:build fingerprint_e2e

// Network end-to-end verification against tls.peet.ws, which echoes the
// client's TLS (ja3/ja4) and HTTP/2 (akamai) fingerprints. Run with:
//
//	go test -tags fingerprint_e2e ./internal/fingerprint/ -run PeetWSE2E -v
//
// These tests require internet access and are excluded from the default build.
//
// The JA4 assertions are EXACT, not prefix-based. A prefix check ("t13d")
// passes for any TLS 1.3 + h2 client, so it could not notice a lost ML-DSA
// signature scheme, a dropped trust_anchors extension or a changed cipher
// list — the whole point of these gates is to catch exactly that drift.
//
// When a profile is re-cut for a new browser build, re-baseline BOTH strings
// (JA4 and the Akamai hash) and keep the update in the same commit as the
// profile data. See internal/fingerprint/PROFILES.md.
package fingerprint_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cybergodev/httpc"
)

// Baseline fingerprints for the shipped profiles, captured 2026-10-21.
const (
	chromeJA4         = "t13d1517h2_8daaf6152771_cb7bf5808d99"
	chromeAkamaiHash  = "52d84b11737d980aef856699f885ca86"
	chromeAkamaiPrint = "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p"

	firefoxJA4         = "t13d1715h2_5b57614c22b0_a54fffd0eb61"
	firefoxAkamaiHash  = "6ea73faa8fc5aac76bded7bd238f6433"
	firefoxAkamaiPrint = "1:65536;2:0;4:131072;5:16384|12517377|0|m,p,a,s"

	// iOS (WebKit) baseline: JA4 derived offline (ja4_test.go) and matching
	// the dominant live iOS observation in public JA4 databases; the Akamai
	// chain is the Safari iOS 26 capture cross-checked against tls-client's
	// Safari_IOS_26_0 profile (BSD-3).
	safariIOSJA4         = "t13d2013h2_a09f3c656075_7f0f34a4126d"
	safariIOSAkamaiHash  = "c52879e43202aeb92740be6e8c86ea96"
	safariIOSAkamaiPrint = "2:0;3:100;4:2097152;9:1|10420225|0|m,s,a,p"
)

// TestPeetWSE2ESafariIOS pins the third family: iOS Safari rides the same
// verified WebKit transport data as chrome-ios with Safari's own header
// values (no Client Hints) and frozen-OS UA.
func TestPeetWSE2ESafariIOS(t *testing.T) {
	out := peetWS(t, "safari-ios")

	if out.TLS.JA4 != safariIOSJA4 {
		t.Errorf("JA4 = %q, want %q", out.TLS.JA4, safariIOSJA4)
	}
	if out.HTTP2.AkamaiFingerprintHash != safariIOSAkamaiHash {
		t.Errorf("akamai hash = %q, want %q (raw=%q)",
			out.HTTP2.AkamaiFingerprintHash, safariIOSAkamaiHash, out.HTTP2.AkamaiFingerprint)
	}
	if !contains(out.UserAgent, "Version/26.0") {
		t.Errorf("User-Agent = %q, want Safari's Version/26.0 token", out.UserAgent)
	}

	t.Logf("safari-ios: ja4=%s akamai=%s hash=%s ua=%s",
		out.TLS.JA4, out.HTTP2.AkamaiFingerprint, out.HTTP2.AkamaiFingerprintHash, out.UserAgent)
}

// peetWSResponse is the subset of tls.peet.ws/api/all the assertions need.
type peetWSResponse struct {
	UserAgent string `json:"user_agent"`
	TLS       struct {
		JA3 string `json:"ja3"`
		JA4 string `json:"ja4"`
	} `json:"tls"`
	HTTPVersion string `json:"http_version"`
	HTTP2       struct {
		AkamaiFingerprint     string `json:"akamai_fingerprint"`
		AkamaiFingerprintHash string `json:"akamai_fingerprint_hash"`
	} `json:"http2"`
}

func peetWS(t *testing.T, preset string) peetWSResponse {
	t.Helper()
	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = preset
	cfg.Timeouts.Request = 60 * time.Second
	cfg.Retry.MaxRetries = 1
	client, err := httpc.New(cfg)
	if err != nil {
		t.Fatalf("httpc.New: %v", err)
	}
	defer func() { _ = client.Close() }()

	res, err := client.Get("https://tls.peet.ws/api/all")
	if err != nil {
		t.Fatalf("GET tls.peet.ws failed (network required): %v", err)
	}
	var out peetWSResponse
	if err := json.Unmarshal([]byte(res.Body()), &out); err != nil {
		t.Fatalf("parse peet.ws response: %v", err)
	}
	return out
}

// TestPeetWSE2EChrome pins the Chrome fingerprint on every layer an echo
// endpoint can observe: the exact JA4 (TLS range, cipher list and extension
// set), the exact Akamai H2 string and its hash, and the UA the preset claims.
// The shipped label is Chrome 155; the JA4/Akamai constants date from the
// Chrome 152 capture and are unchanged because Chrome 152–156 share the same
// ClientHello and H2 chain (see PROFILES.md).
func TestPeetWSE2EChrome(t *testing.T) {
	out := peetWS(t, "chrome-latest")

	if out.HTTPVersion != "h2" {
		t.Errorf("HTTPVersion = %q, want h2 (the fingerprint path never attempts h3)", out.HTTPVersion)
	}

	if out.TLS.JA4 != chromeJA4 {
		t.Errorf("JA4 = %q, want %q\n"+
			"  a mismatch means the ClientHello changed: ciphers, extension set,\n"+
			"  signature algorithms or ALPN. Check profile_chrome.go against a real\n"+
			"  Chrome capture before re-baselining this constant.", out.TLS.JA4, chromeJA4)
	}
	if out.HTTP2.AkamaiFingerprintHash != chromeAkamaiHash {
		t.Errorf("akamai hash = %q, want %q (raw=%q, want %q)",
			out.HTTP2.AkamaiFingerprintHash, chromeAkamaiHash,
			out.HTTP2.AkamaiFingerprint, chromeAkamaiPrint)
	}
	if out.TLS.JA3 == "" {
		t.Error("JA3 missing from peet.ws response")
	}

	t.Logf("chrome: ja4=%s akamai=%s hash=%s",
		out.TLS.JA4, out.HTTP2.AkamaiFingerprint, out.HTTP2.AkamaiFingerprintHash)
}

// TestPeetWSE2EUserAgentCoherence verifies the preset's browser UA is sent
// (the httpc default UA is dropped on the fingerprint path) and that it names
// the version the ClientHello claims.
func TestPeetWSE2EUserAgentCoherence(t *testing.T) {
	out := peetWS(t, "chrome-latest")
	if out.UserAgent == "" || out.UserAgent == "httpc/1.0" {
		t.Errorf("User-Agent = %q, want the preset's Chrome UA", out.UserAgent)
	}
	if !contains(out.UserAgent, "Chrome/155.") {
		t.Errorf("User-Agent = %q, want a Chrome/155 version string to match the spoofed ClientHello", out.UserAgent)
	}
	t.Logf("preset User-Agent: %s", out.UserAgent)
}

// TestPeetWSE2EFirefox pins the second browser family with the same exact
// gates (Firefox: distinct cipher list, SETTINGS chain and pseudo-header
// order). The shipped label is 158 riding the HelloFirefox_148 parrot; the
// JA4 gate is what proves that parrot still matches reality.
func TestPeetWSE2EFirefox(t *testing.T) {
	out := peetWS(t, "firefox-latest")

	if out.HTTPVersion != "h2" {
		t.Errorf("HTTPVersion = %q, want h2", out.HTTPVersion)
	}
	if out.TLS.JA4 != firefoxJA4 {
		t.Errorf("JA4 = %q, want %q", out.TLS.JA4, firefoxJA4)
	}
	if out.HTTP2.AkamaiFingerprintHash != firefoxAkamaiHash {
		t.Errorf("akamai hash = %q, want %q (raw=%q, want %q)",
			out.HTTP2.AkamaiFingerprintHash, firefoxAkamaiHash,
			out.HTTP2.AkamaiFingerprint, firefoxAkamaiPrint)
	}
	if !contains(out.UserAgent, "Firefox/158.") {
		t.Errorf("User-Agent = %q, want a Firefox/158 version string", out.UserAgent)
	}

	t.Logf("firefox: ja4=%s akamai=%s hash=%s",
		out.TLS.JA4, out.HTTP2.AkamaiFingerprint, out.HTTP2.AkamaiFingerprintHash)
}

// TestPeetWSE2EChromeIOS pins the iOS preset on the TLS layer: Chrome on iOS
// is WebKit underneath, so the hello is Safari 26.3's (offline JA4 pinned in
// ja4_test.go, live-matched in public JA4 databases) and the UA is CriOS.
func TestPeetWSE2EChromeIOS(t *testing.T) {
	out := peetWS(t, "chrome-ios")

	if out.TLS.JA4 != safariIOSJA4 {
		t.Errorf("JA4 = %q, want %q (the WebKit hello)", out.TLS.JA4, safariIOSJA4)
	}
	if out.HTTP2.AkamaiFingerprintHash != safariIOSAkamaiHash {
		t.Errorf("akamai hash = %q, want %q (raw=%q, want %q)",
			out.HTTP2.AkamaiFingerprintHash, safariIOSAkamaiHash,
			out.HTTP2.AkamaiFingerprint, safariIOSAkamaiPrint)
	}
	if !contains(out.UserAgent, "CriOS/155.") {
		t.Errorf("User-Agent = %q, want a CriOS/155 string", out.UserAgent)
	}

	t.Logf("chrome-ios: ja4=%s akamai=%s hash=%s ua=%s",
		out.TLS.JA4, out.HTTP2.AkamaiFingerprint, out.HTTP2.AkamaiFingerprintHash, out.UserAgent)
}

// contains reports whether s contains sub.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
