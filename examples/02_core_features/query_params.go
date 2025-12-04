//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
	"github.com/cybergodev/httpc/examples/02_core_features/types"
)

func main() {
	fmt.Println("=== Query Parameters Examples ===\n ")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example 1: Single query parameters
	demonstrateSingleParams(client)

	// Example 2: Multiple query parameters using map
	demonstrateMapParams(client)

	// Example 3: Different value types
	demonstrateValueTypes(client)

	// Example 4: Real-world patterns
	demonstrateRealWorldPatterns(client)

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateSingleParams shows single parameter usage
func demonstrateSingleParams(client httpc.Client) {
	fmt.Println("--- Example 1: Single Query Parameters ---")

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithQuery("name", "John"),
		httpc.WithQuery("age", 30),
		httpc.WithQuery("city", "New York"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var result types.APIResponse
	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("URL Path: %s\n", result.Path)
	fmt.Printf("Query Parameters:\n")
	for key, value := range result.Args {
		fmt.Printf("  %s = %s\n", key, value)
	}
	fmt.Println()
}

// demonstrateMapParams shows map-based parameters (recommended)
func demonstrateMapParams(client httpc.Client) {
	fmt.Println("--- Example 2: Multiple Parameters (Map) ---")

	params := map[string]any{
		"category": "technology",
		"sort":     "date",
		"order":    "desc",
		"limit":    10,
		"offset":   0,
	}

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithQueryMap(params),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var result types.APIResponse
	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Printf("Query Parameters: %+v\n\n", result.Args)
}

// demonstrateValueTypes shows different value types
func demonstrateValueTypes(client httpc.Client) {
	fmt.Println("--- Example 3: Different Value Types ---")

	params := map[string]any{
		"string_value": "hello",
		"int_value":    42,
		"float_value":  3.14,
		"bool_value":   true,
		"another_bool": false,
	}

	resp, err := client.Get("https://echo.hoppscotch.io",
		httpc.WithQueryMap(params),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	var result types.APIResponse
	if err := resp.JSON(&result); err != nil {
		log.Printf("Failed to parse JSON: %v\n", err)
		return
	}

	fmt.Println("All value types are automatically converted to strings:")
	for key, value := range result.Args {
		fmt.Printf("  %s = %s\n", key, value)
	}
	fmt.Println()
}

// demonstrateRealWorldPatterns shows practical query parameter patterns
func demonstrateRealWorldPatterns(client httpc.Client) {
	fmt.Println("=== Real-World Query Parameter Patterns ===\n ")

	// Pattern 1: Pagination
	fmt.Println("--- Pattern 1: Pagination ---")
	paginationParams := map[string]any{
		"page":     1,
		"per_page": 20,
	}
	resp, err := client.Get("https://echo.hoppscotch.io/api/users",
		httpc.WithQueryMap(paginationParams),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		var result types.APIResponse
		resp.JSON(&result)
		fmt.Printf("Pagination params: %+v\n\n", result.Args)
	}

	// Pattern 2: Filtering
	fmt.Println("--- Pattern 2: Filtering ---")
	filterParams := map[string]any{
		"status":     "active",
		"role":       "admin",
		"created_at": "2026-01-01",
	}
	resp, err = client.Get("https://echo.hoppscotch.io/api/users",
		httpc.WithQueryMap(filterParams),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		var result types.APIResponse
		resp.JSON(&result)
		fmt.Printf("Filter params: %+v\n\n", result.Args)
	}

	// Pattern 3: Sorting
	fmt.Println("--- Pattern 3: Sorting ---")
	sortParams := map[string]any{
		"sort_by":    "created_at",
		"sort_order": "desc",
	}
	resp, err = client.Get("https://echo.hoppscotch.io/api/articles",
		httpc.WithQueryMap(sortParams),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		var result types.APIResponse
		resp.JSON(&result)
		fmt.Printf("Sort params: %+v\n\n", result.Args)
	}

	// Pattern 4: Search
	fmt.Println("--- Pattern 4: Search ---")
	searchParams := map[string]any{
		"q":      "golang http client",
		"fields": "title,content",
		"limit":  10,
	}
	resp, err = client.Get("https://echo.hoppscotch.io/api/search",
		httpc.WithQueryMap(searchParams),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		var result types.APIResponse
		resp.JSON(&result)
		fmt.Printf("Search params: %+v\n\n", result.Args)
	}

	// Pattern 5: Combined (pagination + filtering + sorting)
	fmt.Println("--- Pattern 5: Combined Parameters ---")
	combinedParams := map[string]any{
		// Pagination
		"page":     1,
		"per_page": 20,
		// Filtering
		"status":   "published",
		"category": "technology",
		// Sorting
		"sort_by": "date",
		"order":   "desc",
		// Search
		"q": "api",
	}
	resp, err = client.Get("https://echo.hoppscotch.io/api/articles",
		httpc.WithQueryMap(combinedParams),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		var result types.APIResponse
		resp.JSON(&result)
		fmt.Printf("Combined params: %+v\n\n", result.Args)
	}

	// Pattern 6: Special characters (automatically URL-encoded)
	fmt.Println("--- Pattern 6: Special Characters (URL Encoding) ---")
	specialParams := map[string]any{
		"query": "hello world & goodbye",
		"email": "user@example.com",
		"url":   "https://example.com/path?param=value",
	}
	resp, err = client.Get("https://echo.hoppscotch.io",
		httpc.WithQueryMap(specialParams),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		var result types.APIResponse
		resp.JSON(&result)
		fmt.Println("Special characters are automatically URL-encoded:")
		for key, value := range result.Args {
			fmt.Printf("  %s = %s\n", key, value)
		}
		fmt.Println()
	}
}
