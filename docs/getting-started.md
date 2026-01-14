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
    result, err := client.Get("https://api.github.com/users/octocat")
    if err != nil {
        log.Fatal(err)
    }

    // Print response
    fmt.Printf("Status: %d\n", result.StatusCode())
    fmt.Printf("Body: %s\n", result.Body())
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
    result, err := client.Post("https://api.example.com/users",
        httpc.WithJSON(user),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Status: %d\n", result.StatusCode())
    fmt.Printf("Response: %s\n", result.Body())
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
    result, err := client.Get("https://api.example.com/users/1")
    if err != nil {
        log.Fatal(err)
    }

    // Parse JSON response
    var user User
    if err := result.JSON(&user); err != nil {
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
result, err := client.Get(url)
if err != nil {
    log.Printf("Request failed: %v", err)
    return err
}

// Check response status
if !result.IsSuccess() {
    log.Printf("Unexpected status: %d", result.StatusCode())
    return fmt.Errorf("request failed with status %d", result.StatusCode())
}

// Process response
fmt.Println(result.Body())
```

**See also:** [Error Handling Guide](error-handling.md) for comprehensive patterns.

### Response Helpers

```go
result, err := client.Get(url)
if err != nil {
    return err
}

// Status code helpers
if result.IsSuccess() {        // 2xx
    fmt.Println("Success!")
}
if result.IsClientError() {    // 4xx
    fmt.Println("Client error")
}
if result.IsServerError() {    // 5xx
    fmt.Println("Server error")
}

// Parse response
var data map[string]interface{}
if err := result.JSON(&data); err != nil {
    return err
}
```

## Common Patterns

> **Note**: These patterns are referenced throughout the documentation. Master these basics first, then explore specialized guides.

### Client Setup Pattern

**Standard Client (Recommended for Production)**

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()  // Always close to release resources
```

**Custom Client**

```go
config := httpc.DefaultConfig()
config.Timeout = 30 * time.Second
config.UserAgent = "MyApp/1.0"

client, err := httpc.New(config)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**Quick One-Off Requests**

```go
// Uses shared default client - good for scripts
result, err := httpc.Get("https://api.example.com/data")
if err != nil {
    log.Fatal(err)
}
```

### Error Handling Pattern

```go
result, err := client.Get(url)
if err != nil {
    // Network error, timeout, or invalid input
    log.Printf("Request failed: %v", err)
    return err
}

// Check HTTP status (HTTPC returns Result for all status codes)
if !result.IsSuccess() {
    // 4xx or 5xx status
    log.Printf("API error: %d - %s", result.StatusCode(), result.Status())
    return fmt.Errorf("request failed with status %d", result.StatusCode())
}

// Process successful response
fmt.Println(result.Body())
```

### Authenticated Request Pattern

```go
result, err := client.Get("https://api.example.com/protected",
    httpc.WithBearerToken("your-token"),
)
if err != nil {
    return err
}
if !result.IsSuccess() {
    return fmt.Errorf("authentication failed: %d", result.StatusCode())
}
```

### JSON Request/Response Pattern

```go
// POST JSON data
payload := map[string]string{"name": "John", "email": "john@example.com"}
result, err := client.Post("https://api.example.com/users",
    httpc.WithJSON(payload),
)
if err != nil {
    return err
}

// Parse JSON response
var response struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}
if err := result.JSON(&response); err != nil {
    return err
}

fmt.Printf("Created user: %+v\n", response)
```

### Advanced Pattern 1: API Client Wrapper

```go
type APIClient struct {
    client  httpc.Client
    baseURL string
    token   string
}

func NewAPIClient(baseURL, token string) (*APIClient, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }

    return &APIClient{
        client:  client,
        baseURL: baseURL,
        token:   token,
    }, nil
}

func (c *APIClient) Close() error {
    return c.client.Close()
}

func (c *APIClient) GetUser(id int) (*User, error) {
    url := fmt.Sprintf("%s/users/%d", c.baseURL, id)

    result, err := c.client.Get(url,
        httpc.WithBearerToken(c.token),
        httpc.WithTimeout(10*time.Second),
    )
    if err != nil {
        return nil, err
    }

    if !result.IsSuccess() {
        return nil, fmt.Errorf("API returned status %d", result.StatusCode())
    }

    var user User
    if err := result.JSON(&user); err != nil {
        return nil, err
    }

    return &user, nil
}
```

### Advanced Pattern 2: Context-Aware Requests

```go
func fetchData(ctx context.Context, url string) ([]byte, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    defer client.Close()

    result, err := client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithTimeout(30*time.Second),
    )
    if err != nil {
        return nil, err
    }

    if !result.IsSuccess() {
        return nil, fmt.Errorf("status %d", result.StatusCode())
    }

    return result.RawBody(), nil
}

// Usage with timeout
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()

data, err := fetchData(ctx, "https://api.example.com/large-data")
```

---

## Next Steps

Now that you understand the basics, explore these guides:

- **[Request Options](request-options.md)** - Complete reference for all request options
- **[Configuration](configuration.md)** - Client configuration and presets
- **[Error Handling](error-handling.md)** - Advanced error handling patterns
- **[File Downloads](file-download.md)** - Download files with progress tracking
- **[Cookie Management](cookie-api-reference.md)** - Automatic and manual cookie handling
- **[Redirects](redirects.md)** - Handle HTTP redirects
- **[Request Inspection](request-inspection.md)** - Debug and inspect requests

---

## Configuration Basics

### Using Defaults

```go
// Uses secure defaults (TLS 1.2+, 60s timeout, 2 retries)
client, err := httpc.New()
```

### Using Presets

```go
// Development - Permissive settings
client, err := httpc.New(httpc.TestingConfig())

// Production - Balanced (default)
client, err := httpc.New(httpc.DefaultConfig())

// High Security - Strict settings
client, err := httpc.New(httpc.SecureConfig())
```

**See also:** [Configuration Guide](configuration.md) for detailed configuration options.


## Quick Tips

✅ **DO:**
- Always close clients with `defer client.Close()`
- Check errors from all operations
- Use `result.IsSuccess()` to check status codes
- Set appropriate timeouts for your use case
- Reuse client instances for multiple requests

❌ **DON'T:**
- Create a new client for each request
- Ignore errors
- Use package-level functions in production code
- Set very long timeouts without good reason
- Forget to close clients

---
