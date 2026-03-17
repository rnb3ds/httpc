//go:build examples

package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/cybergodev/httpc"
)

// This example demonstrates advanced patterns and optimization techniques

func main() {
	fmt.Println("=== Advanced Patterns & Optimization ===\n")

	// 1. Request/Response Callbacks
	demonstrateCallbacks()

	// 2. Result Pool Optimization
	demonstrateResultPool()

	// 3. Testing Configuration
	demonstrateTestingConfig()

	// 4. Default Client Management
	demonstrateDefaultClient()

	// 5. Memory Stats Comparison
	demonstrateMemoryOptimization()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateCallbacks shows WithOnRequest and WithOnResponse callbacks
func demonstrateCallbacks() {
	fmt.Println("--- Example 1: Request/Response Callbacks ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Request callback - runs before request is sent
	onRequest := func(req httpc.RequestMutator) error {
		fmt.Printf("  [REQUEST] %s %s\n", req.Method(), req.URL())
		fmt.Printf("  [REQUEST] Headers: %d\n", len(req.Headers()))
		return nil
	}

	// Response callback - runs after response is received
	onResponse := func(resp httpc.ResponseMutator) error {
		fmt.Printf("  [RESPONSE] Status: %d %s\n", resp.StatusCode(), resp.Status())
		fmt.Printf("  [RESPONSE] Duration: %v\n", resp.Duration())
		fmt.Printf("  [RESPONSE] Attempts: %d\n", resp.Attempts())
		return nil
	}

	// Make request with callbacks
	resp, err := client.Get("https://httpbin.org/get",
		httpc.WithOnRequest(onRequest),
		httpc.WithOnResponse(onResponse),
		httpc.WithQuery("test", "callbacks"),
	)
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("\nStatus: %d\n\n", resp.StatusCode())

	fmt.Println("Use cases for callbacks:")
	fmt.Println("  - Request logging and debugging")
	fmt.Println("  - Response validation before processing")
	fmt.Println("  - Custom metrics collection")
	fmt.Println("  - Request modification on-the-fly\n")
}

// demonstrateResultPool shows result pool optimization
func demonstrateResultPool() {
	fmt.Println("--- Example 2: Result Pool Optimization ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	const numRequests = 10
	start := time.Now()

	// Without pool - each Result is garbage collected
	fmt.Println("Without pool optimization:")
	for i := 0; i < numRequests; i++ {
		resp, err := client.Get("https://httpbin.org/get")
		if err != nil {
			continue
		}
		_ = resp.Body() // Use the result
		// Result will be garbage collected
	}
	fmt.Printf("  %d requests completed in %v\n", numRequests, time.Since(start))

	// With pool - Results are reused
	start = time.Now()
	fmt.Println("\nWith pool optimization (ReleaseResult):")
	for i := 0; i < numRequests; i++ {
		resp, err := client.Get("https://httpbin.org/get")
		if err != nil {
			continue
		}
		_ = resp.Body()           // Use the result
		httpc.ReleaseResult(resp) // Return to pool for reuse
	}
	fmt.Printf("  %d requests completed in %v\n", numRequests, time.Since(start))

	fmt.Println("\nWhen to use ReleaseResult:")
	fmt.Println("  - High-throughput applications (1000+ requests/sec)")
	fmt.Println("  - Memory-constrained environments")
	fmt.Println("  - Long-running services")
	fmt.Println("\nWARNING: Never use Result after calling ReleaseResult!\n")
}

// demonstrateTestingConfig shows TestingConfig preset
func demonstrateTestingConfig() {
	fmt.Println("--- Example 3: Testing Configuration ---")

	// TestingConfig is optimized for unit tests
	config := httpc.TestingConfig()
	fmt.Println("TestingConfig settings:")
	fmt.Println("  - Short timeout (1s)")
	fmt.Println("  - No retries (faster test failures)")
	fmt.Println("  - TLS verification disabled (for mock servers)")
	fmt.Println()

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Fast request - good for tests
	start := time.Now()
	resp, err := client.Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}
	fmt.Printf("Request completed: Status %d in %v\n", resp.StatusCode(), time.Since(start))

	fmt.Println("\nBest practices for testing:")
	fmt.Println("  1. Use TestingConfig() for fast, reliable tests")
	fmt.Println("  2. Mock external services when possible")
	fmt.Println("  3. Use context.WithTimeout for test timeouts")
	fmt.Println("  4. Example test pattern:")
	fmt.Println()
	fmt.Println("    func TestAPI(t *testing.T) {")
	fmt.Println("        client, _ := httpc.New(httpc.TestingConfig())")
	fmt.Println("        defer client.Close()")
	fmt.Println()
	fmt.Println("        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)")
	fmt.Println("        defer cancel()")
	fmt.Println()
	fmt.Println("        resp, err := client.Request(ctx, \"GET\", testServer.URL)")
	fmt.Println("        // assert.NoError(t, err)")
	fmt.Println("    }\n")
}

// demonstrateDefaultClient shows default client management
func demonstrateDefaultClient() {
	fmt.Println("--- Example 4: Default Client Management ---")

	// Package-level functions use a shared default client
	fmt.Println("Package-level functions (Get, Post, etc.):")

	resp, err := httpc.Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}
	fmt.Printf("  httpc.Get: Status %d\n", resp.StatusCode())

	// Set custom default client
	customClient, err := httpc.New(&httpc.Config{
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Set as default (closes previous default)
	if err := httpc.SetDefaultClient(customClient); err != nil {
		log.Printf("Failed to set default: %v\n", err)
	}
	fmt.Println("  Custom client set as default")

	// Now package-level functions use custom client
	resp, err = httpc.Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}
	fmt.Printf("  httpc.Get (custom): Status %d\n", resp.StatusCode())

	// Close default client (cleanup)
	if err := httpc.CloseDefaultClient(); err != nil {
		log.Printf("Failed to close: %v\n", err)
	}
	fmt.Println("  Default client closed")

	fmt.Println("\nUse cases:")
	fmt.Println("  - Application-wide configuration")
	fmt.Println("  - Lazy initialization")
	fmt.Println("  - Testing with custom client\n")
}

// demonstrateMemoryOptimization shows memory optimization techniques
func demonstrateMemoryOptimization() {
	fmt.Println("--- Example 5: Memory Optimization Techniques ---")

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}

	// Make requests without optimization
	const numRequests = 100
	for i := 0; i < numRequests; i++ {
		resp, err := client.Get("https://httpbin.org/get")
		if err != nil {
			continue
		}
		_ = resp.Body()
		// Don't release - let GC handle it
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	fmt.Printf("Memory after %d requests (without pool):\n", numRequests)
	fmt.Printf("  Heap allocations: %d\n", m2.Mallocs-m1.Mallocs)
	fmt.Printf("  GC cycles: %d\n", m2.NumGC-m1.NumGC)

	// With pool optimization
	runtime.GC()
	runtime.ReadMemStats(&m1)

	for i := 0; i < numRequests; i++ {
		resp, err := client.Get("https://httpbin.org/get")
		if err != nil {
			continue
		}
		_ = resp.Body()
		httpc.ReleaseResult(resp) // Return to pool
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	fmt.Printf("\nMemory after %d requests (with pool):\n", numRequests)
	fmt.Printf("  Heap allocations: %d\n", m2.Mallocs-m1.Mallocs)
	fmt.Printf("  GC cycles: %d\n", m2.NumGC-m1.NumGC)

	client.Close()

	fmt.Println("\nOptimization tips:")
	fmt.Println("  1. Reuse client instances (don't create new clients per request)")
	fmt.Println("  2. Use ReleaseResult() for high-throughput scenarios")
	fmt.Println("  3. Configure appropriate timeouts to avoid goroutine leaks")
	fmt.Println("  4. Use PerformanceConfig() for high-concurrency applications")
	fmt.Println("  5. Close clients when done (releases connection pool)\n")
}
