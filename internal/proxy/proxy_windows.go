//go:build windows

package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// detectPlatform reads proxy settings from Windows registry/system settings
func (d *Detector) detectPlatform() func(*http.Request) (*url.URL, error) {
	proxyServer, proxyOverride, proxyEnable, err := getWindowsProxySettings()
	if err != nil {
		return nil
	}

	if !proxyEnable || proxyServer == "" {
		return nil
	}

	// Parse proxy server string into per-scheme routes
	routes, err := parseWindowsProxyRoutes(proxyServer)
	if err != nil {
		return nil
	}
	bypass := parseProxyOverride(proxyOverride)

	return func(req *http.Request) (*url.URL, error) {
		// Loopback targets bypass the system proxy, matching net/http and the
		// environment path (isLoopbackRequest). ProxyOverride entries (the
		// user's exception list: "<local>", "*.suffix", exact hosts) bypass
		// as well.
		if isLoopbackRequest(req) || windowsProxyBypass(req.URL.Hostname(), bypass) {
			return nil, nil
		}
		// Per-protocol registry values route by scheme; when only one entry
		// is present parseWindowsProxyRoutes assigns it to both schemes,
		// matching Windows' own fallback.
		if req.URL.Scheme == "http" {
			return routes.httpURL, nil
		}
		return routes.httpsURL, nil
	}
}

// getWindowsProxySettings retrieves proxy settings from Windows registry.
// proxyOverride carries the ProxyOverride exception list ("" when unset).
func getWindowsProxySettings() (proxyServer, proxyOverride string, proxyEnable bool, err error) {
	// Try to read from Windows registry using WinAPI
	// Registry path: HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Internet Settings

	var hKey syscall.Handle

	// Open registry key
	keyPath, err := windows.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	if err != nil {
		return "", "", false, err
	}

	if err := syscall.RegOpenKeyEx(
		syscall.HKEY_CURRENT_USER,
		keyPath,
		0,
		syscall.KEY_READ,
		&hKey,
	); err != nil {
		return "", "", false, fmt.Errorf("failed to open registry key: %w", err)
	}
	defer func() { _ = syscall.RegCloseKey(hKey) }() // best-effort registry key cleanup

	// Read ProxyEnable value
	valueName, _ := windows.UTF16PtrFromString("ProxyEnable") // compile-time constant; cannot fail
	var dataType uint32
	var dataSize uint32

	err = syscall.RegQueryValueEx(
		hKey,
		valueName,
		nil,
		&dataType,
		nil,
		&dataSize,
	)

	if err == nil && dataSize == 4 {
		var enableValue uint32
		err = syscall.RegQueryValueEx(
			hKey,
			valueName,
			nil,
			&dataType,
			(*byte)(unsafe.Pointer(&enableValue)),
			&dataSize,
		)
		if err == nil {
			proxyEnable = enableValue != 0
		}
	}

	if !proxyEnable {
		return "", "", false, nil
	}

	// Read ProxyServer value
	valueName, _ = windows.UTF16PtrFromString("ProxyServer") // "ProxyServer" is a compile-time constant; cannot fail
	err = syscall.RegQueryValueEx(
		hKey,
		valueName,
		nil,
		&dataType,
		nil,
		&dataSize,
	)

	if err != nil {
		return "", "", false, nil
	}

	buf := make([]uint16, dataSize/2+1)
	err = syscall.RegQueryValueEx(
		hKey,
		valueName,
		nil,
		&dataType,
		(*byte)(unsafe.Pointer(&buf[0])),
		&dataSize,
	)

	if err == nil {
		proxyServer = windows.UTF16ToString(buf)
	}

	// Read ProxyOverride value (the user's proxy-exception list, e.g.
	// "<local>;*.corp.example"). Absence is not an error — an unset list
	// simply produces no bypass entries.
	valueName, _ = windows.UTF16PtrFromString("ProxyOverride") // compile-time constant; cannot fail
	err = syscall.RegQueryValueEx(
		hKey,
		valueName,
		nil,
		&dataType,
		nil,
		&dataSize,
	)
	if err == nil {
		buf := make([]uint16, dataSize/2+1)
		if err = syscall.RegQueryValueEx(
			hKey,
			valueName,
			nil,
			&dataType,
			(*byte)(unsafe.Pointer(&buf[0])),
			&dataSize,
		); err == nil {
			proxyOverride = windows.UTF16ToString(buf)
		}
	}

	return proxyServer, proxyOverride, proxyEnable, nil
}

// parseWindowsProxyString parses a Windows proxy server string and returns
// the preferred single proxy URL (https entry over http, regardless of
// registry ordering) — its historical contract, pinned by tests. Routing
// that distinguishes schemes uses parseWindowsProxyRoutes instead.
// Format can be:
// - "server:port" (single proxy for all protocols)
// - "http=server:port;https=server:port;ftp=server:port" (per-protocol)
func parseWindowsProxyString(proxyStr string) (*url.URL, error) {
	routes, err := parseWindowsProxyRoutes(proxyStr)
	if err != nil {
		return nil, err
	}
	return routes.httpsURL, nil
}

// windowsProxyRoutes holds the per-scheme proxy URLs parsed from the
// registry ProxyServer value.
type windowsProxyRoutes struct {
	httpURL, httpsURL *url.URL
}

// parseWindowsProxyRoutes parses "server:port" (one proxy for both schemes)
// or per-protocol "http=a:80;https=b:443" syntax. When both entries are
// present each scheme routes to its own entry — Windows itself routes
// per-protocol, and collapsing to the https entry would send plaintext
// http:// traffic to the https proxy port. When only one entry is present it
// applies to both schemes, matching Windows' fallback.
func parseWindowsProxyRoutes(proxyStr string) (*windowsProxyRoutes, error) {
	proxyStr = strings.TrimSpace(proxyStr)
	if proxyStr == "" {
		return nil, fmt.Errorf("empty proxy string")
	}

	// Check for per-protocol configuration
	if strings.Contains(proxyStr, "=") {
		servers := make(map[string]string)
		for part := range strings.SplitSeq(proxyStr, ";") {
			if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
				servers[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
			}
		}
		parse := func(server string) (*url.URL, error) {
			if server == "" {
				return nil, nil
			}
			if !strings.Contains(server, "://") {
				server = "http://" + server
			}
			return url.Parse(server)
		}
		httpURL, err := parse(servers["http"])
		if err != nil {
			return nil, fmt.Errorf("invalid http proxy in per-protocol config: %w", err)
		}
		httpsURL, err := parse(servers["https"])
		if err != nil {
			return nil, fmt.Errorf("invalid https proxy in per-protocol config: %w", err)
		}
		if httpURL == nil && httpsURL == nil {
			return nil, fmt.Errorf("no http/https proxy found in per-protocol config: %q", proxyStr)
		}
		if httpURL == nil {
			httpURL = httpsURL
		}
		if httpsURL == nil {
			httpsURL = httpURL
		}
		return &windowsProxyRoutes{httpURL: httpURL, httpsURL: httpsURL}, nil
	}

	// Simple server:port format (possibly with an explicit scheme).
	// Normalize the bare "socks" scheme to "socks5": net/http's Transport proxy
	// support recognizes socks5/socks5h but not "socks://", so leaving a value
	// like "socks://host:1080" unchanged would yield a "socks" scheme the
	// transport cannot use (silently ignored or a confusing dial error).
	if after, ok := strings.CutPrefix(proxyStr, "socks://"); ok {
		proxyStr = "socks5://" + after
	}

	if !strings.HasPrefix(proxyStr, "http://") &&
		!strings.HasPrefix(proxyStr, "https://") &&
		!strings.HasPrefix(proxyStr, "socks5://") &&
		!strings.HasPrefix(proxyStr, "socks5h://") {
		proxyStr = "http://" + proxyStr
	}

	u, err := url.Parse(proxyStr)
	if err != nil {
		return nil, err
	}
	return &windowsProxyRoutes{httpURL: u, httpsURL: u}, nil
}

// parseProxyOverride splits the registry ProxyOverride value (";"-separated)
// into bypass entries: "<local>", wildcard patterns ("*.corp.example"), and
// exact host names.
func parseProxyOverride(override string) []string {
	override = strings.TrimSpace(override)
	if override == "" {
		return nil
	}
	entries := make([]string, 0, 4)
	for part := range strings.SplitSeq(override, ";") {
		if part = strings.TrimSpace(part); part != "" {
			entries = append(entries, part)
		}
	}
	return entries
}

// windowsProxyBypass reports whether host matches a ProxyOverride entry.
// Entry semantics live in the shared matchProxyBypassEntry ("<local>" matches
// dotless hostnames, "*.suffix" matches strict subdomains, exact otherwise);
// this wrapper keeps the ProxyOverride-specific name for the registry path.
func windowsProxyBypass(host string, entries []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, entry := range entries {
		if matchProxyBypassEntry(host, entry) {
			return true
		}
	}
	return false
}
