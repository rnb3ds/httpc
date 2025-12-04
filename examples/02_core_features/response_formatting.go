//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== Response Formatting Examples ===\n ")

	// Make a simple GET request
	resp, err := httpc.Get("https://httpbin.org/json")
	if err != nil {
		log.Fatal(err)
	}

	// Example 1: String() - Concise text representation
	fmt.Println("1. String() - Concise text representation:")
	fmt.Println(resp.String())
	fmt.Println()

	// Example 2: Html() - HTML-formatted output
	fmt.Println("2. Html() - HTML-formatted output:")
	fmt.Println(resp.Html())
	fmt.Println()

	fmt.Println("=== Examples Complete ===")
}
