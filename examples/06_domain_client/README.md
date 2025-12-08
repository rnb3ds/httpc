# DomainClient Examples

This directory contains examples demonstrating the `DomainClient` feature, which provides automatic cookie and header management for requests to a specific domain.

## What is DomainClient?

`DomainClient` is a specialized HTTP client that:
- Automatically manages cookies per domain
- Persists headers across requests
- Handles both relative and absolute URLs
- Maintains state throughout a session
- Simplifies multi-request workflows

## Examples

### 1. domain_client_example.go
**Complete DomainClient Usage Guide**

Comprehensive examples covering all DomainClient features:
- Basic usage with automatic cookie management
- Automatic header management
- Real-world login scenario
- Manual cookie and header management

```bash
go run -tags examples examples/06_domain_client/domain_client_example.go
```

**Key Features:**
- Automatic cookie persistence from responses
- Persistent headers across requests
- Per-request overrides
- Manual state management API

---

### 2. domain_client_demo.go
**Interactive Demo**

A practical demonstration showing DomainClient in action:
- Setting persistent headers
- Automatic cookie handling
- State management
- Clearing cookies and headers

```bash
go run -tags examples examples/06_domain_client/domain_client_demo.go
```

**Demonstrates:**
- Real-world usage patterns
- State inspection
- Cookie and header lifecycle

---

### 3. domain_client_url_matching.go
**URL Handling and Auto-Persist**

Detailed examples of how DomainClient handles different URL formats:
- Relative paths (with/without leading slash)
- Full URLs with same domain
- Full URLs with different domains
- Auto-persist behavior for request options

```bash
go run -tags examples examples/06_domain_client/domain_client_url_matching.go
```

**URL Patterns:**
- `client.Get("path")` → `https://domain.com/path`
- `client.Get("/path")` → `https://domain.com/path`
- `client.Get("https://domain.com/path")` → Uses client config
- `client.Get("https://other.com/path")` → Still works, different domain

---

## Core Concepts

### Automatic Cookie Management

```go
client, _ := httpc.NewDomain("https://api.example.com")

// Server sets cookies in response
resp1, _ := client.Post("/login", httpc.WithJSON(credentials))

// Cookies automatically sent in subsequent requests
resp2, _ := client.Get("/profile")  // Includes login cookies
resp3, _ := client.Get("/data")     // Still includes cookies
```

### Automatic Header Management

```go
client, _ := httpc.NewDomain("https://api.example.com")

// Set persistent headers
client.SetHeader("Authorization", "Bearer token123")
client.SetHeader("X-API-Key", "your-key")

// All requests automatically include these headers
client.Get("/endpoint1")  // Has both headers
client.Get("/endpoint2")  // Has both headers
```

### Auto-Persist Request Options

```go
client, _ := httpc.NewDomain("https://api.example.com")

// First request with options
client.Get("/login",
    httpc.WithCookieValue("session", "abc"),
    httpc.WithHeader("X-Device", "mobile"),
)

// Options are automatically persisted!
client.Get("/data")  // Automatically includes session cookie and X-Device header
```

### Per-Request Overrides

```go
client, _ := httpc.NewDomain("https://api.example.com")
client.SetHeader("Accept", "application/json")

// Override for specific request (doesn't affect persistent state)
client.Get("/xml-endpoint",
    httpc.WithHeader("Accept", "application/xml"),
)

// Next request uses original persistent header
client.Get("/json-endpoint")  // Accept: application/json
```

## API Reference

### Creating a DomainClient

```go
// With default config
client, err := httpc.NewDomain("https://api.example.com")

// With custom config
config := httpc.DefaultConfig()
config.Timeout = 30 * time.Second
config.EnableCookies = true
client, err := httpc.NewDomain("https://api.example.com", config)
```

### HTTP Methods

```go
client.Get(path, options...)
client.Post(path, options...)
client.Put(path, options...)
client.Patch(path, options...)
client.Delete(path, options...)
client.Head(path, options...)
client.Options(path, options...)
```

### Header Management

```go
// Set single header
client.SetHeader(key, value)

// Set multiple headers
client.SetHeaders(map[string]string{
    "Authorization": "Bearer token",
    "X-API-Key": "key",
})

// Get headers
headers := client.GetHeaders()

// Delete header
client.DeleteHeader(key)

// Clear all headers
client.ClearHeaders()
```

### Cookie Management

```go
// Set single cookie
client.SetCookie(&http.Cookie{
    Name:  "session",
    Value: "abc123",
})

// Set multiple cookies
client.SetCookies([]*http.Cookie{...})

// Get cookies
cookies := client.GetCookies()
cookie := client.GetCookie(name)

// Delete cookie
client.DeleteCookie(name)

// Clear all cookies
client.ClearCookies()
```

### Resource Cleanup

```go
defer client.Close()  // Always close when done
```

## Use Cases

### 1. API Client with Authentication

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// Login
loginResp, _ := client.Post("/auth/login",
    httpc.WithJSON(credentials),
)

// Extract token
var data map[string]string
loginResp.JSON(&data)

// Set auth header for all subsequent requests
client.SetHeader("Authorization", "Bearer "+data["token"])

// All API calls now authenticated
client.Get("/users")
client.Get("/posts")
client.Post("/comments", httpc.WithJSON(comment))
```

### 2. Web Scraping with Session

```go
client, _ := httpc.NewDomain("https://www.example.com")
defer client.Close()

// Set browser-like headers
client.SetHeaders(map[string]string{
    "User-Agent": "Mozilla/5.0...",
    "Accept-Language": "en-US,en;q=0.9",
})

// First request establishes session
client.Get("/")

// Subsequent requests maintain session
client.Get("/page1")
client.Get("/page2")
client.Post("/form", httpc.WithForm(data))
```

### 3. Multi-Step Workflow

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// Step 1: Get CSRF token
resp1, _ := client.Get("/csrf-token")
var csrf map[string]string
resp1.JSON(&csrf)

// Step 2: Set CSRF header
client.SetHeader("X-CSRF-Token", csrf["token"])

// Step 3: Make protected requests
client.Post("/create", httpc.WithJSON(data))
client.Put("/update/123", httpc.WithJSON(updates))
client.Delete("/delete/456")
```

## Advantages

### vs Regular Client

**Regular Client:**
```go
client, _ := httpc.New()

// Must manually manage cookies
resp1, _ := client.Post("/login", httpc.WithJSON(creds))
cookies := resp1.Response.Cookies

// Must manually include cookies in each request
client.Get("/data", httpc.WithCookies(cookies))
client.Get("/profile", httpc.WithCookies(cookies))
```

**DomainClient:**
```go
client, _ := httpc.NewDomain("https://api.example.com")

// Cookies automatically managed
client.Post("/login", httpc.WithJSON(creds))

// Cookies automatically included
client.Get("/data")
client.Get("/profile")
```

### vs Cookie Jar

**Cookie Jar** (regular client):
- Manages cookies automatically
- Works across all domains
- No header persistence
- No manual state control

**DomainClient**:
- Manages cookies automatically
- Domain-specific
- Persists headers
- Full manual state control
- Auto-persists request options

## Best Practices

1. **Always Close the Client**
   ```go
   client, _ := httpc.NewDomain(baseURL)
   defer client.Close()
   ```

2. **Set Common Headers Once**
   ```go
   client.SetHeaders(map[string]string{
       "User-Agent": "MyApp/1.0",
       "Accept": "application/json",
   })
   ```

3. **Use for Same-Domain Requests**
   - Perfect for API clients
   - Great for web scraping
   - Ideal for multi-step workflows

4. **Inspect State When Debugging**
   ```go
   fmt.Printf("Headers: %d\n", len(client.GetHeaders()))
   fmt.Printf("Cookies: %d\n", len(client.GetCookies()))
   ```

5. **Clear State Between Sessions**
   ```go
   client.ClearCookies()
   client.ClearHeaders()
   ```

## Thread Safety

`DomainClient` is **thread-safe**. All methods can be called concurrently from multiple goroutines:

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        client.Get(fmt.Sprintf("/endpoint/%d", id))
    }(i)
}
wg.Wait()
```

## Related Examples

- **[02_core_features/cookies.go](../02_core_features/cookies.go)** - Basic cookie usage
- **[05_cookies/](../05_cookies/)** - Cookie management patterns
- **[04_real_world/rest_api_client.go](../04_real_world/rest_api_client.go)** - Complete API client

## When to Use DomainClient

**Use DomainClient when:**
- ✅ Making multiple requests to the same domain
- ✅ Need automatic cookie management
- ✅ Want persistent headers across requests
- ✅ Building an API client
- ✅ Implementing web scraping
- ✅ Need session management

**Use Regular Client when:**
- ✅ Making requests to multiple different domains
- ✅ Need fine-grained control over each request
- ✅ Don't need state persistence
- ✅ One-off requests
