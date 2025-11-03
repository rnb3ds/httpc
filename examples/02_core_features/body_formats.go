//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
	"github.com/cybergodev/httpc/examples/02_core_features/types"
)

func main() {
	fmt.Println("=== Request Body Formats Examples ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: JSON Body
	demonstrateJSON(client)

	// Example 2: Form Data (application/x-www-form-urlencoded)
	demonstrateForm(client)

	// Example 3: Plain Text
	demonstrateText(client)

	// Example 4: XML Body
	demonstrateXML(client)

	// Example 5: Binary Data
	demonstrateBinary(client)

	// Example 6: Raw Body
	demonstrateRawBody(client)

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateJSON shows JSON request body handling
func demonstrateJSON(client httpc.Client) {
	fmt.Println("--- Example 1: JSON Body ---")

	// Using a struct
	user := types.User{
		Name:  "John Doe",
		Email: "john@example.com",
		Age:   30,
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(user),
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

	fmt.Printf("Status: %d\n", resp.StatusCode)

	// Request header (received by the server)
	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])

	// Response header (returned by the server)
	fmt.Printf("Content-Type: %s\n", resp.Headers["Content-Type"])
	fmt.Printf("Content-Type: %s\n", resp.Headers.Get("content-type")) // recommend case-insensitive

	fmt.Printf("Request Data: %s\n\n", result.Data)

	// Using a map
	data := map[string]any{
		"title":     "New Article",
		"content":   "Article content here",
		"published": true,
		"tags":      []string{"go", "http", "api"},
	}

	resp, err = client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(data),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Map data sent successfully, Status: %d\n\n", resp.StatusCode)
}

// demonstrateForm shows form data handling
func demonstrateForm(client httpc.Client) {
	fmt.Println("--- Example 2: Form Data (application/x-www-form-urlencoded) ---")

	formData := map[string]string{
		"username": "johndoe",
		"password": "secret123",
		"remember": "true",
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithForm(formData),
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

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])
	fmt.Printf("Form Data: %s\n\n", result.Data)
}

// demonstrateText shows plain text body handling
func demonstrateText(client httpc.Client) {
	fmt.Println("--- Example 3: Plain Text Body ---")

	text := "Hello, this is a plain text message!\nIt can contain multiple lines."

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithText(text),
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

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])
	fmt.Printf("Text Data: %s\n\n", result.Data)
}

// demonstrateXML shows XML body handling using WithXML
func demonstrateXML(client httpc.Client) {
	fmt.Println("--- Example 4: XML Body ---")

	// Using WithXML with a struct
	type Person struct {
		XMLName struct{} `xml:"person"`
		Name    string   `xml:"name"`
		Age     int      `xml:"age"`
		City    string   `xml:"city"`
	}

	person := Person{
		Name: "Jane Smith",
		Age:  28,
		City: "New York",
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithXML(person),
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

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])
	fmt.Printf("XML Data: %s\n\n", result.Data)

	// Alternative: Using WithBody and WithContentType for pre-formatted XML
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<person>
    <name>John Doe</name>
    <age>30</age>
    <city>Boston</city>
</person>`

	resp2, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithBody(xmlData),
		httpc.WithContentType("application/xml"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var result2 types.APIResponse
	if err := resp2.JSON(&result2); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp2.StatusCode)
	fmt.Printf("Pre-formatted XML Data: %s\n\n", result2.Data)
}

// demonstrateBinary shows binary data handling
func demonstrateBinary(client httpc.Client) {
	fmt.Println("--- Example 5: Binary Data ---")

	// Simulate binary data (e.g., PNG image header)
	binaryData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	// With explicit content type
	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithBinary(binaryData, "image/png"),
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

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])
	fmt.Printf("Binary Data Length: %d bytes\n\n", len(binaryData))

	// Without content type (defaults to application/octet-stream)
	resp, err = client.Post("https://echo.hoppscotch.io",
		httpc.WithBinary(binaryData),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Default Content-Type: %s\n\n", result.Headers["content-type"])
}

// demonstrateRawBody shows raw body handling
func demonstrateRawBody(client httpc.Client) {
	fmt.Println("--- Example 6: Raw Body with Custom Content-Type ---")

	// Custom data format
	customData := `{"custom": "format", "version": 2}`

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithBody(customData),
		httpc.WithContentType("application/vnd.api+json"),
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

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Content-Type: %s\n", result.Headers["content-type"])
	fmt.Printf("Custom Data: %s\n\n", result.Data)
}
