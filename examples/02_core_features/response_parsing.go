//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
	"github.com/cybergodev/httpc/examples/02_core_features/types"
)

func main() {
	fmt.Println("=== Response Parsing Examples ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: JSON response parsing
	demonstrateJSONParsing(client)

	// Example 2: XML response parsing
	demonstrateXMLParsing(client)

	// Example 3: Status code checking
	demonstrateStatusChecking(client)

	// Example 4: Header access
	demonstrateHeaderAccess(client)

	// Example 5: Response metadata
	demonstrateResponseMetadata(client)

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateJSONParsing shows JSON response parsing
func demonstrateJSONParsing(client httpc.Client) {
	fmt.Println("--- Example 1: JSON Response Parsing ---")

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithQuery("name", "Alice"),
		httpc.WithJSONAccept(),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Parse into struct
	var result types.APIResponse
	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Method: %s\n", result.Method)
	fmt.Printf("Query Args: %+v\n", result.Args)
	fmt.Printf("Headers: %d headers received\n\n", len(result.Headers))

	// Parse into map
	resp2, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"key": "value"}),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var mapResult map[string]any
	if err := resp2.JSON(&mapResult); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Parsed into map: %d keys\n\n", len(mapResult))
}

// demonstrateXMLParsing shows XML response parsing
func demonstrateXMLParsing(client httpc.Client) {
	fmt.Println("--- Example 2: XML Response Parsing ---")

	// Send XML request
	person := types.Person{
		Name: "John Doe",
		Age:  30,
		City: "New York",
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithXML(person),
		httpc.WithXMLAccept(),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Content-Type: %s\n", resp.Response.Headers.Get("Content-Type"))
	fmt.Printf("XML sent successfully\n\n")
}

// demonstrateStatusChecking shows status code checking methods
func demonstrateStatusChecking(client httpc.Client) {
	fmt.Println("--- Example 3: Status Code Checking ---")

	resp, err := client.Get("https://echo.hoppscotch.io")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status Code: %d\n", resp.StatusCode())
	fmt.Printf("Status Text: %s\n", resp.Response.Status)
	fmt.Printf("Is Success (2xx): %v\n", resp.IsSuccess())
	fmt.Printf("Is Redirect (3xx): %v\n", resp.IsRedirect())
	fmt.Printf("Is Client Error (4xx): %v\n", resp.IsClientError())
	fmt.Printf("Is Server Error (5xx): %v\n\n", resp.IsServerError())

	// Practical status checking
	if resp.IsSuccess() {
		fmt.Println("Request successful, safe to process response\n ")
	} else if resp.IsClientError() {
		fmt.Println("Client error - check request parameters\n ")
	} else if resp.IsServerError() {
		fmt.Println("Server error - retry may help\n ")
	}
}

// demonstrateHeaderAccess shows response header access
func demonstrateHeaderAccess(client httpc.Client) {
	fmt.Println("--- Example 4: Response Header Access ---")

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithHeader("X-Custom-Header", "test-value"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Method 1: Direct map access (case-sensitive)
	contentType := resp.Response.Headers["Content-Type"]
	fmt.Printf("Content-Type (direct): %s\n", contentType)

	// Method 2: Get method (case-insensitive, recommended)
	contentType2 := resp.Response.Headers.Get("content-type")
	fmt.Printf("Content-Type (Get): %s\n", contentType2)

	// Check if header exists
	if date := resp.Response.Headers.Get("Date"); date != "" {
		fmt.Printf("Date: %s\n", date)
	}

	// Iterate all headers
	fmt.Println("\nAll response headers:")
	for key, value := range resp.Response.Headers {
		fmt.Printf("  %s: %s\n", key, value)
	}
	fmt.Println()
}

// demonstrateResponseMetadata shows response metadata access
func demonstrateResponseMetadata(client httpc.Client) {
	fmt.Println("--- Example 5: Response Metadata ---")

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithQuery("test", "metadata"),
		httpc.WithMaxRetries(2),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status Code: %d\n", resp.StatusCode())
	fmt.Printf("Status: %s\n", resp.Response.Status)
	fmt.Printf("Content Length: %d bytes\n", resp.Response.ContentLength)
	fmt.Printf("Body Length: %d bytes\n", len(resp.Body()))
	fmt.Printf("Duration: %v\n", resp.Meta.Duration)
	fmt.Printf("Attempts: %d\n", resp.Meta.Attempts)
	fmt.Printf("Number of Headers: %d\n", len(resp.Response.Headers))

	// Access raw body ([]byte)
	fmt.Printf("Raw Body Length: %d bytes\n", len(resp.Response.RawBody))

	// Access body as string
	if len(resp.Body()) > 100 {
		fmt.Printf("Body Preview: %s...\n\n", resp.Body()[:100])
	} else {
		fmt.Printf("Body: %s\n\n", resp.Body())
	}
}
