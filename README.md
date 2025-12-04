# HTTPC - Production-Ready HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](docs/security.md)
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
    fmt.Printf("Status: %d\n", resp.StatusCode)

    // POST with JSON and authentication
    user := map[string]string{"name": "John", "email": "john@example.com"}
    resp, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created: %s\n", resp.Body)
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
fmt.Printf("Status: %d\n", resp.StatusCode)
fmt.Printf("Body: %s\n", resp.Body)
fmt.Printf("Duration: %v\n", resp.Duration)
fmt.Printf("Attempts: %d\n", resp.Attempts)

// Work with cookies
cookie := resp.GetCookie("session_id")
```

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
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
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

**[📖 Security Guide](docs/security.md)**

## Documentation

### Guides
- **[Getting Started](docs/getting-started.md)** - Installation and first steps
- **[Configuration](docs/configuration.md)** - Client configuration and presets
- **[Request Options](docs/request-options.md)** - Complete options reference
- **[Error Handling](docs/error-handling.md)** - Error handling patterns
- **[File Download](docs/file-download.md)** - File downloads with progress
- **[Security](docs/security.md)** - Security features and best practices

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