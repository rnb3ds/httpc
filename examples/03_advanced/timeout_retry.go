//go:build examples

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Timeout and Retry Examples ===\n ")

	// Example 1: Basic timeout
	demonstrateBasicTimeout()

	// Example 2: Context with timeout
	demonstrateContextTimeout()

	// Example 3: Retry configuration
	demonstrateRetry()

	// Example 4: Combined timeout and retry
	demonstrateCombined()

	// Example 5: Disable retries
	demonstrateNoRetry()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateBasicTimeout shows basic timeout usage
func demonstrateBasicTimeout() {
	fmt.Println("--- Example 1: Basic Timeout ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Set a 10-second timeout for this request
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Duration: %v\n", resp.Duration)
	fmt.Printf("Timeout was set to: 10s\n\n")
}

// demonstrateContextTimeout shows context-based timeout
func demonstrateContextTimeout() {
	fmt.Println("--- Example 2: Context with Timeout ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Duration: %v\n", resp.Duration)
	fmt.Printf("Context timeout was: 5s\n\n")
}

// demonstrateRetry shows retry configuration
func demonstrateRetry() {
	fmt.Println("--- Example 3: Retry Configuration ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Configure retry behavior
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithMaxRetries(3),
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Attempts: %d\n", resp.Attempts)
	fmt.Printf("Max Retries: 3\n")
	fmt.Printf("Duration: %v\n\n", resp.Duration)
}

// demonstrateCombined shows combined timeout and retry
func demonstrateCombined() {
	fmt.Println("--- Example 4: Combined Timeout and Retry ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Combine timeout and retry for resilient requests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"data": "important"}),
		httpc.WithContext(ctx),
		httpc.WithTimeout(10*time.Second),
		httpc.WithMaxRetries(3),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Attempts: %d\n", resp.Attempts)
	fmt.Printf("Duration: %v\n", resp.Duration)
	fmt.Printf("Configuration:\n")
	fmt.Printf("  - Context timeout: 30s\n")
	fmt.Printf("  - Request timeout: 10s\n")
	fmt.Printf("  - Max retries: 3\n\n")
}

// demonstrateNoRetry shows how to disable retries
func demonstrateNoRetry() {
	fmt.Println("--- Example 5: Disable Retries ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Disable retries for idempotent operations
	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"action": "create"}),
		httpc.WithMaxRetries(0), // No retries
		httpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Attempts: %d (no retries)\n", resp.Attempts)
	fmt.Printf("Duration: %v\n\n", resp.Duration)
}
