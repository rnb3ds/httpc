package httpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// WithProxy — package-level routing option
// ----------------------------------------------------------------------------

// TestWithProxy_RoutesThroughProxy is the end-to-end check: a package-level
// Get carrying WithProxy must route through the named forward proxy, leaving
// the shared default client (which has no proxy) untouched.
func TestWithProxy_RoutesThroughProxy(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
	}))
	defer target.Close()

	proxy := forwardProxy("with-proxy-A")
	defer proxy.Close()

	// WithAllowPrivateIPs(true) lets the localhost target through SSRF; it is a
	// plain per-request option and must still pass through the routed client.
	result, err := Get(target.URL, WithProxy(proxy.URL), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get(WithProxy) failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "with-proxy-A" {
		t.Errorf("body = %q, want %q (request did not route through the proxy)", got, "with-proxy-A")
	}
	if result.Meta == nil || result.Meta.ProxyURL != proxy.URL {
		t.Errorf("Meta.ProxyURL = %v, want %q", result.Meta, proxy.URL)
	}
}

// TestWithProxy_RoutesDirectWithoutProxy confirms the default path is unchanged:
// a plain Get (no WithProxy) still uses the default client with no proxy.
func TestWithProxy_RoutesDirectWithoutProxy(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("direct")) // best-effort test response
	}))
	defer target.Close()

	result, err := Get(target.URL, WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "direct" {
		t.Errorf("body = %q, want %q", got, "direct")
	}
	if result.Meta != nil && result.Meta.ProxyURL != "" {
		t.Errorf("Meta.ProxyURL = %q, want empty for a direct request", result.Meta.ProxyURL)
	}
}

// TestWithProxy_InvalidURL verifies the proxy URL is validated up front and the
// error wraps ErrInvalidConnection, mirroring ValidateConfig's treatment of
// Config.Connection.ProxyURL.
func TestWithProxy_InvalidURL(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	_, err := Get("https://example.com", WithProxy("://not-a-url"))
	if err == nil {
		t.Fatal("expected an error for an invalid WithProxy URL")
	}
	if !errors.Is(err, ErrInvalidConnection) {
		t.Errorf("error = %v, want it to wrap ErrInvalidConnection", err)
	}
}

// TestWithProxy_OnClientInstanceErrors pins that WithProxy has no per-request
// meaning on an explicit client: it must fail loudly rather than be silently
// ignored (its apply returns an error, see proxyOption.apply).
func TestWithProxy_OnClientInstanceErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = true
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Get("https://example.com", WithProxy("http://127.0.0.1:7890"))
	if err == nil {
		t.Fatal("expected WithProxy on a client instance to return an error")
	}
}

// TestWithProxy_ClientCache verifies the routed client is cached per routing
// key: the same proxy URL reuses one client, a different URL yields a distinct
// client.
func TestWithProxy_ClientCache(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	a1, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-a:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(a) failed: %v", err)
	}
	a2, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-a:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(a) again failed: %v", err)
	}
	if a1.(*clientImpl) != a2.(*clientImpl) {
		t.Error("expected the same routing key to reuse one cached client")
	}

	b1, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-b:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient(b) failed: %v", err)
	}
	if a1.(*clientImpl) == b1.(*clientImpl) {
		t.Error("expected distinct routing keys to use distinct clients")
	}
}

// TestWithProxy_SelfHealAfterClose pins that a directly-closed routed client is
// self-healed on the next routing call, matching getDefaultClient's behavior.
func TestWithProxy_SelfHealAfterClose(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	c1, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-selfheal:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient failed: %v", err)
	}
	if err := c1.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	c2, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-selfheal:8080"})
	if err != nil {
		t.Fatalf("self-heal getOrCreateRoutedClient failed: %v", err)
	}
	if c2 == c1 {
		t.Error("expected a freshly initialized client after close, got the closed one")
	}
}

// TestCloseRoutedClients verifies the cache is cleared and clients are closed.
func TestCloseRoutedClients(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	c1, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-close:8080"})
	if err != nil {
		t.Fatalf("getOrCreateRoutedClient failed: %v", err)
	}
	if err := CloseRoutedClients(); err != nil {
		t.Fatalf("CloseRoutedClients failed: %v", err)
	}
	if !c1.(*clientImpl).engine.IsClosed() {
		t.Error("expected the routed client to be closed by CloseRoutedClients")
	}

	// A subsequent call must recreate the client.
	c2, err := getOrCreateRoutedClient(routingKey{proxy: "http://proxy-close:8080"})
	if err != nil {
		t.Fatalf("recreate after CloseRoutedClients failed: %v", err)
	}
	if c2 == c1 {
		t.Error("expected a fresh client after CloseRoutedClients")
	}
}

// ----------------------------------------------------------------------------
// Option extraction / stripping helpers
// ----------------------------------------------------------------------------

func TestExtractProxyOption(t *testing.T) {
	po := WithProxy("http://proxy:8080").(*proxyOption)

	if got := extractProxyOption(nil); got != nil {
		t.Errorf("extractProxyOption(nil) = %v, want nil", got)
	}
	if got := extractProxyOption([]RequestOption{WithHeader("A", "B")}); got != nil {
		t.Errorf("extractProxyOption without WithProxy = %v, want nil", got)
	}
	if got := extractProxyOption([]RequestOption{WithHeader("A", "B"), po}); got != po {
		t.Errorf("extractProxyOption = %v, want the proxyOption", got)
	}
}

func TestStripRoutingOptions(t *testing.T) {
	po := WithProxy("http://proxy:8080").(*proxyOption)
	fo := WithTLSFingerprint("chrome").(*fingerprintOption)
	h := WithHeader("A", "B")

	if got := stripRoutingOptions(nil); got != nil {
		t.Errorf("stripRoutingOptions(nil) = %v, want nil", got)
	}
	if got := stripRoutingOptions([]RequestOption{h}); len(got) != 1 || got[0] != h {
		t.Errorf("stripRoutingOptions without routing options = %v, want the original option", got)
	}
	got := stripRoutingOptions([]RequestOption{h, po, fo})
	if len(got) != 1 || got[0] != h {
		t.Errorf("stripRoutingOptions = %v, want [headerOption]", got)
	}
}

// TestWithProxy_RequestMethodRoutes verifies the context-taking package-level
// Request honors WithProxy the same way as the verbs.
func TestWithProxy_RequestMethodRoutes(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup
	defer func() { _ = CloseDefaultClient() }() // best-effort cleanup

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("X-Proxy-ID"))) // best-effort test response
	}))
	defer target.Close()

	proxy := forwardProxy("with-proxy-request")
	defer proxy.Close()

	result, err := Request(context.Background(), "GET", target.URL, WithProxy(proxy.URL), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Request(WithProxy) failed: %v", err)
	}
	if got := strings.TrimSpace(result.Body()); got != "with-proxy-request" {
		t.Errorf("body = %q, want %q", got, "with-proxy-request")
	}
}

func TestRoutingKeyStringRedactsProxyCredentials(t *testing.T) {
	// CloseRoutedClients interpolates the key into a returned error, which
	// apps typically log; the raw proxy URL must never reach that string.
	k := routingKey{
		proxy:       "http://user:supersecret@proxy.example.com:8080",
		systemProxy: true,
		fingerprint: "chrome",
	}
	s := k.String()
	if strings.Contains(s, "supersecret") {
		t.Errorf("routingKey.String() leaks the proxy password: %s", s)
	}
	if !strings.Contains(s, "proxy.example.com:8080") {
		t.Errorf("routingKey.String() should keep the dial address for debugging: %s", s)
	}
	if !strings.Contains(s, "chrome") {
		t.Errorf("routingKey.String() should keep the fingerprint: %s", s)
	}

	// An unparseable URL must render as a placeholder, never echo raw input.
	bad := routingKey{proxy: "http://u:p@%zz[invalid"}
	if strings.Contains(bad.String(), "u:p@") {
		t.Errorf("routingKey.String() leaked credentials from an unparseable URL: %s", bad.String())
	}

	// Zero key stays recognizable.
	if z := (routingKey{}).String(); !strings.Contains(z, "systemProxy:false") {
		t.Errorf("zero key rendering unexpected: %s", z)
	}
}
