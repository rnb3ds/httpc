//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
	"github.com/cybergodev/httpc/examples/02_core_features/types"
)

// demonstrateHeaders shows various ways to set request headers
func demonstrateHeaders() {
	fmt.Println("=== Headers Examples ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: Single header
	fmt.Println("--- Single Header ---")
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithHeader("X-Custom-Header", "CustomValue"),
		httpc.WithHeader("X-Request-ID", "12345"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var result types.APIResponse
	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Custom headers sent:\n")
	fmt.Printf("  X-Custom-Header: %s\n", result.Headers["x-custom-header"])
	fmt.Printf("  X-Request-ID: %s\n\n", result.Headers["x-request-id"])

	// Example 2: Multiple headers using map
	fmt.Println("--- Multiple Headers (Map) ---")
	headers := map[string]string{
		"X-API-Version": "v1",
		"X-Client-ID":   "client-123",
		"Accept":        "application/json",
	}

	resp, err = client.Get("https://echo.hoppscotch.io",
		httpc.WithHeaderMap(headers),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Headers sent: %+v\n\n", headers)

	// Example 3: User-Agent
	fmt.Println("--- Custom User-Agent ---")
	resp, err = client.Get("https://echo.hoppscotch.io",
		httpc.WithUserAgent("MyApp/1.0 (httpc)"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("User-Agent: %s\n\n", result.Headers["user-agent"])

	// Example 4: Content-Type and Accept headers
	fmt.Println("--- Content-Type and Accept ---")
	resp, err = client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"key": "value"}),
		httpc.WithJSONAccept(), // Sets Accept: application/json
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])
	fmt.Printf("Accept: %s\n\n", result.Headers["accept"])
}

// demonstrateAuthentication shows various authentication methods
func demonstrateAuthentication() {
	fmt.Println("=== Authentication Examples ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: Bearer Token (JWT)
	fmt.Println("--- Bearer Token Authentication ---")
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithBearerToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.example"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var result types.APIResponse
	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Authorization Header: %s\n\n", result.Headers["authorization"])

	// Example 2: Basic Authentication
	fmt.Println("--- Basic Authentication ---")
	resp, err = client.Get("https://echo.hoppscotch.io",
		httpc.WithBasicAuth("username", "password"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Authorization Header: %s\n", result.Headers["authorization"])
	fmt.Println("(Base64 encoded username:password)\n ")

	// Example 3: API Key in Header
	fmt.Println("--- API Key Authentication ---")
	resp, err = client.Get("https://echo.hoppscotch.io",
		httpc.WithHeader("X-API-Key", "your-api-key-here"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("X-API-Key: %s\n\n", result.Headers["x-api-key"])

	// Example 4: Combined - Auth + Custom Headers
	fmt.Println("--- Combined Authentication and Headers ---")
	resp, err = client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"action": "create"}),
		httpc.WithBearerToken("your-jwt-token"),
		httpc.WithHeader("X-Request-ID", "req-12345"),
		httpc.WithHeader("X-Idempotency-Key", "idem-67890"),
		httpc.WithUserAgent("MyApp/2.0"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Authorization: %s\n", result.Headers["authorization"])
	fmt.Printf("X-Request-ID: %s\n", result.Headers["x-request-id"])
	fmt.Printf("X-Idempotency-Key: %s\n", result.Headers["x-idempotency-key"])
	fmt.Printf("User-Agent: %s\n\n", result.Headers["user-agent"])
}

// demonstrateRealWorldAuth shows practical authentication patterns
func demonstrateRealWorldAuth() {
	fmt.Println("=== Real-World Authentication Patterns ===\n ")

	// Pattern 1: JWT Token from environment or config
	fmt.Println("--- Pattern 1: JWT Token from Config ---")
	token := getTokenFromConfig() // In real app, load from env or config
	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io/api/protected",
		httpc.WithBearerToken(token),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n\n", resp.StatusCode())

	// Pattern 2: API Key for third-party services
	fmt.Println("--- Pattern 2: Third-Party API Key ---")
	apiKey := getAPIKeyFromEnv() // In real app, load from environment
	resp, err = client.Get("https://echo.hoppscotch.io/api/data",
		httpc.WithHeader("X-API-Key", apiKey),
		httpc.WithHeader("X-Client-Version", "1.0.0"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n\n", resp.StatusCode())

	// Pattern 3: Basic Auth for internal services
	fmt.Println("--- Pattern 3: Internal Service Auth ---")
	username, password := getServiceCredentials()
	resp, err = client.Get("https://echo.hoppscotch.io/internal/metrics",
		httpc.WithBasicAuth(username, password),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d\n\n", resp.StatusCode())
}

// Helper functions (in real app, these would load from config/env)
func getTokenFromConfig() string {
	return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.example.token"
}

func getAPIKeyFromEnv() string {
	return "sk_live_1234567890abcdef"
}

func getServiceCredentials() (string, string) {
	return "service-user", "service-password"
}

func main() {
	fmt.Println("=== Headers and Authentication Examples ===\n ")
	demonstrateHeaders()
	demonstrateAuthentication()
	demonstrateRealWorldAuth()

	fmt.Println("=== All Examples Completed ===")
}
