package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Error Handling Examples ===\n ")

	// Example 1: Basic error handling
	demonstrateBasicErrors()

	// Example 2: HTTP status errors
	demonstrateHTTPErrors()

	// Example 3: Timeout errors
	demonstrateTimeoutErrors()

	// Example 4: Context cancellation
	demonstrateContextCancellation()

	// Example 5: Parsing errors
	demonstrateParsingErrors()

	// Example 6: Comprehensive error handling pattern
	demonstrateComprehensivePattern()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateBasicErrors shows basic error handling
func demonstrateBasicErrors() {
	fmt.Println("--- Example 1: Basic Error Handling ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io")
	if err != nil {
		// Network error, DNS error, etc.
		log.Printf("Request failed: %v\n", err)
		return
	}

	// Check if request was successful
	if !resp.IsSuccess() {
		log.Printf("HTTP error: %d - %s\n", resp.StatusCode, resp.Status)
		return
	}

	fmt.Printf("✓ Request successful: %d\n\n", resp.StatusCode)
}

// demonstrateHTTPErrors shows HTTP status code error handling
func demonstrateHTTPErrors() {
	fmt.Println("--- Example 2: HTTP Status Errors ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate different status codes
	testURL := "https://echo.hoppscotch.io"

	resp, err := client.Get(testURL)
	if err != nil {
		log.Printf("Request error: %v\n", err)
		return
	}

	// Detailed status code checking
	switch {
	case resp.IsSuccess():
		// 2xx - Success
		fmt.Println("✓ Success (2xx)")
	case resp.IsRedirect():
		// 3xx - Redirection
		fmt.Printf("→ Redirect (3xx): %s\n", resp.Headers.Get("Location"))
	case resp.IsClientError():
		// 4xx - Client error
		fmt.Printf("✗ Client error (4xx): %d\n", resp.StatusCode)
		switch resp.StatusCode {
		case 400:
			fmt.Println("  Bad Request")
		case 401:
			fmt.Println("  Unauthorized - check authentication")
		case 403:
			fmt.Println("  Forbidden - insufficient permissions")
		case 404:
			fmt.Println("  Not Found")
		case 429:
			fmt.Println("  Too Many Requests - rate limited")
		}
	case resp.IsServerError():
		// 5xx - Server error
		fmt.Printf("✗ Server error (5xx): %d\n", resp.StatusCode)
		fmt.Println("  Server is experiencing issues, retry may help")
	}
	fmt.Println()
}

// demonstrateTimeoutErrors shows timeout error handling
func demonstrateTimeoutErrors() {
	fmt.Println("--- Example 3: Timeout Errors ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Very short timeout to demonstrate timeout handling
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithTimeout(1*time.Nanosecond), // Intentionally too short
	)
	if err != nil {
		// Check if it's a timeout error
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Println("✗ Request timed out")
			fmt.Println("  Consider increasing timeout or checking network")
		} else {
			fmt.Printf("✗ Request failed: %v\n", err)
		}
		fmt.Println()
		return
	}

	fmt.Printf("✓ Request completed: %d\n\n", resp.StatusCode)
}

// demonstrateContextCancellation shows context cancellation handling
func demonstrateContextCancellation() {
	fmt.Println("--- Example 4: Context Cancellation ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately to demonstrate cancellation
	cancel()

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
	)
	if err != nil {
		// Check if it's a cancellation error
		if errors.Is(err, context.Canceled) {
			fmt.Println("✗ Request was cancelled")
			fmt.Println("  This is expected when context is cancelled")
		} else {
			fmt.Printf("✗ Request failed: %v\n", err)
		}
		fmt.Println()
		return
	}

	fmt.Printf("✓ Request completed: %d\n\n", resp.StatusCode)
}

// demonstrateParsingErrors shows JSON/XML parsing error handling
func demonstrateParsingErrors() {
	fmt.Println("--- Example 5: Parsing Errors ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	// Try to parse as JSON
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	var user User
	if err := resp.JSON(&user); err != nil {
		fmt.Println("✗ Failed to parse JSON response")
		fmt.Printf("  Error: %v\n", err)
		fmt.Println("  Tip: Check if response is valid JSON")
		fmt.Println("  Tip: Verify struct tags match response fields")
		fmt.Println()
		return
	}

	fmt.Printf("✓ Successfully parsed: %+v\n\n", user)
}

// demonstrateComprehensivePattern shows comprehensive error handling
func demonstrateComprehensivePattern() {
	fmt.Println("=== Comprehensive Error Handling Pattern ===\n ")

	result, err := fetchUserData(123)
	if err != nil {
		log.Printf("Failed to fetch user data: %v\n", err)
		return
	}

	fmt.Printf("✓ Successfully fetched user data: %+v\n\n", result)
}

// fetchUserData demonstrates a function with comprehensive error handling
func fetchUserData(userID int) (map[string]interface{}, error) {
	client, err := httpc.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	defer client.Close()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Make request
	url := fmt.Sprintf("https://echo.hoppscotch.io/api/users/%d", userID)
	resp, err := client.Get(url,
		httpc.WithContext(ctx),
		httpc.WithBearerToken("your-token"),
		httpc.WithJSONAccept(),
		httpc.WithMaxRetries(2),
	)
	if err != nil {
		// Check specific error types
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("request timed out after 10s: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("request was cancelled: %w", err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check HTTP status
	if !resp.IsSuccess() {
		switch {
		case resp.StatusCode == 404:
			return nil, fmt.Errorf("user %d not found", userID)
		case resp.StatusCode == 401:
			return nil, fmt.Errorf("authentication failed")
		case resp.StatusCode == 403:
			return nil, fmt.Errorf("access denied")
		case resp.StatusCode == 429:
			return nil, fmt.Errorf("rate limit exceeded, retry after: %s",
				resp.Headers.Get("Retry-After"))
		case resp.IsClientError():
			return nil, fmt.Errorf("client error: %d - %s", resp.StatusCode, resp.Status)
		case resp.IsServerError():
			return nil, fmt.Errorf("server error: %d - %s", resp.StatusCode, resp.Status)
		default:
			return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
		}
	}

	// Parse response
	var result map[string]interface{}
	if err := resp.JSON(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}
