//go:build examples

package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Client Configuration Examples ===\n ")

	// Example 1: Default configuration
	demonstrateDefaultConfig()

	// Example 2: Secure configuration
	demonstrateSecureConfig()

	// Example 3: Performance configuration
	demonstratePerformanceConfig()

	// Example 4: Custom configuration
	demonstrateCustomConfig()

	// Example 5: Configuration comparison
	demonstrateConfigComparison()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateDefaultConfig shows default client configuration
func demonstrateDefaultConfig() {
	fmt.Println("--- Example 1: Default Configuration ---")

	// Create client with default settings
	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Default config includes:")
	fmt.Println("  - 30s timeout")
	fmt.Println("  - 3 retries with exponential backoff")
	fmt.Println("  - TLS 1.2+ with certificate validation")
	fmt.Println("  - HTTP/2 enabled")
	fmt.Println("  - Connection pooling\n ")
}

// demonstrateSecureConfig shows secure client configuration
func demonstrateSecureConfig() {
	fmt.Println("--- Example 2: Secure Configuration ---")

	// Create client with enhanced security
	client, err := httpc.NewSecure()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Secure config includes:")
	fmt.Println("  - TLS 1.3 minimum")
	fmt.Println("  - Strict certificate validation")
	fmt.Println("  - Private IP blocking (SSRF protection)")
	fmt.Println("  - Strict content length validation")
	fmt.Println("  - Lower connection limits\n ")
}

// demonstratePerformanceConfig shows performance-optimized configuration
func demonstratePerformanceConfig() {
	fmt.Println("--- Example 3: Performance Configuration ---")

	// Create client optimized for performance
	client, err := httpc.NewPerformance()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Println("Performance config includes:")
	fmt.Println("  - Higher connection limits")
	fmt.Println("  - Longer keep-alive")
	fmt.Println("  - Optimized pooling")
	fmt.Println("  - HTTP/2 with multiplexing\n ")
}

// demonstrateCustomConfig shows custom client configuration
func demonstrateCustomConfig() {
	fmt.Println("--- Example 4: Custom Configuration ---")

	// Create custom configuration
	config := httpc.DefaultConfig()
	config.Timeout = 15 * time.Second
	config.MaxRetries = 5
	config.MaxIdleConns = 200
	config.MaxConnsPerHost = 50
	config.UserAgent = "MyApp/1.0"
	config.FollowRedirects = true
	config.EnableCookies = true

	client, err := httpc.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithQuery("custom", "config"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode())
	fmt.Printf("Duration: %v\n", resp.Meta.Duration)
	fmt.Println("Custom config applied successfully\n ")
}

// demonstrateConfigComparison shows different configuration scenarios
func demonstrateConfigComparison() {
	fmt.Println("=== Configuration Comparison ===\n ")

	// Scenario 1: Quick API calls
	fmt.Println("Scenario 1: Quick API Calls (< 5s)")
	quickConfig := httpc.DefaultConfig()
	quickConfig.Timeout = 2 * time.Second
	quickConfig.MaxRetries = 0
	fmt.Println("  Timeout: 2s, Retries: 0")
	fmt.Println("  Use case: Health checks, fast endpoints\n ")

	// Scenario 2: Standard API calls
	fmt.Println("Scenario 2: Standard API Calls (5-15s)")
	standardConfig := httpc.DefaultConfig()
	standardConfig.Timeout = 10 * time.Second
	standardConfig.MaxRetries = 2
	fmt.Println("  Timeout: 10s, Retries: 2")
	fmt.Println("  Use case: Most REST API calls\n ")

	// Scenario 3: Long operations
	fmt.Println("Scenario 3: Long Operations (15-60s)")
	longConfig := httpc.DefaultConfig()
	longConfig.Timeout = 30 * time.Second
	longConfig.MaxRetries = 3
	fmt.Println("  Timeout: 30s, Retries: 3")
	fmt.Println("  Use case: File uploads, complex queries\n ")

	// Scenario 4: Background jobs
	fmt.Println("Scenario 4: Background Jobs (> 60s)")
	backgroundConfig := httpc.DefaultConfig()
	backgroundConfig.Timeout = 120 * time.Second
	backgroundConfig.MaxRetries = 5
	fmt.Println("  Timeout: 120s, Retries: 5")
	fmt.Println("  Use case: Batch processing, webhooks\n ")

	// Scenario 5: High security
	fmt.Println("Scenario 5: High Security")
	secureConfig := httpc.SecureConfig()
	secureConfig.MinTLSVersion = tls.VersionTLS13
	secureConfig.AllowPrivateIPs = false
	secureConfig.StrictContentLength = true
	fmt.Println("  TLS 1.3+, SSRF protection, strict validation")
	fmt.Println("  Use case: Financial, healthcare, sensitive data\n ")

	// Scenario 6: High throughput
	fmt.Println("Scenario 6: High Throughput")
	perfConfig := httpc.PerformanceConfig()
	perfConfig.MaxIdleConns = 500
	perfConfig.MaxConnsPerHost = 100
	fmt.Println("  High connection limits, optimized pooling")
	fmt.Println("  Use case: Web scraping, bulk operations\n ")
}
