//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== HTTP Response Decompression Example ===\n")

	// Example 1: Automatic gzip decompression
	fmt.Println("1. Automatic gzip decompression:")
	headers := map[string]string{
		"Accept-Encoding": "gzip, deflate", // Request compressed response
		"User-Agent":      "httpc-example/1.0",
	}

	resp, err := httpc.Get("https://httpbin.org/gzip", httpc.WithHeaderMap(headers))
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	fmt.Printf("   Status: %d\n", resp.StatusCode)
	fmt.Printf("   Content-Encoding: %s\n", resp.Headers.Get("Content-Encoding"))
	fmt.Printf("   Decompressed body length: %d bytes\n", len(resp.Body))
	fmt.Printf("   Body preview: %.100s...\n\n", resp.Body)

	// Example 2: Deflate decompression
	// Note: Some servers may not properly support deflate encoding
	fmt.Println("2. Testing with another gzip endpoint:")
	resp2, err := httpc.Get("https://www.github.com", httpc.WithHeaderMap(headers))
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	fmt.Printf("   Status: %d\n", resp2.StatusCode)
	fmt.Printf("   Content-Encoding: %s\n", resp2.Headers.Get("Content-Encoding"))
	fmt.Printf("   Decompressed body length: %d bytes\n", len(resp2.Body))
	fmt.Printf("   Body preview: %.100s...\n\n", resp2.Body)

	// Example 3: Without compression
	fmt.Println("3. Without compression (no Accept-Encoding header):")
	resp3, err := httpc.Get("https://httpbin.org/get")
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	fmt.Printf("   Status: %d\n", resp3.StatusCode)
	fmt.Printf("   Content-Encoding: %s\n", resp3.Headers.Get("Content-Encoding"))
	fmt.Printf("   Body length: %d bytes\n", len(resp3.Body))
	fmt.Printf("   Body preview: %.100s...\n\n", resp3.Body)

	fmt.Println("\n✓ All compression examples completed successfully!")
	fmt.Println("\nNote:")
	fmt.Println("  - The library automatically detects and decompresses gzip and deflate responses")
	fmt.Println("  - Decompression is based on the Content-Encoding header")
	fmt.Println("  - Brotli (br) is not supported due to zero-dependency constraint")
	fmt.Println("  - To request compressed responses, set Accept-Encoding header")
}
