//go:build windows

package proxy

import (
	"testing"
)

// TestParseProxyOverride_Boundaries pins the trimming arms of the registry
// override splitter: an empty (or whitespace-only) value yields nil, and
// blank segments between separators are dropped rather than emitted as
// empty-string entries.
func TestParseProxyOverride_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		override string
		want     []string
	}{
		{name: "empty", override: "", want: nil},
		{name: "whitespace only", override: "   ", want: nil},
		{name: "single entry", override: "intranet.example.com", want: []string{"intranet.example.com"}},
		{name: "blank segments dropped", override: ";;a.example;; ;b.example;", want: []string{"a.example", "b.example"}},
		{name: "entries trimmed", override: " <local> ; *.corp.example ", want: []string{"<local>", "*.corp.example"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProxyOverride(tt.override)
			if len(got) != len(tt.want) {
				t.Fatalf("parseProxyOverride(%q) = %v, want %v", tt.override, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestWindowsProxyBypass_Boundaries pins the wrapper's own guards (empty host,
// empty entry list with a non-dotless host) — the arms the semantic test in
// proxy_windows_routes_test.go does not reach.
func TestWindowsProxyBypass_Boundaries(t *testing.T) {
	if windowsProxyBypass("  ", []string{"<local>"}) {
		t.Error("empty/whitespace host must not bypass")
	}
	if windowsProxyBypass("", nil) {
		t.Error("empty host with no entries must not bypass")
	}
}

// TestParseWindowsProxyRoutes_Schemes pins the scheme-handling arms of the
// registry ProxyServer parser: the "socks://" normalization, the http(s)/
// socks5( h) passthrough, and the bare host:port fallback that applies to
// everything else (including portless bare hosts). Uppercase schemes are
// covered here too: parsing delegates to validation.ParseSystemProxyURL,
// where url.Parse lowercases schemes.
func TestParseWindowsProxyRoutes_Schemes(t *testing.T) {
	tests := []struct {
		name       string
		proxyStr   string
		wantScheme string
		wantHost   string
		wantErr    bool
	}{
		{name: "socks scheme normalized", proxyStr: "socks://sockshost:1080", wantScheme: "socks5", wantHost: "sockshost:1080"},
		{name: "uppercase scheme accepted", proxyStr: "HTTP://ph:8080", wantScheme: "http", wantHost: "ph:8080"},
		{name: "http passthrough", proxyStr: "http://ph:8080", wantScheme: "http", wantHost: "ph:8080"},
		{name: "https passthrough", proxyStr: "https://ph:8443", wantScheme: "https", wantHost: "ph:8443"},
		{name: "socks5h passthrough", proxyStr: "socks5h://ph:1080", wantScheme: "socks5h", wantHost: "ph:1080"},
		{name: "bare host:port gets http fallback", proxyStr: "ph:3128", wantScheme: "http", wantHost: "ph:3128"},
		{name: "portless bare host gets http fallback", proxyStr: "ph", wantScheme: "http", wantHost: "ph"},
		{name: "unknown scheme errors", proxyStr: "socks4://ph:1080", wantErr: true},
		{name: "empty string errors", proxyStr: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := parseWindowsProxyRoutes(tt.proxyStr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseWindowsProxyRoutes(%q) = %v, want error", tt.proxyStr, routes)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWindowsProxyRoutes(%q): %v", tt.proxyStr, err)
			}
			u := routes.httpsURL
			if u == nil || u.Scheme != tt.wantScheme || u.Host != tt.wantHost {
				t.Errorf("https route = %+v, want scheme %s host %s", u, tt.wantScheme, tt.wantHost)
			}
		})
	}
}
