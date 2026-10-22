package proxy

import (
	"net/http"
	"strings"
	"testing"
)

// Windows-only boundary tests (parseProxyOverride, windowsProxyBypass,
// parseWindowsProxyRoutes) live in boundary_windows_test.go behind the
// windows build tag: the symbols they exercise are defined in
// proxy_windows.go only, and an untagged test file broke `go test
// ./internal/proxy/` on every other platform.

// TestParseProxyURL_RejectsUnknownScheme pins the scheme allow-list arm: an
// explicitly schemed URL outside http/https/socks5/socks5h errors instead of
// silently dialing somewhere unexpected, and the error redacts any embedded
// credentials (the parse-error path quotes the sanitized form only).
func TestParseProxyURL_RejectsUnknownScheme(t *testing.T) {
	for _, raw := range []string{"httpx://proxy.example:8080", "socks4://proxy.example:1080", "ftp://proxy.example:21"} {
		u, err := parseProxyURL(raw)
		if err == nil {
			t.Errorf("parseProxyURL(%q) = %v, want scheme-rejection error", raw, u)
			continue
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("error for %q leaks credential material: %v", raw, err)
		}
	}
}

// TestShouldUseProxy_Boundaries pins the remaining NO_PROXY matcher arms not
// covered by TestShouldUseProxy: CIDR entries for IP hosts, host:port entries
// with matching and mismatching ports, and entries that are only whitespace.
func TestShouldUseProxy_Boundaries(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		port    string
		noProxy string
		want    bool // true = SHOULD use proxy (no bypass)
	}{
		{name: "CIDR match bypasses", host: "10.1.2.3", port: "80", noProxy: "10.0.0.0/8", want: false},
		{name: "CIDR non-match still proxies", host: "192.168.1.1", port: "80", noProxy: "10.0.0.0/8", want: true},
		{name: "host:port exact bypasses", host: "example.com", port: "8080", noProxy: "example.com:8080", want: false},
		{name: "host:port mismatch proxies", host: "example.com", port: "9090", noProxy: "example.com:8080", want: true},
		{name: "host without port entry bypasses any port", host: "example.com", port: "9090", noProxy: "example.com", want: false},
		{name: "IP literal entry with port", host: "10.0.0.5", port: "3128", noProxy: "10.0.0.5:3128", want: false},
		{name: "whitespace-only entries ignored", host: "example.com", port: "80", noProxy: " , , ", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseProxy(tt.host, tt.port, tt.noProxy)
			if got != tt.want {
				t.Errorf("shouldUseProxy(%q, %q, %q) = %v, want %v", tt.host, tt.port, tt.noProxy, got, tt.want)
			}
		})
	}
}

// TestDetectFromEnvironment_InvalidSchemeErrors pins the fail-loud arm of the
// environment detector: a SET but INVALID proxy variable surfaces its parse
// error for the schemes it governs rather than silently direct-connecting.
func TestDetectFromEnvironment_InvalidSchemeErrors(t *testing.T) {
	t.Setenv("HTTP_PROXY", "not a valid proxy url zzz://x")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")

	fn := NewDetector().GetProxyFunc()
	if fn == nil {
		t.Fatal("detector returned nil proxy func with HTTP_PROXY set (invalid values must not disable detection)")
	}

	req, _ := http.NewRequest(http.MethodGet, "http://origin.example/", nil)
	if _, err := fn(req); err == nil {
		t.Error("invalid HTTP_PROXY should surface its parse error for http requests")
	}
}
