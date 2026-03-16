# HTTPC - Production-Ready HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Zero Deps](https://img.shields.io/badge/deps-zero-brightgreen.svg)](go.mod)

A high-performance HTTP client library for Go with enterprise-grade security, zero external dependencies, and production-ready defaults.

**[中文文档](README_zh-CN.md)**

---

## Features

| Feature | Description |
|---------|-------------|
| **Secure by Default** | TLS 1.2+, SSRF protection, CRLF injection prevention |
| **High Performance** | Connection pooling, HTTP/2, goroutine-safe, sync.Pool optimization |
| **Built-in Resilience** | Smart retry with exponential backoff and jitter |
| **Developer Friendly** | Clean API, intuitive options pattern, comprehensive documentation |
| **Zero Dependencies** | Pure Go standard library, no external packages |
| **Production Ready** | Battle-tested defaults, extensive test coverage |

---

## Installation

```bash
go get -u github.com/cybergodev/httpc
```

---

## Quick Start (5 Minutes)

### Simple GET Request

```go
package main

import (
    "fmt"
    "log"

    "github.com/cybergodev/httpc"
)

func main() {
    // Package-level function - convenient for simple requests
    result, err := httpc.Get("https://httpbin.org/get")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d, Duration: %v\n", result.StatusCode(), result.Meta.Duration)
}
```

### POST JSON with Authentication

```go
user := map[string]string{"name": "John", "email": "john@example.com"}
result, err := httpc.Post("https://httpbin.org/post",
    httpc.WithJSON(user),
    httpc.WithBearerToken("your-token"),
    httpc.WithTimeout(30*time.Second),
)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Response: %s\n", result.Body())
```

---

## HTTP Methods

```go
// GET with query parameters
result, _ := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQueryMap(map[string]any{"limit": 20}),
)

// POST JSON body
result, _ := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(map[string]string{"name": "John"}),
)

// PUT / PATCH / DELETE
result, _ := httpc.Put(url, httpc.WithJSON(data))
result, _ := httpc.Patch(url, httpc.WithJSON(partialData))
result, _ := httpc.Delete(url)
```

---

## Request Options

| Category | Options |
|----------|---------|
| **Headers** | `WithHeader(key, value)`, `WithHeaderMap(map)`, `WithUserAgent(ua)` |
| **Auth** | `WithBearerToken(token)`, `WithBasicAuth(user, pass)` |
| **Query** | `WithQuery(key, value)`, `WithQueryMap(map)` |
| **Body** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithBinary([]byte)` |
| **Files** | `WithFile(field, filename, content)`, `WithFormData(form)` |
| **Cookies** | `WithCookie(cookie)`, `WithCookieString("a=1; b=2")` |
| **Control** | `WithTimeout(dur)`, `WithMaxRetries(n)`, `WithContext(ctx)` |
| **Redirects** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **Callbacks** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

---

## Response Handling

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// Quick access
fmt.Println(result.StatusCode())     // 200
fmt.Println(result.RawBody())        // Response body ([]byte)
fmt.Println(result.Body())           // Response body (string)

// Status checks
if result.IsSuccess() { }            // 2xx
if result.IsClientError() { }        // 4xx
if result.IsServerError() { }        // 5xx

// Parse JSON response
var data map[string]interface{}
result.Unmarshal(&data)

// Metadata
fmt.Println(result.Meta.Duration)      // Request duration
fmt.Println(result.Meta.Attempts)      // Retry count
fmt.Println(result.Meta.RedirectCount) // Redirect count
```

---

## Context & Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := httpc.Get("https://api.example.com",
    httpc.WithContext(ctx),
)
if errors.Is(err, context.DeadlineExceeded) {
    fmt.Println("Request timed out")
}
```

---

## File Download

```go
// Simple download
result, _ := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("Downloaded: %s at %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// Download with progress
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    pct := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r%.1f%% - %s/s", pct, httpc.FormatSpeed(speed))
}
result, _ := httpc.DownloadWithOptions(url, opts)
```

---

## Domain Client (Session Management)

For multiple requests to the same domain with automatic cookie and header management:

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// Login - server sets cookies
client.Post("/login", httpc.WithJSON(credentials))

// Set persistent header (used for all requests)
client.SetHeader("Authorization", "Bearer "+token)

// Subsequent requests include cookies + headers automatically
profile, _ := client.Get("/profile")
data, _ := client.Get("/data")
```

---

## Error Handling

```go
result, err := httpc.Get(url)
if err != nil {
    var clientErr *httpc.ClientError
    if errors.As(err, &clientErr) {
        fmt.Printf("Error: %s (code: %s)\n", clientErr.Message, clientErr.Code())
        fmt.Printf("Retryable: %v\n", clientErr.IsRetryable())
    }
    return err
}

// Check response status
if !result.IsSuccess() {
    return fmt.Errorf("unexpected status code: %d", result.StatusCode())
}
```

---

## Configuration

### Preset Configurations

```go
// Production-ready defaults (recommended)
client, _ := httpc.New(httpc.DefaultConfig())

// Maximum security (SSRF protection enabled)
client, _ := httpc.New(httpc.SecureConfig())

// High throughput
client, _ := httpc.New(httpc.PerformanceConfig())

// Lightweight (no retries)
client, _ := httpc.New(httpc.MinimalConfig())

// Testing only - disables security features!
client, _ := httpc.New(httpc.TestingConfig())
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
client, _ := httpc.New(config)
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Timeout` | `time.Duration` | `30s` | Overall request timeout |
| `DialTimeout` | `time.Duration` | `10s` | TCP connection timeout |
| `MaxIdleConns` | `int` | `50` | Max idle connections |
| `MaxConnsPerHost` | `int` | `10` | Max connections per host |
| `MaxRetries` | `int` | `3` | Max retry attempts |
| `RetryDelay` | `time.Duration` | `1s` | Initial retry delay |
| `BackoffFactor` | `float64` | `2.0` | Backoff multiplier |
| `EnableJitter` | `bool` | `true` | Add jitter to retries |
| `ProxyURL` | `string` | `""` | Proxy URL (http/socks5) |
| `EnableSystemProxy` | `bool` | `false` | Auto-detect system proxy |
| `EnableHTTP2` | `bool` | `true` | Enable HTTP/2 |
| `EnableCookies` | `bool` | `false` | Enable cookie jar |
| `MinTLSVersion` | `uint16` | `TLS 1.2` | Minimum TLS version |
| `MaxResponseBodySize` | `int64` | `10MB` | Max response body size |
| `UserAgent` | `string` | `"httpc/1.0"` | Default User-Agent |
| `FollowRedirects` | `bool` | `true` | Follow redirects |
| `MaxRedirects` | `int` | `10` | Max redirect count |
| `AllowPrivateIPs` | `bool` | `true` | Allow private IPs (SSRF) |

---

## Middleware

### Built-in Middleware

```go
// Request logging
httpc.LoggingMiddleware(log.Printf)

// Panic recovery
httpc.RecoveryMiddleware()

// Request ID
httpc.RequestIDMiddleware("X-Request-ID", nil)

// Timeout enforcement
httpc.TimeoutMiddleware(30*time.Second)

// Metrics collection
httpc.MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {})

// Security audit
httpc.AuditMiddleware(func(a httpc.AuditEvent) {})
```

### Chain Multiple Middleware

```go
chainedMiddleware := httpc.Chain(
    httpc.RecoveryMiddleware(),
    httpc.LoggingMiddleware(log.Printf),
    httpc.RequestIDMiddleware("X-Request-ID", nil),
)
config.Middlewares = []httpc.MiddlewareFunc{chainedMiddleware}
```

### Custom Middleware

```go
func CustomMiddleware() httpc.MiddlewareFunc {
    return func(next httpc.Handler) httpc.Handler {
        return func(ctx context.Context, req httpc.RequestMutator) (httpc.ResponseMutator, error) {
            // Before request
            req.SetHeader("X-Custom", "value")

            // Call next handler
            resp, err := next(ctx, req)

            // After response
            return resp, err
        }
    }
}
```

---

## Proxy Configuration

```go
// Manual proxy
config := &httpc.Config{
    ProxyURL: "http://127.0.0.1:8080",
    // Or SOCKS5: "socks5://127.0.0.1:1080"
}

// System proxy auto-detection (Windows/macOS/Linux)
config := &httpc.Config{
    EnableSystemProxy: true,  // Reads from environment and system settings
}
```

---

## Security Features

| Feature | Description |
|---------|-------------|
| **TLS 1.2+** | Modern encryption standards by default |
| **SSRF Protection** | DNS validation blocks private IPs |
| **CRLF Injection Prevention** | Header and URL validation |
| **Path Traversal Protection** | Safe file operations |
| **Domain Whitelist** | Restrict redirects to allowed domains |
| **Response Size Limit** | Configurable limit to prevent memory exhaustion |

### Redirect Domain Whitelist

```go
config := &httpc.Config{
    RedirectWhitelist: []string{"api.example.com", "secure.example.com"},
}
```

### SSRF Protection

By default, `AllowPrivateIPs` is `true` for compatibility. Enable SSRF protection when making requests to user-provided URLs:

```go
// Enable SSRF protection
cfg := httpc.DefaultConfig()
cfg.AllowPrivateIPs = false
client, _ := httpc.New(cfg)

// Or use the secure preset
client, _ := httpc.New(httpc.SecureConfig())
```

---

## Concurrency Safety

HTTPC is designed to be goroutine-safe:

```go
client, _ := httpc.New()
defer client.Close()

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        result, _ := client.Get("https://api.example.com")
        // Process response...
    }()
}
wg.Wait()
```

### Performance Optimization

```go
// Release Result back to pool after use (reduces GC pressure)
result, _ := httpc.Get(url)
defer httpc.ReleaseResult(result)
```

**Thread Safety Guarantees:**
- All `Client` methods are safe for concurrent use
- Package-level functions safely use a shared default client
- Response objects can be safely read from multiple goroutines
- Internal metrics use atomic operations

---

## Documentation

| Resource | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Installation and first steps |
| [Configuration](docs/configuration.md) | Client configuration and presets |
| [Request Options](docs/request-options.md) | Complete options reference |
| [Error Handling](docs/error-handling.md) | Error handling patterns |
| [File Download](docs/file-download.md) | File download with progress |
| [HTTP Redirects](docs/redirects.md) | Redirect handling and tracking |
| [Security](SECURITY.md) | Security features and best practices |

### Example Code

| Directory | Description |
|-----------|-------------|
| [01_quickstart](examples/01_quickstart) | Basic usage |
| [02_core_features](examples/02_core_features) | Headers, auth, body formats |
| [03_advanced](examples/03_advanced) | File upload, download, retry, middleware |

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

If this project helps you, please give it a Star! ⭐
