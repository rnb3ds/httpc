//go:build windows

package proxy

import "testing"

// Per-protocol registry values must route by scheme: both entries present →
// each scheme gets its own proxy; only one entry present → it serves both
// schemes (Windows' own fallback). The old https-always collapse sent
// plaintext http:// traffic to the https proxy port.
func TestParseWindowsProxyRoutes_PerProtocol(t *testing.T) {
	routes, err := parseWindowsProxyRoutes("http=proxy-http:8080;https=proxy-https:8443")
	if err != nil {
		t.Fatal(err)
	}
	if got := routes.httpURL.Host; got != "proxy-http:8080" {
		t.Errorf("http route = %q, want proxy-http:8080", got)
	}
	if got := routes.httpsURL.Host; got != "proxy-https:8443" {
		t.Errorf("https route = %q, want proxy-https:8443", got)
	}

	// Only an http entry: it serves both schemes.
	routes, err = parseWindowsProxyRoutes("http=proxy-http:8080")
	if err != nil {
		t.Fatal(err)
	}
	if routes.httpURL == nil || routes.httpsURL != routes.httpURL || routes.httpURL.Host != "proxy-http:8080" {
		t.Errorf("http-only config: http=%v https=%v, want both proxy-http:8080", routes.httpURL, routes.httpsURL)
	}

	// Only an https entry: it serves both schemes.
	routes, err = parseWindowsProxyRoutes("https=proxy-https:8443")
	if err != nil {
		t.Fatal(err)
	}
	if routes.httpsURL == nil || routes.httpURL != routes.httpsURL || routes.httpsURL.Host != "proxy-https:8443" {
		t.Errorf("https-only config: http=%v https=%v, want both proxy-https:8443", routes.httpURL, routes.httpsURL)
	}

	// Single server:port serves both schemes.
	routes, err = parseWindowsProxyRoutes("proxy-single:3128")
	if err != nil {
		t.Fatal(err)
	}
	if routes.httpURL != routes.httpsURL || routes.httpURL.Host != "proxy-single:3128" {
		t.Errorf("single config: http=%v https=%v", routes.httpURL, routes.httpsURL)
	}

	// Neither http nor https entry is an error.
	if _, err := parseWindowsProxyRoutes("ftp=proxy-ftp:21"); err == nil {
		t.Error("ftp-only config accepted")
	}
}

// parseWindowsProxyString keeps its historical https-preferred contract.
func TestParseWindowsProxyString_StillPrefersHTTPS(t *testing.T) {
	u, err := parseWindowsProxyString("http=proxy-http:8080;https=proxy-https:8443")
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "proxy-https:8443" {
		t.Errorf("host = %q, want proxy-https:8443 (https preferred)", u.Host)
	}
}

func TestWindowsProxyBypass(t *testing.T) {
	entries := parseProxyOverride("<local>; *.corp.example.com ;intranet.example.com;")

	// "<local>": dotless hostnames bypass; dotted ones do not.
	if !windowsProxyBypass("fileserver", entries) {
		t.Error("dotless intranet host should bypass (<local>)")
	}
	if windowsProxyBypass("fileserver.example.com", entries) {
		t.Error("dotted host must not match <local>")
	}

	// Wildcards: strict subdomains only, label boundary respected.
	if !windowsProxyBypass("a.corp.example.com", entries) {
		t.Error("subdomain should bypass (*.corp.example.com)")
	}
	if windowsProxyBypass("corp.example.com", entries) {
		t.Error("apex must not match *.corp.example.com (list it separately)")
	}
	if windowsProxyBypass("xcorp.example.com", entries) {
		t.Error("suffix without label boundary must not match")
	}

	// Exact entries (dotted names, so <local> cannot interfere).
	if !windowsProxyBypass("intranet.example.com", entries) {
		t.Error("exact entry should bypass")
	}
	if windowsProxyBypass("intranet.example.org", entries) {
		t.Error("different TLD must not match exact entry")
	}

	// Case-insensitive on both sides.
	if !windowsProxyBypass("A.CORP.EXAMPLE.COM", entries) {
		t.Error("matching should be case-insensitive")
	}

	// Empty override list: nothing bypasses (dotted host, no <local> interplay).
	if windowsProxyBypass("intranet.example.com", nil) {
		t.Error("no entries: nothing should bypass")
	}
}
