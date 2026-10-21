//go:build examples

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

// This example demonstrates browser transport fingerprinting
// (Security.TLSFingerprint): the client impersonates a browser's TLS
// ClientHello, HTTP/2 settings, and header block so the wire looks like a real
// Chrome/Firefox build rather than the Go HTTP stack.

func main() {
	fmt.Println("=== TLS Fingerprint Examples ===")

	// 1. The standard Go stack (no spoofing) — the baseline.
	demonstrateStandardStack()

	// 2. Chrome profile with an OS suffix.
	demonstrateChromeProfile()

	// 3. Firefox profile via a short alias.
	demonstrateFirefoxProfile()

	// 4. Disabling session-ticket resumption for rotation pipelines.
	demonstrateNoSessionTickets()

	fmt.Println("\n=== All Examples Completed ===")
}

// request makes one GET and reports the outcome. It never exits on error so a
// network failure on one profile does not stop the remaining examples.
func request(label string, client httpc.Client) {
	resp, err := client.Get("https://httpbin.org/get",
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("%s: request failed: %v\n", label, err)
		return
	}
	fmt.Printf("  %s: Status %d over %s\n", label, resp.StatusCode(), resp.Proto())
}

// demonstrateStandardStack shows the default, non-spoofed Go stack.
func demonstrateStandardStack() {
	fmt.Println("--- Example 1: Standard Go Stack (baseline) ---")

	// Empty TLSFingerprint (the default) uses the standard crypto/tls stack.
	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Println("  TLSFingerprint: \"\" (standard crypto/tls, no spoofing)")
	request("standard", client)
	fmt.Println()
}

// demonstrateChromeProfile shows a Chrome profile with an OS suffix.
func demonstrateChromeProfile() {
	fmt.Println("--- Example 2: Chrome Profile (chrome-windows) ---")

	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = "chrome-windows"

	client, err := httpc.New(cfg)
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Println("  TLSFingerprint: \"chrome-windows\"")
	fmt.Println("  Spoofs Chrome's ClientHello (JA3/JA4), HTTP/2 frames, and headers")
	fmt.Println("  with a coherent Chrome User-Agent.")
	request("chrome", client)
	fmt.Println()
}

// demonstrateFirefoxProfile shows a Firefox profile via a short alias.
func demonstrateFirefoxProfile() {
	fmt.Println("--- Example 3: Firefox Profile (short alias \"firefox\") ---")

	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = "firefox" // resolves to the Firefox profile

	client, err := httpc.New(cfg)
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Println("  TLSFingerprint: \"firefox\" (short alias for the Firefox profile)")
	request("firefox", client)
	fmt.Println()

	fmt.Println("Other presets:")
	fmt.Println("  chrome, chrome-latest, chrome-155, chrome-152")
	fmt.Println("  chrome-windows / -macos / -linux / -android / -ios")
	fmt.Println("  firefox, firefox-latest, firefox-158, firefox-148")
	fmt.Println("  firefox-windows / -macos / -linux / -android / -ios")
	fmt.Println("  android-chrome-latest, ios-chrome-latest, safari-ios")
	fmt.Println()
}

// demonstrateNoSessionTickets forces a full handshake on every connection.
func demonstrateNoSessionTickets() {
	fmt.Println("--- Example 4: Disable Session Tickets (full handshake each time) ---")

	cfg := httpc.DefaultConfig()
	cfg.Security.TLSFingerprint = "chrome-latest"
	// Chrome profiles cache session tickets per pool (like a real browser), so
	// a repeat connection resumes instead of full-handshaking. Set true to make
	// every connection fully independent — for fingerprint-rotation pipelines
	// or debugging a target.
	cfg.Security.TLSFingerprintDisableSessionTickets = true

	client, err := httpc.New(cfg)
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Println("  TLSFingerprint: \"chrome-latest\"")
	fmt.Println("  TLSFingerprintDisableSessionTickets: true (no PSK resumption)")
	request("chrome-no-tickets", client)
	fmt.Println()

	fmt.Println("Notes:")
	fmt.Println("  - The engine (redirects, retries, middleware, streaming) stays in control;")
	fmt.Println("    only the transport layer is replaced.")
	fmt.Println("  - Proxy pools, rotation, and circuit breaking keep working.")
	fmt.Println("  - The preset's header set is authoritative: a same-named request header")
	fmt.Println("    is replaced by the browser profile's value to keep the wire coherent.")
}
