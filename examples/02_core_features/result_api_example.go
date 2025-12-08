//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== HTTPC Result API Example ===\n")

	config := httpc.DefaultConfig()
	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	basicUsage(client)
	fmt.Println()
	clearSeparation(client)
	fmt.Println()
	convenienceMethods(client)
}

func basicUsage(client httpc.Client) {
	fmt.Println("1. Basic Usage with Result API")
	fmt.Println("--------------------------------")

	result, err := client.Get("https://httpbin.org/get",
		httpc.WithQuery("page", 1),
		httpc.WithHeader("X-Custom-Header", "example"),
	)

	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", result.StatusCode())
	fmt.Printf("Duration: %v\n", result.Meta.Duration)
	fmt.Printf("Attempts: %d\n", result.Meta.Attempts)
	fmt.Printf("Body length: %d bytes\n", len(result.Body()))
}

func clearSeparation(client httpc.Client) {
	fmt.Println("2. Clear Separation of Request/Response")
	fmt.Println("----------------------------------------")

	result, err := client.Post("https://httpbin.org/post",
		httpc.WithJSON(map[string]string{
			"name":  "John Doe",
			"email": "john@example.com",
		}),
		httpc.WithHeader("Authorization", "Bearer token123"),
	)

	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Println("Request Information:")
	fmt.Printf("  Headers: %d\n", len(result.Request.Headers))
	if authHeader := result.Request.Headers.Get("Authorization"); authHeader != "" {
		fmt.Printf("  Authorization: %s\n", authHeader)
	}

	fmt.Println("\nResponse Information:")
	fmt.Printf("  Status: %d %s\n", result.Response.StatusCode, result.Response.Status)
	fmt.Printf("  Content-Length: %d\n", result.Response.ContentLength)
	fmt.Printf("  Headers: %d\n", len(result.Response.Headers))

	fmt.Println("\nMetadata:")
	fmt.Printf("  Duration: %v\n", result.Meta.Duration)
	fmt.Printf("  Attempts: %d\n", result.Meta.Attempts)
}

func convenienceMethods(client httpc.Client) {
	fmt.Println("3. Convenience Methods")
	fmt.Println("----------------------")

	result, err := client.Get("https://httpbin.org/cookies/set?session=abc123&user=john")

	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Println("Status Checks:")
	fmt.Printf("  IsSuccess: %v\n", result.IsSuccess())
	fmt.Printf("  IsRedirect: %v\n", result.IsRedirect())
	fmt.Printf("  IsClientError: %v\n", result.IsClientError())
	fmt.Printf("  IsServerError: %v\n", result.IsServerError())

	fmt.Println("\nCookie Access:")
	if len(result.ResponseCookies()) > 0 {
		fmt.Printf("  Response cookies: %d\n", len(result.ResponseCookies()))
		for _, cookie := range result.ResponseCookies() {
			fmt.Printf("    - %s = %s\n", cookie.Name, cookie.Value)
		}
	}

	if sessionCookie := result.GetCookie("session"); sessionCookie != nil {
		fmt.Printf("  Session cookie value: %s\n", sessionCookie.Value)
	}

	fmt.Println("\nJSON Parsing:")
	var data map[string]interface{}
	if err := result.JSON(&data); err == nil {
		fmt.Printf("  Parsed JSON with %d keys\n", len(data))
	}
}
