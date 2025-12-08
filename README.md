# HTTPC - Production-Ready HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Performance](https://img.shields.io/badge/performance-high%20performance-green.svg)](https://github.com/cybergodev/json)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](https://github.com/cybergodev/json)

A high-performance HTTP client library for Go with enterprise-grade security, zero external dependencies, and production-ready defaults. Built for applications that demand reliability, security, and performance.

**[📖 中文文档](README_zh-CN.md)** | **[📚 Full Documentation](docs)**

---

## Why HTTPC?

- 🛡️ **Secure by Default** - TLS 1.2+, SSRF protection, CRLF injection prevention
- ⚡ **High Performance** - Connection pooling, HTTP/2, goroutine-safe operations
- 📊 **Built-in Resilience** - Smart retry with exponential backoff and jitter
- 🎯 **Developer Friendly** - Clean API, functional options, comprehensive error handling
- 🔧 **Zero Dependencies** - Pure Go stdlib, no external packages
- 🚀 **Production Ready** - Battle-tested defaults, extensive test coverage


## Quick Start

```bash
go get -u github.com/cybergodev/httpc
```

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // Simple GET request
    resp, err := httpc.Get("https://api.example.com/users")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d\n", resp.StatusCode())

    // POST with JSON and authentication
    user := map[string]string{"name": "John", "email": "john@example.com"}
    resp, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created: %s\n", resp.Body())
}
```

**[📖 See more examples](examples)** | **[🚀 Getting Started Guide](docs/getting-started.md)**

## Core Features

### HTTP Methods

All standard HTTP methods with clean, intuitive API:

```go
// GET - Retrieve data
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithBearerToken("token"),
)

// POST - Create resource
resp, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
    httpc.WithBearerToken("token"),
)

// PUT - Full update
resp, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),
)

// PATCH - Partial update
resp, err := httpc.Patch("https://api.example.com/users/123",
    httpc.WithJSON(map[string]string{"email": "new@example.com"}),
)

// DELETE - Remove resource
resp, err := httpc.Delete("https://api.example.com/users/123")

// HEAD, OPTIONS, and custom methods also supported
```

### Request Options

Customize requests using functional options (all options start with `With`):

```go
// Headers & Authentication
httpc.WithHeader("X-API-Key", "key")
httpc.WithBearerToken("token")
httpc.WithBasicAuth("user", "pass")

// Query Parameters
httpc.WithQuery("page", 1)
httpc.WithQueryMap(map[string]interface{}{"page": 1, "limit": 20})

// Request Body
httpc.WithJSON(data)              // JSON body
httpc.WithForm(formData)          // Form data
httpc.WithFile("file", "doc.pdf", content)  // File upload

// Cookies
httpc.WithCookieString("session=abc123; token=xyz789")  // Parse cookie string
httpc.WithCookieValue("name", "value")                  // Single cookie
httpc.WithCookie(cookie)                                // http.Cookie object
httpc.WithCookies(cookies)                              // Multiple cookies

// Redirects
httpc.WithFollowRedirects(false)  // Disable automatic redirect following
httpc.WithMaxRedirects(5)         // Limit maximum redirects (0-50)

// Timeout & Retry
httpc.WithTimeout(30*time.Second)
httpc.WithMaxRetries(3)
httpc.WithContext(ctx)

// Combine multiple options
resp, err := httpc.Post(url,
    httpc.WithJSON(data),
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(30*time.Second),
    httpc.WithMaxRetries(2),
)
```

**[📖 Complete Options Reference](docs/request-options.md)**

### Response Handling

```go
resp, err := httpc.Get(url)
if err != nil {
    log.Fatal(err)
}

// Status checking
if resp.IsSuccess() {        // 2xx
    fmt.Println("Success!")
}

// Parse JSON response
var result map[string]interface{}
if err := resp.JSON(&result); err != nil {
    log.Fatal(err)
}

// Access response data
fmt.Printf("Status: %d\n", resp.StatusCode())
fmt.Printf("Body: %s\n", resp.Body())
fmt.Printf("Duration: %v\n", resp.Meta.Duration)
fmt.Printf("Attempts: %d\n", resp.Meta.Attempts)

// Work with cookies
cookie := resp.GetCookie("session_id")
```

### Automatic Response Decompression

HTTPC automatically detects and decompresses compressed HTTP responses:

```go
// Request compressed response
resp, err := httpc.Get("https://api.example.com/data",
    httpc.WithHeader("Accept-Encoding", "gzip, deflate"),
)

// Response is automatically decompressed
fmt.Printf("Decompressed body: %s\n", resp.Body())
fmt.Printf("Original encoding: %s\n", resp.Response.Headers.Get("Content-Encoding"))
```

**Supported Encodings:**
- ✅ **gzip** - Fully supported (compress/gzip)
- ✅ **deflate** - Fully supported (compress/flate)

**Note:** Decompression is automatic when the server sends a `Content-Encoding` header. The library handles this transparently, so you always receive decompressed content.

### File Download

```go
// Simple download
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("Downloaded: %s at %s\n", 
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// Download with progress tracking
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s", percentage, httpc.FormatSpeed(speed))
}
result, err := httpc.DownloadWithOptions(url, opts)

// Resume interrupted downloads
opts.ResumeDownload = true
result, err := httpc.DownloadWithOptions(url, opts)

// Download with authentication
result, err := httpc.DownloadFile(url, "file.zip",
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(5*time.Minute),
)
```

**[📖 File Download Guide](docs/file-download.md)**

## Configuration

### Quick Start with Presets

```go
// Default - Balanced for production (recommended)
client, err := httpc.New()

// Secure - Maximum security (strict validation, minimal retries)
client, err := httpc.NewSecure()

// Performance - Optimized for high throughput
client, err := httpc.NewPerformance()

// Minimal - Lightweight for simple requests
client, err := httpc.NewMinimal()

// Testing - Permissive for development (NEVER use in production)
client, err := httpc.New(httpc.TestingConfig())
```

### Custom Configuration

```go
config := &httpc.Config{
    Timeout:             30 * time.Second,
    MaxRetries:          3,
    MaxIdleConns:        100,
    MaxConnsPerHost:     20,
    MinTLSVersion:       tls.VersionTLS12,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
    UserAgent:           "MyApp/1.0",
    EnableHTTP2:         true,
}
client, err := httpc.New(config)
```

**[📖 Configuration Guide](docs/configuration.md)**

## Error Handling

```go
resp, err := httpc.Get(url)
if err != nil {
    // Check for specific error types
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Status)
    }
    
    // Check for timeout
    if strings.Contains(err.Error(), "timeout") {
        return fmt.Errorf("request timed out")
    }
    
    return err
}

// Check response status
if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode())
}
```

**[📖 Error Handling Guide](docs/error-handling.md)**

## Advanced Features

### Client Lifecycle Management

```go
// Create reusable client
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()  // Always close to release resources

// Or use package-level functions (auto-managed)
defer httpc.CloseDefaultClient()
resp, err := httpc.Get(url)
```

### Automatic Retries

```go
// Configure at client level
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.BackoffFactor = 2.0
client, err := httpc.New(config)

// Or per-request
resp, err := httpc.Get(url, httpc.WithMaxRetries(5))
```

### Context Support

```go
// Timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
resp, err := client.Get(url, httpc.WithContext(ctx))

// Cancellation
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel()
}()
resp, err := client.Get(url, httpc.WithContext(ctx))
```

### HTTP Redirects

```go
// Automatic redirect following (default)
resp, err := httpc.Get("https://example.com/redirect")
fmt.Printf("Followed %d redirects\n", resp.Meta.RedirectCount)

// Disable redirects for specific request
resp, err := httpc.Get(url, httpc.WithFollowRedirects(false))
if resp.IsRedirect() {
    fmt.Printf("Redirect to: %s\n", resp.Response.Headers.Get("Location"))
}

// Limit redirects
resp, err := httpc.Get(url, httpc.WithMaxRedirects(5))

// Track redirect chain
for i, url := range resp.Meta.RedirectChain {
    fmt.Printf("%d. %s\n", i+1, url)
}
```

**[📖 Redirect Guide](docs/redirects.md)**

### Cookie Management

```go
// Automatic cookie handling
config := httpc.DefaultConfig()
config.EnableCookies = true
client, err := httpc.New(config)

// Login sets cookies
client.Post("https://example.com/login", httpc.WithForm(credentials))

// Subsequent requests include cookies automatically
client.Get("https://example.com/profile")

// Manual cookie setting
// Parse cookie string (from browser dev tools or server response)
resp, err := httpc.Get("https://api.example.com/data",
    httpc.WithCookieString("PSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc"),
)

// Set individual cookies
resp, err = httpc.Get("https://api.example.com/data",
    httpc.WithCookieValue("session", "abc123"),
    httpc.WithCookieValue("token", "xyz789"),
)

// Use http.Cookie objects for advanced settings
cookie := &http.Cookie{
    Name:     "secure_session",
    Value:    "encrypted_value",
    Secure:   true,
    HttpOnly: true,
    SameSite: http.SameSiteStrictMode,
}
resp, err = httpc.Get("https://api.example.com/data", httpc.WithCookie(cookie))
```

**[📖 Cookie API Reference](docs/cookie-api-reference.md)**

### Domain Client - Automatic State Management

For applications that make multiple requests to the same domain, `DomainClient` provides automatic Cookie and Header management:

```go
// Create domain-specific client
client, err := httpc.NewDomain("https://api.example.com")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// First request - server sets cookies
resp1, err := client.Get("/login",
    httpc.WithJSON(credentials),
)

// Cookies from resp1 are automatically saved and sent in subsequent requests
resp2, err := client.Get("/profile")  // Cookies automatically included

// Set persistent headers (sent with all requests)
client.SetHeader("Authorization", "Bearer "+token)
client.SetHeader("X-API-Key", "your-api-key")

// All subsequent requests include these headers
resp3, err := client.Get("/data")  // Headers + Cookies automatically included

// Override per-request (doesn't affect persistent state)
resp4, err := client.Get("/special",
    httpc.WithHeader("Accept", "application/xml"),  // Override for this request only
)

// Manual cookie management
client.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})
client.SetCookies([]*http.Cookie{
    {Name: "pref", Value: "dark"},
    {Name: "lang", Value: "en"},
})

// Query state
cookies := client.GetCookies()
headers := client.GetHeaders()
sessionCookie := client.GetCookie("session")

// Clear state
client.DeleteCookie("session")
client.DeleteHeader("X-API-Key")
client.ClearCookies()
client.ClearHeaders()
```

**Real-World Example - Login Flow:**

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// Step 1: Login (server sets session cookie)
loginResp, _ := client.Post("/auth/login",
    httpc.WithJSON(map[string]string{
        "username": "user@example.com",
        "password": "secret",
    }),
)

// Step 2: Extract token and set as persistent header
var loginData map[string]string
loginResp.JSON(&loginData)
client.SetHeader("Authorization", "Bearer "+loginData["token"])

// Step 3: Make API calls (cookies + auth header automatically sent)
profileResp, _ := client.Get("/api/user/profile")
dataResp, _ := client.Get("/api/user/data")
settingsResp, _ := client.Put("/api/user/settings",
    httpc.WithJSON(newSettings),
)

// All requests automatically include:
// - Session cookies from login response
// - Authorization header
// - Any other persistent headers/cookies
```

**Key Features:**
- **Automatic Cookie Persistence** - Cookies from responses are saved and sent in subsequent requests
- **Automatic Header Persistence** - Set headers once, used in all requests
- **Per-Request Overrides** - Use `WithCookies()` and `WithHeaderMap()` to override for specific requests
- **Thread-Safe** - All operations are goroutine-safe
- **Manual Control** - Full API for inspecting and modifying state

**[📖 See full example](examples/domain_client_example.go)**

## Security & Performance

### Security Features
- **TLS 1.2+ by default** - Modern encryption standards
- **SSRF Protection** - Pre-DNS and post-DNS validation blocks private IPs
- **CRLF Injection Prevention** - Header and URL validation
- **Input Validation** - Comprehensive validation of all user inputs
- **Path Traversal Protection** - Safe file operations
- **Configurable Limits** - Response size, timeout, connection limits

### Performance Optimizations
- **Connection Pooling** - Efficient connection reuse with per-host limits
- **HTTP/2 Support** - Multiplexing for better performance
- **Goroutine-Safe** - All operations thread-safe with atomic operations
- **Smart Retry** - Exponential backoff with jitter reduces server load
- **Memory Efficient** - Configurable limits prevent memory exhaustion

### Concurrency Safety

HTTPC is designed for concurrent use from the ground up:

```go
// ✅ Safe: Share a single client across goroutines
client, _ := httpc.New()
defer client.Close()

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        resp, _ := client.Get("https://api.example.com")
        // Process response...
    }()
}
wg.Wait()
```

**Thread Safety Guarantees:**
- ✅ All `Client` methods are safe for concurrent use
- ✅ Package-level functions (`Get`, `Post`, etc.) use a shared default client safely
- ✅ Response objects can be read from multiple goroutines after return
- ✅ Internal metrics and connection pools use atomic operations
- ✅ Config is deep-copied on client creation to prevent modification issues

**Best Practices:**
- Create one client and reuse it across your application
- Don't modify `Config` after passing it to `New()`
- Response objects are safe to read but shouldn't be modified concurrently

**Testing:** Run `make test-race` to verify race-free operation in your code.

**[📖 Security Guide](SECURITY.md)**

## Documentation

### Guides
- **[Getting Started](docs/getting-started.md)** - Installation and first steps
- **[Configuration](docs/configuration.md)** - Client configuration and presets
- **[Request Options](docs/request-options.md)** - Complete options reference
- **[Error Handling](docs/error-handling.md)** - Error handling patterns
- **[File Download](docs/file-download.md)** - File downloads with progress
- **[HTTP Redirects](docs/redirects.md)** - Redirect handling and tracking
- **[Request Inspection](docs/request-inspection.md)** - Inspect request details
- **[Security](SECURITY.md)** - Security features and best practices

### Examples
- **[Quick Start](examples/01_quickstart)** - Basic usage
- **[Core Features](examples/02_core_features)** - Headers, auth, body formats
- **[Advanced](examples/03_advanced)** - File uploads, downloads, retries
- **[Real World](examples/04_real_world)** - Complete API client

## Contributing

Contributions welcome! Please open an issue first for major changes.

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

**Made with ❤️ by the CyberGoDev team**