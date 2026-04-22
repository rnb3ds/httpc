//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

// This example demonstrates HTTP methods with focus on less common ones.
// For GET/POST/PUT/DELETE basics, see 01_quickstart/basic_usage.go

func main() {
	fmt.Println("=== HTTP Methods Examples ===")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 1. HEAD - Get headers only (no body)
	demonstrateHEAD(client)

	// 2. OPTIONS - Discover allowed methods
	demonstrateOPTIONS(client)

	// 3. PATCH - Partial update
	demonstratePATCH(client)

	// 4. Method comparison
	demonstrateMethodComparison()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateHEAD shows HEAD request usage
func demonstrateHEAD(client httpc.Client) {
	fmt.Println("--- Example 1: HEAD (Headers Only) ---")

	resp, err := client.Head("https://httpbin.org/get")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Content-Type: %s\n", resp.Response.Headers.Get("Content-Type"))
	fmt.Printf("Content-Length: %d\n", resp.Response.ContentLength)
	fmt.Printf("Body size: %d bytes (HEAD returns empty body)\n", len(resp.Body()))

	fmt.Println("\nUse cases:")
	fmt.Println("  - Check if resource exists (404 vs 200)")
	fmt.Println("  - Get file size before downloading")
	fmt.Println("  - Check Last-Modified for caching")
	fmt.Println("  - Verify resource metadata without transfer overhead")
}

// demonstrateOPTIONS shows OPTIONS request usage
func demonstrateOPTIONS(client httpc.Client) {
	fmt.Println("--- Example 2: OPTIONS (Allowed Methods) ---")

	resp, err := client.Options("https://httpbin.org/post")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	if allow := resp.Response.Headers.Get("Allow"); allow != "" {
		fmt.Printf("Allow: %s\n", allow)
	}
	if cors := resp.Response.Headers.Get("Access-Control-Allow-Methods"); cors != "" {
		fmt.Printf("CORS Methods: %s\n", cors)
	}

	fmt.Println("\nUse cases:")
	fmt.Println("  - CORS preflight requests")
	fmt.Println("  - Discover API capabilities")
	fmt.Println("  - Check allowed methods before making actual request")
}

// demonstratePATCH shows PATCH request usage
func demonstratePATCH(client httpc.Client) {
	fmt.Println("--- Example 3: PATCH (Partial Update) ---")

	// PATCH only updates specified fields
	partialUpdate := map[string]any{
		"status": "inactive",
	}

	resp, err := client.Patch("https://httpbin.org/patch",
		httpc.WithJSON(partialUpdate),
		httpc.WithBearerToken("your-token"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())

	// Parse response to see what was sent
	var result map[string]any
	if err := resp.Unmarshal(&result); err == nil {
		if json, ok := result["json"].(map[string]any); ok {
			fmt.Printf("Sent data: %+v\n", json)
		}
	}

	fmt.Println("\nPATCH vs PUT:")
	fmt.Println("  - PATCH: Update only specified fields")
	fmt.Println("  - PUT: Replace entire resource")
	fmt.Println("  - Use PATCH for partial updates to reduce payload")
}

// demonstrateMethodComparison shows method comparison table
func demonstrateMethodComparison() {
	fmt.Println("--- HTTP Methods Quick Reference ---")
	fmt.Println()
	fmt.Println("Method   | Body | Idempotent | Use Case")
	fmt.Println("---------|------|------------|----------------------------------")
	fmt.Println("GET      | No   | Yes        | Retrieve data")
	fmt.Println("HEAD     | No   | Yes        | Get headers only (no body)")
	fmt.Println("POST     | Yes  | No         | Create resource, submit data")
	fmt.Println("PUT      | Yes  | Yes        | Replace entire resource")
	fmt.Println("PATCH    | Yes  | No         | Partial update")
	fmt.Println("DELETE   | No   | Yes        | Remove resource")
	fmt.Println("OPTIONS  | No   | Yes        | Discover allowed methods")
	fmt.Println()
	fmt.Println("Idempotent: Multiple identical requests have same effect as single request")
	fmt.Println()
	fmt.Println("Basic examples (GET/POST/PUT/DELETE): See 01_quickstart/basic_usage.go")
}
