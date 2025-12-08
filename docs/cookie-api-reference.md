# Cookie API Reference

Quick reference for working with cookies in httpc.

## Request Cookies vs Response Cookies

| Type | Description | Source | Usage |
|------|-------------|--------|-------|
| **Request Cookies** | Cookies sent TO the server | `Cookie` header | Authentication, session tracking |
| **Response Cookies** | Cookies received FROM the server | `Set-Cookie` header | Server setting new cookies |

## API Methods

### Response Cookies (from server)

Methods to access cookies returned by the server via `Set-Cookie` header:

| Method | Description | Example |
|--------|-------------|---------|
| `result.Response.Cookies` | All response cookies | `for _, c := range result.Response.Cookies { ... }` |
| `result.GetCookie(name)` | Get specific cookie | `cookie := result.GetCookie("session")` |
| `result.HasCookie(name)` | Check if cookie exists | `if result.HasCookie("session") { ... }` |

### Request Cookies (sent to server)

Methods to inspect cookies that were sent in the request via `Cookie` header:

| Method | Description | Example |
|--------|-------------|---------|
| `result.Request.Cookies` | All request cookies | `for _, c := range result.Request.Cookies { ... }` |
| `result.GetRequestCookie(name)` | Get specific cookie | `cookie := result.GetRequestCookie("session")` |
| `result.HasRequestCookie(name)` | Check if cookie was sent | `if result.HasRequestCookie("session") { ... }` |

## Complete Example

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    client, _ := httpc.New()
    defer client.Close()

    // Send request with cookies
    result, err := client.Get("https://api.example.com",
        httpc.WithCookieValue("auth", "token123"),
        httpc.WithCookieValue("session", "abc456"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // ============================================
    // REQUEST COOKIES (what we sent)
    // ============================================
    
    // Get all request cookies
    requestCookies := result.Request.Cookies
    fmt.Printf("Sent %d cookies\n", len(requestCookies))
    
    // Get specific request cookie
    authCookie := result.GetRequestCookie("auth")
    if authCookie != nil {
        fmt.Printf("Auth: %s\n", authCookie.Value)
    }
    
    // Check if cookie was sent
    if result.HasRequestCookie("session") {
        fmt.Println("Session cookie was sent")
    }

    // ============================================
    // RESPONSE COOKIES (what server sent back)
    // ============================================
    
    // Get all response cookies
    fmt.Printf("Received %d cookies\n", len(result.Response.Cookies))
    
    // Get specific response cookie
    newSession := result.GetCookie("new_session")
    if newSession != nil {
        fmt.Printf("New session: %s\n", newSession.Value)
    }
    
    // Check if server set a cookie
    if result.HasCookie("new_session") {
        fmt.Println("Server set new session")
    }
}
```

## Setting Request Cookies

Multiple ways to add cookies to requests:

```go
// Method 1: Single cookie by name/value
result, err := client.Get(url,
    httpc.WithCookieValue("session", "abc123"),
)

// Method 2: http.Cookie struct
result, err := client.Get(url,
    httpc.WithCookie(http.Cookie{
        Name:  "session",
        Value: "abc123",
    }),
)

// Method 3: Multiple cookies at once
cookies := []http.Cookie{
    {Name: "cookie1", Value: "value1"},
    {Name: "cookie2", Value: "value2"},
}
result, err := client.Get(url,
    httpc.WithCookies(cookies),
)

// Method 4: Cookie string (from browser)
result, err := client.Get(url,
    httpc.WithCookieString("session=abc123; token=xyz789"),
)
```

## Cookie Jar (Automatic Cookie Management)

Enable cookie jar for automatic cookie persistence:

```go
config := httpc.DefaultConfig()
config.EnableCookies = true
client, _ := httpc.New(config)

// First request - server sets cookies
result1, _ := client.Get("https://api.example.com/login")
fmt.Printf("Server set %d cookies\n", len(result1.Response.Cookies))

// Second request - cookie jar automatically sends cookies
result2, _ := client.Get("https://api.example.com/profile")

// Verify cookies were sent automatically
if result2.HasRequestCookie("session") {
    fmt.Println("Cookie jar automatically sent session cookie")
}
```

## Debugging Cookies

### Verify cookies were sent correctly

```go
result, err := client.Get(url,
    httpc.WithCookieValue("auth", "token123"),
)

// Check if cookie was actually sent
if !result.HasRequestCookie("auth") {
    log.Println("WARNING: auth cookie was not sent!")
}

// Get the actual value that was sent
authCookie := result.GetRequestCookie("auth")
if authCookie != nil && authCookie.Value != "token123" {
    log.Printf("WARNING: auth cookie value mismatch: %s", authCookie.Value)
}
```

### Compare request vs response

```go
result, err := client.Get(url,
    httpc.WithCookieValue("client_cookie", "value1"),
)

fmt.Println("Sent to server:")
for _, cookie := range result.Request.Cookies {
    fmt.Printf("  → %s = %s\n", cookie.Name, cookie.Value)
}

fmt.Println("Received from server:")
for _, cookie := range result.Response.Cookies {
    fmt.Printf("  ← %s = %s\n", cookie.Name, cookie.Value)
}
```

## Common Patterns

### Authentication Flow

```go
// Login request
loginResult, _ := client.Post("https://api.example.com/login",
    httpc.WithJSON(map[string]string{
        "username": "user",
        "password": "pass",
    }),
)

// Get session cookie from response
sessionCookie := loginResult.GetCookie("session")
if sessionCookie == nil {
    log.Fatal("Login failed: no session cookie")
}

// Use session cookie in subsequent requests
profileResult, _ := client.Get("https://api.example.com/profile",
    httpc.WithCookie(*sessionCookie),
)

// Verify session was sent
if !profileResult.HasRequestCookie("session") {
    log.Println("WARNING: session cookie not sent")
}
```

### Cookie Validation

```go
result, err := client.Get(url,
    httpc.WithCookieValue("required_cookie", "value"),
)

// Validate required cookies were sent
requiredCookies := []string{"required_cookie", "csrf_token"}
for _, name := range requiredCookies {
    if !result.HasRequestCookie(name) {
        log.Printf("ERROR: Required cookie '%s' was not sent", name)
    }
}
```

## API Design

All cookie inspection methods are available as Result methods for clean, intuitive API:

```go
// Result methods
cookies := result.Request.Cookies
cookie := result.GetRequestCookie("session")
exists := result.HasRequestCookie("session")
```

## See Also

- [Request Inspection](./request-inspection.md) - Detailed guide on inspecting requests
- [Cookie Examples](../examples/02_core_features/cookies.go) - More cookie examples
- [Configuration](./configuration.md) - Cookie jar configuration
