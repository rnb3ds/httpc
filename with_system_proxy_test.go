package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// WithSystemProxy — package-level routing option
// ----------------------------------------------------------------------------

// setSystemProxyEnv installs the given proxy environment values for the test.
// t.Setenv serializes env mutation across tests, so tests that mutate
// HTTP(S)_PROXY/NO_PROXY do not race.
func setSystemProxyEnv(t *testing.T, httpProxy, httpsProxy, noProxy string) {
	t.Helper()
	t.Setenv("HTTP_PROXY", httpProxy)
	t.Setenv("HTTPS_PROXY", httpsProxy)
	t.Setenv("NO_PROXY", noProxy)
}

// fakeSystemProxy is a proxy that responds directly (without reaching the
// target). It exists because system-proxy detection bypasses loopback targets
// (matching net/http), so an end-to-end test needs a NON-loopback target that
// never actually has to be reachable — the fake proxy short-circuits.
func fakeSystemProxy(id string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxied-" + id)) // best-effort test response
	}))
}

// TestWithSystemProxy_RoutesThroughEnvProxy is the end-to-end check: a
// package-level Get carrying WithSystemProxy routes through a client with
// system-proxy detection enabled, which reads the HTTP_PROXY env var and
// forwards the request through the named proxy.
func TestWithSystemProxy_RoutesThroughEnvProxy(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	proxy := fakeSystemProxy("sys-a")
	defer proxy.Close()

	// Point the system proxy at our fake proxy. t.Setenv serializes env
	// mutation across tests.
	setSystemProxyEnv(t, proxy.URL, "", "")

	// example.com is non-loopback, so the detector does NOT bypass it; the fake
	// proxy short-circuits, so the target is never actually contacted.
	// WithAllowPrivateIPs(true) skips the proxied-target SSRF guard's DNS.
	result, err := Get("http://example.com/", WithSystemProxy(), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get(WithSystemProxy) failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "proxied-sys-a" {
		t.Errorf("body = %q, want %q (request did not route through the system proxy)", got, "proxied-sys-a")
	}
}

// TestWithSystemProxy_NoProxyConfigured verifies that WithSystemProxy degrades
// to a direct connection when no system proxy is configured — the request must
// still succeed without an error.
func TestWithSystemProxy_NoProxyConfigured(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("direct")) // best-effort test response
	}))
	defer target.Close()

	// Clear proxy env vars so detection finds nothing.
	setSystemProxyEnv(t, "", "", "")

	result, err := Get(target.URL, WithSystemProxy(), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get(WithSystemProxy, no proxy) failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "direct" {
		t.Errorf("body = %q, want %q (direct connection when no system proxy)", got, "direct")
	}
}

// TestWithSystemProxy_OnClientInstanceErrors pins that WithSystemProxy has no
// per-request meaning on an explicit client: it must fail loudly rather than be
// silently ignored (see systemProxyOption.apply).
func TestWithSystemProxy_OnClientInstanceErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = true
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Get("https://example.com", WithSystemProxy())
	if err == nil {
		t.Fatal("expected WithSystemProxy on a client instance to return an error")
	}
}

// TestWithSystemProxy_ExplicitProxyPrecedence pins that WithProxy takes
// precedence over WithSystemProxy: a call carrying both routes through the
// explicit proxy (and collapses onto the explicit-proxy cache key).
func TestWithSystemProxy_ExplicitProxyPrecedence(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
	}))
	defer target.Close()

	explicit := forwardProxy("explicit-proxy")
	defer explicit.Close()
	sysProxy := forwardProxy("sys-proxy")
	defer sysProxy.Close()

	// The system proxy points at sys-proxy, but WithProxy must win.
	setSystemProxyEnv(t, sysProxy.URL, "", "")

	result, err := Get(target.URL,
		WithProxy(explicit.URL),
		WithSystemProxy(),
		WithAllowPrivateIPs(true),
	)
	if err != nil {
		t.Fatalf("Get(WithProxy + WithSystemProxy) failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "explicit-proxy" {
		t.Errorf("body = %q, want %q (WithProxy must take precedence over WithSystemProxy)", got, "explicit-proxy")
	}
}

// TestWithSystemProxy_CacheIsolation verifies the routed-client cache keeps
// system-proxy clients distinct from explicit-proxy and default clients.
func TestWithSystemProxy_CacheIsolation(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	sys, err := getOrCreateRoutedClient(routingKey{systemProxy: true})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(systemProxy) failed: %v", err)
	}
	explicit, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(proxy) failed: %v", err)
	}
	if sys.(*clientImpl) == explicit.(*clientImpl) {
		t.Error("expected system-proxy and explicit-proxy clients to be distinct")
	}

	// The system-proxy flag combined with a fingerprint is a distinct key too.
	fp, err := getOrCreateRoutedClient(routingKey{systemProxy: true, fingerprint: "chrome"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(systemProxy+fingerprint) failed: %v", err)
	}
	if sys.(*clientImpl) == fp.(*clientImpl) {
		t.Error("expected system-proxy and system-proxy+fingerprint clients to be distinct")
	}
}

// TestExtractSystemProxyOption pins the static extraction of the marker option.
func TestExtractSystemProxyOption(t *testing.T) {
	if got := extractSystemProxyOption(nil); got {
		t.Error("extractSystemProxyOption(nil) = true, want false")
	}
	if got := extractSystemProxyOption([]RequestOption{WithHeader("A", "B")}); got {
		t.Error("extractSystemProxyOption without WithSystemProxy = true, want false")
	}
	if got := extractSystemProxyOption([]RequestOption{WithHeader("A", "B"), WithSystemProxy()}); !got {
		t.Error("extractSystemProxyOption with WithSystemProxy = false, want true")
	}
}

// TestWithSystemProxy_RequestMethodRoutes verifies the context-taking
// package-level Request honors WithSystemProxy the same way as the verbs.
func TestWithSystemProxy_RequestMethodRoutes(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	proxy := fakeSystemProxy("sys-request")
	defer proxy.Close()
	setSystemProxyEnv(t, proxy.URL, "", "")

	result, err := Request(context.Background(), "GET", "http://example.com/", WithSystemProxy(), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Request(WithSystemProxy) failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "proxied-sys-request" {
		t.Errorf("body = %q, want %q", got, "proxied-sys-request")
	}
}

// TestStripRoutingOptions_SystemProxy pins that the marker option is stripped
// alongside the other routing options.
func TestStripRoutingOptions_SystemProxy(t *testing.T) {
	h := WithHeader("A", "B")
	got := stripRoutingOptions([]RequestOption{h, WithSystemProxy(), WithProxy("http://p:1"), WithTLSFingerprint("chrome")})
	if len(got) != 1 || got[0] != h {
		t.Errorf("stripRoutingOptions = %v, want [headerOption]", got)
	}
}
