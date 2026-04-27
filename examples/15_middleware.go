//go:build examples

package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cybergodev/httpc"
)

// This example demonstrates middleware usage patterns

func main() {
	fmt.Println("=== Middleware Examples ===\n ")

	// Example 1: Basic logging middleware
	demonstrateLoggingMiddleware()

	// Example 2: Request ID middleware
	demonstrateRequestIDMiddleware()

	// Example 3: Metrics collection middleware
	demonstrateMetricsMiddleware()

	// Example 4: Recovery middleware
	demonstrateRecoveryMiddleware()

	// Example 5: Audit middleware
	demonstrateAuditMiddleware()

	// Example 6: Header middleware
	demonstrateHeaderMiddleware()

	// Example 7: Chaining middlewares
	demonstrateMiddlewareChain()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateLoggingMiddleware shows basic request/response logging
func demonstrateLoggingMiddleware() {
	fmt.Println("--- Example 1: Logging Middleware ---")

	// Create client with logging middleware
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.LoggingMiddleware(log.Printf),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Make a request - logging is automatic
	resp, err := client.Get("https://httpbin.org/get",
		httpc.WithQuery("test", "logging"),
	)
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n\n", resp.StatusCode())
}

// demonstrateRequestIDMiddleware shows adding unique request IDs
func demonstrateRequestIDMiddleware() {
	fmt.Println("--- Example 2: Request ID Middleware ---")

	// Create client with request ID middleware
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.RequestIDMiddleware("X-Request-ID", nil),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Make multiple requests - each gets unique ID
	for i := 1; i <= 3; i++ {
		resp, err := client.Get("https://httpbin.org/get")
		if err != nil {
			log.Printf("Request %d failed: %v\n", i, err)
			continue
		}
		fmt.Printf("Request %d: Status %d\n", i, resp.StatusCode())
	}
	fmt.Println()
}

// metricData stores metrics for a request pattern
type metricData struct {
	reqCount int
	totalDur time.Duration
	errCount int
}

// demonstrateMetricsMiddleware shows collecting request metrics
func demonstrateMetricsMiddleware() {
	fmt.Println("--- Example 3: Metrics Middleware ---")

	// Metrics storage
	metrics := make(map[string]*metricData)

	var mu sync.Mutex

	// Create client with metrics middleware
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {
			mu.Lock()
			key := fmt.Sprintf("%s %s", method, url)
			if _, exists := metrics[key]; !exists {
				metrics[key] = &metricData{reqCount: 0, totalDur: 0, errCount: 0}
			}
			metrics[key].reqCount++
			metrics[key].totalDur += duration
			if err != nil {
				metrics[key].errCount++
			}
			mu.Unlock()
		}),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Make multiple requests
	for i := 1; i <= 5; i++ {
		_, err := client.Get("https://httpbin.org/delay/1")
		if err != nil {
			_ = err // error details captured by metrics middleware above
		}
	}

	// Print metrics summary
	fmt.Println("\nMetrics Summary:")
	for key, m := range metrics {
		fmt.Printf("  %s: %d requests, %v total, %d errors\n", key, m.reqCount, m.totalDur, m.errCount)
	}
	fmt.Println()
}

// demonstrateRecoveryMiddleware shows panic recovery
func demonstrateRecoveryMiddleware() {
	fmt.Println("--- Example 4: Recovery Middleware ---")

	// Create client with recovery middleware
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.RecoveryMiddleware(),
		httpc.LoggingMiddleware(log.Printf),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Normal request - should succeed
	resp, err := client.Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Normal request: %v\n", err)
	} else {
		fmt.Printf("Normal request succeeded: Status %d\n", resp.StatusCode())
	}

	fmt.Println()
}

// demonstrateAuditMiddleware shows security audit logging
func demonstrateAuditMiddleware() {
	fmt.Println("--- Example 5: Audit Middleware ---")

	// Create client with audit middleware
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.AuditMiddleware(func(event httpc.AuditEvent) {
			log.Printf("[AUDIT] %s %s -> %d (%v) attempts=%d user=%s ip=%s",
				event.Method, event.URL, event.StatusCode, event.Duration, event.Attempts,
				event.UserID, event.SourceIP)
		}),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Make request with user context
	ctx := context.WithValue(context.Background(), httpc.UserIDKey, "user-123")
	ctx = context.WithValue(ctx, httpc.SourceIPKey, "192.168.1.1")
	resp, err := client.Get("https://httpbin.org/get", httpc.WithContext(ctx))
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Request completed: Status %d\n\n", resp.StatusCode())
}

// demonstrateHeaderMiddleware shows adding default headers via middleware
func demonstrateHeaderMiddleware() {
	fmt.Println("--- Example 6: Header Middleware ---")

	// Create client with header middleware for default headers on every request
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.HeaderMiddleware(map[string]string{
			"X-App-Version": "1.0.0",
			"X-Client-ID":   "my-client",
		}),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://httpbin.org/headers")
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Headers added to every request via middleware\n ")
}

// demonstrateMiddlewareChain shows combining multiple middlewares
func demonstrateMiddlewareChain() {
	fmt.Println("--- Example 7: Middleware Chain ---")

	// Approach 1: Configure via slice (order preserved)
	config := httpc.DefaultConfig()
	config.Middleware.Middlewares = []httpc.MiddlewareFunc{
		httpc.RequestIDMiddleware("X-Correlation-ID", nil),
		httpc.RecoveryMiddleware(),
		httpc.LoggingMiddleware(log.Printf),
		httpc.TimeoutMiddleware(30 * time.Second),
	}

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://httpbin.org/get",
		httpc.WithQuery("chained", "middlewares"),
	)
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Slice approach: Status %d\n", resp.StatusCode())

	// Approach 2: Using httpc.Chain() to compose middlewares into one
	// This is useful when you want to pass a single MiddlewareFunc
	chain := httpc.Chain(
		httpc.RequestIDMiddleware("X-Request-ID", nil),
		httpc.RecoveryMiddleware(),
		httpc.LoggingMiddleware(log.Printf),
	)

	config2 := httpc.DefaultConfig()
	config2.Middleware.Middlewares = []httpc.MiddlewareFunc{chain}
	client2, err := httpc.New(config2)
	if err != nil {
		log.Fatal(err)
	}
	defer client2.Close()

	resp, err = client2.Get("https://httpbin.org/get",
		httpc.WithQuery("chain", "composed"),
	)
	if err != nil {
		log.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Chain() approach: Status %d\n", resp.StatusCode())
	fmt.Println("\nBoth approaches produce the same result.")
	fmt.Println("Use Chain() when composing middleware dynamically or passing as a single argument.")
}
