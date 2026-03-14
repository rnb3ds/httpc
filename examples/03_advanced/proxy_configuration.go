package main

import (
	"fmt"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Proxy Configuration Examples ===")
	fmt.Println()

	// Example 1: Manual proxy (highest priority)
	fmt.Println("Example 1: Manual proxy URL")
	fmt.Println("When ProxyURL is set, it will be used regardless of EnableSystemProxy")
	client1, err := httpc.New(&httpc.Config{
		ProxyURL:      "http://127.0.0.1:7890",
		Timeout:       10 * time.Second,
		BackoffFactor: 2.0,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
	})
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
	} else {
		fmt.Println("✓ Client created with manual proxy: http://127.0.0.1:7890")
		client1.Close()
	}
	fmt.Println()

	// Example 2: Enable system proxy detection
	fmt.Println("Example 2: Enable system proxy detection")
	fmt.Println("When ProxyURL is empty and EnableSystemProxy is true,")
	fmt.Println("the client will automatically detect system proxy settings")
	fmt.Println("(Windows registry, environment variables, etc.)")
	client2, err := httpc.New(&httpc.Config{
		EnableSystemProxy: true,
		Timeout:           10 * time.Second,
		BackoffFactor:     2.0,
		MaxRetries:        3,
		RetryDelay:        1 * time.Second,
	})
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
	} else {
		fmt.Println("✓ Client created with system proxy detection enabled")
		fmt.Println("  (Will use system proxy if configured, otherwise direct connection)")
		client2.Close()
	}
	fmt.Println()

	// Example 3: Direct connection (no proxy)
	fmt.Println("Example 3: Direct connection (default behavior)")
	fmt.Println("When both ProxyURL is empty and EnableSystemProxy is false,")
	fmt.Println("the client will connect directly without any proxy")
	client3, err := httpc.New(&httpc.Config{
		Timeout:       10 * time.Second,
		BackoffFactor: 2.0,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
	})
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
	} else {
		fmt.Println("✓ Client created with direct connection (no proxy)")
		client3.Close()
	}
	fmt.Println()

	// Example 4: Manual proxy takes priority over system proxy
	fmt.Println("Example 4: Proxy priority demonstration")
	fmt.Println("Even when EnableSystemProxy is true,")
	fmt.Println("manual ProxyURL takes priority")
	client4, err := httpc.New(&httpc.Config{
		ProxyURL:          "http://127.0.0.1:7890",
		EnableSystemProxy: true, // This is ignored because ProxyURL is set
		Timeout:           10 * time.Second,
		BackoffFactor:     2.0,
		MaxRetries:        3,
		RetryDelay:        1 * time.Second,
	})
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
	} else {
		fmt.Println("✓ Client created with manual proxy (system proxy detection ignored)")
		fmt.Println("  Using proxy: http://127.0.0.1:7890")
		client4.Close()
	}
	fmt.Println()

	// Summary
	fmt.Println("=== Configuration Priority ===")
	fmt.Println("1. ProxyURL (manual proxy) - Highest priority")
	fmt.Println("2. EnableSystemProxy (auto-detect system proxy)")
	fmt.Println("3. Direct connection (no proxy) - Default")
	fmt.Println()

	fmt.Println("=== Common Use Cases ===")
	fmt.Println()
	fmt.Println("Case A: Behind a corporate proxy")
	fmt.Println("  config := httpc.Config{")
	fmt.Println("      ProxyURL: \"http://proxy.company.com:8080\",")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("Case B: Using system proxy (Windows/Mac/Linux)")
	fmt.Println("  config := httpc.Config{")
	fmt.Println("      EnableSystemProxy: true,")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("Case C: Direct connection (default)")
	fmt.Println("  config := httpc.Config{")
	fmt.Println("      // Both ProxyURL and EnableSystemProxy are empty/false")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("Case D: VPN/Proxy software (e.g., Clash, V2Ray)")
	fmt.Println("  Option 1: Use environment variables")
	fmt.Println("    // set HTTPS_PROXY=http://127.0.0.1:7890")
	fmt.Println("    config := httpc.Config{")
	fmt.Println("        EnableSystemProxy: true, // Will read env vars")
	fmt.Println("    }")
	fmt.Println("  Option 2: Specify proxy directly")
	fmt.Println("    config := httpc.Config{")
	fmt.Println("        ProxyURL: \"http://127.0.0.1:7890\",")
	fmt.Println("    }")
}
