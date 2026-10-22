//go:build examples

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

// This example demonstrates timeout and retry control: per-request
// WithTimeout, client-level retry budgets with exponential backoff and
// jitter, and lifting the timeout for one slow call.
// Perfect for: Calling slow or flaky upstreams without hangs

func main() {
	fmt.Println("=== Timeout and Retry Examples ===")

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

	// Example 6: Custom retry configuration
	demonstrateCustomRetryConfig()

	// Example 7: Lift the timeout for one slow call
	demonstrateNoTimeout()

	// Example 8: Retry non-idempotent requests (opt-in)
	demonstrateNonIdempotentRetry()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateBasicTimeout shows basic timeout usage
func demonstrateBasicTimeout() {
	fmt.Println("--- Example 1: Basic Timeout ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Set a 10-second timeout for this request
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Duration: %v\n", resp.Meta.Duration)
	fmt.Printf("Timeout was set to: 10s\n\n")
}

// demonstrateContextTimeout shows context-based timeout
func demonstrateContextTimeout() {
	fmt.Println("--- Example 2: Context with Timeout ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

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

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Duration: %v\n", resp.Meta.Duration)
	fmt.Printf("Context timeout was: 5s\n\n")
}

// demonstrateRetry shows retry configuration
func demonstrateRetry() {
	fmt.Println("--- Example 3: Retry Configuration ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Configure retry behavior
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithMaxRetries(3),
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Attempts: %d\n", resp.Meta.Attempts)
	fmt.Printf("Max Retries: 3\n")
	fmt.Printf("Duration: %v\n\n", resp.Meta.Duration)
}

// demonstrateCombined shows combined timeout and retry
func demonstrateCombined() {
	fmt.Println("--- Example 4: Combined Timeout and Retry ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

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

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Attempts: %d\n", resp.Meta.Attempts)
	fmt.Printf("Duration: %v\n", resp.Meta.Duration)
	fmt.Printf("Configuration:\n")
	fmt.Printf("  - Context timeout: 30s\n")
	fmt.Printf("  - Request timeout: 10s\n")
	fmt.Printf("  - Max retries: 3\n\n")
}

// demonstrateNoRetry shows how to disable retries
func demonstrateNoRetry() {
	fmt.Println("--- Example 5: Disable Retries ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

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

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Attempts: %d (no retries)\n", resp.Meta.Attempts)
	fmt.Printf("Duration: %v\n\n", resp.Meta.Duration)
}

// demonstrateCustomRetryConfig shows retry configuration tuning
func demonstrateCustomRetryConfig() {
	fmt.Println("--- Example 6: Custom Retry Configuration ---")

	// Fine-tune retry behavior via config fields
	config := httpc.DefaultConfig()
	config.Retry.MaxRetries = 5
	config.Retry.Delay = 200 * time.Millisecond
	config.Retry.BackoffFactor = 2.0
	config.Retry.EnableJitter = true

	client, err := httpc.New(config)
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithTimeout(15*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Attempts: %d\n", resp.Meta.Attempts)
	fmt.Printf("Duration: %v\n\n", resp.Meta.Duration)

	fmt.Println("Custom retry settings:")
	fmt.Println("  - MaxRetries: 5")
	fmt.Println("  - Base delay: 200ms")
	fmt.Println("  - Backoff factor: 2.0 (200ms, 400ms, 800ms, 1.6s, 3.2s)")
	fmt.Println("  - Jitter enabled: prevents retry storms")
}

// demonstrateNoTimeout shows lifting the client's default timeout for one call.
// By default every request is bounded by Timeouts.Request (180s). Two escapes:
//
//   - WithTimeout(d): an explicit deadline for one call (longer or shorter)
//   - WithNoTimeout(): remove the deadline entirely for one call; bound the
//     call with an outer context instead if needed
//
// Essential for AI APIs and long-polling endpoints that may take minutes to
// produce a first byte.
func demonstrateNoTimeout() {
	fmt.Println("--- Example 7: Lift Timeout for One Slow Call ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// An AI-style call that may take minutes: give it an explicit long
	// deadline without touching the client's own 180s default.
	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"prompt": "Write a haiku about HTTP."}),
		httpc.WithTimeout(900*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d (explicit 900s deadline for this call)\n", resp.StatusCode())

	// Long-polling style call with no client deadline at all: the context
	// alone decides when to give up.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err = client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
		httpc.WithNoTimeout(), // lift the client default for this call only
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Status: %d (no client deadline; context controls cancellation)\n\n", resp.StatusCode())
}

// demonstrateNonIdempotentRetry shows opt-in retries for POST/PATCH. By
// default, non-idempotent requests execute exactly once: a failure after the
// server may have received the body is surfaced, not retried, because a retry
// could duplicate the operation. WithRetryNonIdempotent(true) opts in to
// retries — use it only on replay-safe endpoints, ideally with an
// idempotency key the server can deduplicate on.
func demonstrateNonIdempotentRetry() {
	fmt.Println("--- Example 8: Retry Non-Idempotent Requests (opt-in) ---")

	client, err := httpc.NewDefault()
	if err != nil {
		log.Printf("Failed to create client: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// POST is retried only because WithRetryNonIdempotent(true) is present.
	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithJSON(map[string]string{"action": "process"}),
		httpc.WithHeader("Idempotency-Key", "req-12345"),
		httpc.WithRetryNonIdempotent(true), // opt in to POST retries
		httpc.WithMaxRetries(2),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d, Attempts: %d\n", resp.StatusCode(), resp.Meta.Attempts)
	fmt.Println("Without the opt-in, POST/PATCH run exactly once even on retryable")
	fmt.Println("network errors — duplicate side effects are never risked silently.")
}
