//go:build darwin

package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// detectPlatform reads proxy settings from macOS system configuration
// using the networksetup command-line tool
func (d *Detector) detectPlatform() func(*http.Request) (*url.URL, error) {
	// Resolve the network service once: each getMacOSProxy call previously
	// re-ran -listallnetworkservices, doubling the exec round-trips (and the
	// windows of opportunity for the 2s timeout to fire) on every detection.
	service := d.getPrimaryNetworkService()
	if service == "" {
		return nil
	}

	// Per-scheme routes, mirroring parseWindowsProxyRoutes on Windows: HTTP
	// traffic uses the Web Proxy, HTTPS traffic the Secure Web Proxy, and
	// when only one of the two is configured it serves both schemes — the
	// macOS fallback, identical to the previous single-proxy behavior. The
	// earlier version preferred the Web Proxy for EVERY scheme, sending
	// https:// traffic through the plaintext HTTP proxy whenever both were
	// configured (Windows routes per scheme; net/http's env path is
	// per-scheme as well).
	httpURL := d.getMacOSProxy(service, "getwebproxy")
	httpsURL := d.getMacOSProxy(service, "getsecurewebproxy")
	if httpURL == nil {
		httpURL = httpsURL
	}
	if httpsURL == nil {
		httpsURL = httpURL
	}
	if httpURL == nil && httpsURL == nil {
		return nil
	}

	bypass := d.getMacOSBypassDomains(service)

	return func(req *http.Request) (*url.URL, error) {
		// Loopback targets and the user's bypass-domains exception list
		// ("*.corp.example", CIDR ranges, ...) stay direct — matching the
		// Windows ProxyOverride path and the environment path (NO_PROXY).
		// The bypass list was previously ignored entirely, leaking intranet
		// URLs to the proxy and breaking direct intranet routing.
		if isLoopbackRequest(req) || darwinProxyBypass(req.URL.Hostname(), bypass) {
			return nil, nil
		}
		if req.URL.Scheme == "http" {
			return httpURL, nil
		}
		return httpsURL, nil
	}
}

// darwinProxyBypass reports whether host matches a macOS bypass-domains
// entry: the shared system-proxy grammar (see matchProxyBypassEntry) plus
// IP and CIDR entries, which macOS supports and Windows ProxyOverride does
// not.
func darwinProxyBypass(host string, entries []string) bool {
	for _, entry := range entries {
		if matchProxyBypassEntry(host, entry) || matchBypassCIDR(host, entry) {
			return true
		}
	}
	return false
}

// matchBypassCIDR reports whether entry is an IP or CIDR bypass entry
// ("10.0.0.1", "192.168.0.0/16") covering host, which must be an IP literal.
func matchBypassCIDR(host, entry string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	if entry == ip.String() {
		return true
	}
	_, cidr, err := net.ParseCIDR(strings.TrimSpace(entry))
	return err == nil && cidr.Contains(ip)
}

// getMacOSProxy retrieves proxy settings using the networksetup command.
// toolType is "getwebproxy" (HTTP) or "getsecurewebproxy" (HTTPS); service is
// the network service name to query (resolved once by the caller).
func (d *Detector) getMacOSProxy(service, toolType string) *url.URL {
	// Run networksetup command to get proxy settings. Bounded by a timeout:
	// this runs while holding the Detector write lock on the client-construction
	// path, so a wedged networksetup would otherwise block NewClient forever.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "networksetup", "-"+toolType, service)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil
	}

	// Parse output
	// Format:
	// Enabled: Yes/No
	// Server: hostname
	// Port: port
	// Authenticated Proxy Enabled: 0/1
	// ...
	output := stdout.String()
	return d.parseNetworkSetupOutput(output)
}

// getMacOSBypassDomains returns the system proxy bypass domains configured
// for the service ("*.corp.example", "192.168.0.0/16", ...), or nil when none
// are configured. Bounded by the same 2s timeout as the proxy reads.
func (d *Detector) getMacOSBypassDomains(service string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "networksetup", "-getproxybypassdomains", service)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil
	}

	var entries []string
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		// Real bypass entries are single tokens. When no bypass domains are
		// configured, networksetup prints a prose sentence ("There aren't
		// any bypass domains set on this computer.") — the whitespace check
		// filters it out along with empty lines.
		if entry := strings.TrimSpace(line); entry != "" && !strings.ContainsAny(entry, " \t") {
			entries = append(entries, entry)
		}
	}
	return entries
}

// getPrimaryNetworkService returns the primary network service name
func (d *Detector) getPrimaryNetworkService() string {
	// Get list of network services. Bounded by a timeout — see getMacOSProxy.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "networksetup", "-listallnetworkservices")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		// Fallback: try common service names
		return ""
	}

	lines := strings.Split(stdout.String(), "\n")
	if len(lines) < 2 {
		return ""
	}

	// First line is a header, skip it
	// Look for common network services in order of preference
	preferredServices := []string{
		"Wi-Fi",
		"Ethernet",
		"USB 10/100/1000 LAN",
		"Thunderbolt Ethernet",
		"Bluetooth PAN",
	}

	availableServices := lines[1:]

	// Find first preferred service that exists
	for _, preferred := range preferredServices {
		for _, available := range availableServices {
			available = strings.TrimSpace(available)
			if available == "" || strings.Contains(available, "*") {
				continue // Skip disabled services (marked with *)
			}
			if strings.EqualFold(available, preferred) {
				return available
			}
		}
	}

	// If no preferred service found, use first available one
	for _, available := range availableServices {
		available = strings.TrimSpace(available)
		if available != "" && !strings.Contains(available, "*") {
			return available
		}
	}

	return ""
}

// parseNetworkSetupOutput parses networksetup command output
func (d *Detector) parseNetworkSetupOutput(output string) *url.URL {
	lines := strings.Split(output, "\n")

	var enabled bool
	var server string
	var port int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "enabled":
			enabled = strings.EqualFold(value, "yes") || value == "1"
		case "server":
			server = value
		case "port":
			if p, err := strconv.Atoi(value); err == nil {
				port = p
			}
		}
	}

	if !enabled || server == "" || port == 0 {
		return nil
	}

	// Construct proxy URL
	proxyStr := fmt.Sprintf("http://%s:%d", server, port)
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil
	}

	return proxyURL
}
