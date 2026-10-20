//go:build fingerprint_e2e

// Network end-to-end verification against tls.peet.ws, which echoes the
// client's TLS (ja3/ja4) and HTTP/2 (akamai) fingerprints. Run with:
//
//	go test -tags fingerprint_e2e ./internal/fingerprint/ -run PeetWSE2E -v
//
// These tests require internet access and are excluded from the default build.
package fingerprint_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cybergodev/httpc"
)

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

// TestPeetWSE2EChrome asserts a Chrome preset yields a Chrome-grade
// fingerprint on every layer we can observe from the response: a Chrome
// User-Agent coherent with the ClientHello, a known Chrome JA4 prefix, and
// negotiated HTTP/2 (the fingerprint path deliberately stops at h2 — H3 is
// not attempted because the backend's H3 fallback would lose the TLS
// fingerprint).
func TestPeetWSE2EChrome(t *testing.T) {
	out := peetWS(t, "chrome-latest")

	if out.HTTPVersion == "h2" {
		t.Logf("negotiated h2, akamai=%s hash=%s", out.HTTP2.AkamaiFingerprint, out.HTTP2.AkamaiFingerprintHash)
		// Byte-exact regression gate for the in-house transport: the Chrome
		// 152 H2 fingerprint (SETTINGS sequence, pseudo-header and header
		// order, priorities) as captured from the surf-based implementation
		// this port replaced.
		if out.HTTP2.AkamaiFingerprintHash != "52d84b11737d980aef856699f885ca86" {
			t.Errorf("akamai hash = %q, want 52d84b11737d980aef856699f885ca86", out.HTTP2.AkamaiFingerprintHash)
		}
	} else {
		t.Errorf("HTTPVersion = %q, want h2 (fingerprint path is h2/h1 only)", out.HTTPVersion)
	}

	// JA4 for Chrome TLS 1.3 clients starts with "t13d" and contains h2 in
	// the ALPN slot; a Go stdlib JA4 would differ structurally.
	if len(out.TLS.JA4) < 4 || out.TLS.JA4[:4] != "t13d" {
		t.Errorf("JA4 = %q, want a t13d... Chrome-style TLS 1.3 fingerprint", out.TLS.JA4)
	}
	if out.TLS.JA3 == "" {
		t.Error("JA3 missing from peet.ws response")
	}
}

// TestPeetWSE2EUserAgentCoherence verifies the preset's browser UA is sent
// (the httpc default UA is dropped on the fingerprint path).
func TestPeetWSE2EUserAgentCoherence(t *testing.T) {
	out := peetWS(t, "chrome-latest")
	if out.UserAgent == "" || out.UserAgent == "httpc/1.0" {
		t.Errorf("User-Agent = %q, want the preset's Chrome UA", out.UserAgent)
	}
	t.Logf("preset User-Agent: %s", out.UserAgent)
}

// TestPeetWSE2EFirefox smoke-checks a second browser family, with the same
// byte-exact hash gate (Firefox 148: stream id 3, distinct SETTINGS chain
// and pseudo-header order).
func TestPeetWSE2EFirefox(t *testing.T) {
	out := peetWS(t, "firefox-latest")
	if out.HTTPVersion != "h2" {
		t.Errorf("HTTPVersion = %q, want h2 (fingerprint path is h2/h1 only)", out.HTTPVersion)
	}
	if out.HTTP2.AkamaiFingerprintHash != "6ea73faa8fc5aac76bded7bd238f6433" {
		t.Errorf("akamai hash = %q, want 6ea73faa8fc5aac76bded7bd238f6433", out.HTTP2.AkamaiFingerprintHash)
	}
	if len(out.TLS.JA4) < 4 || out.TLS.JA4[:4] != "t13d" {
		t.Errorf("JA4 = %q, want t13d...", out.TLS.JA4)
	}
}
