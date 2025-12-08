//go:build examples

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/cybergodev/httpc"
)

func main() {
	// Example 1: Basic usage with automatic cookie management
	fmt.Println("=== Example 1: Automatic Cookie Management ===")
	basicCookieExample()

	fmt.Println("\n=== Example 2: Automatic Header Management ===")
	basicHeaderExample()

	fmt.Println("\n=== Example 3: Real-World Login Scenario ===")
	loginScenarioExample()

	fmt.Println("\n=== Example 4: Manual Cookie and Header Management ===")
	manualManagementExample()
}

func basicCookieExample() {
	// Create a domain client for example
	client, err := httpc.NewDomain("https://www.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// First request - server may set cookies
	resp1, err := client.Get("/")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("First request: Status=%d, Cookies received=%d\n",
		resp1.StatusCode(), len(resp1.Response.Cookies))

	// Second request - cookies are automatically sent
	resp2, err := client.Get("/search?q=golang")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Second request: Status=%d (cookies automatically sent)\n",
		resp2.StatusCode())
}

func basicHeaderExample() {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Set persistent headers
	err = client.SetHeader("User-Agent", "MyApp/1.0")
	if err != nil {
		log.Fatal(err)
	}
	err = client.SetHeader("Accept", "application/json")
	if err != nil {
		log.Fatal(err)
	}

	// All subsequent requests will include these headers
	fmt.Println("Persistent headers set: User-Agent, Accept")

	// You can override headers per-request
	_, err = client.Get("/api/data",
		httpc.WithHeader("Accept", "application/xml"), // Override
	)
	if err != nil {
		log.Printf("Request with override: %v\n", err)
	}
}

func loginScenarioExample() {
	// Simulate a typical login flow
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Step 1: Login (server sets session cookie)
	fmt.Println("Step 1: Logging in...")
	loginResp, err := client.Post("/auth/login",
		httpc.WithJSON(map[string]string{
			"username": "user@example.com",
			"password": "secret123",
		}),
	)
	if err != nil {
		log.Printf("Login failed: %v\n", err)
		return
	}

	// Extract token from response
	var loginData map[string]string
	if err := loginResp.JSON(&loginData); err != nil {
		log.Printf("Failed to parse login response: %v\n", err)
		return
	}

	fmt.Printf("Login successful! Cookies received: %d\n", len(loginResp.Response.Cookies))

	// Step 2: Set authorization header for subsequent requests
	if token, ok := loginData["token"]; ok {
		err = client.SetHeader("Authorization", "Bearer "+token)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Authorization header set")
	}

	// Step 3: Make API calls (cookies and auth header automatically sent)
	fmt.Println("Step 2: Fetching user profile...")
	profileResp, err := client.Get("/api/user/profile")
	if err != nil {
		log.Printf("Failed to fetch profile: %v\n", err)
		return
	}
	fmt.Printf("Profile fetched: Status=%d\n", profileResp.StatusCode())

	// Step 4: Make another API call (still using same cookies and headers)
	fmt.Println("Step 3: Fetching user data...")
	dataResp, err := client.Get("/api/user/data")
	if err != nil {
		log.Printf("Failed to fetch data: %v\n", err)
		return
	}
	fmt.Printf("Data fetched: Status=%d\n", dataResp.StatusCode())
}

func manualManagementExample() {
	client, err := httpc.NewDomain("https://api.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Manually set cookies
	err = client.SetCookie(&http.Cookie{
		Name:  "session_id",
		Value: "abc123xyz",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Set multiple cookies at once
	err = client.SetCookies([]*http.Cookie{
		{Name: "user_pref", Value: "dark_mode"},
		{Name: "lang", Value: "en"},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Cookies set: %d\n", len(client.GetCookies()))

	// Set multiple headers
	err = client.SetHeaders(map[string]string{
		"X-API-Key":    "your-api-key",
		"X-Request-ID": "req-12345",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Headers set: %d\n", len(client.GetHeaders()))

	// Make request with managed state
	_, err = client.Get("/api/endpoint")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
	}

	// Get specific cookie
	sessionCookie := client.GetCookie("session_id")
	if sessionCookie != nil {
		fmt.Printf("Session cookie: %s=%s\n", sessionCookie.Name, sessionCookie.Value)
	}

	// Delete a cookie
	client.DeleteCookie("lang")
	fmt.Printf("Cookies after delete: %d\n", len(client.GetCookies()))

	// Clear all cookies
	client.ClearCookies()
	fmt.Printf("Cookies after clear: %d\n", len(client.GetCookies()))

	// Clear all headers
	client.ClearHeaders()
	fmt.Printf("Headers after clear: %d\n", len(client.GetHeaders()))
}
