package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/cybergodev/httpc"
)

// demonstrateCookies shows various cookie handling patterns
func demonstrateCookies() {
	fmt.Println("=== Cookie Examples ===\n ")

	// Example 1: Setting cookies in requests
	demonstrateRequestCookies()

	// Example 2: Reading cookies from responses
	demonstrateResponseCookies()

	// Example 3: Automatic cookie management with Cookie Jar
	demonstrateCookieJar()

	// Example 4: Advanced cookie attributes
	demonstrateAdvancedCookies()
}

// demonstrateRequestCookies shows how to send cookies with requests
func demonstrateRequestCookies() {
	fmt.Println("--- Example 1: Setting Request Cookies ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Method 1: Simple name-value cookie
	resp, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("session_id", "abc123"),
		httpc.WithCookieValue("user_pref", "dark_mode"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Method 1: Simple name-value cookie")
	fmt.Println("Sending simple name-value cookies:")
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n\n", resp.Body)

	// Method 2: Cookie with attributes
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    "xyz789",
		Path:     "/api",
		Domain:   "httpbin.org",
		Expires:  time.Now().Add(24 * time.Hour),
		Secure:   true,
		HttpOnly: true,
	}

	resp, err = client.Get("https://httpbin.org/cookies",
		httpc.WithCookie(cookie),
		httpc.WithCookieValue("login_token", "login_abc123"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Method 2: Cookie with attributes")
	fmt.Println("Sending cookie with attributes:")
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n\n", resp.Body)

	// Method 3: Multiple cookies at once
	cookies := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
		{Name: "cookie3", Value: "value3"},
	}

	resp, err = client.Get("https://httpbin.org/cookies",
		httpc.WithCookies(cookies),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Method 3: Multiple cookies at once")
	fmt.Println("Sending multiple cookies at once:")
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n\n", resp.Body)
}

// demonstrateResponseCookies shows how to read cookies from responses
func demonstrateResponseCookies() {
	fmt.Println("--- Example 2: Reading Response Cookies ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Use response-headers endpoint which directly sets cookies without redirect
	// Note: httpbin.org/cookies/set uses redirects, so cookies won't appear in final response
	resp, err := client.Get("https://httpbin.org/response-headers?Set-Cookie=session=abc123&Set-Cookie=user=john")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Number of cookies received: %d\n", len(resp.Cookies))

	// Method 1: Iterate through all cookies
	fmt.Println("\nAll cookies:")
	for _, cookie := range resp.Cookies {
		fmt.Printf("  %s = %s\n", cookie.Name, cookie.Value)
	}

	// Method 2: Get specific cookie by name
	sessionCookie := resp.GetCookie("session")
	if sessionCookie != nil {
		fmt.Printf("\nSession cookie value: %s\n", sessionCookie.Value)
	}

	// Method 3: Check if cookie exists
	if resp.HasCookie("user") {
		fmt.Println("User cookie is present")
	}

	if !resp.HasCookie("nonexistent") {
		fmt.Println("Nonexistent cookie is not present")
	}

	fmt.Println()
}

// demonstrateCookieJar shows automatic cookie management
func demonstrateCookieJar() {
	fmt.Println("--- Example 3: Automatic Cookie Management (Cookie Jar) ---")

	// Create a cookie jar
	jar, err := httpc.NewCookieJar()
	if err != nil {
		log.Printf("Failed to create cookie jar: %v\n", err)
		return
	}

	// Create client with cookie jar enabled
	config := httpc.DefaultConfig()
	config.EnableCookies = true
	config.CookieJar = jar

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// First request - server sets cookies via redirect
	fmt.Println("Step 1: Server sets cookies via redirect")
	resp1, err := client.Get("https://httpbin.org/cookies/set?session=xyz789&user=alice")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n", resp1.StatusCode)
	fmt.Printf("Cookies in final response: %d (expected: 0, cookies are in redirect response)\n", len(resp1.Cookies))

	// Check cookies in jar
	u, _ := url.Parse("https://httpbin.org")
	cookiesInJar := jar.Cookies(u)
	fmt.Printf("Cookies stored in jar: %d\n", len(cookiesInJar))
	for _, cookie := range cookiesInJar {
		fmt.Printf("  - %s = %s\n", cookie.Name, cookie.Value)
	}

	fmt.Println("\nStep 2: Cookies automatically sent in subsequent requests")
	// Second request - cookies are automatically sent
	resp2, err := client.Get("https://httpbin.org/cookies")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n", resp2.StatusCode)
	fmt.Printf("Response: %s\n", resp2.Body)

	fmt.Println("\nStep 3: Cookies persist across multiple requests")
	// Third request - cookies still present
	resp3, err := client.Get("https://httpbin.org/cookies")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n", resp3.StatusCode)
	fmt.Printf("Response: %s\n\n", resp3.Body)
}

// demonstrateAdvancedCookies shows advanced cookie scenarios
func demonstrateAdvancedCookies() {
	fmt.Println("--- Example 4: Advanced Cookie Scenarios ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Scenario 1: Session management
	fmt.Println("Scenario 1: Session Management")
	sessionCookie := &http.Cookie{
		Name:     "session_token",
		Value:    "secure_token_12345",
		Path:     "/",
		MaxAge:   3600, // 1 hour
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	resp, err := client.Post("https://httpbin.org/post",
		httpc.WithCookie(sessionCookie),
		httpc.WithJSON(map[string]string{"action": "login"}),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n", resp.StatusCode)

	// Scenario 2: Multiple cookies for different purposes
	fmt.Println("\nScenario 2: Multiple Purpose Cookies")
	cookies := []*http.Cookie{
		{
			Name:     "auth",
			Value:    "bearer_token",
			Path:     "/api",
			Secure:   true,
			HttpOnly: true,
		},
		{
			Name:   "preferences",
			Value:  "theme_dark_lang_en", // Use underscore or other separator, not semicolon
			Path:   "/",
			MaxAge: 86400 * 30, // 30 days
		},
		{
			Name:   "analytics",
			Value:  "tracking_id_123",
			Path:   "/",
			MaxAge: 86400 * 365, // 1 year
		},
	}

	resp, err = client.Get("https://httpbin.org/cookies",
		httpc.WithCookies(cookies),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n\n", resp.Body)

	// Scenario 3: Combining cookies with other options
	fmt.Println("Scenario 3: Cookies with Authentication")
	resp, err = client.Get("https://httpbin.org/cookies",
		httpc.WithCookieValue("session", "active"),
		httpc.WithBearerToken("jwt_token_here"),
		httpc.WithHeader("X-Request-ID", "12345"),
		httpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", resp.Body)
}

func main() {
	demonstrateCookies()
}
