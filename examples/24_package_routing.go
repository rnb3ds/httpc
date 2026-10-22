//go:build examples

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

// This example demonstrates the package-level routing options:
//
//	WithProxy          — route the call through an explicit proxy
//	WithSystemProxy    — route the call through the system proxy
//	WithTLSFingerprint — route the call through a browser fingerprint client
//
// These apply only to package-level functions (httpc.Get, httpc.Post,
// httpc.Request, httpc.Download, ...). Each routes a single call to a dedicated
// client, leaving the shared default client untouched. Each (proxy, fingerprint)
// combination is cached and can be reclaimed with httpc.CloseRoutedClients().

func main() {
	fmt.Println("=== Package-Level Routing Examples ===")

	// 1. Default client (direct).
	demonstrateDirect()

	// 2. Explicit proxy — set the placeholder to your running proxy.
	demonstrateExplicitProxy("http://127.0.0.1:7890")

	// 3. System proxy (env vars / platform settings).
	demonstrateSystemProxy()

	// 4. Browser fingerprint, verified against the JA4 echo service.
	demonstrateFingerprint()

	// 5. Explicit proxy + browser fingerprint, combined.
	demonstrateProxyAndFingerprint("http://127.0.0.1:7890")

	fmt.Println("\n=== All Examples Completed ===")
}

const echoURL = "https://ipinfo.io/json"

type echoResult struct {
	IP   string `json:"ip"`
	City string `json:"city"`
}

// printExitIP reports the exit IP reported by ipinfo.io, if the body parses.
func printExitIP(label, body string) {
	var e echoResult
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		fmt.Printf("  %s: %s\n", label, body)
		return
	}
	fmt.Printf("  %s: %s (%s)\n", label, e.IP, e.City)
}

func proxyLabel(proxyURL string) string {
	if proxyURL == "" {
		return "direct"
	}
	return proxyURL
}

// demonstrateDirect shows the default package-level client (no routing option).
func demonstrateDirect() {
	fmt.Println("--- Example 1: Default Client (direct) ---")
	res, err := httpc.Get(echoURL, httpc.WithTimeout(15*time.Second))
	if err != nil {
		log.Printf("  direct request failed: %v\n", err)
		return
	}
	printExitIP("  exit IP", res.Body())
}

// demonstrateExplicitProxy routes one call through an explicit proxy.
func demonstrateExplicitProxy(proxyURL string) {
	fmt.Printf("--- Example 2: WithProxy (%s) ---\n", proxyURL)
	res, err := httpc.Get(echoURL,
		httpc.WithProxy(proxyURL),
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("  proxy request failed (is the proxy running?): %v\n", err)
		return
	}
	fmt.Printf("  proxy used: %s\n", proxyLabel(res.Meta.ProxyURL))
	printExitIP("  exit IP", res.Body())
}

// demonstrateSystemProxy routes one call through the system proxy.
func demonstrateSystemProxy() {
	fmt.Println("--- Example 3: WithSystemProxy ---")
	res, err := httpc.Get(echoURL,
		httpc.WithSystemProxy(),
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("  system proxy request failed: %v\n", err)
		return
	}
	fmt.Printf("  proxy used: %s\n", proxyLabel(res.Meta.ProxyURL))
	printExitIP("  exit IP", res.Body())
}

// demonstrateFingerprint routes one call through a Chrome fingerprint client
// and verifies the spoofed User-Agent / JA4 via the echo service.
func demonstrateFingerprint() {
	fmt.Println("--- Example 4: WithTLSFingerprint (chrome) ---")
	const ja4URL = "https://tls.peet.ws/api/all"

	res, err := httpc.Get(ja4URL,
		httpc.WithTLSFingerprint("chrome"),
		httpc.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Printf("  fingerprint request failed: %v\n", err)
		return
	}

	var fp struct {
		UserAgent string `json:"user_agent"`
		TLS       struct {
			JA4 string `json:"ja4"`
		} `json:"tls"`
	}
	if err := json.Unmarshal([]byte(res.Body()), &fp); err != nil {
		log.Printf("  parse echo response failed: %v\n", err)
		return
	}
	fmt.Printf("  User-Agent: %s\n", fp.UserAgent)
	fmt.Printf("  JA4       : %s\n", fp.TLS.JA4)
}

// demonstrateProxyAndFingerprint combines the two routing options.
func demonstrateProxyAndFingerprint(proxyURL string) {
	fmt.Printf("--- Example 5: WithProxy + WithTLSFingerprint (%s + chrome) ---\n", proxyURL)
	res, err := httpc.Get(echoURL,
		httpc.WithProxy(proxyURL),
		httpc.WithTLSFingerprint("chrome"),
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("  combined request failed: %v\n", err)
		return
	}
	fmt.Printf("  proxy used: %s\n", proxyLabel(res.Meta.ProxyURL))
	printExitIP("  exit IP", res.Body())
}
