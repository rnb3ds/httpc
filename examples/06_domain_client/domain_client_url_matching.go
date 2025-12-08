//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== DomainClient URL Matching Example ===\n")

	// Create a domain client for example.com
	config := httpc.DefaultConfig()
	config.EnableCookies = true
	config.FollowRedirects = true
	config.MaxRedirects = 10

	client, err := httpc.NewDomain("https://www.example.com", config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: Relative path without leading slash
	fmt.Println("Example 1: Relative path without slash")
	fmt.Println("  client.Get(\"aa.html\")")
	fmt.Println("  https://www.example.com/aa.html")
	_, err = client.Get("aa.html")
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	// Example 2: Relative path with leading slash
	fmt.Println("\nExample 2: Relative path with slash")
	fmt.Println("  client.Get(\"/aa.html\")")
	fmt.Println("  https://www.example.com/aa.html")
	_, err = client.Get("/aa.html")
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	// Example 3: Full URL with same domain
	fmt.Println("\nExample 3: Full URL with same domain")
	fmt.Println("  client.Get(\"https://www.example.com/aa.html\")")
	fmt.Println("  Domain matches! Uses client config and cookies")
	_, err = client.Get("https://www.example.com/aa.html")
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	// Example 4: Full URL with different domain
	fmt.Println("\nExample 4: Full URL with different domain")
	fmt.Println("  client.Get(\"https://api.example.com/test\")")
	fmt.Println("  Different domain, but request still works")
	_, err = client.Get("https://api.example.com/test")
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	// Example 5: Auto-persist request options
	fmt.Println("\n=== Auto-Persist Request Options Example ===")

	// First request with cookies and headers via options
	fmt.Println("\nStep 1: First request with options")
	fmt.Println("  client.Get(\"/login\",")
	fmt.Println("    WithCookieValue(\"session\", \"abc123\"),")
	fmt.Println("    WithHeader(\"X-Auth\", \"token\"))")
	_, err = client.Get("/login",
		httpc.WithCookieValue("session", "abc123"),
		httpc.WithHeader("X-Auth", "token"),
	)
	if err != nil {
		log.Printf("  Error: %v\n", err)
	} else {
		fmt.Println("  Options automatically persisted!")
	}

	// Second request without options - uses persisted values
	fmt.Println("\nStep 2: Second request without options")
	fmt.Println("  client.Get(\"/api/data\")")
	fmt.Println("  Automatically uses persisted cookies and headers!")
	_, err = client.Get("/api/data")
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	// Third request with full URL - still uses persisted values
	fmt.Println("\nStep 3: Request with full URL (same domain)")
	fmt.Println("  client.Get(\"https://www.example.com/api/profile\")")
	fmt.Println("  Persisted options still apply!")
	_, err = client.Get("https://www.example.com/api/profile")
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	// Fourth request with new options - overrides persisted values
	fmt.Println("\nStep 4: Request with new options")
	fmt.Println("  client.Get(\"/api/settings\",")
	fmt.Println("    WithHeader(\"X-Auth\", \"new-token\"))")
	fmt.Println("  New options override and persist!")
	_, err = client.Get("/api/settings",
		httpc.WithHeader("X-Auth", "new-token"),
	)
	if err != nil {
		log.Printf("  Error: %v\n", err)
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("DomainClient automatically:")
	fmt.Println("  Handles relative paths (with or without leading slash)")
	fmt.Println("  Detects full URLs and matches domain")
	fmt.Println("  Persists cookies from responses AND request options")
	fmt.Println("  Persists headers from request options")
	fmt.Println("  Maintains consistent configuration")
	fmt.Println("  Allows cross-domain requests when needed")
}
