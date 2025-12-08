//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Request Cookie Inspection Example ===\n")

	// Create client
	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: Inspect request cookies using RequestHeaders
	fmt.Println("Example 1: Direct inspection via RequestHeaders")
	fmt.Println("------------------------------------------------")

	resp1, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("session", "abc123"),
		httpc.WithCookieValue("user_id", "12345"),
		httpc.WithCookieValue("theme", "dark"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Method 1: Direct access to Cookie header
	cookieHeader := resp1.Request.Headers.Get("Cookie")
	fmt.Printf("Raw Cookie header sent: %s\n\n", cookieHeader)

	// Example 2: Use helper functions to parse request cookies
	fmt.Println("Example 2: Using helper functions")
	fmt.Println("----------------------------------")

	resp2, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieString("BSID=4418ECBB; PSTM=1733760779; token=xyz789"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Get all request cookies using Response method
	requestCookies := resp2.RequestCookies()
	fmt.Printf("Request cookies sent (%d total):\n", len(requestCookies))
	for _, cookie := range requestCookies {
		fmt.Printf("  - %s = %s\n", cookie.Name, cookie.Value)
	}
	fmt.Println()

	// Get specific request cookie
	tokenCookie := resp2.GetRequestCookie("token")
	if tokenCookie != nil {
		fmt.Printf("Token cookie value: %s\n", tokenCookie.Value)
	}

	// Check if cookie was sent
	if resp2.HasRequestCookie("BSID") {
		fmt.Println("BSID cookie was sent in request")
	}
	fmt.Println()

	// Example 3: Compare request vs response cookies
	fmt.Println("Example 3: Request vs Response cookies")
	fmt.Println("---------------------------------------")

	resp3, err := client.Get("https://httpbin.org/cookies/set?server_cookie=from_server",
		httpc.WithCookieValue("client_cookie", "from_client"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Request cookies (what we sent)
	fmt.Println("Cookies sent in request:")
	requestCookies3 := resp3.RequestCookies()
	for _, cookie := range requestCookies3 {
		fmt.Printf("  [OK] %s = %s\n", cookie.Name, cookie.Value)
	}

	// Response cookies (what server sent back)
	fmt.Println("\nCookies received from server:")
	for _, cookie := range resp3.ResponseCookies() {
		fmt.Printf("  [OK] %s = %s\n", cookie.Name, cookie.Value)
	}
	fmt.Println()

	// Example 4: Inspect all request headers
	fmt.Println("Example 4: All request headers")
	fmt.Println("-------------------------------")

	resp4, err := client.Get("https://httpbin.org/headers",
		httpc.WithHeader("X-Custom-Header", "custom-value"),
		httpc.WithCookieValue("auth", "secret123"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Headers actually sent in request:")
	for name, values := range resp4.Request.Headers {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", name, value)
		}
	}
	fmt.Println()

	// Example 5: Cookie jar automatic cookies
	fmt.Println("Example 5: Cookie jar automatic cookies")
	fmt.Println("----------------------------------------")

	// Create client with cookie jar
	config := httpc.DefaultConfig()
	config.EnableCookies = true
	jarClient, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer jarClient.Close()

	// First request - server sets cookies
	fmt.Println("First request: Server sets cookies")
	resp5a, err := jarClient.Get("https://httpbin.org/cookies/set?persistent=session123")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Server set cookies:")
	for _, cookie := range resp5a.ResponseCookies() {
		fmt.Printf("  [OK] %s = %s\n", cookie.Name, cookie.Value)
	}

	// Second request - cookie jar automatically sends cookies
	fmt.Println("\nSecond request: Cookie jar automatically sends cookies")
	resp5b, err := jarClient.Get("https://httpbin.org/cookies")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Cookies automatically sent by jar:")
	autoCookies := resp5b.RequestCookies()
	for _, cookie := range autoCookies {
		fmt.Printf("  [OK] %s = %s\n", cookie.Name, cookie.Value)
	}
	fmt.Println()

	// Example 6: Debugging - verify cookies were sent correctly
	fmt.Println("Example 6: Debugging cookie issues")
	fmt.Println("-----------------------------------")

	resp6, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("debug1", "value1"),
		httpc.WithCookieValue("debug2", "value2"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Verify expected cookies were sent
	expectedCookies := []string{"debug1", "debug2"}
	fmt.Println("Verifying cookies were sent:")
	for _, name := range expectedCookies {
		if resp6.HasRequestCookie(name) {
			cookie := resp6.GetRequestCookie(name)
			fmt.Printf("  [OK] %s = %s (sent successfully)\n", name, cookie.Value)
		} else {
			fmt.Printf("  [X] %s (NOT sent)\n", name)
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("Use resp.Request.Headers to inspect actual request headers")
	fmt.Println("Use resp.RequestCookies() to parse request cookies")
	fmt.Println("Use resp.Response.Cookies for server's Set-Cookie response")
}
