# HTTPC - Production-Ready HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Zero Deps](https://img.shields.io/badge/deps-zero-brightgreen.svg)](go.mod)

A high-performance HTTP client library for Go with enterprise-grade security, zero external dependencies, and production-ready defaults.

**[中文文档](README_zh-CN.md)**

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| 🔒 **Secure by Default** | TLS 1.2+, SSRF protection, CRLF injection prevention |
| ⚡ **High Performance** | Connection pooling, HTTP/2, goroutine-safe, sync.Pool optimization |
| 🔄 **Built-in Resilience** | Smart retry with exponential backoff and jitter |
| 🛠️ **Developer Friendly** | Clean API, intuitive options pattern, comprehensive documentation |
| 📦 **Zero Dependencies** | Pure Go standard library, no external packages |
| ✅ **Production Ready** | Battle-tested defaults, extensive test coverage |

---

## 📦 Installation

```bash
go get -u github.com/cybergodev/httpc
```

---

## 🚀 Quick Start (5 Minutes)

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
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/cybergodev/httpc"
)

func main() {
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
}
```

---

## 📖 HTTP Methods

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

// HEAD / OPTIONS
result, _ := httpc.Head(url)
result, _ := httpc.Options(url)
```

---

## 🔧 Request Options

### Headers

```go
// Single header
httpc.WithHeader("Authorization", "Bearer token")

// Multiple headers
httpc.WithHeaderMap(map[string]string{
    "X-Custom": "value",
    "X-Request-ID": "123",
})

// User-Agent
httpc.WithUserAgent("my-app/1.0")
```

### Authentication

```go
// Bearer token
httpc.WithBearerToken("your-jwt-token")

// Basic auth
httpc.WithBasicAuth("username", "password")
```

### Query Parameters

```go
// Single parameter
httpc.WithQuery("page", 1)

// Multiple parameters
httpc.WithQueryMap(map[string]any{"page": 1, "limit": 20})
```

### Request Body

```go
// JSON
httpc.WithJSON(data)

// XML
httpc.WithXML(data)

// Form data (application/x-www-form-urlencoded)
httpc.WithForm(map[string]string{"key": "value"})

// Multipart form data (file upload)
httpc.WithFormData(formData)
httpc.WithFile("file", "document.pdf", fileBytes)

// Raw body
httpc.WithBody([]byte("raw data"))
httpc.WithBinary(binaryData, "application/pdf")
```

### Cookies

```go
// Single cookie
httpc.WithCookie(http.Cookie{Name: "session", Value: "abc123"})

// Multiple cookies from map
httpc.WithCookieMap(map[string]string{
    "session_id": "abc123",
    "user_pref":  "dark_mode",
})

// Cookie string
httpc.WithCookieString("session=abc123; token=xyz")
```

### Request Control

```go
// Context for cancellation
httpc.WithContext(ctx)

// Timeout
httpc.WithTimeout(30 * time.Second)

// Retry configuration
httpc.WithMaxRetries(3)

// Redirect control
httpc.WithFollowRedirects(false)
httpc.WithMaxRedirects(5)
```

### Callbacks

```go
// Before request
httpc.WithOnRequest(func(req httpc.RequestMutator) error {
    log.Printf("Sending %s %s", req.Method(), req.URL())
    return nil
})

// After response
httpc.WithOnResponse(func(resp httpc.ResponseMutator) error {
    log.Printf("Received %d", resp.StatusCode())
    return nil
})
```

### Complete Options Reference

| Category | Options |
|----------|---------|
| **Headers** | `WithHeader(key, value)`, `WithHeaderMap(map)`, `WithUserAgent(ua)` |
| **Auth** | `WithBearerToken(token)`, `WithBasicAuth(user, pass)` |
| **Query** | `WithQuery(key, value)`, `WithQueryMap(map)` |
| **Body** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithFormData(form)`, `WithFile(field, filename, content)`, `WithBody([]byte)`, `WithBinary([]byte, contentType?)` |
| **Cookies** | `WithCookie(cookie)`, `WithCookieMap(map)`, `WithCookieString("a=1; b=2")`, `WithSecureCookie(config)` |
| **Control** | `WithTimeout(dur)`, `WithMaxRetries(n)`, `WithContext(ctx)` |
| **Redirects** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **Callbacks** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

---

## 📥 Response Handling

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// Quick access
fmt.Println(result.StatusCode())     // 200
fmt.Println(result.Proto())          // "HTTP/1.1" or "HTTP/2.0"
fmt.Println(result.RawBody())        // Response body ([]byte)
fmt.Println(result.Body())           // Response body (string)

// Status checks
if result.IsSuccess() { }            // 2xx
if result.IsRedirect() { }           // 3xx
if result.IsClientError() { }        // 4xx
if result.IsServerError() { }        // 5xx

// Parse JSON response
var data map[string]interface{}
result.Unmarshal(&data)

// Cookie access
cookie := result.GetCookie("session")
if result.HasCookie("session") { }

// Request cookies sent
reqCookie := result.GetRequestCookie("token")

// Save response to file
result.SaveToFile("response.json")

// Metadata
fmt.Println(result.Meta.Duration)      // Request duration
fmt.Println(result.Meta.Attempts)      // Retry count
fmt.Println(result.Meta.RedirectCount) // Redirect count
fmt.Println(result.Meta.RedirectChain) // Redirect URLs

// String representation (safe for logging)
fmt.Println(result.String())
```

---

## ⏱️ Context & Cancellation

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

## 📁 File Download

### Simple Download

```go
result, _ := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("Downloaded: %s at %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))
```

### Download with Progress

```go
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    pct := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r%.1f%% - %s/s", pct, httpc.FormatSpeed(speed))
}
result, _ := httpc.DownloadWithOptions(url, opts)
```

### Resume Download

```go
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ResumeDownload = true
result, _ := httpc.DownloadWithOptions(url, opts)
if result.Resumed {
    fmt.Println("Download resumed from previous position")
}
```

---

## 🌐 Domain Client (Session Management)

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

// Cookie management
client.SetCookie(&http.Cookie{Name: "session", Value: "abc"})
client.GetCookie("session")
client.DeleteCookie("session")
client.ClearCookies()

// Header management
client.SetHeaders(map[string]string{"X-App": "v1"})
client.GetHeaders()
client.DeleteHeader("X-Old")
client.ClearHeaders()
```

---

## ⚠️ Error Handling

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

### Error Types

```go
const (
    ErrorTypeUnknown        // Unknown or unclassified error
    ErrorTypeNetwork        // Network-level error (connection refused, DNS failure)
    ErrorTypeTimeout        // Request timeout
    ErrorTypeContextCanceled // Context canceled
    ErrorTypeResponseRead   // Error reading response body
    ErrorTypeTransport      // HTTP transport error
    ErrorTypeRetryExhausted // All retries exhausted
    ErrorTypeTLS            // TLS handshake error
    ErrorTypeCertificate    // Certificate validation error
    ErrorTypeDNS            // DNS resolution error
    ErrorTypeValidation     // Request validation error
    ErrorTypeHTTP           // HTTP-level error (4xx, 5xx)
)
```

---

## ⚙️ Configuration

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
    // Timeouts
    Timeout:               30 * time.Second,
    DialTimeout:           10 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    IdleConnTimeout:       90 * time.Second,

    // Connection
    MaxIdleConns:      100,
    MaxConnsPerHost:   20,
    EnableHTTP2:       true,
    EnableCookies:     false,

    // Security
    MinTLSVersion:       tls.VersionTLS12,
    MaxTLSVersion:       tls.VersionTLS13,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
    AllowPrivateIPs:     true,

    // Retry
    MaxRetries:    3,
    RetryDelay:    1 * time.Second,
    BackoffFactor: 2.0,
    EnableJitter:  true,

    // Other
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    MaxRedirects:    10,
}
client, _ := httpc.New(config)
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| **Timeouts** ||||
| `Timeout` | `time.Duration` | `30s` | Overall request timeout |
| `DialTimeout` | `time.Duration` | `10s` | TCP connection timeout |
| `TLSHandshakeTimeout` | `time.Duration` | `10s` | TLS handshake timeout |
| `ResponseHeaderTimeout` | `time.Duration` | `30s` | Response header timeout |
| `IdleConnTimeout` | `time.Duration` | `90s` | Idle connection timeout |
| **Connection** ||||
| `MaxIdleConns` | `int` | `50` | Max idle connections |
| `MaxConnsPerHost` | `int` | `10` | Max connections per host |
| `ProxyURL` | `string` | `""` | Proxy URL (http/socks5) |
| `EnableSystemProxy` | `bool` | `false` | Auto-detect system proxy |
| `EnableHTTP2` | `bool` | `true` | Enable HTTP/2 |
| `EnableCookies` | `bool` | `false` | Enable cookie jar |
| `EnableDoH` | `bool` | `false` | Enable DNS-over-HTTPS |
| `DoHCacheTTL` | `time.Duration` | `5m` | DoH cache duration |
| **Security** ||||
| `TLSConfig` | `*tls.Config` | `nil` | Custom TLS config |
| `MinTLSVersion` | `uint16` | `TLS 1.2` | Minimum TLS version |
| `MaxTLSVersion` | `uint16` | `TLS 1.3` | Maximum TLS version |
| `InsecureSkipVerify` | `bool` | `false` | Skip TLS verification (testing only!) |
| `MaxResponseBodySize` | `int64` | `10MB` | Max response body size |
| `AllowPrivateIPs` | `bool` | `true` | Allow private IPs (SSRF) |
| `ValidateURL` | `bool` | `true` | Enable URL validation |
| `ValidateHeaders` | `bool` | `true` | Enable header validation |
| `StrictContentLength` | `bool` | `true` | Strict content-length check |
| `RedirectWhitelist` | `[]string` | `nil` | Allowed redirect domains |
| **Retry** ||||
| `MaxRetries` | `int` | `3` | Max retry attempts |
| `RetryDelay` | `time.Duration` | `1s` | Initial retry delay |
| `BackoffFactor` | `float64` | `2.0` | Backoff multiplier |
| `EnableJitter` | `bool` | `true` | Add jitter to retries |
| `CustomRetryPolicy` | `RetryPolicy` | `nil` | Custom retry logic |
| **Other** ||||
| `Middlewares` | `[]MiddlewareFunc` | `nil` | Middleware chain |
| `UserAgent` | `string` | `"httpc/1.0"` | Default User-Agent |
| `Headers` | `map[string]string` | `{}` | Default headers |
| `FollowRedirects` | `bool` | `true` | Follow redirects |
| `MaxRedirects` | `int` | `10` | Max redirect count |

---

## 🔌 Middleware

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

// Static headers
httpc.HeaderMiddleware(map[string]string{
    "X-App-Version": "1.0.0",
})

// Metrics collection
httpc.MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {
    metrics.Record(method, url, statusCode, duration)
})

// Security audit
httpc.AuditMiddleware(func(a httpc.AuditEvent) {
    log.Printf("[AUDIT] %s %s -> %d (%v)", a.Method, a.URL, a.StatusCode, a.Duration)
})
```

### Chain Multiple Middleware

```go
chainedMiddleware := httpc.Chain(
    httpc.RecoveryMiddleware(),
    httpc.LoggingMiddleware(log.Printf),
    httpc.RequestIDMiddleware("X-Request-ID", nil),
    httpc.HeaderMiddleware(map[string]string{"X-App": "v1"}),
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

## 🔀 Proxy Configuration

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

## 🔒 Security Features

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

## 🔄 Concurrency Safety

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

## 📚 Documentation

| Resource | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Installation and first steps |
| [Configuration](docs/configuration.md) | Client configuration and presets |
| [Request Options](docs/request-options.md) | Complete options reference |
| [Error Handling](docs/error-handling.md) | Error handling patterns |
| [File Download](docs/file-download.md) | File download with progress |
| [HTTP Redirects](docs/redirects.md) | Redirect handling and tracking |
| [Cookie API](docs/cookie-api-reference.md) | Cookie management |
| [Security](SECURITY.md) | Security features and best practices |

### Example Code

| Directory | Description |
|-----------|-------------|
| [01_quickstart](examples/01_quickstart) | Basic usage |
| [02_core_features](examples/02_core_features) | Headers, auth, body formats |
| [03_advanced](examples/03_advanced) | File upload, download, retry, middleware |

---

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

---

If this project helps you, please give it a Star! ⭐
