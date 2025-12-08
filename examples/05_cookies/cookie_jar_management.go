//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	// Create client with cookie jar enabled
	config := httpc.DefaultConfig()
	config.EnableCookies = true
	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("=== Cookie Jar Automatic Management Demo ===\n")

	// Request 1: Add first cookie
	fmt.Println("Request 1: Adding cookie 'session=abc123'")
	resp1, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("session", "abc123"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cookies sent: %s\n", resp1.Request.Headers.Get("Cookie"))
	fmt.Printf("Server received: %s\n\n", resp1.Body())

	// Request 2: Add second cookie (first cookie persists)
	fmt.Println("Request 2: Adding cookie 'token=xyz789'")
	resp2, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("token", "xyz789"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cookies sent: %s\n", resp2.Request.Headers.Get("Cookie"))
	fmt.Printf("Server received: %s\n\n", resp2.Body())

	// Request 3: No manual cookies - jar automatically sends both
	fmt.Println("Request 3: No manual cookies (jar sends both automatically)")
	resp3, err := client.Get("https://httpbin.org/cookies")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cookies sent: %s\n", resp3.Request.Headers.Get("Cookie"))
	fmt.Printf("Server received: %s\n\n", resp3.Body())

	// Request 4: Add third cookie - all three are sent
	fmt.Println("Request 4: Adding cookie 'user=john'")
	resp4, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("user", "john"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cookies sent: %s\n", resp4.Request.Headers.Get("Cookie"))
	fmt.Printf("Server received: %s\n\n", resp4.Body())

	fmt.Println("=== Cookie Override Demo ===\n")

	// Request 5: Override existing cookie
	fmt.Println("Request 5: Overriding 'session' cookie with new value")
	resp5, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("session", "new_session_value"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cookies sent: %s\n", resp5.Request.Headers.Get("Cookie"))
	fmt.Printf("Server received: %s\n", resp5.Body())
}
