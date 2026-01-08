//go:build examples

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/cybergodev/httpc"
)

// This example demonstrates DomainClient for automatic state management
// Consolidates: 06_domain_client/* (all domain client examples)

func main() {
	fmt.Println("=== DomainClient Examples ===\n ")

	// 1. Basic Usage
	demonstrateBasicUsage()

	// 2. State Management
	demonstrateStateManagement()

	// 3. URL Matching
	demonstrateURLMatching()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateBasicUsage shows basic DomainClient usage
func demonstrateBasicUsage() {
	fmt.Println("--- Basic DomainClient Usage ---")

	// Create domain-specific client
	client, err := httpc.NewDomain("https://www.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Set persistent headers (sent with every request)
	err = client.SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	if err != nil {
		log.Fatal(err)
	}
	err = client.SetHeader("Accept-Language", "en-US,en;q=0.9")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✓ Created DomainClient for www.example.com\n")
	fmt.Printf("✓ Set %d persistent headers\n", len(client.GetHeaders()))

	// First request
	resp1, err := client.Get("/")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("✓ First request: Status %d\n", resp1.StatusCode())
	fmt.Printf("✓ Received %d cookies\n", len(resp1.ResponseCookies()))

	// Second request - headers and cookies automatically sent
	resp2, err := client.Get("/search?q=golang")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("✓ Second request: Status %d\n", resp2.StatusCode())
	fmt.Printf("✓ Persistent headers: %d\n", len(client.GetHeaders()))
	fmt.Printf("✓ Persistent cookies: %d\n\n", len(client.GetCookies()))
}

// demonstrateStateManagement shows cookie and header management
func demonstrateStateManagement() {
	fmt.Println("--- State Management ---")

	client, err := httpc.NewDomain("https://httpbin.org")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Set persistent headers
	client.SetHeader("X-API-Version", "v1")
	client.SetHeader("X-Client-ID", "client-123")
	fmt.Printf("✓ Set %d headers\n", len(client.GetHeaders()))

	// Add cookies manually
	client.SetCookie(&http.Cookie{
		Name:  "session",
		Value: "abc123",
	})
	client.SetCookie(&http.Cookie{
		Name:  "preferences",
		Value: "dark_mode",
	})
	fmt.Printf("✓ Set %d cookies\n", len(client.GetCookies()))

	// Make request - all state automatically sent
	resp, err := client.Get("/cookies")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("✓ Request with state: Status %d\n", resp.StatusCode())

	// Override header for single request
	resp, err = client.Get("/get",
		httpc.WithHeader("X-API-Version", "v2"), // Override for this request only
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("✓ Request with override: Status %d\n", resp.StatusCode())
	fmt.Printf("✓ Persistent headers still intact: %d\n", len(client.GetHeaders()))

	// Clear state
	client.ClearCookies()
	fmt.Printf("✓ Cleared cookies: %d remaining\n", len(client.GetCookies()))

	client.ClearHeaders()
	fmt.Printf("✓ Cleared headers: %d remaining\n\n", len(client.GetHeaders()))
}

// demonstrateURLMatching shows URL matching behavior
func demonstrateURLMatching() {
	fmt.Println("--- URL Matching ---")

	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Valid: Relative paths
	validPaths := []string{
		"/users",
		"/users/123",
		"/api/v1/data",
		"/?query=test",
	}

	fmt.Println("Valid relative paths:")
	for _, path := range validPaths {
		resp, err := client.Get(path)
		if err != nil {
			fmt.Printf("  ✗ %s: %v\n", path, err)
		} else {
			fmt.Printf("  ✓ %s: Status %d\n", path, resp.StatusCode())
		}
	}

	// Invalid: Different domains (will error)
	fmt.Println("\nInvalid (different domain):")
	invalidURLs := []string{
		"https://other-domain.com/path",
		"http://api.example.com/path", // Different scheme
	}

	for _, url := range invalidURLs {
		_, err := client.Get(url)
		if err != nil {
			fmt.Printf("  ✓ %s: Correctly rejected\n", url)
		} else {
			fmt.Printf("  ✗ %s: Should have been rejected\n", url)
		}
	}
}
