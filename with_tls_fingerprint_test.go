package httpc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// WithTLSFingerprint — package-level routing option
// ----------------------------------------------------------------------------

// fingerprintHandler responds with the browser preset's User-Agent so the test
// can assert the fingerprinted client actually impersonated the browser (the
// fingerprint path replaces the httpc/1.0 default UA with the preset's).
func fingerprintHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.UserAgent())) // best-effort test response
	}
}

// TestWithTLSFingerprint_Chrome verifies a package-level Get carrying
// WithTLSFingerprint routes through a fingerprinted client: the User-Agent seen
// by the server is the Chrome preset's, not the httpc/1.0 default.
func TestWithTLSFingerprint_Chrome(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	srv := httptest.NewServer(fingerprintHandler())
	defer srv.Close()

	result, err := Get(srv.URL, WithTLSFingerprint("chrome"), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get(WithTLSFingerprint) failed: %v", err)
	}
	ua := result.Body()
	if strings.Contains(ua, "httpc/1.0") {
		t.Errorf("User-Agent = %q, want the Chrome preset UA (not the httpc/1.0 default)", ua)
	}
	if !strings.Contains(ua, "Chrome/") {
		t.Errorf("User-Agent = %q, want a Chrome/ token from the preset", ua)
	}
}

// TestWithTLSFingerprint_InvalidPreset verifies an unknown preset fails with
// ErrInvalidSecurity, mirroring ValidateConfig's treatment of
// Config.Security.TLSFingerprint.
func TestWithTLSFingerprint_InvalidPreset(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	_, err := Get("https://example.com", WithTLSFingerprint("not-a-browser"))
	if err == nil {
		t.Fatal("expected an error for an unknown WithTLSFingerprint preset")
	}
	if !errors.Is(err, ErrInvalidSecurity) {
		t.Errorf("error = %v, want it to wrap ErrInvalidSecurity", err)
	}
}

// TestWithTLSFingerprint_OnClientInstanceErrors pins that
// WithTLSFingerprint has no per-request meaning on an explicit client: it must
// fail loudly rather than be silently ignored (see fingerprintOption.apply).
func TestWithTLSFingerprint_OnClientInstanceErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = true
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Get("https://example.com", WithTLSFingerprint("chrome"))
	if err == nil {
		t.Fatal("expected WithTLSFingerprint on a client instance to return an error")
	}
}

// TestWithTLSFingerprint_AliasCollapses verifies alias spellings resolve to one
// cached client: "Chrome" (with case/space) and "chrome" must share an entry
// because the routing key is the canonical resolved preset name.
func TestWithTLSFingerprint_AliasCollapses(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	c1, err := getOrCreateRoutedClient(routingKey{fingerprint: "chrome"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(chrome) failed: %v", err)
	}
	c2, err := getOrCreateRoutedClient(routingKey{fingerprint: "  Chrome  "})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(Chrome) failed: %v", err)
	}
	if c1.(*clientImpl) != c2.(*clientImpl) {
		t.Error("expected alias spellings to collapse onto one cached client")
	}
}

// TestWithTLSFingerprint_CombinedWithProxy verifies the two routing options
// compose into one distinct key: fingerprint + proxy together produce a client
// distinct from fingerprint-only and proxy-only.
func TestWithTLSFingerprint_CombinedWithProxy(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	both, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy:8080", fingerprint: "chrome"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(proxy+fingerprint) failed: %v", err)
	}
	fpOnly, err := getOrCreateRoutedClient(routingKey{fingerprint: "chrome"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(fingerprint) failed: %v", err)
	}
	proxyOnly, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(proxy) failed: %v", err)
	}
	if both.(*clientImpl) == fpOnly.(*clientImpl) || both.(*clientImpl) == proxyOnly.(*clientImpl) {
		t.Error("expected the combined key to yield a distinct client")
	}
}

// TestExtractFingerprintOption pins the static extraction of the routing option.
func TestExtractFingerprintOption(t *testing.T) {
	fo := WithTLSFingerprint("firefox").(*fingerprintOption)

	if got := extractFingerprintOption(nil); got != nil {
		t.Errorf("extractFingerprintOption(nil) = %v, want nil", got)
	}
	if got := extractFingerprintOption([]RequestOption{WithHeader("A", "B")}); got != nil {
		t.Errorf("extractFingerprintOption without WithTLSFingerprint = %v, want nil", got)
	}
	if got := extractFingerprintOption([]RequestOption{WithHeader("A", "B"), fo}); got != fo {
		t.Errorf("extractFingerprintOption = %v, want the fingerprintOption", got)
	}
}
