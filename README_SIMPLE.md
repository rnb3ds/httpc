# HTTPC - Simple HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A simple, secure, and high-performance HTTP client library for Go.

## Features

- 🛡️ **Secure by Default** - TLS 1.2+, input validation
- ⚡ **High Performance** - Connection pooling, concurrent requests
- 🔄 **Built-in Resilience** - Automatic retries with backoff
- 🎯 **Simple API** - Easy to use, minimal configuration
- 📥 **File Downloads** - Progress tracking and resume support

## Quick Start

### Installation

```bash
go get -u github.com/cybergodev/httpc
```

### Basic Usage

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
    fmt.Printf("Status: %d\nBody: %s\n", resp.StatusCode, resp.Body)

    // POST JSON data
    user := map[string]string{
        "name":  "John Doe",
        "email": "john@example.com",
    }
    resp, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d\nBody: %s\n", resp.StatusCode, resp.Body)
}
```

## HTTP Methods

```go
// All HTTP methods are supported
resp, err := httpc.Get(url, options...)
resp, err := httpc.Post(url, options...)
resp, err := httpc.Put(url, options...)
resp, err := httpc.Patch(url, options...)
resp, err := httpc.Delete(url, options...)
resp, err := httpc.Head(url, options...)
resp, err := httpc.Options(url, options...)

// Generic request method
resp, err := httpc.Request(ctx, "CUSTOM", url, options...)
```

## Request Options

### Headers and Authentication

```go
// Set headers
httpc.Get(url,
    httpc.WithHeader("X-Custom", "value"),
    httpc.WithUserAgent("MyApp/1.0"),
    httpc.WithContentType("application/json"),
)

// Authentication
httpc.Get(url,
    httpc.WithBearerToken("your-token"),
    httpc.WithBasicAuth("username", "password"),
)
```

### Query Parameters

```go
// Single parameter
httpc.Get(url, httpc.WithQuery("page", 1))

// Multiple parameters
httpc.Get(url, httpc.WithQueryMap(map[string]interface{}{
    "page":  1,
    "limit": 20,
}))
```

### Request Body

```go
// JSON body
httpc.Post(url, httpc.WithJSON(data))

// Form data
httpc.Post(url, httpc.WithForm(map[string]string{
    "username": "john",
    "password": "secret",
}))

// Text body
httpc.Post(url, httpc.WithText("Hello, World!"))

// Binary data
httpc.Post(url, httpc.WithBinary([]byte{...}, "image/png"))
```

### File Upload

```go
// Simple file upload
httpc.Post(url, httpc.WithFile("file", "document.pdf", fileContent))

// Multiple files with form fields
formData := &httpc.FormData{
    Fields: map[string]string{
        "title": "My Document",
    },
    Files: map[string]*httpc.FileData{
        "document": {
            Filename: "report.pdf",
            Content:  pdfContent,
        },
    },
}
httpc.Post(url, httpc.WithFormData(formData))
```

### Timeout and Context

```go
// Set timeout
httpc.Get(url, httpc.WithTimeout(30*time.Second))

// Use context
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()
httpc.Get(url, httpc.WithContext(ctx))
```

## Response Handling

```go
resp, err := httpc.Get(url)
if err != nil {
    log.Fatal(err)
}

// Check status
if resp.IsSuccess() {
    fmt.Println("Request successful")
}

// Parse JSON
var result map[string]interface{}
err = resp.JSON(&result)

// Access response data
fmt.Printf("Status: %d\n", resp.StatusCode)
fmt.Printf("Body: %s\n", resp.Body)
fmt.Printf("Duration: %v\n", resp.Duration)
```

## File Downloads

```go
// Simple download
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Downloaded: %s\n", httpc.FormatBytes(result.BytesWritten))
```

## Configuration

```go
// Use default configuration (recommended)
client, err := httpc.New()

// Custom configuration
config := httpc.DefaultConfig()
config.Timeout = 30 * time.Second
config.MaxRetries = 3
client, err := httpc.New(config)

// Strict security preset
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelStrict))
```

## Error Handling

```go
resp, err := httpc.Get(url)
if err != nil {
    // Check for HTTP errors
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Status)
    }
    return err
}

// Check response status
if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

## License

MIT License - see [LICENSE](LICENSE) file for details.