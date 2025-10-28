//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

// Dev Test File

func main() {
	// 1. Create a client (uses secure defaults)
	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 2. Make a simple GET request
	resp, err := client.Get("https://api.github.com/users/octocat")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Body: %s\n", resp.Body)

	// 3. POST JSON data
	user := map[string]string{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	resp, err = client.Post("https://api.example.com/users",
		httpc.WithJSON(user),
		httpc.WithBearerToken("your-token"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created: %d\n", resp.StatusCode)
}
