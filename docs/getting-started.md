# Getting Started with HTTPC

This guide will help you get started with HTTPC in just a few minutes.

## Installation

### Requirements

- Go 1.24 or higher
- Internet connection (for downloading dependencies)

### Install via go get

```bash
go get -u github.com/cybergodev/httpc
```

### Verify Installation

Create a simple test file:

```go
package main

import (
    "fmt"
    "github.com/cybergodev/httpc"
)

func main() {
    client, err := httpc.New()
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    fmt.Println("HTTPC installed successfully!")
}
```

Run it:

```bash
go run main.go
```

## Your First Request

### Simple GET Request

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // Create a client
    client, err := httpc.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Make a GET request
    resp, err := client.Get("https://api.github.com/users/octocat")
    if err != nil {
        log.Fatal(err)
    }

    // Print response
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)
}
```

### POST JSON Data

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

type User struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    client, err := httpc.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Create user data
    user := User{
        Name:  "John Doe",
        Email: "john@example.com",
    }

    // POST JSON
    resp, err := client.Post("https://api.example.com/users",
        httpc.WithJSON(user),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Response: %s\n", resp.Body)
}
```

### Parse JSON Response

```go
type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    client, err := httpc.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // GET request
    resp, err := client.Get("https://api.example.com/users/1")
    if err != nil {
        log.Fatal(err)
    }

    // Parse JSON response
    var user User
    if err := resp.JSON(&user); err != nil {
        log.Fatal(err)
    }

    fmt.Printf("User: %+v\n", user)
}
```

## Basic Concepts

### Client Lifecycle

Always create a client and close it when done:

```go
// Create client
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()  // Important: always close the client

// Use client for multiple requests
resp1, _ := client.Get(url1)
resp2, _ := client.Get(url2)
resp3, _ := client.Post(url3, httpc.WithJSON(data))
```

**Why close?** Closing the client releases resources like connection pools and goroutines.

### Request Options

Customize requests with options:

```go
resp, err := client.Get(url,
    httpc.WithTimeout(10*time.Second),
    httpc.WithHeader("X-Custom", "value"),
    httpc.WithBearerToken("your-token"),
)
```

**See also:** [Request Options Guide](request-options.md) for complete reference.

### Error Handling

Always check errors:

```go
resp, err := client.Get(url)
if err != nil {
    log.Printf("Request failed: %v", err)
    return err
}

// Check response status
if !resp.IsSuccess() {
    log.Printf("Unexpected status: %d", resp.StatusCode)
    return fmt.Errorf("request failed with status %d", resp.StatusCode)
}

// Process response
fmt.Println(resp.Body)
```

**See also:** [Error Handling Guide](error-handling.md) for comprehensive patterns.

### Response Helpers

```go
resp, err := client.Get(url)
if err != nil {
    return err
}

// Status code helpers
if resp.IsSuccess() {        // 2xx
    fmt.Println("Success!")
}
if resp.IsClientError() {    // 4xx
    fmt.Println("Client error")
}
if resp.IsServerError() {    // 5xx
    fmt.Println("Server error")
}

// Parse response
var data map[string]interface{}
if err := resp.JSON(&data); err != nil {
    return err
}
```

## Common Patterns

### Pattern 1: Simple API Client

```go
type APIClient struct {
    client httpc.Client
    baseURL string
    token string
}

func NewAPIClient(baseURL, token string) (*APIClient, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    
    return &APIClient{
        client: client,
        baseURL: baseURL,
        token: token,
    }, nil
}

func (c *APIClient) Close() error {
    return c.client.Close()
}

func (c *APIClient) GetUser(id int) (*User, error) {
    url := fmt.Sprintf("%s/users/%d", c.baseURL, id)
    
    resp, err := c.client.Get(url,
        httpc.WithBearerToken(c.token),
        httpc.WithTimeout(10*time.Second),
    )
    if err != nil {
        return nil, err
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
    }
    
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, err
    }
    
    return &user, nil
}
```

### Pattern 2: Package-Level Functions

For quick one-off requests:

```go
// No need to create a client
resp, err := httpc.Get("https://api.example.com/data")
if err != nil {
    log.Fatal(err)
}

fmt.Println(resp.Body)
```

**Note:** Package-level functions use a shared default client. For production code, prefer creating your own client instance.

### Pattern 3: Context-Aware Requests

```go
func fetchData(ctx context.Context, url string) ([]byte, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    defer client.Close()
    
    resp, err := client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithTimeout(30*time.Second),
    )
    if err != nil {
        return nil, err
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("status %d", resp.StatusCode)
    }
    
    return resp.RawBody, nil
}

// Usage
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()

data, err := fetchData(ctx, url)
```

## Configuration Basics

### Using Defaults

```go
// Uses secure defaults (TLS 1.2+, 60s timeout, 2 retries)
client, err := httpc.New()
```

### Using Presets

```go
// Development - Permissive settings
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelPermissive))

// Production - Balanced (default)
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelBalanced))

// High Security - Strict settings
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelStrict))
```

**See also:** [Configuration Guide](configuration.md) for detailed configuration options.

## Next Steps

Now that you know the basics, explore more features:

1. **[Request Options](request-options.md)** - Headers, authentication, body formats
2. **[Error Handling](error-handling.md)** - Comprehensive error handling patterns
3. **[Configuration](configuration.md)** - Client configuration and presets
4. **[Circuit Breaker](circuit-breaker.md)** - Automatic fault protection
5. **[File Download](file-download.md)** - Download files with progress tracking
6. **[Best Practices](best-practices.md)** - Recommended usage patterns
7. **[Examples](examples.md)** - Working code examples

## Quick Tips

✅ **DO:**
- Always close clients with `defer client.Close()`
- Check errors from all operations
- Use `resp.IsSuccess()` to check status codes
- Set appropriate timeouts for your use case
- Reuse client instances for multiple requests

❌ **DON'T:**
- Create a new client for each request
- Ignore errors
- Use package-level functions in production code
- Set very long timeouts without good reason
- Forget to close clients

## Troubleshooting

### "connection refused"

The server is not running or the URL is incorrect.

```go
// Check the URL
resp, err := client.Get("https://api.example.com/data")  // Correct
// Not: http://localhost:8080 (if server is not running)
```

### "timeout"

The request took too long. Increase timeout or check network:

```go
resp, err := client.Get(url,
    httpc.WithTimeout(30*time.Second),  // Increase timeout
)
```

### "TLS handshake error"

TLS configuration issue. Check certificate or use permissive mode for testing:

```go
// For testing only (not for production!)
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelPermissive))
```

## Getting Help

- **Questions?** Open a [GitHub Discussion](https://github.com/cybergodev/httpc/discussions)
- **Bug reports?** Open a [GitHub Issue](https://github.com/cybergodev/httpc/issues)
- **More examples?** Check the [examples directory](../examples)

---
