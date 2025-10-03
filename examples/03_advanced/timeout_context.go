package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Testing WithContext and WithTimeout Conflict Resolution ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Test 1: Only WithTimeout
	testOnlyWithTimeout(client)

	// Test 2: Only WithContext
	testOnlyWithContext(client)

	// Test 3: Both WithContext (with deadline) and WithTimeout
	testBothWithDeadline(client)

	// Test 4: Both WithContext (without deadline) and WithTimeout
	testBothWithoutDeadline(client)

	// Test 5: WithContext with shorter deadline than WithTimeout
	testContextShorterDeadline(client)

	// Test 6: WithContext with longer deadline than WithTimeout
	testContextLongerDeadline(client)

}

func testOnlyWithTimeout(client httpc.Client) {
	fmt.Println("--- Test 1: Only WithTimeout ---")
	fmt.Println("Expected: Request uses 10s timeout from WithTimeout")

	start := time.Now()
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithTimeout(10*time.Second),
	)
	duration := time.Since(start)

	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("✓ Success: Status %d, Duration: %v\n", resp.StatusCode, duration)
	}
	fmt.Println()
}

func testOnlyWithContext(client httpc.Client) {
	fmt.Println("--- Test 2: Only WithContext (with 10s timeout) ---")
	fmt.Println("Expected: Request uses 10s timeout from context")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
	)
	duration := time.Since(start)

	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("✓ Success: Status %d, Duration: %v\n", resp.StatusCode, duration)
	}
	fmt.Println()
}

func testBothWithDeadline(client httpc.Client) {
	fmt.Println("--- Test 3: Both WithContext (10s) and WithTimeout (5s) ---")
	fmt.Println("Expected: Context deadline takes precedence, WithTimeout is ignored")
	fmt.Println("Result: Request should complete successfully (not timeout)")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
		httpc.WithTimeout(5*time.Second), // This should be ignored
	)
	duration := time.Since(start)

	if err != nil {
		log.Printf("✗ Error: %v (Duration: %v)\n", err, duration)
		fmt.Println("  Note: If this timed out at ~5s, WithTimeout was NOT ignored (bug)")
	} else {
		fmt.Printf("✓ Success: Status %d, Duration: %v\n", resp.StatusCode, duration)
		fmt.Println("  ✓ Context deadline took precedence, no conflict!")
	}
	fmt.Println()
}

func testBothWithoutDeadline(client httpc.Client) {
	fmt.Println("--- Test 4: Both WithContext (no deadline) and WithTimeout (10s) ---")
	fmt.Println("Expected: WithTimeout is applied since context has no deadline")

	ctx := context.Background()

	start := time.Now()
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
		httpc.WithTimeout(10*time.Second),
	)
	duration := time.Since(start)

	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("✓ Success: Status %d, Duration: %v\n", resp.StatusCode, duration)
		fmt.Println("  ✓ WithTimeout was applied correctly!")
	}
	fmt.Println()
}

func testContextShorterDeadline(client httpc.Client) {
	fmt.Println("--- Test 5: Context (5s) shorter than WithTimeout (30s) ---")
	fmt.Println("Expected: Context deadline takes precedence")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
		httpc.WithTimeout(30*time.Second), // Longer timeout, should be ignored
	)
	duration := time.Since(start)

	if err != nil {
		log.Printf("Error: %v (Duration: %v)\n", err, duration)
	} else {
		fmt.Printf("✓ Success: Status %d, Duration: %v\n", resp.StatusCode, duration)
		fmt.Println("  ✓ Context's shorter deadline was respected!")
	}
	fmt.Println()
}

func testContextLongerDeadline(client httpc.Client) {
	fmt.Println("--- Test 6: Context (30s) longer than WithTimeout (5s) ---")
	fmt.Println("Expected: Context deadline takes precedence, WithTimeout ignored")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithContext(ctx),
		httpc.WithTimeout(5*time.Second), // Shorter timeout, should be ignored
	)
	duration := time.Since(start)

	if err != nil {
		log.Printf("✗ Error: %v (Duration: %v)\n", err, duration)
		if duration < 10*time.Second {
			fmt.Println("  ✗ BUG: WithTimeout was NOT ignored!")
		}
	} else {
		fmt.Printf("✓ Success: Status %d, Duration: %v\n", resp.StatusCode, duration)
		fmt.Println("  ✓ Context's longer deadline was respected, WithTimeout ignored!")
	}
	fmt.Println()
}
