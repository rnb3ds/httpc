//go:build examples

package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	// Create a new HTTP client
	client, err := httpc.New(httpc.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Example cookie string (like what you'd copy from browser developer tools)
	cookieString := "BSID=4418ECBB1281B550; PSTM=1733760779; BDS=kUwNTVFcEUBUItoc; BAD=01E8D701159F774:FG=1; MCITY=-257%3A; UPN=12314753"

	fmt.Println("Using WithCookieString to parse and set multiple cookies:")
	fmt.Printf("Cookie string: %s\n\n", cookieString)

	// Make a request to httpbin.org/cookies to see the cookies being sent
	response, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieString(cookieString),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Response Status: %s\n", response.Status)
	fmt.Printf("Response Body:\n%s\n", response.Body)

	// You can also combine WithCookieString with other cookie methods
	fmt.Println("\nCombining WithCookieString with other cookie methods:")

	response2, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieString("session=abc123; token=xyz789"),
		httpc.WithCookieValue("manual", "cookie"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Combined cookies response:\n%s\n", response2.Body)

	// Example with empty cookie string (no error, just no cookies added)
	fmt.Println("\nUsing empty cookie string:")
	response3, err := client.Get("https://httpbin.org/cookies",
		httpc.WithCookieString(""), // Empty string is valid
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Empty cookie string response:\n%s\n", response3.Body)
}
