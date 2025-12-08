//go:build examples

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== DomainClient Demo ===\n")

	// Example: Using DomainClient for example
	demoExampleClient()

	fmt.Println("\n=== Demo Complete ===")
}

func demoExampleClient() {
	// Create a domain client for example
	client, err := httpc.NewDomain("https://www.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Set persistent headers
	err = client.SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	if err != nil {
		log.Fatal(err)
	}
	err = client.SetHeader("Accept-Language", "en-US,en;q=0.9")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("[OK] Created DomainClient for www.example.com")
	fmt.Printf("[OK] Set %d persistent headers\n", len(client.GetHeaders()))

	// First request
	fmt.Println("\n[OK] Making first request to /")
	resp1, err := client.Get("/")
	if err != nil {
		log.Printf("First request error: %v\n", err)
		return
	}
	fmt.Printf("[OK] Status: %d\n", resp1.StatusCode())
	fmt.Printf("[OK] Received %d cookies from server\n", len(resp1.ResponseCookies()))

	// Display received cookies
	cookies := resp1.ResponseCookies()
	if len(cookies) > 0 {
		fmt.Println("\nCookies received:")
		for i, cookie := range cookies {
			if i < 3 { // Show first 3 cookies
				fmt.Printf("  - %s = %s\n", cookie.Name, truncate(cookie.Value, 20))
			}
		}
		if len(cookies) > 3 {
			fmt.Printf("  ... and %d more\n", len(cookies)-3)
		}
	}

	// Second request - cookies automatically sent
	fmt.Println("\n[OK] Making second request to /search?q=golang")
	resp2, err := client.Get("/search?q=golang")
	if err != nil {
		log.Printf("Second request error: %v\n", err)
		return
	}
	fmt.Printf("[OK] Status: %d\n", resp2.StatusCode())
	fmt.Printf("[OK] Cookies automatically sent: %d\n", len(client.GetCookies()))
	fmt.Printf("[OK] Headers automatically sent: %d\n", len(client.GetHeaders()))

	// Add a new cookie manually
	fmt.Println("\n[OK] Adding custom cookie manually")
	err = client.SetCookie(&http.Cookie{
		Name:  "custom_pref",
		Value: "dark_mode",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("[OK] Total cookies now: %d\n", len(client.GetCookies()))

	// Third request with override header
	fmt.Println("\n[OK] Making third request with header override")
	resp3, err := client.Get("/",
		httpc.WithHeader("Accept", "text/html"), // Override for this request only
	)
	if err != nil {
		log.Printf("Third request error: %v\n", err)
		return
	}
	fmt.Printf("[OK] Status: %d\n", resp3.StatusCode())
	fmt.Println("[OK] Persistent headers still intact after override")

	// Show final state
	fmt.Println("\n=== Final State ===")
	fmt.Printf("Persistent Headers: %d\n", len(client.GetHeaders()))
	fmt.Printf("Persistent Cookies: %d\n", len(client.GetCookies()))

	// Demonstrate state management
	fmt.Println("\n[OK] Clearing all cookies")
	client.ClearCookies()
	fmt.Printf("[OK] Cookies after clear: %d\n", len(client.GetCookies()))

	fmt.Println("\n[OK] Clearing all headers")
	client.ClearHeaders()
	fmt.Printf("[OK] Headers after clear: %d\n", len(client.GetHeaders()))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
