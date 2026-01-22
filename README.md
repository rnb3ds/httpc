# HTTPC - Production-Ready HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Performance](https://img.shields.io/badge/performance-high%20performance-green.svg)](https://github.com/cybergodev/json)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](https://github.com/cybergodev/json)

A high-performance HTTP client library for Go with enterprise-grade security, zero external dependencies, and production-ready defaults. Built for applications that demand reliability, security, and performance.

**[📖 中文文档](README_zh-CN.md)** | **[📚 Full Documentation](docs)**

---

## ✨ Core Features

- 🛡️ **Secure by Default** - TLS 1.2+, SSRF protection, CRLF injection prevention
- ⚡ **High Performance** - Connection pooling, HTTP/2, goroutine-safe operations
- 📊 **Built-in Resilience** - Smart retry with exponential backoff and jitter
- 🎯 **Developer Friendly** - Clean API, functional options, comprehensive error handling
- 🔧 **Zero Dependencies** - Pure Go stdlib, no external packages
- 🚀 **Production Ready** - Battle-tested defaults, extensive test coverage


## 📦 Installation

```bash
go get -u github.com/cybergodev/httpc
```

## 🚀 Quick Start

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // Simple GET request
    result, err := httpc.Get("https://api.example.com/users")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d\n", result.StatusCode())

    // POST with JSON and authentication
    user := map[string]string{"name": "John", "email": "john@example.com"}
    result, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created: %s\n", result.Body())
}
```

> **Default Request Headers**: By default, use "httpc.DefaultConfig()", which automatically includes a `User-Agent: httpc/1.0` header with all requests. To customize default headers:
> - **User-Agent**: Set `config.UserAgent` or use `httpc.WithUserAgent("your-custom-agent")`
> - **Custom Headers**: Set `config.Headers` map when creating a client for client-level default headers
> - **Per-Request**: Use `httpc.WithHeader()` or `httpc.WithHeaderMap()` to override for specific requests

**[📖 See more examples](examples)** | **[🚀 Getting Started Guide](docs/getting-started.md)**

## 📖 Core Features

### HTTP Methods

All standard HTTP methods with clean, intuitive API:

```go
// GET - Retrieve data
result, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithBearerToken("token"),
)

// POST - Create resource
result, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
    httpc.WithBearerToken("your-token"),
)

// PUT - Full update
result, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),
)

// PATCH - Partial update
result, err := httpc.Patch("https://api.example.com/users/123",
    httpc.WithJSON(map[string]string{"email": "new@example.com"}),
)

// DELETE - Remove resource
result, err := httpc.Delete("https://api.example.com/users/123")

// HEAD, OPTIONS, and custom methods also supported
```

### Request Options

Customize requests using functional options (all options start with `With`):

```go
// Headers & Authentication
httpc.WithHeader("x-api-key", "key")
httpc.WithBearerToken("token")
httpc.WithBasicAuth("user", "pass")

// Query Parameters
httpc.WithQuery("page", 1)
httpc.WithQueryMap(map[string]interface{}{"page": 1, "limit": 20})

// Request Body
httpc.WithJSON(data)              // JSON body
httpc.WithXML(data)               // XML body
httpc.WithForm(formData)          // Form data (URL-encoded)
httpc.WithFormData(data)          // Multipart form data (for file uploads)
httpc.WithText("content")         // Plain text
httpc.WithBinary(data, "image/png")  // Binary data with content type
httpc.WithFile("file", "doc.pdf", content)  // Single file upload

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
httpc.WithMaxRetries(3)         // 0-10 retries allowed
httpc.WithContext(ctx)

// Combine multiple options
result, err := httpc.Post(url,
    httpc.WithJSON(data),
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(30*time.Second),
    httpc.WithMaxRetries(2),
)
```

**[📖 Complete Options Reference](docs/request-options.md)**

### Response Data Access

HTTPC returns a `Result` object that provides structured access to request and response information:

```go
result, err := httpc.Get("https://api.example.com/users/123")
if err != nil {
    log.Fatal(err)
}

// Quick access methods
statusCode := result.StatusCode()    // HTTP status code
body := result.Body()                // Response body as string
rawBody := result.RawBody()          // Response body as []byte

// Detailed response information
response := result.Response
fmt.Printf("Status: %d %s\n", response.StatusCode, response.Status)
fmt.Printf("Content-Length: %d\n", response.ContentLength)
fmt.Printf("Headers: %v\n", response.Headers)
fmt.Printf("Cookies: %v\n", response.Cookies)

// Request information
request := result.Request
fmt.Printf("Request Headers: %v\n", request.Headers)
fmt.Printf("Request Cookies: %v\n", request.Cookies)

// Metadata
meta := result.Meta
fmt.Printf("Duration: %v\n", meta.Duration)
fmt.Printf("Attempts: %d\n", meta.Attempts)
fmt.Printf("Redirects: %d\n", meta.RedirectCount)
```

### Response Handling

```go
result, err := httpc.Get(url)
if err != nil {
    log.Fatal(err)
}

// Status checking
if result.IsSuccess() {        // 2xx
    fmt.Println("Success!")
}

// Parse JSON response
var data map[string]interface{}
if err := result.JSON(&data); err != nil {
    log.Fatal(err)
}

// Access response data
fmt.Printf("Status: %d\n", result.StatusCode())
fmt.Printf("Body: %s\n", result.Body())
fmt.Printf("Duration: %v\n", result.Meta.Duration)
fmt.Printf("Attempts: %d\n", result.Meta.Attempts)

// Work with response cookies
cookie := result.GetCookie("session_id")
if result.HasCookie("session_id") {
    fmt.Println("Session cookie found")
}
responseCookies := result.ResponseCookies()  // Get all response cookies

// Access request cookies
requestCookies := result.RequestCookies()  // Get all request cookies
requestCookie := result.GetRequestCookie("auth_token")

// String representation of result
fmt.Println(result.String())

// Access detailed response information
fmt.Printf("Content-Length: %d\n", result.Response.ContentLength)
fmt.Printf("Response Headers: %v\n", result.Response.Headers)
fmt.Printf("Request Headers: %v\n", result.Request.Headers)

// Save response to file
err = result.SaveToFile("response.html")
```

### Automatic Response Decompression

HTTPC automatically detects and decompresses compressed HTTP responses:

```go
// Request compressed response
result, err := httpc.Get("https://api.example.com/data",
    httpc.WithHeader("Accept-Encoding", "gzip, deflate"),
)

// Response is automatically decompressed
fmt.Printf("Decompressed body: %s\n", result.Body())
fmt.Printf("Original encoding: %s\n", result.Response.Headers.Get("Content-Encoding"))
```

**Supported Encodings:**
- ✅ **gzip** - Fully supported (compress/gzip)
- ✅ **deflate** - Fully supported (compress/flate)
- ❌ **br** (Brotli) - Not supported
- ❌ **compress** (LZW) - Not supported

**Note:** Decompression is automatic when the server sends a `Content-Encoding` header. The library handles this transparently, so you always receive decompressed content.

### File Download

File downloads include built-in security protections:
- **UNC path blocking** - Prevents access to Windows network paths
- **System path protection** - Blocks writes to critical system directories
- **Path traversal detection** - Prevents directory escape attacks
- **Resume support** - Automatically resumes interrupted downloads

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

## 🔧 Configuration

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
// WARNING: Disables TLS verification, reduces security
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
result, err := httpc.Get(url)
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
// Note: HTTPC returns Result for all status codes (including 4xx and 5xx)
// HTTPError is NOT automatically returned for non-2xx status codes
if !result.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", result.StatusCode())
}

// Access detailed error information
if result.IsClientError() {
    fmt.Printf("Client error (4xx): %d\n", result.StatusCode())
} else if result.IsServerError() {
    fmt.Printf("Server error (5xx): %d\n", result.StatusCode())
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
result, err := httpc.Get(url)
```

### Automatic Retries

```go
// Configure at client level
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.BackoffFactor = 2.0
client, err := httpc.New(config)

// Or per-request
result, err := httpc.Get(url, httpc.WithMaxRetries(5))
```

### Context Support

```go
// Timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
result, err := client.Get(url, httpc.WithContext(ctx))

// Cancellation
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel()
}()
result, err := client.Get(url, httpc.WithContext(ctx))
```

### HTTP Redirects

```go
// Automatic redirect following (default)
result, err := httpc.Get("https://example.com/redirect")
fmt.Printf("Followed %d redirects\n", result.Meta.RedirectCount)

// Disable redirects for specific request
result, err := httpc.Get(url, httpc.WithFollowRedirects(false))
if result.IsRedirect() {
    fmt.Printf("Redirect to: %s\n", result.Response.Headers.Get("Location"))
}

// Limit redirects
result, err := httpc.Get(url, httpc.WithMaxRedirects(5))

// Track redirect chain
for i, url := range result.Meta.RedirectChain {
    fmt.Printf("%d. %s\n", i+1, url)
}
```

**[📖 Redirect Guide](docs/redirects.md)**

### Cookie Management

```go
// Automatic cookie handling
// Note: EnableCookies is false by default in DefaultConfig()
config := httpc.DefaultConfig()
config.EnableCookies = true  // Must explicitly enable for automatic cookie handling
client, err := httpc.New(config)

// Login sets cookies
client.Post("https://example.com/login", httpc.WithForm(credentials))

// Subsequent requests include cookies automatically
client.Get("https://example.com/profile")

// Manual cookie setting
// Parse cookie string (from browser dev tools or server response)
result, err := httpc.Get("https://api.example.com/data",
    httpc.WithCookieString("PSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc"),
)

// Set individual cookies
result, err = httpc.Get("https://api.example.com/data",
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
result, err = httpc.Get("https://api.example.com/data", httpc.WithCookie(cookie))
```

**Note:** For automatic cookie state management across multiple requests, consider using `DomainClient` which automatically handles cookie persistence.

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

// Request Homepage
resp0, err := client.Get("/")

// First request - server sets cookies
resp1, err := client.Post("/login",
    httpc.WithJSON(credentials),
)

// Cookies from resp1 are automatically saved and sent in subsequent requests
resp2, err := client.Get("/profile")  // Cookies automatically included

// Set persistent headers (sent with all requests)
client.SetHeader("Authorization", "Bearer "+token)
client.SetHeader("x-api-key", "your-api-key")

// Set multiple headers at once
err = client.SetHeaders(map[string]string{
    "Authorization": "Bearer " + token,
    "x-api-key": "your-api-key",
})

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

**File Downloads with DomainClient:**

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// Set authentication header (used for all requests including downloads)
client.SetHeader("Authorization", "Bearer "+token)

// Simple download with automatic state management
result, err := client.DownloadFile("/files/report.pdf", "downloads/report.pdf")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Downloaded: %s at %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// Download with progress tracking
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s", percentage, httpc.FormatSpeed(speed))
}
result, err = client.DownloadWithOptions("/files/large-file.zip", opts)
```

**Key Features:**
- **Automatic Cookie Persistence** - Cookies from responses are saved and sent in subsequent requests
- **Automatic Header Persistence** - Set headers once, used in all requests
- **File Download Support** - Download files with automatic state management (cookies/headers)
- **Per-Request Overrides** - Use `WithCookies()` and `WithHeaderMap()` to override for specific requests
- **Thread-Safe** - All operations are goroutine-safe
- **Manual Control** - Full API for inspecting and modifying state
- **Automatic Cookie Enabling** - `NewDomain()` automatically enables cookies regardless of config

**[📖 See full example](examples/03_advanced/domain_client.go)**

### Proxy Configuration

HTTPC supports flexible proxy configuration with three modes:

#### Proxy Priority

```
Priority 1: ProxyURL (manual proxy)        - Highest priority
Priority 2: EnableSystemProxy (auto-detect system proxy)
Priority 3: Direct connection (no proxy)   - Default
```

#### 1. Manual Proxy (Highest Priority)

Specify a proxy URL directly. This takes priority over all other proxy settings.

```go
// Direct proxy specification
config := &httpc.Config{
    ProxyURL: "http://127.0.0.1:1234",
    Timeout:  30 * time.Second,
}
client, err := httpc.New(config)

// SOCKS5 proxy
config := &httpc.Config{
    ProxyURL: "socks5://127.0.0.1:1080",
}
client, err := httpc.New(config)

// Corporate proxy with authentication
config := &httpc.Config{
    ProxyURL: "http://user:pass@proxy.company.com:8080",
}
client, err := httpc.New(config)
```

#### 2. System Proxy Detection

Enable automatic detection of system proxy settings. This includes:

- **Windows**: Reads from Registry
- **macOS**: Reads from System Preferences
_- **Linux**: Reads from system settings
- **All Platforms**: Falls back to environment variables (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)

```go
// Enable system proxy detection
config := &httpc.Config{
    EnableSystemProxy: true,
}
client, err := httpc.New(config)
// Will automatically use system proxy if configured
```

**Environment Variables:**

```bash
# Set proxy via environment variables
export HTTP_PROXY=http://127.0.0.1:1234
export HTTPS_PROXY=http://127.0.0.1:1234
export NO_PROXY=localhost,127.0.0.1,.local.com

# Then enable system proxy detection in code
config := &httpc.Config{
    EnableSystemProxy: true,
}
```

#### 3. Direct Connection (Default)

When `ProxyURL` is empty and `EnableSystemProxy` is `false`, connections are made directly without any proxy.

```go
// Default behavior - direct connection
client, err := httpc.New()

// Explicit direct connection
config := &httpc.Config{
    // ProxyURL is empty (default)
    // EnableSystemProxy is false (default)
}
client, err := httpc.New(config)
```

**[📖 See full example](examples/03_advanced/proxy_configuration.go)**

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
        result, _ := client.Get("https://api.example.com")
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

## 🤝 Contributing

Contributions, issue reports, and suggestions are welcome!

## 📄 License

MIT License - See [LICENSE](LICENSE) file for details.

---

**Crafted with care for the Go community** ❤️ | If this project helps you, please give it a ⭐️ Star!
