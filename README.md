# HTTPC - Modern HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](docs/security.md)
[![Performance](https://img.shields.io/badge/performance-high%20performance-green.svg)](https://github.com/cybergodev/json)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](https://github.com/cybergodev/json)

An elegant, high-performance HTTP client library for Go, engineered for production-grade applications. Features enterprise-level security, intelligent concurrency control with goroutine-safe operations, zero-allocation buffer pooling, and adaptive connection management. Designed to handle thousands of concurrent requests while maintaining memory efficiency and thread safety across all operations.

#### **[📖 中文文档](README_zh-CN.md)** - User guide

---

## ✨ Why HTTPC?

- 🛡️ **Secure by Default** - TLS 1.2+, input validation, CRLF protection, SSRF prevention
- ⚡ **High Performance** - Goroutine-safe operations, zero-allocation buffer pooling (90% less GC pressure), intelligent connection reuse
- 🚀 **Massive Concurrency** - Handle concurrent requests with adaptive semaphore control and per-host connection limits
- 🔒 **Thread-Safe** - All operations are goroutine-safe with lock-free atomic counters and synchronized state management
- 🔄 **Built-in Resilience** - Circuit breaker, intelligent retry with exponential backoff, graceful degradation
- 🎯 **Developer Friendly** - Simple API, rich options, comprehensive error handling
- 📊 **Observable** - Real-time metrics, structured logging, health checks
- 🔧 **Zero Config** - Secure defaults, works out of the box

## 📋 Quick Reference

- **[Quick Reference Guide](QUICK_REFERENCE.md)** - Cheat sheet for common tasks

---

## 📑 Table of Contents

- [Quick Start](#-quick-start)
- [HTTP Request Methods](#-http-request-methods)
- [Request Options](#-request-options-explained)
- [Response Handling](#-response-handling)
- [File Download](#-file-download)
- [Configuration](#-configuration)
- [Error Handling](#-error-handling)
- [Advanced Features](#-advanced-features)
- [Performance](#-performance)


## 🚀 Quick Start

### Installation

```bash
go get -u github.com/cybergodev/httpc
```

### 5-Minute Tutorial

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // Make a simple GET request
    resp, err := httpc.Get("https://api.example.com/users")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)

    // POST JSON data
    user := map[string]string{
        "name":  "John Doe",
        "email": "john@example.com",
    }

    // Make a simple POST request
    resp, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)
}
```

## 🌐 HTTP Request Methods

### 💡 Core Concepts: Request Methods vs Option Methods

Before using HTTPC, it's important to understand these two concepts:

<table>
<tr>
<th width="50%">🎯 Request Methods</th>
<th width="50%">⚙️ Option Methods</th>
</tr>
<tr>
<td>

**Purpose**: Specify the HTTP request type ("what to do")

**Characteristics**:
- Methods of the Client object
- Determine the HTTP verb
- First parameter is the URL

**Examples**:
```go
client.Get(url, ...)
client.Post(url, ...)
client.Put(url, ...)
client.Delete(url, ...)
```

</td>
<td>

**Purpose**: Customize request parameters ("how to do it")

**Characteristics**:
- Functions starting with `With`
- Used to configure request details
- Passed as variadic parameters

**Examples**:
```go
httpc.WithJSON(data)
httpc.WithQuery("key", "val")
httpc.WithBearerToken("token")
httpc.WithTimeout(30*time.Second)
```

</td>
</tr>
</table>

#### 📝 Usage Pattern

```go
// Basic syntax
resp, err := httpc.RequestMethod(url, option1, option2, ...)

// Real example: POST request + JSON data + auth + timeout
resp, err := httpc.Post("https://api.example.com/users",   // ← Request method
    httpc.WithJSON(userData),                              // ← Option method
    httpc.WithBearerToken("token"),                        // ← Option method
    httpc.WithTimeout(30*time.Second),                     // ← Option method
)
```

---

### 📋 Request Methods Quick Reference

| Request Method                       | HTTP Verb | Purpose               | Common Use Cases             |
|--------------------------------------|-----------|-----------------------|------------------------------|
| `Get(url, opts...)`                  | GET       | Retrieve resource     | Query lists, get details     |
| `Post(url, opts...)`                 | POST      | Create resource       | Submit forms, create records |
| `Put(url, opts...)`                  | PUT       | Full update           | Replace entire resource      |
| `Patch(url, opts...)`                | PATCH     | Partial update        | Update specific fields       |
| `Delete(url, opts...)`               | DELETE    | Delete resource       | Remove records               |
| `Head(url, opts...)`                 | HEAD      | Get headers only      | Check resource existence     |
| `Options(url, opts...)`              | OPTIONS   | Get supported methods | CORS preflight               |
| `Request(ctx, method, url, opts...)` | Custom    | Custom method         | Special requirements         |

---

### GET - Retrieve Resource

**Purpose**: Fetch data from the server without modifying server state.

```go
// 1. Simplest GET request (no options)
resp, err := httpc.Get("https://api.example.com/users")

// 2. With query parameters (using WithQuery option)
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),        // ← Option: add ?page=1
    httpc.WithQuery("limit", 10),      // ← Option: add &limit=10
)
// Actual request: GET /users?page=1&limit=10

// 3. With authentication and headers (using WithBearerToken and WithHeader options)
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithBearerToken("your-token"),            // ← Option: add authentication
    httpc.WithHeader("Accept", "application/json"), // ← Option: set header
)
```

### POST - Create Resource

**Purpose**: Submit data to the server, typically to create a new resource.

```go
// 1. POST JSON data (using WithJSON option)
user := map[string]interface{}{
    "name":  "John Doe",
    "email": "john@example.com",
}
resp, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),  // ← Option: set JSON request body
)

// 2. POST form data (using WithForm option)
resp, err := httpc.Post("https://api.example.com/login",
    httpc.WithForm(map[string]string{  // ← Option: set form data
        "username": "john",
        "password": "secret",
    }),
)

// 3. POST file upload (using WithFile option)
resp, err := httpc.Post("https://api.example.com/upload",
    httpc.WithFile("file", "document.pdf", fileContent),  // ← Option: upload file
    httpc.WithBearerToken("your-token"),                  // ← Option: add authentication
)
```

### PUT - Full Resource Update

**Purpose**: Completely replace a resource on the server.

```go
// PUT update entire user object (using WithJSON and WithBearerToken options)
updatedUser := map[string]interface{}{
    "name":  "John Smith",
    "email": "john.smith@example.com",
    "age":   30,
}
resp, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),         // ← Option: set JSON data
    httpc.WithBearerToken("your-token"), // ← Option: add authentication
)
```

### PATCH - Partial Resource Update

**Purpose**: Update only specific fields of a resource.

```go
// PATCH update only email field (using WithJSON option)
updates := map[string]interface{}{
    "email": "newemail@example.com",
}
resp, err := httpc.Patch("https://api.example.com/users/123",
    httpc.WithJSON(updates),             // ← Option: set fields to update
    httpc.WithBearerToken("your-token"), // ← Option: add authentication
)
```

### DELETE - Delete Resource

**Purpose**: Remove a resource from the server.

```go
// 1. Delete specific resource (using WithBearerToken option)
resp, err := httpc.Delete("https://api.example.com/users/123",
    httpc.WithBearerToken("your-token"),  // ← Option: add authentication
)

// 2. Delete with query parameters (using WithQuery option)
resp, err := httpc.Delete("https://api.example.com/cache",
    httpc.WithQuery("key", "session-123"),  // ← Option: specify key to delete
    httpc.WithBearerToken("your-token"),    // ← Option: add authentication
)
```

### HEAD - Get Headers Only

**Purpose**: Check if a resource exists without fetching the response body.

```go
// Check if resource exists (usually no options needed)
resp, err := httpc.Head("https://api.example.com/users/123")
if err == nil && resp.StatusCode == 200 {
    fmt.Println("Resource exists")
    fmt.Printf("Content length: %d\n", resp.ContentLength)
}
```

### OPTIONS - Get Supported Methods

**Purpose**: Query the HTTP methods supported by the server.

```go
// Query supported methods for API endpoint (usually no options needed)
resp, err := httpc.Options("https://api.example.com/users")
allowedMethods := resp.Headers.Get("Allow")
fmt.Println("Supported methods:", allowedMethods)  // e.g.: GET, POST, PUT, DELETE
```

### Request - Generic Request Method

**Purpose**: Send requests with custom HTTP methods.

```go
// Use custom HTTP method (with option methods)
ctx := context.Background()
resp, err := httpc.Request(ctx, "CUSTOM", "https://api.example.com/resource",
    httpc.WithJSON(data),                // ← Option: set data
    httpc.WithHeader("X-Custom", "val"), // ← Option: custom header
)
```

## ⚙️ Request Options Explained

Option methods are used to customize various aspects of requests. All option methods start with `With` and can be combined.

### 📋 Option Methods Category Quick Reference

| Category                                        | Purpose                   | Number of Options |
|-------------------------------------------------|---------------------------|-------------------|
| [Headers](#1-header-options)                    | Set HTTP request headers  | 7                 |
| [Authentication](#2-authentication-options)     | Add authentication info   | 2                 |
| [Query Parameters](#3-query-parameter-options)  | Add URL query parameters  | 2                 |
| [Request Body](#4-request-body-options)         | Set request body content  | 7                 |
| [File Upload](#5-file-upload-options)           | Upload files              | 1                 |
| [Timeout & Retry](#6-timeout-and-retry-options) | Control timeout and retry | 3                 |
| [Cookies](#7-cookie-options)                    | Manage cookies            | 3                 |

---

### 1️⃣ Header Options

Used to set HTTP request headers.

**Complete Option List**:
- `WithHeader(key, value)` - Set single header
- `WithHeaderMap(headers)` - Set multiple headers
- `WithUserAgent(ua)` - Set User-Agent
- `WithContentType(ct)` - Set Content-Type
- `WithAccept(accept)` - Set Accept
- `WithJSONAccept()` - Set Accept to application/json
- `WithXMLAccept()` - Set Accept to application/xml

```go
// Set single header
httpc.Get(url,
    httpc.WithHeader("X-Custom-Header", "value"),
)

// Set multiple headers
httpc.Get(url,
    httpc.WithHeaderMap(map[string]string{
        "X-API-Version": "v1",
        "X-Client-ID":   "client-123",
    }),
)

// Convenience methods for common headers
httpc.Get(url,
    httpc.WithUserAgent("MyApp/1.0"),              // User-Agent
    httpc.WithContentType("application/json"),     // Content-Type
    httpc.WithAccept("application/json"),          // Accept
    httpc.WithJSONAccept(),                        // Accept: application/json
    httpc.WithXMLAccept(),                         // Accept: application/xml
)
```

---

### 2️⃣ Authentication Options

Used to add authentication information.

**Complete Option List**:
- `WithBearerToken(token)` - Bearer Token authentication
- `WithBasicAuth(username, password)` - Basic authentication

```go
// Bearer Token authentication (JWT)
httpc.Get(url,
    httpc.WithBearerToken("your-jwt-token"),
)

// Basic authentication
httpc.Get(url,
    httpc.WithBasicAuth("username", "password"),
)

// API Key authentication (using WithHeader)
httpc.Get(url,
    httpc.WithHeader("X-API-Key", "your-api-key"),
)
```

---

### 3️⃣ Query Parameter Options

Used to add URL query parameters (`?key=value&...`).

**Complete Option List**:
- `WithQuery(key, value)` - Add single query parameter
- `WithQueryMap(params)` - Add multiple query parameters

```go
// Add single query parameter
httpc.Get(url,
    httpc.WithQuery("page", 1),
    httpc.WithQuery("filter", "active"),
)
// Result: GET /api?page=1&filter=active

// Add multiple query parameters
httpc.Get(url,
    httpc.WithQueryMap(map[string]interface{}{
        "page":   1,
        "limit":  20,
        "sort":   "created_at",
        "order":  "desc",
    }),
)
// Result: GET /api?page=1&limit=20&sort=created_at&order=desc
```

---

### 4️⃣ Request Body Options

Used to set request body content, supporting multiple formats.

**Complete Option List**:
- `WithJSON(data)` - JSON format request body
- `WithXML(data)` - XML format request body
- `WithForm(data)` - Form format request body
- `WithText(text)` - Plain text request body
- `WithBody(data)` - Raw request body
- `WithFormData(formData)` - Multipart form data
- `WithBinary(data, contentType)` - Binary data

```go
// JSON format (most common)
httpc.Post(url,
    httpc.WithJSON(map[string]interface{}{
        "name": "John",
        "age":  30,
    }),
)
// Content-Type: application/json

// XML format
httpc.Post(url,
    httpc.WithXML(struct {
        Name string `xml:"name"`
        Age  int    `xml:"age"`
    }{Name: "John", Age: 30}),
)
// Content-Type: application/xml

// Form format (application/x-www-form-urlencoded)
httpc.Post(url,
    httpc.WithForm(map[string]string{
        "username": "john",
        "password": "secret",
    }),
)
// Content-Type: application/x-www-form-urlencoded

// Plain text
httpc.Post(url,
    httpc.WithText("Hello, World!"),
)
// Content-Type: text/plain

// Binary data
httpc.Post(url,
    httpc.WithBinary([]byte{0x89, 0x50, 0x4E, 0x47}, "image/png"),
)
// Content-Type: image/png

// Raw data + custom Content-Type
httpc.Post(url,
    httpc.WithBody(customData),
    httpc.WithContentType("application/vnd.api+json"),
)

// Multipart form data (for file uploads)
httpc.Post(url,
    httpc.WithFormData(formData),
)
// Content-Type: multipart/form-data
```

---

### 5️⃣ File Upload Options

Used to upload files to the server.

**Complete Option List**:
- `WithFile(fieldName, filename, content)` - Upload single file (convenience method)

```go
// Simple single file upload
httpc.Post(url,
    httpc.WithFile("file", "document.pdf", fileContent),
)

// Multiple files + form fields (using WithFormData from Request Body Options)
formData := &httpc.FormData{
    Fields: map[string]string{
        "title":       "My Document",
        "description": "Important file",
        "category":    "reports",
    },
    Files: map[string]*httpc.FileData{
        "document": {
            Filename:    "report.pdf",
            Content:     pdfContent,
            ContentType: "application/pdf",
        },
        "thumbnail": {
            Filename:    "preview.jpg",
            Content:     jpgContent,
            ContentType: "image/jpeg",
        },
    },
}
httpc.Post(url,
    httpc.WithFormData(formData),
    httpc.WithBearerToken("token"),  // Can combine with other options
)
```

---

### 6️⃣ Timeout and Retry Options

Used to control request timeout and retry behavior.

**Complete Option List**:
- `WithTimeout(duration)` - Set request timeout
- `WithMaxRetries(n)` - Set maximum retries
- `WithContext(ctx)` - Use Context for control

```go
// Set request timeout
httpc.Get(url,
    httpc.WithTimeout(30 * time.Second),
)

// Set maximum retries
httpc.Get(url,
    httpc.WithMaxRetries(3),
)

// Use Context to control timeout and cancellation
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()
httpc.Get(url,
    httpc.WithContext(ctx),
)

// Combined usage
httpc.Post(url,
    httpc.WithJSON(data),
    httpc.WithTimeout(30 * time.Second),
    httpc.WithMaxRetries(2),
)
```

---

### 7️⃣ Cookie Options

Used to add cookies to requests.

**Complete Option List**:
- `WithCookie(cookie)` - Add full cookie
- `WithCookies(cookies)` - Add multiple cookies
- `WithCookieValue(name, value)` - Add simple cookie

```go
// Simple cookie (name and value only)
httpc.Get(url,
    httpc.WithCookieValue("session_id", "abc123"),
)

// Full cookie (with attributes)
httpc.Get(url,
    httpc.WithCookie(&http.Cookie{
        Name:     "session",
        Value:    "xyz789",
        Path:     "/",
        Domain:   "example.com",
        Secure:   true,
        HttpOnly: true,
    }),
)

// Multiple cookies
httpc.Get(url,
    httpc.WithCookies([]*http.Cookie{
        {Name: "cookie1", Value: "value1"},
        {Name: "cookie2", Value: "value2"},
    }),
)
```

---

### 💡 Option Methods Combination Examples

Option methods can be freely combined to meet various complex requirements:

```go
// Example 1: Complete API request
resp, err := httpc.Post("https://api.example.com/users",
    // Request body
    httpc.WithJSON(userData),
    // Authentication
    httpc.WithBearerToken("your-token"),
    // Headers
    httpc.WithHeader("X-Request-ID", "req-123"),
    httpc.WithUserAgent("MyApp/1.0"),
    // Timeout and retry
    httpc.WithTimeout(30*time.Second),
    httpc.WithMaxRetries(2),
)

// Example 2: File upload + authentication + timeout
resp, err := httpc.Post("https://api.example.com/upload",
    httpc.WithFile("file", "report.pdf", fileContent),
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(60*time.Second),
)

// Example 3: Query + authentication + custom headers
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQuery("limit", 20),
    httpc.WithBearerToken("token"),
    httpc.WithHeader("X-API-Version", "v2"),
)
```

## 📦 Response Handling

The Response object provides convenient methods for handling HTTP responses.

### Response Structure

```go
type Response struct {
    StatusCode    int            // HTTP status code
    Status        string         // HTTP status text
    Headers       http.Header    // Response headers
    Body          string         // Response body as string
    RawBody       []byte         // Response body as bytes
    ContentLength int64          // Content length
    Proto         string         // HTTP protocol version
    Duration      time.Duration  // Request duration
    Attempts      int            // Number of retry attempts
    Cookies       []*http.Cookie // Response cookies
}
```

### Status Checking

```go
resp, err := client.Get(url)

// Check success (2xx)
if resp.IsSuccess() {
    fmt.Println("Request successful")
}

// Check redirect (3xx)
if resp.IsRedirect() {
    fmt.Println("Redirected")
}

// Check client error (4xx)
if resp.IsClientError() {
    fmt.Println("Client error")
}

// Check server error (5xx)
if resp.IsServerError() {
    fmt.Println("Server error")
}
```

### Parsing Response Body

```go
// Parse JSON
var result map[string]interface{}
err := resp.JSON(&result)

// Parse XML
var data XMLStruct
err := resp.XML(&data)

// Access raw body
bodyString := resp.Body
bodyBytes := resp.RawBody
```

### Working with Cookies

```go
// Get specific cookie
cookie := resp.GetCookie("session_id")
if cookie != nil {
    fmt.Println("Session:", cookie.Value)
}

// Check if cookie exists
if resp.HasCookie("auth_token") {
    fmt.Println("Authenticated")
}

// Get all cookies
for _, cookie := range resp.Cookies {
    fmt.Printf("%s: %s\n", cookie.Name, cookie.Value)
}
```

### Response Metadata

```go
// Request duration
fmt.Printf("Request took: %v\n", resp.Duration)

// Number of retry attempts
fmt.Printf("Attempts: %d\n", resp.Attempts)

// Content length
fmt.Printf("Size: %d bytes\n", resp.ContentLength)

// Protocol version
fmt.Printf("Protocol: %s\n", resp.Proto)
```

## 📥 File Download

HTTPC provides powerful file download capabilities with progress tracking, resume support, and streaming for large files.

### Simple File Download

```go
// Download a file to disk using package-level function
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Downloaded: %s\n", httpc.FormatBytes(result.BytesWritten))
fmt.Printf("Speed: %s\n", httpc.FormatSpeed(result.AverageSpeed))
```

### Download with Progress Tracking (Package-Level)

```go
// Configure download options
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.Overwrite = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s",
        percentage,
        httpc.FormatSpeed(speed),
    )
}

// Download with progress using package-level function
result, err := httpc.DownloadWithOptions(
    "https://example.com/large-file.zip",
    opts,
    httpc.WithTimeout(10*time.Minute),
)
```

### Download with Progress Tracking (Client Instance)

```go
client, _ := httpc.New()
defer client.Close()

// Configure download options
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.Overwrite = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s",
        percentage,
        httpc.FormatSpeed(speed),
    )
}

// Download with progress using client instance
result, err := client.DownloadWithOptions(
    "https://example.com/large-file.zip",
    opts,
    httpc.WithTimeout(10*time.Minute),
)
```

### Resume Interrupted Downloads

```go
// Enable resume for interrupted downloads
opts := httpc.DefaultDownloadOptions("downloads/file.zip")
opts.ResumeDownload = true  // Resume from where it left off
opts.Overwrite = false      // Don't overwrite, append instead

// Works with both package-level function and client instance
result, err := httpc.DownloadWithOptions(url, opts)
if result.Resumed {
    fmt.Println("Download resumed successfully")
}
```

### Save Response to File

```go
// Alternative: Save any response to file
resp, err := client.Get("https://example.com/data.json")
if err != nil {
    log.Fatal(err)
}

// Save response body to file
err = resp.SaveToFile("data.json")
```

### Download Options

```go
opts := &httpc.DownloadOptions{
    FilePath:         "downloads/file.zip",  // Required: destination path
    Overwrite:        true,                  // Overwrite existing files
    ResumeDownload:   false,                 // Resume partial downloads
    CreateDirs:       true,                  // Create parent directories
    BufferSize:       32 * 1024,             // Buffer size (32KB default)
    ProgressInterval: 500 * time.Millisecond, // Progress update frequency
    ProgressCallback: progressFunc,          // Progress callback function
    FileMode:         0644,                  // File permissions
}

result, err := client.DownloadWithOptions(url, opts)
```

### Download with Authentication

```go
// Download protected files
result, err := client.DownloadFile(
    "https://api.example.com/files/protected.zip",
    "downloads/protected.zip",
    httpc.WithBearerToken("your-token"),
    httpc.WithTimeout(5*time.Minute),
)
```

## 🔧 Configuration

### Default Configuration

```go
// Uses secure defaults
client, err := httpc.New()
```

**Default Settings:**
- Timeout: 60 seconds
- MaxRetries: 2
- TLS: 1.2-1.3
- HTTP/2: Enabled
- Connection pooling: Enabled
- Max concurrent requests: 500
- Max response body size: 50 MB

### Security Presets

```go
// Permissive (Development/Testing)
client, err := httpc.New(httpc.TestingConfig())

// Balanced (Production - Default)
client, err := httpc.New(httpc.DefaultConfig())

// Strict (High Security)
client, err := httpc.New(httpc.SecureConfig())
```

### Custom Configuration

```go
config := &httpc.Config{
    // Network settings
    Timeout:               30 * time.Second,
    DialTimeout:           10 * time.Second,
    KeepAlive:             30 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 20 * time.Second,
    IdleConnTimeout:       60 * time.Second,

    // Connection pooling
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
    MaxConnsPerHost:     20,

    // Security settings
    MinTLSVersion:         tls.VersionTLS12,
    MaxTLSVersion:         tls.VersionTLS13,
    InsecureSkipVerify:    false,
    MaxResponseBodySize:   50 * 1024 * 1024, // 50 MB
    MaxConcurrentRequests: 500,
    ValidateURL:           true,
    ValidateHeaders:       true,
    AllowPrivateIPs:       false,

    // Retry settings
    MaxRetries:    2,
    RetryDelay:    2 * time.Second,
    MaxRetryDelay: 60 * time.Second,
    BackoffFactor: 2.0,
    Jitter:        true,

    // Headers and features
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    EnableHTTP2:     true,
    EnableCookies:   true,
    Headers: map[string]string{
        "Accept": "application/json",
    },
}

client, err := httpc.New(config)
```

## 🚨 Error Handling

### Intelligent Error Handling

```go
resp, err := httpc.Get(url)
if err != nil {
    // Check for HTTP errors
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Status)
        fmt.Printf("URL: %s\n", httpErr.URL)
        fmt.Printf("Method: %s\n", httpErr.Method)
    }

    // Check for circuit breaker
    if strings.Contains(err.Error(), "circuit breaker is open") {
        // Service is down, use fallback
        return fallbackData, nil
    }

    // Check for timeout
    if strings.Contains(err.Error(), "timeout") {
        // Handle timeout
        return nil, fmt.Errorf("request timed out")
    }

    return err
}

// Check response status
if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

### Error Types

- **HTTPError**: HTTP error responses (4xx, 5xx)
- **Timeout errors**: Request timeout exceeded
- **Circuit breaker errors**: Service temporarily unavailable
- **Validation errors**: Invalid URL or headers
- **Network errors**: Connection failures

## 🎯 Advanced Features

### Circuit Breaker

Automatically prevents cascading failures by temporarily blocking requests to failing services.

```go
// Circuit breaker is enabled by default
// It opens after consecutive failures and closes after recovery
client, err := httpc.New()

resp, err := client.Get(url)
if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
    // Use fallback or cached data
    return getCachedData()
}
```

### Automatic Retries

Intelligent retry mechanism with exponential backoff and jitter.

```go
// Configure retry behavior
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.RetryDelay = 1 * time.Second
config.MaxRetryDelay = 30 * time.Second
config.BackoffFactor = 2.0
config.Jitter = true

client, err := httpc.New(config)

// Per-request retry override
resp, err := client.Get(url,
    httpc.WithMaxRetries(5),
)
```

### Connection Pooling

Efficient connection reuse for better performance.

```go
config := httpc.DefaultConfig()
config.MaxIdleConns = 100        // Total idle connections
config.MaxIdleConnsPerHost = 10  // Idle connections per host
config.MaxConnsPerHost = 20      // Max connections per host
config.IdleConnTimeout = 90 * time.Second

client, err := httpc.New(config)
```

### Cookie Management

Automatic cookie handling with cookie jar support.

```go
// Automatic cookie management (enabled by default)
client, err := httpc.New()

// First request sets cookies
resp1, _ := client.Get("https://example.com/login",
    httpc.WithForm(map[string]string{
        "username": "john",
        "password": "secret",
    }),
)

// Subsequent requests automatically include cookies
resp2, _ := client.Get("https://example.com/profile")

// Custom cookie jar
jar, _ := httpc.NewCookieJar()
config := httpc.DefaultConfig()
config.CookieJar = jar
client, err := httpc.New(config)
```

### Resource Management

**New in v1.0.0**: Proper resource cleanup for long-running applications.

```go
package main

import (
    "github.com/cybergodev/httpc"
)

func main() {
    // Ensure default client is cleaned up on application shutdown
    defer httpc.CloseDefaultClient()

    // Use package-level functions
    resp, err := httpc.Get("https://api.example.com/data")
    // ...
}
```

**Setting Custom Default Client**:

```go
// Create a custom client
config := httpc.DefaultConfig()
config.Timeout = 60 * time.Second
client, err := httpc.New(config)
if err != nil {
    log.Fatal(err)
}

// Set as default client (returns error if previous client fails to close)
if err := httpc.SetDefaultClient(client); err != nil {
    log.Printf("Warning: failed to close previous client: %v", err)
}

// Clean up on shutdown
defer httpc.CloseDefaultClient()
```

**Important Notes**:
- `CloseDefaultClient()` releases all resources (connections, goroutines, etc.)
- After closing, the default client will be re-initialized on next use
- `SetDefaultClient()` now returns an error (breaking change from previous versions)

### Context Support

Full context support for cancellation and deadlines.

```go
// Context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Request(ctx, "GET", url)

// Context with cancellation
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel() // Cancel after 5 seconds
}()

resp, err := client.Get(url, httpc.WithContext(ctx))
```

## 📊 Performance

### Concurrency & Thread Safety
- **Massive Concurrency**: Handle concurrent requests with adaptive semaphore-based throttling
- **Goroutine-Safe**: All operations use atomic counters and synchronized state management
- **Lock-Free Metrics**: Real-time performance tracking without contention
- **Per-Host Limits**: Intelligent connection distribution prevents host overload

### Memory Optimization
- **Zero-Allocation Pooling**: Reusable buffer pools reduce GC pressure by 90%
- **Smart Buffer Sizing**: Adaptive buffer allocation based on response patterns
- **Memory Bounds**: Configurable limits prevent memory exhaustion
- **Efficient Cleanup**: Automatic resource reclamation with sync.Pool

### Network Performance
- **Connection Pooling**: Intelligent connection reuse with per-host tracking
- **HTTP/2 Multiplexing**: Multiple concurrent streams over single connection
- **Keep-Alive Optimization**: Persistent connections with configurable timeouts
- **Low Latency**: Optimized request/response processing pipeline

### Reliability
- **Panic Recovery**: Comprehensive error handling prevents crashes
- **Circuit Breaker**: Automatic failure detection and recovery
- **Graceful Degradation**: Continues operation under partial failures
- **Resource Limits**: Prevents resource exhaustion with configurable bounds

## 📖 Documentation

### 📚 Complete Documentation

- **[📖 Documentation](docs)** - Complete documentation hub
- **[🚀 Getting Started](docs/getting-started.md)** - Installation and first steps
- **[⚙️ Configuration](docs/configuration.md)** - Client configuration and presets
- **[🔧 Request Options](docs/request-options.md)** - Customizing HTTP requests
- **[❗ Error Handling](docs/error-handling.md)** - Comprehensive error handling
- **[📥 File Download](docs/file-download.md)** - File downloads with progress
- **[🔄 Circuit Breaker](docs/circuit-breaker.md)** - Automatic fault protection
- **[✅ Best Practices](docs/best-practices.md)** - Recommended usage patterns
- **[🔒 Security](docs/security.md)** - Security features and compliance
- **[💡 Examples](examples)** - Code examples and tutorials

### 💻 Code Examples

- **[Quick Start](examples/01_quickstart)** - Basic usage examples
- **[Core Features](examples/02_core_features)** - Headers, auth, body formats, cookies
- **[Advanced](examples/03_advanced)** - File uploads, downloads, timeouts, retries
- **[Real World](examples/04_real_world)** - Complete REST API client implementation

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.


## 🌟 Star History

If you find this project useful, please consider giving it a star! ⭐

---

**Made with ❤️ by the CyberGoDev team**