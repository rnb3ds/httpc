# HTTPC - Secure HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Dependencies](https://img.shields.io/badge/deps-minimal-brightgreen.svg)](go.mod)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-%E2%9C%93-brightgreen.svg)](docs/09_concurrency-safety.md)

A fast, secure HTTP client library for Go with sensible defaults, minimal dependencies, and built-in resilience.

**[中文文档](README_zh-CN.md)** | **[www.cybergo.dev/httpc](https://www.cybergo.dev/httpc)**

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start-5-minutes)
- [HTTP Methods](#http-methods)
- [Request Options](#request-options)
- [Response Handling](#response-handling)
- [Context & Cancellation](#context--cancellation)
- [File Download](#file-download)
- [Streaming Bodies](#streaming-bodies)
- [Domain Client (Session Management)](#domain-client-session-management)
- [Session Manager](#session-manager)
- [Configuration](#configuration)
- [Middleware](#middleware)
- [Proxy Configuration](#proxy-configuration)
- [Security Features](#security-features)
- [Error Handling](#error-handling)
- [Concurrency Safety](#concurrency-safety)
- [Interfaces](#interfaces)
- [Documentation](#documentation)
- [License](#license)

---

## Features

| Feature | Description |
|---------|-------------|
| **Secure by Default** | TLS 1.2+, SSRF protection, CRLF injection prevention, path traversal blocking |
| **High Performance** | Connection pooling, HTTP/2, goroutine-safe, `sync.Pool` optimization |
| **Built-in Resilience** | Smart retry with exponential backoff and jitter — non-idempotent requests (POST/PATCH) execute exactly once by default |
| **Automatic Decompression** | Transparent gzip/deflate handling with zip-bomb protection |
| **Developer Friendly** | Clean API, intuitive options pattern, comprehensive documentation |
| **Minimal Dependencies** | 1 dependency (golang.org/x/sys), pure Go stdlib |
| **Cookie Management** | Full cookie jar support with security validation |
| **File Operations** | Secure file download with progress tracking and resume support |
| **Streaming** | `io.Reader` request bodies, zero-copy `io.Pipe` uploads, disk-streaming downloads |

---

## Installation

```bash
go get -u github.com/cybergodev/httpc
```

**Requirements:** Go 1.25+

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

### Using Client Instance (Recommended)

```go
package main

import (
    "fmt"
    "log"

    "github.com/cybergodev/httpc"
)

func main() {
    // Create a reusable client with default configuration
    client, err := httpc.NewDefault()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Make multiple requests
    result, err := client.Get("https://api.example.com/users",
        httpc.WithQuery("page", 1),
        httpc.WithQuery("limit", 20),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d\n", result.StatusCode())
}
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

// HEAD / OPTIONS
result, _ := httpc.Head(url)
result, _ := httpc.Options(url)

// Generic request with custom method
result, _ := httpc.Request(ctx, "PROPFIND", url)
```

---

## Request Options

### Headers

```go
// Single header
httpc.WithHeader("Authorization", "Bearer token")

// Multiple headers
httpc.WithHeaderMap(map[string]string{
    "X-Custom":      "value",
    "X-Request-ID":  "123",
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
formData := &httpc.FormData{
    Fields: map[string]string{"key": "value"},
    Files: map[string]*httpc.FileData{
        "upload": {Filename: "doc.pdf", Content: fileBytes, ContentType: "application/pdf"},
    },
}
httpc.WithFormData(formData)

// Or use shorthand for single file upload
httpc.WithFile("file", "document.pdf", fileBytes)

// Raw body (auto-detect Content-Type)
httpc.WithBody([]byte("raw data"))
httpc.WithBody(data, httpc.BodyJSON) // explicit BodyKind

// io.Reader body (streamed upload; set Content-Type explicitly)
httpc.WithBody(fileReader)

// BodyKind constants: BodyAuto, BodyJSON, BodyXML, BodyForm, BodyBinary, BodyMultipart

httpc.WithBinary(binaryData, "application/pdf")

// Stream response body (effective only with Download; see Streaming Bodies)
httpc.WithStreamBody(true)
```

### Cookies

```go
// Single cookie
httpc.WithCookies([]http.Cookie{{Name: "session", Value: "abc123"}})

// Batch multiple cookies (efficient, pre-allocates capacity)
httpc.WithCookies([]http.Cookie{
    {Name: "session_id", Value: "abc123"},
    {Name: "user_pref", Value: "dark_mode"},
})

// Multiple cookies from map
httpc.WithCookieMap(map[string]string{
    "session_id": "abc123",
    "user_pref":  "dark_mode",
})

// Cookie string
httpc.WithCookieString("session=abc123; token=xyz")

// Secure cookie with validation (validates cookie security attributes)
cfg := httpc.DefaultCookieSecurityConfig()
cfg.RequireSecure = true
httpc.WithSecureCookie(cfg)
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

// Per-request SSRF override (escape hatch for one trusted internal URL)
httpc.WithAllowPrivateIPs(true)
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
| **Body** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithFormData(*FormData)`, `WithFile(field, filename, content)`, `WithBody(data, ...BodyKind)`, `WithBinary([]byte, ...contentType)`, `WithStreamBody(bool)` |
| **Cookies** | `WithCookies([]http.Cookie)`, `WithCookieMap(map)`, `WithCookieString("a=1; b=2")`, `WithSecureCookie(config)` |
| **Control** | `WithTimeout(dur)`, `WithMaxRetries(n)`, `WithContext(ctx)`, `WithAllowPrivateIPs(bool)` |
| **Redirects** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **Callbacks** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

---

## Response Handling

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// Result struct composition:
// result.Request  -> *RequestInfo  (URL, Method, Headers, Cookies)
// result.Response -> *ResponseInfo (StatusCode, Status, Proto, Headers, Body, RawBody, ContentLength, Cookies)
// result.Meta     -> *RequestMeta  (Duration, Attempts, ProxyURL, RedirectCount, RedirectChain)

// Quick access methods (nil-safe)
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
if err := result.Unmarshal(&data); err != nil {
    log.Fatal(err)
}

// Cookie access
cookie := result.GetCookie("session")
if result.HasCookie("session") { }

// Request cookies sent
reqCookie := result.GetRequestCookie("token")
if result.HasRequestCookie("token") { }

// Get all cookies
allResponse := result.ResponseCookies()
allRequest := result.RequestCookies()

// Save response to file
if err := result.SaveToFile("response.json"); err != nil {
    log.Fatal(err)
}

// Metadata
fmt.Println(result.Meta.Duration)      // Request duration
fmt.Println(result.Meta.Attempts)      // Retry count
fmt.Println(result.Meta.ProxyURL)      // Proxy that served the request ("" = direct)
fmt.Println(result.Meta.RedirectCount) // Redirect count
fmt.Println(result.Meta.RedirectChain) // Redirect URLs

// String representation (safe for logging - masks sensitive headers)
fmt.Println(result.String())
```

### Automatic Decompression

HTTPC transparently decompresses `gzip` and `deflate` responses. It advertises
`Accept-Encoding: gzip, deflate` by default (override or extend it with
`WithHeader("Accept-Encoding", ...)`). A decompression-bomb guard caps the
decompressed size at `Security.MaxDecompressedBodySize` (default 100 MB).

```go
result, _ := httpc.Get("https://httpbin.org/gzip",
    httpc.WithHeaderMap(map[string]string{"Accept-Encoding": "gzip, deflate"}),
)
fmt.Println(result.Body()) // already decompressed
```

> **Note:** Any `Content-Encoding` other than `gzip`, `deflate` and `identity`
> (e.g. Brotli `br`, LZW `compress`, `zstd`, or a multi-token list like
> `gzip, br`) is **rejected with an error** instead of silently returning raw
> compressed bytes as the body. Since httpc only advertises `gzip, deflate`,
> this normally only happens if you set `Accept-Encoding` manually.

> **Streaming:** requests with `WithStreamBody(true)` (including `Download`)
> send `Accept-Encoding: identity` — the streaming path hands you the raw
> transport bytes, so it never requests compressed content. A manual
> `Accept-Encoding` on a streaming request is honored as-is.

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

### Simple Download

File downloads include built-in security protections:
- **UNC path blocking** - Prevents access to Windows network paths
- **System path protection** - Blocks writes to critical system directories
- **Path traversal detection** - Prevents directory escape attacks
- **Resume support** - Automatically resumes interrupted downloads

```go
result, _ := httpc.Download(context.Background(),
    "https://example.com/file.zip",
    &httpc.DownloadConfig{FilePath: "downloads/file.zip"},
)
fmt.Printf("Downloaded: %s at %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))
```

### Download with Progress

```go
opts := httpc.DefaultDownloadConfig()
opts.FilePath = "downloads/large.zip"
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    pct := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r%.1f%% - %s/s", pct, httpc.FormatSpeed(speed))
}
result, _ := httpc.Download(context.Background(), url, opts)
```

### Resume Download

```go
opts := httpc.DefaultDownloadConfig()
opts.FilePath = "downloads/large.zip"
opts.ResumeDownload = true
result, _ := httpc.Download(context.Background(), url, opts)
if result.Resumed {
    fmt.Println("Download resumed from previous position")
}
```

### Download with Context

`Download` accepts a `context.Context` directly, so cancellation and timeouts
apply out of the box — there is no separate "WithContext" entry point:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
defer cancel()

result, _ := httpc.Download(ctx,
    "https://example.com/large.zip",
    &httpc.DownloadConfig{FilePath: "downloads/large.zip"},
)

// Full control: download config + context + request options
result, _ := httpc.Download(ctx, url, opts,
    httpc.WithBearerToken("your-token"),
)
```

### DownloadConfig Fields

| Field | Type | Description |
|-------|------|-------------|
| `FilePath` | `string` | Destination path for the downloaded file |
| `ProgressCallback` | `DownloadProgressCallback` | Progress callback: `func(downloaded, total int64, speed float64)` |
| `Overwrite` | `bool` | Overwrite existing file (default: `false`) |
| `ResumeDownload` | `bool` | Resume interrupted download (default: `false`) |
| `Checksum` | `string` | Expected checksum for verification |
| `ChecksumAlgorithm` | `ChecksumAlgorithm` | Checksum algorithm (e.g., `httpc.ChecksumSHA256`) |

### Download Functions

| Function | Description |
|----------|-------------|
| `Download(ctx, url, cfg, ...options)` | **Canonical entry point** — single function for all package-level downloads |

### DownloadResult Fields

| Field | Type | Description |
|-------|------|-------------|
| `FilePath` | `string` | Local file path written to |
| `BytesWritten` | `int64` | Total bytes written |
| `Duration` | `time.Duration` | Download duration |
| `AverageSpeed` | `float64` | Average download speed (bytes/sec) |
| `StatusCode` | `int` | HTTP status code |
| `ContentLength` | `int64` | Content-Length from server |
| `Resumed` | `bool` | Whether download was resumed |
| `ResponseCookies` | `[]*http.Cookie` | Cookies from response |
| `ActualChecksum` | `string` | Computed checksum of downloaded file |
| `Proto` | `string` | HTTP protocol version (e.g., "HTTP/1.1", "HTTP/2.0") |
| `ResponseHeaders` | `http.Header` | Response headers |
| `RequestURL` | `string` | Actual URL requested |
| `RequestMethod` | `string` | HTTP method used |
| `RequestHeaders` | `http.Header` | Request headers sent |

---

## Streaming Bodies

For large payloads, HTTPC avoids buffering entire bodies in memory.

### Streaming Request Body (Upload)

`WithBody` accepts `io.Reader` directly. Content-Type is **not** auto-detected
for readers, so set it explicitly:

```go
// Stream from a file or any io.Reader
result, err := httpc.Post("https://example.com/upload",
    httpc.WithBody(file),
    httpc.WithHeader("Content-Type", "application/octet-stream"),
)
```

Zero-copy streaming with `io.Pipe` — the producer goroutine writes while the
HTTP transport consumes concurrently:

```go
pr, pw := io.Pipe()
go func() {
    defer pw.Close()
    // Write chunks to pw ...
}()

result, err := httpc.Post("https://example.com/upload",
    httpc.WithBody(pr),
    httpc.WithHeader("Content-Type", "application/octet-stream"),
)
```

> **Security:** `WithBody(io.Reader)` **bypasses request-body-size validation**.
> Wrap untrusted readers with `io.LimitReader` to enforce a cap:
> ```go
> httpc.WithBody(io.LimitReader(reader, 10<<20)) // 10 MB cap
> ```

### Streaming Response Body (Download)

`Download()` streams the response body directly to disk — it never buffers the
full body. `WithStreamBody(true)` is applied automatically:

```go
result, _ := httpc.Download(ctx,
    "https://example.com/large.zip",
    &httpc.DownloadConfig{FilePath: "downloads/large.zip"},
)
```

> **Note:** Regular `Get` / `Post` / etc. **always buffer the full response body**
> into memory. Passing `WithStreamBody(true)` to these methods returns
> `ErrStreamBodyRequiresDownload` — use `Download` for large responses.
> See [File Download](#file-download) for details.

---

## Domain Client (Session Management)

For multiple requests to the same domain with automatic cookie and header management:

```go
client, _ := httpc.NewDomainDefault("https://api.example.com")
defer client.Close()

// Login - server sets cookies
client.Post("/login", httpc.WithJSON(credentials))

// Set persistent header (used for all requests)
client.SetHeader("Authorization", "Bearer "+token)

// Subsequent requests include cookies + headers automatically
profile, _ := client.Get("/profile")
data, _ := client.Get("/data")
```

### Cookie Management

```go
// Single cookie
client.SetCookie(&http.Cookie{Name: "session", Value: "abc"})

// Multiple cookies
client.SetCookies([]*http.Cookie{
    {Name: "session", Value: "abc"},
    {Name: "token", Value: "xyz"},
})

// Read cookies
cookie := client.GetCookie("session")   // Single cookie
allCookies := client.GetCookies()        // All cookies

// Remove cookies
client.DeleteCookie("session")
client.ClearCookies()
```

### Header Management

```go
client.SetHeader("X-Custom", "value")
client.SetHeaders(map[string]string{"X-App": "v1", "X-Version": "1.0"})
headers := client.GetHeaders()
client.DeleteHeader("X-Old")
client.ClearHeaders()
```

### Accessors

```go
client.URL()     // Full base URL
client.Domain()  // Domain only
client.Session() // Underlying SessionManager
```

### File Downloads (relative paths)

```go
result, _ := client.Download(ctx, "/files/data.csv", &httpc.DownloadConfig{FilePath: "data.csv"})
result, _ := client.Download(ctx, "/files/large.zip", downloadOpts)
```

### All HTTP Methods

```go
result, _ := client.Get("/users")
result, _ := client.Post("/users", httpc.WithJSON(data))
result, _ := client.Put("/users/1", httpc.WithJSON(data))
result, _ := client.Patch("/users/1", httpc.WithJSON(data))
result, _ := client.Delete("/users/1")
result, _ := client.Head("/users")
result, _ := client.Options("/users")
result, _ := client.Request(ctx, "PROPFIND", "/resource")
```

---

## Session Manager

The `SessionManager` provides thread-safe cookie and header management, used internally by `DomainClient` but also available standalone:

```go
// Create session manager
sm, _ := httpc.NewSessionManagerDefault()

// Or with cookie security validation
cfg := httpc.DefaultSessionConfig()
cfg.CookieSecurity = httpc.StrictCookieSecurityConfig()
sm, _ := httpc.NewSessionManager(cfg)

// Manage cookies
sm.SetCookie(&http.Cookie{Name: "session", Value: "abc"})
sm.SetCookies([]*http.Cookie{{Name: "token", Value: "xyz"}})
cookie := sm.GetCookie("session")
allCookies := sm.GetCookies()
sm.DeleteCookie("session")
sm.ClearCookies()

// Manage headers
sm.SetHeader("Authorization", "Bearer token")
sm.SetHeaders(map[string]string{"X-App": "v1"})
headers := sm.GetHeaders()
sm.DeleteHeader("X-Old")
sm.ClearHeaders()

// Update from response
sm.UpdateFromResult(result)
sm.UpdateFromCookies(responseCookies)

// Cookie security
sm.SetCookieSecurity(httpc.StrictCookieSecurityConfig())
```

---

## Configuration

### Preset Configurations

```go
// Recommended defaults
client, _ := httpc.NewDefault()

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
config := httpc.Config{
    // Timeouts
    Timeouts: httpc.TimeoutConfig{
        Request:        30 * time.Second,
        Dial:           10 * time.Second,
        TLSHandshake:   10 * time.Second,
        ResponseHeader: 30 * time.Second,
        IdleConn:       90 * time.Second,
    },

    // Connection
    Connection: httpc.ConnectionConfig{
        MaxIdleConns:    100,
        MaxConnsPerHost: 20,
        EnableHTTP2:     true,
        EnableCookies:   false,
    },

    // Security
    Security: httpc.SecurityConfig{
        MinTLSVersion:       tls.VersionTLS12,
        MaxTLSVersion:       tls.VersionTLS13,
        MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
        AllowPrivateIPs:     false,
    },

    // Retry
    Retry: httpc.RetryConfig{
        MaxRetries:    3,
        Delay:         1 * time.Second,
        BackoffFactor: 2.0,
        EnableJitter:  true,
    },

    // Defaults (per-request defaults: User-Agent, headers, redirect policy)
    Defaults: httpc.RequestDefaults{
        UserAgent:       "MyApp/1.0",
        FollowRedirects: true,
        MaxRedirects:    10,
    },
}

// Validate configuration before creating client (New() also validates internally)
if err := config.Validate(); err != nil {
    log.Fatal(err)
}

client, _ := httpc.New(config)

// Inspect configuration (sensitive values are automatically masked)
fmt.Println(config.String())
```

### Configuration Options

| Function | Description |
|----------|-------------|
| `DefaultConfig()` | Recommended defaults |
| `SecureConfig()` | Maximum security (SSRF protection enabled) |
| `PerformanceConfig()` | High throughput |
| `MinimalConfig()` | Lightweight (no retries) |
| `TestingConfig()` | Testing only - disables security features |
| `Config.Validate()` | Validate configuration, returns error (function form: `ValidateConfig(cfg)`) |
| `Config.String()` | Safe string representation (sensitive values masked) |

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| **Timeouts** (`Timeouts: httpc.TimeoutConfig{...}`) ||||
| `Timeouts.Request` | `time.Duration` | `180s` | Overall request timeout |
| `Timeouts.Dial` | `time.Duration` | `10s` | TCP connection timeout |
| `Timeouts.TLSHandshake` | `time.Duration` | `10s` | TLS handshake timeout |
| `Timeouts.ResponseHeader` | `time.Duration` | `0` | Response header timeout (0 = disabled, uses context timeout) |
| `Timeouts.IdleConn` | `time.Duration` | `90s` | Idle connection timeout |
| **Connection** (`Connection: httpc.ConnectionConfig{...}`) ||||
| `Connection.MaxIdleConns` | `int` | `50` | Max idle connections |
| `Connection.MaxConnsPerHost` | `int` | `10` | Max connections per host |
| `Connection.ProxyURL` | `string` | `""` | Proxy URL (http/https/socks5/socks5h) |
| `Connection.EnableSystemProxy` | `bool` | `false` | Auto-detect system proxy |
| `Connection.ProxyPool` | `[]string` | `nil` | Proxy URLs for rotation (see [Proxy Configuration](#proxy-configuration)) |
| `Connection.ProxyPoolStrategy` | `ProxyStrategy` | `ProxyStrategyRoundRobin` | Proxy selection algorithm (`ProxyStrategyRoundRobin` or `ProxyStrategyRandom`) |
| `Connection.ProxyFailureThreshold` | `int` | `3` | Consecutive connection failures before circuit-breaking a proxy |
| `Connection.ProxyCooldown` | `time.Duration` | `30s` | How long a circuit-broken proxy stays out of rotation |
| `Connection.ProxyRotateOnStatus` | `[]int` | `nil` | HTTP status codes that trigger proxy rotation (e.g., `[]int{403}`) |
| `Connection.ProxyRotatePerRequest` | `bool` | `false` | Close idle connections before each request so the transport re-evaluates the proxy pool, guaranteeing per-request rotation (adds overhead: no connection reuse). Requires ProxyPool; no effect with ProxyURL |
| `Connection.EnableHTTP2` | `bool` | `true` | Enable HTTP/2 |
| `Connection.EnableCookies` | `bool` | `false` | Enable cookie jar |
| `Connection.EnableDoH` | `bool` | `false` | Enable DNS-over-HTTPS |
| `Connection.DoHCacheTTL` | `time.Duration` | `5m` | DoH cache duration |
| `Connection.MaxResponseHeaderBytes` | `int64` | `0` | Max response header size (0 = Go stdlib default 1MB) |
| **Security** (`Security: httpc.SecurityConfig{...}`) ||||
| `Security.TLSConfig` | `*tls.Config` | `nil` | Custom TLS config |
| `Security.MinTLSVersion` | `uint16` | `TLS 1.2` | Minimum TLS version |
| `Security.MaxTLSVersion` | `uint16` | `TLS 1.3` | Maximum TLS version |
| `Security.InsecureSkipVerify` | `bool` | `false` | Skip TLS verification (testing only!) |
| `Security.TLSFingerprint` | `string` | `""` | Browser fingerprint preset — see [Browser Transport Fingerprinting](#browser-transport-fingerprinting) |
| `Security.TLSFingerprintDisableSessionTickets` | `bool` | `false` | Force a full TLS handshake on every fingerprinted connection (Chrome profiles resume sessions by default) |
| `Security.CertificatePinner` | `CertificatePinner` | `nil` | Certificate pinning — reject MITM even if CA compromised (works with `TLSFingerprint`) |
| `Security.MaxResponseBodySize` | `int64` | `10MB` | Max response body size |
| `Security.MaxRequestBodySize` | `int64` | `0` | Max request body size (0 = no limit; does not fall back to MaxResponseBodySize). `io.Reader` bodies are enforced at send time |
| `Security.AllowPrivateIPs` | `bool` | `false` | Allow private IPs (SSRF protection enabled by default) |
| `Security.ValidateURL` | `bool` | `true` | Enable URL validation |
| `Security.ValidateHeaders` | `bool` | `true` | Enable header validation |
| `Security.StrictContentLength` | `bool` | `true` | Strict content-length check |
| `Security.RedirectWhitelist` | `[]string` | `nil` | Allowed redirect domains |
| `Security.MaxDecompressedBodySize` | `int64` | `100MB` | Max decompressed body size (zip bomb protection) |
| `Security.SSRFExemptCIDRs` | `[]string` | `nil` | CIDR ranges exempted from SSRF blocking |
| **Retry** (`Retry: httpc.RetryConfig{...}`) ||||
| `Retry.MaxRetries` | `int` | `3` | Max retry attempts |
| `Retry.Delay` | `time.Duration` | `1s` | Initial retry delay |
| `Retry.BackoffFactor` | `float64` | `2.0` | Backoff multiplier |
| `Retry.EnableJitter` | `bool` | `true` | Add jitter to retries |
| `Retry.MaxRetryDelay` | `time.Duration` | `30s` | Cap on maximum retry delay |
| `Retry.CustomPolicy` | `RetryPolicy` | `nil` | Custom retry logic |
| **Middleware** (`Middleware: httpc.MiddlewareConfig{...}`) ||||
| `Middleware.Middlewares` | `[]MiddlewareFunc` | `nil` | Middleware chain |
| **Defaults** (`Defaults: httpc.RequestDefaults{...}`) ||||
| `Defaults.UserAgent` | `string` | `"httpc/1.0"` | Default User-Agent |
| `Defaults.Headers` | `map[string]string` | `{}` | Default headers |
| **Security** (`Security: httpc.SecurityConfig{...}`) ||||
| `Security.FollowRedirects` | `bool` | `true` | Follow redirects |
| `Security.MaxRedirects` | `int` | `10` | Redirect limit; counts the initial request, so 10 follows at most 9 redirects |

---

## Middleware

Every configurable middleware follows the same pattern: a `XxxConfig` struct,
a `DefaultXxxConfig()` constructor, and a `XxxMiddleware(cfg)` factory.
Pass `nil` to accept defaults.

### Built-in Middleware

```go
// Request logging
httpc.LoggingMiddleware(&httpc.LoggingConfig{LogFunc: log.Printf})

// Panic recovery (no config needed)
httpc.RecoveryMiddleware()

// Request ID (nil config = defaults: "X-Request-ID" header, crypto/rand generator)
httpc.RequestIDMiddleware(&httpc.RequestIDConfig{HeaderName: "X-Request-ID"})

// Timeout enforcement
httpc.TimeoutMiddleware(&httpc.TimeoutMiddlewareConfig{Duration: 30 * time.Second})

// Static headers
httpc.HeaderMiddleware(&httpc.HeaderConfig{
    Headers: map[string]string{"X-App-Version": "1.0.0"},
})

// Metrics collection
httpc.MetricsMiddleware(&httpc.MetricsConfig{
    OnMetrics: func(method, url string, statusCode int, duration time.Duration, err error) {
        metrics.Record(method, url, statusCode, duration)
    },
})

// Security audit
auditCfg := httpc.DefaultAuditConfig()
auditCfg.OnAudit = func(a httpc.AuditEvent) {
    log.Printf("[AUDIT] %s %s -> %d (%v)", a.Method, a.URL, a.StatusCode, a.Duration)
}
httpc.AuditMiddleware(auditCfg)

// Audit with custom config (JSON format, include headers)
auditCfgJSON := httpc.DefaultAuditConfig()
auditCfgJSON.OnAudit = func(a httpc.AuditEvent) { log.Printf("[AUDIT] %v", a) }
auditCfgJSON.IncludeHeaders = true
auditCfgJSON.Format = "json"
httpc.AuditMiddleware(auditCfgJSON)
```

### AuditEvent Fields

| Field | Type | Description |
|-------|------|-------------|
| `Timestamp` | `time.Time` | Request timestamp |
| `Method` | `string` | HTTP method |
| `URL` | `string` | Request URL |
| `StatusCode` | `int` | Response status code |
| `Duration` | `time.Duration` | Request duration |
| `Attempts` | `int` | Retry attempts |
| `Error` | `error` | Error if any |
| `SourceIP` | `string` | Source IP (set via context key `SourceIPKey`) |
| `UserID` | `string` | User ID (set via context key `UserIDKey`) |
| `RedirectChain` | `[]string` | Redirect URLs |
| `ReqHeaders` | `map[string][]string` | Request headers (when `IncludeHeaders: true`) |
| `RespHeaders` | `map[string][]string` | Response headers (when `IncludeHeaders: true`) |

`AuditEvent` supports JSON serialization via `MarshalJSON()`.

### Chain Multiple Middleware

```go
chainedMiddleware := httpc.Chain(
    httpc.RecoveryMiddleware(),
    httpc.LoggingMiddleware(&httpc.LoggingConfig{LogFunc: log.Printf}),
    httpc.RequestIDMiddleware(nil),
    httpc.HeaderMiddleware(&httpc.HeaderConfig{Headers: map[string]string{"X-App": "v1"}}),
)
config.Middleware.Middlewares = []httpc.MiddlewareFunc{chainedMiddleware}
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
config := httpc.DefaultConfig()
config.Connection.ProxyURL = "http://127.0.0.1:8080"
// Or HTTPS proxy: "https://proxy.example.com:8443"

// System proxy auto-detection (Windows/macOS/Linux)
config := httpc.DefaultConfig()
config.Connection.EnableSystemProxy = true // Reads from environment and system settings
```

### Proxy Pool (Rotation + Circuit Breaking)

Distribute requests across multiple proxies with automatic failover and status-based rotation:

```go
config := httpc.DefaultConfig()
config.Connection.ProxyPool = []string{
    "http://proxy1.example.com:7070",
    "http://proxy2.example.com:8080",
    "socks5://proxy3.example.com:1080",
}
// Strategy: ProxyStrategyRoundRobin (default) or ProxyStrategyRandom
config.Connection.ProxyPoolStrategy = httpc.ProxyStrategyRoundRobin

// Circuit breaking: after 5 consecutive connection failures, skip the proxy
// for 60s, then retry it (half-open probe). Defaults: threshold=3, cooldown=30s.
config.Connection.ProxyFailureThreshold = 5
config.Connection.ProxyCooldown = 60 * time.Second

// Rotate proxy on 403 (CF/WAF IP blocking). The request is retried
// through a different proxy IP. Requires Retry.MaxRetries > 0.
// Unlike connection failures, status-based rotation does NOT circuit-break
// the proxy — blocks are often target-specific.
config.Connection.ProxyRotateOnStatus = []int{403}
config.Retry.MaxRetries = 3
```

### Per-Request Proxy Rotation

By default, HTTP connection reuse causes consecutive requests to the same host
to reuse the previous proxy tunnel, bypassing pool selection. Enable
`ProxyRotatePerRequest` to guarantee each independent `Get`/`Post` call routes
through a different proxy:

```go
config.Connection.ProxyRotatePerRequest = true
```

This closes idle connections at the start of each request so the transport
re-evaluates the proxy pool. It adds a small overhead (no connection reuse) but
is essential for scraping or fingerprint-rotation use cases. Requires
`ProxyPool`; has no effect with `ProxyURL`.

### Which Proxy Served a Request?

Every `Result` reports the proxy that produced its final response, so rotation
can be verified or logged per request (empty string means a direct connection):

```go
result, _ := client.Get("https://httpbin.org/ip")
fmt.Println(result.Meta.ProxyURL) // e.g. "http://proxy2.example.com:8080"
```

With retries each attempt may use a different proxy; `ProxyURL` reflects the
attempt that returned the response (`Meta.Attempts` holds the count).

**Priority:** `ProxyURL` > `ProxyPool` > `EnableSystemProxy` > direct.

---

## Security Features

| Feature | Description |
|---------|-------------|
| **TLS 1.2+** | Modern encryption standards by default |
| **Certificate Pinning** | Defense against MITM even with a compromised CA |
| **SSRF Protection** | Two-layer DNS validation blocks private IPs |
| **CRLF Injection Prevention** | Header and URL validation |
| **Path Traversal Protection** | Safe file operations |
| **Domain Whitelist** | Restrict redirects to allowed domains |
| **Response Size Limit** | Configurable limit to prevent memory exhaustion |

### Redirect Domain Whitelist

```go
config := httpc.DefaultConfig()
config.Security.RedirectWhitelist = []string{"api.example.com", "secure.example.com"}
```

### SSRF Protection

By default, `AllowPrivateIPs` is `false` (SSRF protection enabled), blocking connections to private/reserved IP addresses. Set to `true` only when connecting to internal services:

```go
// SSRF protection is enabled by default
client, _ := httpc.New(httpc.DefaultConfig())

// Allow private IPs for internal service access
cfg := httpc.DefaultConfig()
cfg.Security.AllowPrivateIPs = true
client, _ := httpc.New(cfg)

// Or exempt specific CIDRs (e.g., VPN/VPC ranges)
cfg := httpc.DefaultConfig()
cfg.Security.SSRFExemptCIDRs = []string{"10.0.0.0/8", "100.64.0.0/10"}
client, _ := httpc.New(cfg)
```

For a single trusted internal call without relaxing the whole client, use the
per-request `WithAllowPrivateIPs` option (it overrides the client's SSRF policy
for that one request only):

```go
// Default client blocks private IPs; this call opts in for one request
result, err := httpc.Get("http://localhost:8080/health",
    httpc.WithAllowPrivateIPs(true),
)
```

### Certificate Pinning

Certificate pinning defends against man-in-the-middle attacks even when a trusted
Certificate Authority is compromised: the TLS handshake is rejected unless the
server presents a pinned public key. Enable it by assigning a `CertificatePinner`
to `Security.CertificatePinner`.

```go
// Pin by base64-encoded SHA-256 hash of the SubjectPublicKeyInfo (SPKI).
// Supply multiple hashes to support key rotation (accept if ANY matches).
pinner, err := httpc.NewSPKIHashPinner(
    "YLh1dUR9y6Kja30RrAn7JKnbQG/uEtLMkBgFF2fuihg=", // current key
    "C5+lpZ7tcVwmwQIMcRtPbsQtWLABXhQzejna0wHFr8M=", // backup key (rotation)
)
if err != nil {
    log.Fatal(err)
}

cfg := httpc.DefaultConfig()
cfg.Security.CertificatePinner = pinner
client, err := httpc.New(cfg)
```

Certificate pinning also works on the fingerprint path (`Security.TLSFingerprint`): the pinner
runs as the spoofed handshake's `VerifyPeerCertificate` hook. With `InsecureSkipVerify` set,
the pinner becomes the only verification gate and pins the leaf certificate (verified chains
are unavailable when verification is skipped).

Generate an SPKI hash from a certificate:

```bash
openssl x509 -in cert.pem -pubkey -noout | openssl pkey -pubin -outform der \
  | openssl dgst -sha256 -binary | openssl enc -base64
```

| Function | Description |
|----------|-------------|
| `NewSPKIHashPinner(hashes ...string)` | Pin by base64 SHA-256 SPKI hashes (recommended; HPKP format) |
| `NewPublicKeyPinner(publicKeys ...[]byte)` | Pin by DER-encoded PKIX public keys |
| `NewCertificatePinnerChain(pinners ...CertificatePinner)` | Combine multiple pinners (accept if ANY matches) |

### Security Warning Output

By default, security warnings are printed to stderr when using insecure configurations (e.g., `TestingConfig()`, `InsecureSkipVerify: true`). Redirect or suppress these warnings:

```go
// Suppress security warnings (e.g., in CI environments)
httpc.SetSecurityWarnOutput(io.Discard)

// Or redirect to a custom logger
httpc.SetSecurityWarnOutput(os.Stderr)
```

---

## Browser Transport Fingerprinting

Modern WAFs (Cloudflare, Akamai, DataDome) fingerprint more than the User-Agent: the TLS
ClientHello (JA3/JA4), the HTTP/2 frames (SETTINGS values and order, WINDOW_UPDATE,
pseudo-header order, priorities, HPACK behavior), and the header block itself — including
a User-Agent coherent with the ClientHello version. Setting `Security.TLSFingerprint`
replaces httpc's transport layer with a browser emulation covering all of them at once,
over fully fingerprinted HTTP/2 (preferred) and HTTP/1.1 (fallback):

```go
cfg := httpc.DefaultConfig()
cfg.Security.TLSFingerprint = "chrome-latest" // or "chrome-windows", "firefox-latest", ...
client, err := httpc.New(cfg)
```

Everything above the transport stays in charge: redirects, retries, middleware, streaming
downloads, and cookie handling keep their httpc semantics.

**Scope**: fingerprinting makes the *transport layers* — TLS, HTTP/2 framing, ordered header
block — indistinguishable from the spoofed browser, which defeats passive fingerprinting
(JA3/JA4, the Akamai H2 fingerprint, header order). It does **not** defeat — no pure HTTP
client can — defenses that execute JavaScript (Cloudflare Managed Challenge / Turnstile,
DataDome, Akamai sensor), behavioral scoring, or IP reputation; those call for a real
browser or an authorized challenge-solving step. Profiles are snapshots of one browser
build and age as browsers ship: `internal/fingerprint/PROFILES.md` documents the update
runbook, and a weekly CI job watches for drift (`.github/workflows/fingerprint-drift.yml`).

**Preset names** are browser profiles with optional OS suffixes: `chrome` / `chrome-latest`
/ `chrome-155` (= Chrome 155) and `firefox` / `firefox-latest` / `firefox-158`
(= Firefox 158), each as `*-windows` / `*-macos` / `*-linux` desktop variants and
`*-android` / `*-ios` mobile variants (e.g. `chrome-windows`, `firefox-ios`), plus the
retired-major pins `chrome-152` and `firefox-148`, the legacy compound aliases
`android-chrome-latest` and `ios-chrome-latest`, and `safari-ios` (=
`ios-safari-latest`) — Safari on the same verified WebKit data as the other iOS
presets. There is no desktop-Safari profile: macOS Safari's ClientHello differs and
no verified data for it exists yet.

Behavior notes:

- **Header set is authoritative**: the preset replaces same-named request headers
  (User-Agent, Accept, Accept-Language, ...) with the browser profile's values so the wire
  stays coherent with the spoofed ClientHello; custom headers pass through untouched. An
  explicitly set User-Agent does **not** override the preset on this path. Names are written
  with the browser's own casing — lower-case `sec-ch-ua*`, Title-Case everything else — on
  HTTP/1.1 as well as HTTP/2.
- **Protocols**: ALPN decides. A server that speaks HTTP/2 gets HTTP/2, one that only speaks
  HTTP/1.1 gets HTTP/1.1, and no request is ever replayed: there is no manual fallback between
  the two. HTTP/3 is never attempted — a QUIC connection uses a separate TLS stack that
  bypasses `CertificatePinner`, and losing the pin silently is worse than not racing protocols.
  `Connection.EnableHTTP2=false` pins the path to HTTP/1.1 and rewrites the ClientHello's ALPN
  list to match.
- **Proxies**: proxy pools, rotation, and circuit breaking keep working (HTTP, HTTPS and
  SOCKS5/5h). Plain-HTTP targets through an HTTP proxy traverse a CONNECT tunnel; HTTPS proxies
  speak HTTP/1.1 CONNECT only and their own certificate is not verified (only the target's
  handshake is fingerprinted and verified). The proxy is fixed per connection pool — rotation
  selects a different pool rather than re-pointing a live one.
- **Redirect coherence**: on every redirect hop the profile's first-hop values are corrected
  the way a browser does: `sec-fetch-site` is recomputed against the previous hop's URL
  (`same-origin` / `same-site` / `cross-site`), `sec-fetch-user` is dropped (first hop
  only), and a 301/302/303 that downgrades a fetch-style request to GET keeps fetch-style
  headers instead of switching to navigation headers.
- **Certificate pinning works**: `CertificatePinner` runs on every fingerprinted handshake
  (see the pinning section above). `Security.TLSConfig` remains mutually exclusive with
  fingerprinting (a custom `tls.Config` cannot shape a spoofed ClientHello).
- **Decompression**: the profile advertises `gzip, deflate, br, zstd` and the fingerprint
  stack decodes all of them (streaming preserved), handing the engine identity bodies —
  the engine cannot see the original `Content-Encoding`, so the decoded stream is capped
  by `MaxResponseBodySize` (`MaxDecompressedBodySize` does not apply on this path). An
  unknown `Content-Encoding` fails the request loudly instead of returning compressed
  bytes.
- **SSRF protection** runs as a resolve-and-validate pre-flight per request (including each
  redirect hop); it is marginally weaker than the standard path's dial-layer validation
  (theoretical DNS TOCTOU window).
- **Limitations**: fingerprinted connections are not counted in pool metrics and skip
  `Connection.EnableDoH`. Android presets (`*-android`)
  currently emit the desktop fingerprint bytes — placeholders pending real device
  captures (see `internal/fingerprint/PROFILES.md`). iOS presets (`*-ios`, including
  `safari-ios`) use real WebKit data (Safari 26.3 TLS + HTTP/2, byte-verified live),
  with the regular-header order still a desktop approximation.
- **Session resumption**: the Chrome profiles cache TLS session tickets per connection
  pool and resume repeat connections with a PSK, exactly like a real browser revisiting
  a host (verified live in `session_resumption_test.go`). Profiles whose ClientHello
  carries no pre_shared_key extension (Firefox, the iOS WebKit profiles) always
  full-handshake. Set `Security.TLSFingerprintDisableSessionTickets` to force a full
  handshake on every connection.
- **Verified end-to-end**: `chrome-latest` negotiates h2 with the Chrome Akamai fingerprint
  `1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p` (hash `52d84b11737d980aef856699f885ca86`)
  and a Chrome 155 UA; `firefox-latest` yields
  `1:65536;2:0;4:131072;5:16384|12517377|0|m,p,a,s` (hash `6ea73faa8fc5aac76bded7bd238f6433`)
  with a Firefox 158 UA; `chrome-ios` and `safari-ios` yield the WebKit JA4
  `t13d2013h2_a09f3c656075_7f0f34a4126d` with Akamai
  `2:0;3:100;4:2097152;9:1|10420225|0|m,s,a,p` (hash `c52879e43202aeb92740be6e8c86ea96`) —
  see `internal/fingerprint/e2e_network_test.go`
  (`go test -tags fingerprint_e2e ./internal/fingerprint/ -run PeetWSE2E`).

The fingerprint data lives in this repository (`internal/fingerprint`): httpc builds the
ClientHello spec, the HTTP/2 SETTINGS sequence and the header tables, and hands them to
[`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) through its profile API —
so a profile can target a browser build ahead of any upstream release. The transport itself
(uTLS handshake, HTTP/2 framing, header order and casing) comes from that library, and
[`andybalholm/brotli`](https://github.com/andybalholm/brotli) +
[`klauspost/compress`](https://github.com/klauspost/compress) (zstd) handle decoding.
Profile data was originally ported from [`enetx/surf`](https://github.com/enetx/surf) (MIT);
httpc does not depend on that client library, nor on `enetx/http`, `enetx/http2` or
`refraction-networking/utls`. Leave `TLSFingerprint` empty to keep the standard stack with
no extra dependencies.

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

### ClientError Fields

| Field | Type | Description |
|-------|------|-------------|
| `Type` | `ErrorType` | Error classification |
| `Message` | `string` | Human-readable error description |
| `Cause` | `error` | Underlying error (unwrap with `%w`) |
| `URL` | `string` | Request URL |
| `Method` | `string` | HTTP method |
| `Attempts` | `int` | Total attempts made, including the first |
| `StatusCode` | `int` | HTTP status code (if applicable) |
| `Host` | `string` | Reserved; currently never set by the engine |

### Error Types

```go
const (
    ErrorTypeUnknown        // Unknown or unclassified error
    ErrorTypeNetwork        // Network-level error (connection refused, DNS failure)
    ErrorTypeTimeout        // Request timeout
    ErrorTypeContextCanceled // Context canceled
    ErrorTypeResponseRead   // Error reading response body
    ErrorTypeTransport      // HTTP transport error
    ErrorTypeRetryExhausted // Reserved for retry exhaustion; retries actually exhaust with the last error's type kept — check Attempts / IsRetryable()
    ErrorTypeTLS            // TLS handshake error
    ErrorTypeCertificate    // Certificate validation error
    ErrorTypeDNS            // DNS resolution error
    ErrorTypeValidation     // Request validation error
    ErrorTypeHTTP           // HTTP-level error (4xx, 5xx)
)
```

### Sentinel Errors

```go
var (
    ErrClientClosed         // Client has been closed
    ErrNilConfig            // Nil configuration provided
    ErrInvalidHeader        // Header validation failed
    ErrInvalidTimeout       // Timeout is negative or exceeds limits
    ErrInvalidRetry         // Retry configuration is invalid
    ErrInvalidConnection    // Connection configuration is invalid
    ErrInvalidSecurity      // Security configuration is invalid
    ErrInvalidMiddleware    // Middleware configuration is invalid
    ErrEmptyFilePath        // File path is empty
    ErrFileExists           // File already exists (and Overwrite is false)
    ErrResponseBodyEmpty    // Response body is empty
    ErrResponseBodyTooLarge // Response body exceeds size limit
)
```

### Error Classification

```go
// Check error type using errors.As
var clientErr *httpc.ClientError
if errors.As(err, &clientErr) {
    fmt.Printf("Type: %s, Retryable: %v\n", clientErr.Code(), clientErr.IsRetryable())
}
```

---

## Concurrency Safety

HTTPC is designed to be goroutine-safe:

```go
client, _ := httpc.NewDefault() // Default configuration
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

### Default Client Management

Package-level functions (`Get`, `Post`, etc.) use a shared default client. You can customize it:

```go
// Set a custom default client
customClient, _ := httpc.New(httpc.SecureConfig())
_ = httpc.SetDefaultClient(customClient)

// Close and reset the default client
_ = httpc.CloseDefaultClient()
```

**Thread Safety Guarantees:**
- All `Client` methods are safe for concurrent use
- Package-level functions safely use a shared default client
- `Result` objects are immutable once returned — concurrent reads are safe; just avoid multiple goroutines assigning to the same `Result` variable
- Internal metrics use atomic operations

---

## Interfaces

HTTPC exposes interfaces for testing and extensibility. Use these when you need to mock HTTP calls in your tests:

### Doer

The simplest interface — a single `Request` method:

```go
type Doer interface {
    Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error)
}
```

### Client

The full client interface — extends `Doer` with convenience methods and download support:

```go
type Client interface {
    Doer
    Get(url string, options ...RequestOption) (*Result, error)
    Post(url string, options ...RequestOption) (*Result, error)
    Put(url string, options ...RequestOption) (*Result, error)
    Patch(url string, options ...RequestOption) (*Result, error)
    Delete(url string, options ...RequestOption) (*Result, error)
    Head(url string, options ...RequestOption) (*Result, error)
    Options(url string, options ...RequestOption) (*Result, error)
    Download(ctx context.Context, url string, cfg *DownloadConfig, options ...RequestOption) (*DownloadResult, error)
    Close() error
}
```

### DomainClienter

Extends `Client` with domain-scoped session management:

```go
type DomainClienter interface {
    Client
    URL() string
    Domain() string
    SetHeader(key, value string) error
    SetHeaders(headers map[string]string) error
    DeleteHeader(key string)
    ClearHeaders()
    GetHeaders() map[string]string
    SetCookie(cookie *http.Cookie) error
    SetCookies(cookies []*http.Cookie) error
    DeleteCookie(name string)
    ClearCookies()
    GetCookies() []*http.Cookie
    GetCookie(name string) *http.Cookie
    Session() *SessionManager
}
```

### Usage in Tests

```go
type MockClient struct {
    httpc.Client // embed for forward compatibility
}

func (m *MockClient) Get(url string, options ...httpc.RequestOption) (*httpc.Result, error) {
    return &httpc.Result{
        Response: &httpc.ResponseInfo{StatusCode: 200, Body: `{"ok": true}`},
    }, nil
}
```

---

## Documentation

| Resource | Description |
|----------|-------------|
| [Getting Started](docs/01_getting-started.md) | Installation and first steps |
| [Configuration](docs/02_configuration.md) | Client configuration and presets |
| [Request Options](docs/03_request-options.md) | Complete options reference |
| [Error Handling](docs/04_error-handling.md) | Error handling patterns |
| [HTTP Redirects](docs/05_redirects.md) | Redirect handling and tracking |
| [Cookie API](docs/06_cookie-api.md) | Cookie management |
| [File Download](docs/07_file-download.md) | File download with progress |
| [Request Inspection](docs/08_request-inspection.md) | Request/response inspection |
| [Concurrency Safety](docs/09_concurrency-safety.md) | Thread safety guarantees |
| [Security](SECURITY.md) | Security features and best practices |

### Example Code

22 runnable examples covering all features, ordered from basic to advanced.
Each example is a standalone `package main` guarded by a `//go:build examples`
tag (so it stays out of the normal build), so run them one at a time:

```bash
go run -tags examples examples/01_basic_usage.go
```

> Examples that call live endpoints (`httpbin.org`, `example.com`) require
> network access. The certificate-pinning and SSRF-protection examples are
> self-contained — a rejection *is* the protection working as designed.

| Category | Examples |
|----------|----------|
| **Basics** | [01_basic_usage](examples/01_basic_usage.go), [02_http_methods](examples/02_http_methods.go), [03_response_handling](examples/03_response_handling.go), [04_compression](examples/04_compression.go) |
| **Core Features** | [05_request_options](examples/05_request_options.go), [06_error_handling](examples/06_error_handling.go), [07_timeout_retry](examples/07_timeout_retry.go), [08_client_configuration](examples/08_client_configuration.go), [09_redirects](examples/09_redirects.go), [10_cookies_advanced](examples/10_cookies_advanced.go) |
| **Stateful Clients** | [11_session](examples/11_session.go), [12_domain_client](examples/12_domain_client.go), [13_proxy_configuration](examples/13_proxy_configuration.go), [14_doh](examples/14_doh.go) |
| **Advanced** | [15_middleware](examples/15_middleware.go), [16_concurrent_requests](examples/16_concurrent_requests.go), [17_file_operations](examples/17_file_operations.go), [18_rest_api_client](examples/18_rest_api_client.go), [19_advanced_patterns](examples/19_advanced_patterns.go) |
| **Security** | [20_certificate_pinning](examples/20_certificate_pinning.go), [21_ssrf_protection](examples/21_ssrf_protection.go) |
| **Streaming** | [22_streaming_bodies](examples/22_streaming_bodies.go) |

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

If this project helps you, please give it a Star!
