// Quick example showing the difference between request and response cookies
//
// Run: go run examples/quick_cookie_inspection.go

//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Make request with cookies
	resp, err := client.Get("https://httpbin.org/cookies/set?server_cookie=from_server",
		httpc.WithCookieValue("client_cookie", "from_client"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Cookie Inspection Quick Reference ===\n")

	// Request Cookies (what we sent TO the server)
	fmt.Println("1. Request Cookies (sent TO server):")
	fmt.Println("   Method: resp.RequestCookies()")
	requestCookies := resp.RequestCookies()
	for _, cookie := range requestCookies {
		fmt.Printf("   [OK] %s = %s\n", cookie.Name, cookie.Value)
	}

	// Check specific request cookie
	if resp.HasRequestCookie("client_cookie") {
		cookie := resp.GetRequestCookie("client_cookie")
		fmt.Printf("   [OK] Found: %s = %s\n", cookie.Name, cookie.Value)
	}

	fmt.Println()

	// Response Cookies (what server sent back TO us)
	fmt.Println("2. Response Cookies (received FROM server):")
	fmt.Println("   Method: resp.Response.Cookies")
	for _, cookie := range resp.Response.Cookies {
		fmt.Printf("   [OK] %s = %s\n", cookie.Name, cookie.Value)
	}

	// Check specific response cookie
	if resp.HasCookie("server_cookie") {
		cookie := resp.GetCookie("server_cookie")
		fmt.Printf("   [OK] Found: %s = %s\n", cookie.Name, cookie.Value)
	}

	fmt.Println()

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Println("Request Cookies:  resp.RequestCookies() / resp.GetRequestCookie(name) / resp.HasRequestCookie(name)")
	fmt.Println("Response Cookies: resp.Response.Cookies / resp.GetCookie(name) / resp.HasCookie(name)")
}
