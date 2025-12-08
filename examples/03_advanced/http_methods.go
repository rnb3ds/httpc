//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== HTTP Methods Examples ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: GET - Retrieve data
	demonstrateGET(client)

	// Example 2: POST - Create new resource
	demonstratePOST(client)

	// Example 3: PUT - Update entire resource
	demonstratePUT(client)

	// Example 4: PATCH - Partial update
	demonstratePATCH(client)

	// Example 5: DELETE - Remove resource
	demonstrateDELETE(client)

	// Example 6: HEAD - Get headers only
	demonstrateHEAD(client)

	// Example 7: OPTIONS - Get allowed methods
	demonstrateOPTIONS(client)

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateGET shows GET request usage
func demonstrateGET(client httpc.Client) {
	fmt.Println("--- Example 1: GET (Retrieve Data) ---")

	resp, err := client.Get("https://echo.hoppscotch.io/users/123",
		httpc.WithQuery("include", "profile"),
		httpc.WithBearerToken("token"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Use case: Retrieve user data, list resources, search\n ")
}

// demonstratePOST shows POST request usage
func demonstratePOST(client httpc.Client) {
	fmt.Println("--- Example 2: POST (Create Resource) ---")

	newUser := map[string]any{
		"name":  "John Doe",
		"email": "john@example.com",
		"role":  "user",
	}

	resp, err := client.Post("https://echo.hoppscotch.io/users",
		httpc.WithJSON(newUser),
		httpc.WithBearerToken("token"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Use case: Create new user, submit form, upload data\n ")
}

// demonstratePUT shows PUT request usage
func demonstratePUT(client httpc.Client) {
	fmt.Println("--- Example 3: PUT (Replace Resource) ---")

	updatedUser := map[string]any{
		"id":     123,
		"name":   "John Doe Updated",
		"email":  "john.updated@example.com",
		"role":   "admin",
		"status": "active",
	}

	resp, err := client.Put("https://echo.hoppscotch.io/users/123",
		httpc.WithJSON(updatedUser),
		httpc.WithBearerToken("token"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Use case: Replace entire resource, full update")
	fmt.Println("Note: PUT replaces the entire resource\n ")
}

// demonstratePATCH shows PATCH request usage
func demonstratePATCH(client httpc.Client) {
	fmt.Println("--- Example 4: PATCH (Partial Update) ---")

	// Only update specific fields
	partialUpdate := map[string]any{
		"status": "inactive",
		"role":   "moderator",
	}

	resp, err := client.Patch("https://echo.hoppscotch.io/users/123",
		httpc.WithJSON(partialUpdate),
		httpc.WithBearerToken("token"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Use case: Update specific fields, partial modification")
	fmt.Println("Note: PATCH only updates specified fields\n ")
}

// demonstrateDELETE shows DELETE request usage
func demonstrateDELETE(client httpc.Client) {
	fmt.Println("--- Example 5: DELETE (Remove Resource) ---")

	resp, err := client.Delete("https://echo.hoppscotch.io/users/123",
		httpc.WithBearerToken("token"),
		httpc.WithHeader("X-Reason", "Account closed"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	if resp.IsSuccess() {
		fmt.Println("Resource deleted successfully")
	}
	fmt.Println("Use case: Delete user, remove resource, cancel subscription\n ")
}

// demonstrateHEAD shows HEAD request usage
func demonstrateHEAD(client httpc.Client) {
	fmt.Println("--- Example 6: HEAD (Get Headers Only) ---")

	resp, err := client.Head("https://echo.hoppscotch.io/large-file")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Content-Length: %d\n", resp.Response.ContentLength)
	fmt.Printf("Content-Type: %s\n", resp.Response.Headers.Get("Content-Type"))
	fmt.Printf("Last-Modified: %s\n", resp.Response.Headers.Get("Last-Modified"))
	fmt.Println("Use case: Check file size, verify resource exists, get metadata")
	fmt.Println("Note: HEAD returns headers only, no body\n ")
}

// demonstrateOPTIONS shows OPTIONS request usage
func demonstrateOPTIONS(client httpc.Client) {
	fmt.Println("--- Example 7: OPTIONS (Get Allowed Methods) ---")

	resp, err := client.Options("https://echo.hoppscotch.io/users")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Allow: %s\n", resp.Response.Headers.Get("Allow"))
	fmt.Printf("Access-Control-Allow-Methods: %s\n", resp.Response.Headers.Get("Access-Control-Allow-Methods"))
	fmt.Println("Use case: CORS preflight, discover allowed methods")
	fmt.Println("Note: OPTIONS returns allowed HTTP methods\n ")
}
