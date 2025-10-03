# HTTPC Quick Reference Guide

## Table of Contents

- [Installation](#installation)
- [Basic Usage](#basic-usage)
- [Quick Lookup](#quick-lookup)
- [HTTP Methods](#http-methods)
- [Request Options](#request-options)
  - [Headers](#headers)
  - [Authentication](#authentication)
  - [Query Parameters](#query-parameters)
  - [Body Formats](#body-formats)
  - [File Upload](#file-upload)
  - [Timeout & Retry](#timeout--retry)
  - [Cookies](#cookies)
  - [Request Options Reference](#request-options-reference)
- [Response Handling](#response-handling)
  - [Response Structure](#response-structure)
  - [Status Checking](#status-checking)
  - [Parsing](#parsing)
  - [Cookies](#cookies-1)
  - [Headers](#headers-1)
  - [Metadata](#metadata)
- [Configuration](#configuration)
  - [Default Config](#default-config)
  - [Security Presets](#security-presets)
  - [Custom Config](#custom-config)
  - [Configuration Fields Reference](#configuration-fields-reference)
- [Error Handling](#error-handling)
  - [Error Types](#error-types)
  - [Error Checking](#error-checking)
- [Common Patterns](#common-patterns)
  - [REST API Client](#rest-api-client)
  - [With Context](#with-context)
  - [File Upload with Metadata](#file-upload-with-metadata)
  - [Retry with Backoff](#retry-with-backoff)
  - [Batch Requests](#batch-requests)
  - [Streaming Large Response](#streaming-large-response)
  - [Conditional Requests](#conditional-requests)
  - [Rate Limiting](#rate-limiting)
- [Package-Level Functions](#package-level-functions)
- [File Download](#file-download)
  - [Simple Download](#simple-download)
  - [Download with Progress](#download-with-progress)
  - [Resume Download](#resume-download)
  - [Download Options](#download-options)
  - [Download Result](#download-result)
  - [Save Response to File](#save-response-to-file)
  - [Utility Functions](#utility-functions)
- [Advanced Features](#advanced-features)
  - [Custom Cookie Jar](#custom-cookie-jar)
  - [Proxy Configuration](#proxy-configuration)
  - [Custom TLS Configuration](#custom-tls-configuration)
  - [Default Headers](#default-headers)
  - [Response Body Size Limit](#response-body-size-limit)
  - [Disable Redirects](#disable-redirects)
  - [Concurrent Request Limiting](#concurrent-request-limiting)
- [Best Practices](#best-practices)
- [Performance Tips](#performance-tips)
- [Common Issues & Solutions](#common-issues--solutions)
- [Tips & Tricks](#tips--tricks)
- [Links](#links)

## Installation

```bash
go get -u github.com/cybergodev/httpc
```

## Basic Usage

```go
// Create client
client, err := httpc.New()
defer client.Close()

// Simple GET
resp, err := client.Get("https://api.example.com/users")

// POST JSON
resp, err := client.Post(url, httpc.WithJSON(data))
```

## Quick Lookup

| I want to...        | Use this                                                   |
|---------------------|------------------------------------------------------------|
| Send GET request    | `client.Get(url)`                                          |
| Send POST with JSON | `client.Post(url, httpc.WithJSON(data))`                   |
| Add headers         | `httpc.WithHeader("key", "value")`                         |
| Add authentication  | `httpc.WithBearerToken("token")`                           |
| Set timeout         | `httpc.WithTimeout(30*time.Second)`                        |
| Upload file         | `httpc.WithFile("field", "file.pdf", content)`             |
| Download file       | `httpc.DownloadFile(url, "path/to/file")`                  |
| Parse JSON response | `resp.JSON(&result)`                                       |
| Check if successful | `resp.IsSuccess()`                                         |
| Get response header | `resp.Headers.Get("Content-Type")`                         |
| Handle errors       | `errors.As(err, &httpErr)`                                 |
| Configure client    | `httpc.New(&httpc.Config{...})`                            |
| Use security preset | `httpc.New(httpc.ConfigPreset(httpc.SecurityLevelStrict))` |
| Enable retries      | `config.MaxRetries = 3`                                    |
| Set proxy           | `config.ProxyURL = "http://proxy:8080"`                    |
| Manage cookies      | `httpc.WithCookieValue("name", "value")`                   |

## HTTP Methods

| Method  | Usage               | Example                                        |
|---------|---------------------|------------------------------------------------|
| GET     | Retrieve resources  | `client.Get(url, options...)`                  |
| POST    | Create resources    | `client.Post(url, options...)`                 |
| PUT     | Update resources    | `client.Put(url, options...)`                  |
| PATCH   | Partial update      | `client.Patch(url, options...)`                |
| DELETE  | Remove resources    | `client.Delete(url, options...)`               |
| HEAD    | Get headers only    | `client.Head(url, options...)`                 |
| OPTIONS | Get allowed methods | `client.Options(url, options...)`              |
| Generic | Custom method       | `client.Request(ctx, method, url, options...)` |

## Request Options

### Headers

```go
httpc.WithHeader("X-Custom", "value")
httpc.WithHeaderMap(map[string]string{"X-API": "v1"})
httpc.WithUserAgent("MyApp/1.0")
httpc.WithContentType("application/json")
httpc.WithAccept("application/json")
httpc.WithJSONAccept()
httpc.WithXMLAccept()
```

### Authentication

```go
httpc.WithBearerToken("jwt-token")
httpc.WithBasicAuth("username", "password")
httpc.WithHeader("X-API-Key", "api-key")
```

### Query Parameters

```go
httpc.WithQuery("page", 1)
httpc.WithQueryMap(map[string]interface{}{
    "page": 1,
    "limit": 20,
})
```

### Body Formats

```go
httpc.WithJSON(data)                              // JSON
httpc.WithXML(data)                               // XML
httpc.WithForm(map[string]string{...})            // Form data
httpc.WithText("content")                         // Plain text
httpc.WithBinary([]byte{...}, "image/png")        // Binary
httpc.WithBody(data)                              // Raw body
```

### File Upload

```go
// Single file
httpc.WithFile("file", "doc.pdf", content)

// Multiple files
httpc.WithFormData(&httpc.FormData{
    Fields: map[string]string{"title": "Doc"},
    Files: map[string]*httpc.FileData{
        "file": {
            Filename: "doc.pdf",
            Content: content,
            ContentType: "application/pdf",
        },
    },
})
```

### Timeout & Retry

```go
httpc.WithTimeout(30 * time.Second)
httpc.WithMaxRetries(3)
httpc.WithContext(ctx)
```

### Cookies

```go
httpc.WithCookieValue("session", "abc123")
httpc.WithCookie(&http.Cookie{Name: "auth", Value: "xyz"})
httpc.WithCookies([]*http.Cookie{...})
```

### Request Options Reference

| Category    | Option            | Description           | Example                                     |
|-------------|-------------------|-----------------------|---------------------------------------------|
| **Headers** | `WithHeader`      | Set single header     | `WithHeader("X-Custom", "value")`           |
|             | `WithHeaderMap`   | Set multiple headers  | `WithHeaderMap(map[string]string{...})`     |
|             | `WithUserAgent`   | Set User-Agent        | `WithUserAgent("MyApp/1.0")`                |
|             | `WithContentType` | Set Content-Type      | `WithContentType("application/json")`       |
|             | `WithAccept`      | Set Accept header     | `WithAccept("application/json")`            |
|             | `WithJSONAccept`  | Accept JSON           | `WithJSONAccept()`                          |
|             | `WithXMLAccept`   | Accept XML            | `WithXMLAccept()`                           |
| **Auth**    | `WithBearerToken` | Bearer token auth     | `WithBearerToken("token")`                  |
|             | `WithBasicAuth`   | Basic authentication  | `WithBasicAuth("user", "pass")`             |
| **Query**   | `WithQuery`       | Single query param    | `WithQuery("page", 1)`                      |
|             | `WithQueryMap`    | Multiple query params | `WithQueryMap(map[string]interface{}{...})` |
| **Body**    | `WithJSON`        | JSON body             | `WithJSON(data)`                            |
|             | `WithXML`         | XML body              | `WithXML(data)`                             |
|             | `WithForm`        | Form data             | `WithForm(map[string]string{...})`          |
|             | `WithText`        | Plain text            | `WithText("content")`                       |
|             | `WithBinary`      | Binary data           | `WithBinary([]byte{...}, "image/png")`      |
|             | `WithBody`        | Raw body              | `WithBody(data)`                            |
| **Files**   | `WithFile`        | Single file upload    | `WithFile("field", "file.pdf", content)`    |
|             | `WithFormData`    | Multipart form data   | `WithFormData(&FormData{...})`              |
| **Control** | `WithTimeout`     | Request timeout       | `WithTimeout(30*time.Second)`               |
|             | `WithContext`     | Request context       | `WithContext(ctx)`                          |
|             | `WithMaxRetries`  | Max retry attempts    | `WithMaxRetries(3)`                         |
| **Cookies** | `WithCookie`      | Single cookie         | `WithCookie(&http.Cookie{...})`             |
|             | `WithCookies`     | Multiple cookies      | `WithCookies([]*http.Cookie{...})`          |
|             | `WithCookieValue` | Simple cookie         | `WithCookieValue("name", "value")`          |

## Response Handling

### Response Structure

```go
type Response struct {
    StatusCode    int              // HTTP status code
    Status        string           // HTTP status text
    Headers       http.Header      // Response headers
    Body          string           // Response body as string
    RawBody       []byte           // Response body as bytes
    ContentLength int64            // Content length
    Proto         string           // Protocol version (e.g., "HTTP/2.0")
    Duration      time.Duration    // Request duration
    Attempts      int              // Number of retry attempts
    Request       interface{}      // Original *http.Request
    Response      interface{}      // Original *http.Response
    Cookies       []*http.Cookie   // Response cookies
}
```

### Status Checking

```go
resp.IsSuccess()      // 2xx (200-299)
resp.IsRedirect()     // 3xx (300-399)
resp.IsClientError()  // 4xx (400-499)
resp.IsServerError()  // 5xx (500-599)

// Example usage
if resp.IsSuccess() {
    fmt.Println("Request succeeded")
} else if resp.IsClientError() {
    fmt.Printf("Client error: %d\n", resp.StatusCode)
}
```

### Parsing

```go
// Parse JSON
var result map[string]interface{}
err := resp.JSON(&result)

// Parse JSON into struct
type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}
var user User
err := resp.JSON(&user)

// Parse XML
var data XMLStruct
err := resp.XML(&data)

// Get body as string
body := resp.Body

// Get body as bytes
raw := resp.RawBody
```

### Cookies

```go
// Get specific cookie
cookie := resp.GetCookie("session")
if cookie != nil {
    fmt.Printf("Session: %s\n", cookie.Value)
}

// Check if cookie exists
hasAuth := resp.HasCookie("auth")

// Get all cookies
allCookies := resp.Cookies
for _, cookie := range allCookies {
    fmt.Printf("%s = %s\n", cookie.Name, cookie.Value)
}
```

### Headers

```go
// Get single header
contentType := resp.Headers.Get("Content-Type")

// Get all values for a header
setCookies := resp.Headers.Values("Set-Cookie")

// Check if header exists
if resp.Headers.Get("X-Custom") != "" {
    fmt.Println("Custom header present")
}

// Iterate all headers
for key, values := range resp.Headers {
    for _, value := range values {
        fmt.Printf("%s: %s\n", key, value)
    }
}
```

### Metadata

```go
// Status information
fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)
fmt.Printf("Protocol: %s\n", resp.Proto)

// Performance metrics
fmt.Printf("Duration: %v\n", resp.Duration)
fmt.Printf("Attempts: %d\n", resp.Attempts)

// Content information
fmt.Printf("Content-Length: %d bytes\n", resp.ContentLength)
```

## Configuration

### Default Config

```go
client, err := httpc.New()  // Secure defaults
```

### Security Presets

```go
// Permissive (Development)
httpc.New(httpc.ConfigPreset(httpc.SecurityLevelPermissive))

// Balanced (Production - Default)
httpc.New(httpc.ConfigPreset(httpc.SecurityLevelBalanced))

// Strict (High Security)
httpc.New(httpc.ConfigPreset(httpc.SecurityLevelStrict))
```

### Custom Config

```go
config := &httpc.Config{
    // Timeouts
    Timeout:               30 * time.Second,
    DialTimeout:           15 * time.Second,
    KeepAlive:             30 * time.Second,
    TLSHandshakeTimeout:   15 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    IdleConnTimeout:       90 * time.Second,

    // Connection Pool
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
    MaxConnsPerHost:     20,

    // Security
    MinTLSVersion:         tls.VersionTLS12,
    MaxTLSVersion:         tls.VersionTLS13,
    InsecureSkipVerify:    false,
    MaxResponseBodySize:   50 * 1024 * 1024,  // 50MB
    MaxConcurrentRequests: 500,
    ValidateURL:           true,
    ValidateHeaders:       true,
    AllowPrivateIPs:       false,

    // Retry
    MaxRetries:    3,
    RetryDelay:    2 * time.Second,
    MaxRetryDelay: 60 * time.Second,
    BackoffFactor: 2.0,
    Jitter:        true,

    // Features
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    EnableHTTP2:     true,
    EnableCookies:   true,

    // Optional
    ProxyURL:  "http://proxy.example.com:8080",
    Headers:   map[string]string{"X-Custom": "value"},
}
client, err := httpc.New(config)
```

### Configuration Fields Reference

| Category            | Field                   | Description               | Default     |
|---------------------|-------------------------|---------------------------|-------------|
| **Timeouts**        | `Timeout`               | Overall request timeout   | 60s         |
|                     | `DialTimeout`           | TCP connection timeout    | 15s         |
|                     | `KeepAlive`             | Keep-alive probe interval | 30s         |
|                     | `TLSHandshakeTimeout`   | TLS handshake timeout     | 15s         |
|                     | `ResponseHeaderTimeout` | Response header timeout   | 30s         |
|                     | `IdleConnTimeout`       | Idle connection timeout   | 90s         |
| **Connection Pool** | `MaxIdleConns`          | Max idle connections      | 100         |
|                     | `MaxIdleConnsPerHost`   | Max idle per host         | 10          |
|                     | `MaxConnsPerHost`       | Max connections per host  | 20          |
| **Security**        | `MinTLSVersion`         | Minimum TLS version       | TLS 1.2     |
|                     | `MaxTLSVersion`         | Maximum TLS version       | TLS 1.3     |
|                     | `InsecureSkipVerify`    | Skip TLS verification     | false       |
|                     | `MaxResponseBodySize`   | Max response body size    | 50MB        |
|                     | `MaxConcurrentRequests` | Max concurrent requests   | 500         |
|                     | `ValidateURL`           | Validate URL format       | true        |
|                     | `ValidateHeaders`       | Validate headers          | true        |
|                     | `AllowPrivateIPs`       | Allow private IPs         | false       |
| **Retry**           | `MaxRetries`            | Max retry attempts        | 2           |
|                     | `RetryDelay`            | Initial retry delay       | 2s          |
|                     | `MaxRetryDelay`         | Max retry delay           | 60s         |
|                     | `BackoffFactor`         | Backoff multiplier        | 2.0         |
|                     | `Jitter`                | Add random jitter         | true        |
| **Features**        | `UserAgent`             | User-Agent header         | "httpc/1.0" |
|                     | `FollowRedirects`       | Follow redirects          | true        |
|                     | `EnableHTTP2`           | Enable HTTP/2             | true        |
|                     | `EnableCookies`         | Enable cookie jar         | true        |
| **Optional**        | `ProxyURL`              | HTTP proxy URL            | ""          |
|                     | `Headers`               | Default headers           | {}          |
|                     | `TLSConfig`             | Custom TLS config         | nil         |
|                     | `CookieJar`             | Custom cookie jar         | nil         |

## Error Handling

### Error Types

```go
// HTTPError - HTTP error response
type HTTPError struct {
    StatusCode int
    Status     string
    URL        string
    Method     string
}
```

### Error Checking

```go
resp, err := client.Get(url)
if err != nil {
    // Check HTTP error
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Status)
        fmt.Printf("URL: %s\n", httpErr.URL)
        fmt.Printf("Method: %s\n", httpErr.Method)
    }

    // Check circuit breaker
    if strings.Contains(err.Error(), "circuit breaker is open") {
        return fallbackData, nil
    }

    // Check timeout
    if strings.Contains(err.Error(), "timeout") {
        return nil, fmt.Errorf("request timed out")
    }

    // Check context cancellation
    if errors.Is(err, context.Canceled) {
        return nil, fmt.Errorf("request cancelled")
    }

    return err
}

// Check response status
if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

## Common Patterns

### REST API Client

```go
type APIClient struct {
    client  httpc.Client
    baseURL string
    token   string
}

func (c *APIClient) GetUser(id string) (*User, error) {
    resp, err := c.client.Get(c.baseURL+"/users/"+id,
        httpc.WithBearerToken(c.token),
        httpc.WithJSONAccept(),
    )
    if err != nil {
        return nil, err
    }
    
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, err
    }
    
    return &user, nil
}
```

### With Context

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Get(url, httpc.WithContext(ctx))
```

### File Upload with Metadata

```go
formData := &httpc.FormData{
    Fields: map[string]string{
        "title":       "Document",
        "description": "Important file",
    },
    Files: map[string]*httpc.FileData{
        "file": {
            Filename:    "doc.pdf",
            Content:     fileContent,
            ContentType: "application/pdf",
        },
    },
}

resp, err := client.Post(url,
    httpc.WithFormData(formData),
    httpc.WithBearerToken(token),
    httpc.WithTimeout(60*time.Second),
)
```

### Retry with Backoff

```go
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.RetryDelay = 1 * time.Second
config.MaxRetryDelay = 30 * time.Second
config.BackoffFactor = 2.0
config.Jitter = true

client, err := httpc.New(config)
```

### Batch Requests

```go
type Result struct {
    URL  string
    Resp *httpc.Response
    Err  error
}

func fetchAll(client httpc.Client, urls []string) []Result {
    results := make([]Result, len(urls))
    var wg sync.WaitGroup

    for i, url := range urls {
        wg.Add(1)
        go func(idx int, u string) {
            defer wg.Done()
            resp, err := client.Get(u)
            results[idx] = Result{URL: u, Resp: resp, Err: err}
        }(i, url)
    }

    wg.Wait()
    return results
}
```

### Streaming Large Response

```go
resp, err := client.Get(url)
if err != nil {
    return err
}

// Process response body in chunks
reader := bytes.NewReader(resp.RawBody)
buffer := make([]byte, 4096)

for {
    n, err := reader.Read(buffer)
    if n > 0 {
        // Process chunk
        processChunk(buffer[:n])
    }
    if err == io.EOF {
        break
    }
    if err != nil {
        return err
    }
}
```

### Conditional Requests

```go
// Using ETag
resp1, _ := client.Get(url)
etag := resp1.Headers.Get("ETag")

resp2, _ := client.Get(url,
    httpc.WithHeader("If-None-Match", etag),
)
if resp2.StatusCode == 304 {
    fmt.Println("Content not modified, use cached version")
}

// Using Last-Modified
lastModified := resp1.Headers.Get("Last-Modified")
resp3, _ := client.Get(url,
    httpc.WithHeader("If-Modified-Since", lastModified),
)
```

### Rate Limiting

```go
import "golang.org/x/time/rate"

type RateLimitedClient struct {
    client  httpc.Client
    limiter *rate.Limiter
}

func (c *RateLimitedClient) Get(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    if err := c.limiter.Wait(context.Background()); err != nil {
        return nil, err
    }
    return c.client.Get(url, opts...)
}

// Usage
limiter := rate.NewLimiter(rate.Limit(10), 1) // 10 requests per second
rateLimited := &RateLimitedClient{
    client:  client,
    limiter: limiter,
}
```

## Package-Level Functions

```go
// Quick requests without creating a client
resp, err := httpc.Get(url)
resp, err := httpc.Post(url, httpc.WithJSON(data))
resp, err := httpc.Put(url, options...)
resp, err := httpc.Patch(url, options...)
resp, err := httpc.Delete(url, options...)
resp, err := httpc.Head(url, options...)
resp, err := httpc.Options(url, options...)
resp, err := httpc.Do(ctx, method, url, options...)

// File download
result, err := httpc.DownloadFile(url, filePath)
```

## File Download

### Simple Download
```go
result, err := httpc.DownloadFile(url, "downloads/file.zip")
fmt.Printf("Downloaded: %s\n", httpc.FormatBytes(result.BytesWritten))
fmt.Printf("Speed: %s\n", httpc.FormatSpeed(result.AverageSpeed))
fmt.Printf("Duration: %v\n", result.Duration)
```

### Download with Progress
```go
opts := httpc.DefaultDownloadOptions("downloads/file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s/%s - %s",
        percentage,
        httpc.FormatBytes(downloaded),
        httpc.FormatBytes(total),
        httpc.FormatSpeed(speed))
}
result, err := client.DownloadFileWithOptions(url, opts)
```

### Resume Download
```go
opts := httpc.DefaultDownloadOptions("downloads/file.zip")
opts.ResumeDownload = true
result, err := client.DownloadFileWithOptions(url, opts)
if result.Resumed {
    fmt.Println("Download resumed from previous attempt")
}
```

### Download Options

```go
opts := &httpc.DownloadOptions{
    FilePath:         "downloads/file.zip",
    ProgressCallback: progressFunc,
    ProgressInterval: 500 * time.Millisecond,  // Progress update frequency
    BufferSize:       32 * 1024,               // 32KB buffer
    CreateDirs:       true,                    // Create parent directories
    Overwrite:        false,                   // Don't overwrite existing files
    ResumeDownload:   false,                   // Don't resume partial downloads
    FileMode:         0644,                    // File permissions
}
result, err := client.DownloadFileWithOptions(url, opts)
```

### Download Result

```go
type DownloadResult struct {
    FilePath      string        // Downloaded file path
    BytesWritten  int64         // Total bytes written
    Duration      time.Duration // Download duration
    AverageSpeed  float64       // Average speed (bytes/sec)
    StatusCode    int           // HTTP status code
    ContentLength int64         // Content length from header
    Resumed       bool          // Whether download was resumed
}
```

### Save Response to File
```go
resp, err := client.Get(url)
err = resp.SaveToFile("output.json")
```

### Utility Functions

```go
// Format bytes to human-readable string
size := httpc.FormatBytes(1024 * 1024)  // "1.00 MB"

// Format speed to human-readable string
speed := httpc.FormatSpeed(1024 * 1024)  // "1.00 MB/s"
```

## Advanced Features

### Custom Cookie Jar

```go
// Create custom cookie jar
jar, err := httpc.NewCookieJar()

config := &httpc.Config{
    CookieJar:     jar,
    EnableCookies: true,
}
client, err := httpc.New(config)
```

### Proxy Configuration

```go
config := &httpc.Config{
    ProxyURL: "http://proxy.example.com:8080",
}
client, err := httpc.New(config)
```

### Custom TLS Configuration

```go
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    MaxVersion: tls.VersionTLS13,
    // Add custom certificates, etc.
}

config := &httpc.Config{
    TLSConfig: tlsConfig,
}
client, err := httpc.New(config)
```

### Default Headers

```go
config := &httpc.Config{
    Headers: map[string]string{
        "X-API-Version": "v1",
        "X-Client-ID":   "my-app",
    },
}
client, err := httpc.New(config)
```

### Response Body Size Limit

```go
config := &httpc.Config{
    MaxResponseBodySize: 10 * 1024 * 1024,  // 10MB limit
}
client, err := httpc.New(config)
```

### Disable Redirects

```go
config := &httpc.Config{
    FollowRedirects: false,
}
client, err := httpc.New(config)
```

### Concurrent Request Limiting

```go
config := &httpc.Config{
    MaxConcurrentRequests: 100,  // Limit concurrent requests
}
client, err := httpc.New(config)
```

## Best Practices

1. **Always close the client**: `defer client.Close()`
2. **Reuse clients**: Create once, use for multiple requests
3. **Use context**: For cancellation and timeouts
4. **Handle errors properly**: Check error types and status codes
5. **Set appropriate timeouts**: Avoid hanging requests
6. **Use connection pooling**: Configure pool sizes for your workload
7. **Enable HTTP/2**: Better performance (enabled by default)
8. **Validate responses**: Check `IsSuccess()` before processing
9. **Limit response body size**: Prevent memory exhaustion
10. **Use security presets**: Choose appropriate security level for your environment

## Performance Tips

- **Reuse HTTP clients**: Create once, use for multiple requests
- **Configure connection pool**: Adjust `MaxIdleConns` and `MaxConnsPerHost` for your workload
- **Use HTTP/2**: Enabled by default for better performance
- **Set reasonable timeouts**: Prevent hanging requests
- **Use context**: For proper cancellation and timeout control
- **Enable buffer pooling**: Automatic, reduces GC pressure
- **Limit concurrent requests**: Use `MaxConcurrentRequests` to prevent overload
- **Batch requests**: Use goroutines for parallel requests

## Common Issues & Solutions

### Issue: Request Timeout

```go
// Solution: Increase timeout
client.Get(url, httpc.WithTimeout(60*time.Second))

// Or configure globally
config := httpc.DefaultConfig()
config.Timeout = 60 * time.Second
```

### Issue: Too Many Open Connections

```go
// Solution: Adjust connection pool settings
config := httpc.DefaultConfig()
config.MaxIdleConns = 50
config.MaxIdleConnsPerHost = 5
config.MaxConnsPerHost = 10
```

### Issue: Large Response Body

```go
// Solution: Set response body size limit
config := httpc.DefaultConfig()
config.MaxResponseBodySize = 100 * 1024 * 1024  // 100MB
```

### Issue: SSL/TLS Errors

```go
// Solution 1: Update TLS version
config := httpc.DefaultConfig()
config.MinTLSVersion = tls.VersionTLS12

// Solution 2: For development only (NOT for production)
config.InsecureSkipVerify = true
```

### Issue: Rate Limiting

```go
// Solution: Implement retry with backoff
config := httpc.DefaultConfig()
config.MaxRetries = 5
config.RetryDelay = 1 * time.Second
config.MaxRetryDelay = 30 * time.Second
config.BackoffFactor = 2.0
config.Jitter = true
```

## Tips & Tricks

### Debugging Requests

```go
resp, err := client.Get(url)
if err != nil {
    log.Printf("Error: %v", err)
}

// Log request details
log.Printf("Status: %d", resp.StatusCode)
log.Printf("Duration: %v", resp.Duration)
log.Printf("Attempts: %d", resp.Attempts)
log.Printf("Protocol: %s", resp.Proto)
```

### Reusing Response Data

```go
resp, _ := client.Get(url)

// Parse JSON multiple times
var data1 map[string]interface{}
resp.JSON(&data1)

var data2 MyStruct
resp.JSON(&data2)  // RawBody is still available
```

### Custom User Agent

```go
// Per request
client.Get(url, httpc.WithUserAgent("MyBot/1.0"))

// Global default
config := httpc.DefaultConfig()
config.UserAgent = "MyBot/1.0"
```

### Testing with Private IPs

```go
// Allow requests to localhost/private IPs (for testing)
config := httpc.DefaultConfig()
config.AllowPrivateIPs = true
```

## Links

- [Full Documentation](README.md)
- [Getting Started Guide](docs/getting-started.md)
- [Configuration Guide](docs/configuration.md)
- [Request Options Guide](docs/request-options.md)
- [Circuit Breaker](docs/circuit-breaker.md)
- [Examples](examples)

