# HTTPC Library Complete Usage Guide

## 🚀 Quick Start

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
    // Create client
    client, err := httpc.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Send GET request
    resp, err := client.Get("https://api.github.com/users/octocat")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Status Code: %d\n", resp.StatusCode)
    fmt.Printf("Response Body: %s\n", resp.Body)
}
```

## 📋 Table of Contents

1. [Client Creation and Configuration](#client-creation-and-configuration)
2. [HTTP Request Methods](#http-request-methods)
3. [Request Options Details](#request-options-details)
4. [Response Handling](#response-handling)
5. [File Operations](#file-operations)
6. [Error Handling](#error-handling)
7. [Advanced Features](#advanced-features)
8. [Best Practices](#best-practices)
9. [Common Issues](#common-issues)

## 🔧 Client Creation and Configuration

### Default Configuration
```go
// Use secure default configuration
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Security Preset Configuration
```go
// Balanced mode (default) - suitable for most applications
client, err := httpc.New(httpc.DefaultConfig())

// Strict mode - suitable for high security requirement environments
client, err := httpc.New(httpc.SecureConfig())
```

### Custom Configuration
```go
config := &httpc.Config{
    Timeout:             30 * time.Second,
    MaxIdleConns:        100,
    MaxConnsPerHost:     20,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50MB
    MaxRetries:          3,
    RetryDelay:          2 * time.Second,
    BackoffFactor:       2.0,
    UserAgent:           "MyApp/1.0",
    EnableHTTP2:         true,
    EnableCookies:       true,
}

client, err := httpc.New(config)
```

### Package-level Functions (Quick Usage)
```go
// Use directly without creating client instance
resp, err := httpc.Get("https://api.example.com/data")
resp, err := httpc.Post("https://api.example.com/users", httpc.WithJSON(userData))
```

## 🌐 HTTP Request Methods

### GET Requests
```go
// Basic GET request
resp, err := client.Get("https://api.example.com/users")

// GET request with query parameters
resp, err := client.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQuery("limit", 10),
)

// GET request with authentication
resp, err := client.Get("https://api.example.com/users",
    httpc.WithBearerToken("your-token"),
    httpc.WithHeader("Accept", "application/json"),
)
```

### POST Requests
```go
// POST JSON data
user := map[string]interface{}{
    "name":  "John Doe",
    "email": "john.doe@example.com",
}

resp, err := client.Post("https://api.example.com/users",
    httpc.WithBearerToken("your-token"),
    httpc.WithJSON(user),
	
    // Content-Type is set automatically based on the data type
    // httpc.WithContentType("application/json"),
)

// POST form data
resp, err := client.Post("https://api.example.com/login",
    httpc.WithForm(map[string]string{
        "username": "johndoe",
        "password": "password123",
    }),
	
    // Content-Type is set automatically based on the data type
    // httpc.WithContentType("application/x-www-form-urlencoded")
)

// POST text data
resp, err := client.Post("https://api.example.com/webhook",
    httpc.WithText("Hello, World!"),

    // Content-Type is set automatically based on the data type
    // httpc.WithContentType("text/plain")
)
```

### PUT and PATCH Requests
```go
// PUT - complete update
updatedUser := map[string]interface{}{
    "name":  "Jane Smith",
    "email": "jane.smith@example.com",
    "age":   30,
}

resp, err := client.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),
    httpc.WithBearerToken("your-token"),
)

// PATCH - partial update
updates := map[string]interface{}{
    "email": "newemail@example.com",
}

resp, err := client.Patch("https://api.example.com/users/123",
    httpc.WithJSON(updates),
    httpc.WithBearerToken("your-token"),
)
```

### DELETE Requests
```go
// Delete resource
resp, err := client.Delete("https://api.example.com/users/123",
    httpc.WithBearerToken("your-token"),
)

// Delete with query parameters
resp, err := client.Delete("https://api.example.com/cache",
    httpc.WithQuery("key", "session-123"),
    httpc.WithBearerToken("your-token"),
)
```

### HEAD and OPTIONS Requests
```go
// HEAD - check if resource exists
resp, err := client.Head("https://api.example.com/users/123")
if err == nil && resp.StatusCode == 200 {
    fmt.Println("Resource exists")
}

// OPTIONS - query supported methods
resp, err := client.Options("https://api.example.com/users")
allowedMethods := resp.Headers.Get("Allow")
fmt.Println("Supported methods:", allowedMethods)
```

## ⚙️ Request Options Details

### Header Settings
```go
// Set single header
httpc.WithHeader("X-API-Key", "your-api-key")

// Set multiple headers
httpc.WithHeaderMap(map[string]string{
    "X-API-Version": "v1",
    "X-Client-ID":   "client-123",
})

// Convenience methods
httpc.WithUserAgent("MyApp/1.0")
httpc.WithContentType("application/json")
httpc.WithAccept("application/json")
httpc.WithJSONAccept() // equivalent to WithAccept("application/json")
```

### Authentication
```go
// Bearer Token authentication (JWT)
httpc.WithBearerToken("your-jwt-token")

// Basic authentication
httpc.WithBasicAuth("username", "password")

// API Key authentication (via header)
httpc.WithHeader("X-API-Key", "your-api-key")
```

### Query Parameters
```go
// Single parameter
httpc.WithQuery("page", 1)
httpc.WithQuery("filter", "active")

// Multiple parameters
httpc.WithQueryMap(map[string]interface{}{
    "page":   1,
    "limit":  20,
    "sort":   "created_at",
    "order":  "desc",
})
```

### Request Body
```go
// JSON format
httpc.WithJSON(map[string]interface{}{
    "name": "John Doe",
    "age":  30,
})

// Form format
httpc.WithForm(map[string]string{
    "username": "johndoe",
    "password": "password123",
})

// Plain text
httpc.WithText("Hello, World!")

// Binary data
httpc.WithBinary([]byte{0x89, 0x50, 0x4E, 0x47}, "image/png")

// Raw data
httpc.WithBody(customData)
```

### Timeout and Retry
```go
// Set timeout
httpc.WithTimeout(30 * time.Second)

// Set maximum retry attempts
httpc.WithMaxRetries(3)

// Use context
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()
httpc.WithContext(ctx)
```

### Cookie
```go
// Simple Cookie
httpc.WithCookieValue("session_id", "abc123")

// Complete Cookie
httpc.WithCookie(&http.Cookie{
    Name:     "session",
    Value:    "xyz789",
    Path:     "/",
    Domain:   "example.com",
    Secure:   true,
    HttpOnly: true,
})

// Multiple Cookies
httpc.WithCookies([]*http.Cookie{
    {Name: "cookie1", Value: "value1"},
    {Name: "cookie2", Value: "value2"},
})
```

## 📦 Response Handling

### Basic Response Information
```go
resp, err := client.Get(url)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Status Code: %d\n", resp.StatusCode)
fmt.Printf("Status Text: %s\n", resp.Status)
fmt.Printf("Response Body: %s\n", resp.Body)
fmt.Printf("Raw Bytes: %d bytes\n", len(resp.RawBody))
fmt.Printf("Request Duration: %v\n", resp.Duration)
fmt.Printf("Retry Attempts: %d\n", resp.Attempts)
```

### Status Code Checking
```go
resp, err := client.Get(url)
if err != nil {
    return err
}

// Status code checking methods
if resp.IsSuccess() {        // 2xx
    fmt.Println("Request successful")
}
if resp.IsRedirect() {       // 3xx
    fmt.Println("Redirect")
}
if resp.IsClientError() {    // 4xx
    fmt.Println("Client error")
}
if resp.IsServerError() {    // 5xx
    fmt.Println("Server error")
}

// Direct status code checking
switch resp.StatusCode {
case 200:
    fmt.Println("Success")
case 404:
    fmt.Println("Not found")
case 500:
    fmt.Println("Server error")
}
```

### JSON Response Parsing
```go
type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

resp, err := client.Get("https://api.example.com/users/1")
if err != nil {
    return err
}

if !resp.IsSuccess() {
    return fmt.Errorf("API error: %d", resp.StatusCode)
}

var user User
if err := resp.JSON(&user); err != nil {
    return fmt.Errorf("JSON parsing failed: %w", err)
}

fmt.Printf("User: %+v\n", user)
```

### Header and Cookie Handling
```go
resp, err := client.Get(url)
if err != nil {
    return err
}

// Get response headers
contentType := resp.Headers.Get("Content-Type")
fmt.Printf("Content Type: %s\n", contentType)

// Get specific Cookie
sessionCookie := resp.GetCookie("session_id")
if sessionCookie != nil {
    fmt.Printf("Session ID: %s\n", sessionCookie.Value)
}

// Check if Cookie exists
if resp.HasCookie("auth_token") {
    fmt.Println("Authenticated")
}

// Iterate through all Cookies
for _, cookie := range resp.Cookies {
    fmt.Printf("Cookie: %s = %s\n", cookie.Name, cookie.Value)
}
```

## 📁 File Operations

### File Download
```go
// Simple file download
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Download completed: %s\n", httpc.FormatBytes(result.BytesWritten))
fmt.Printf("Average speed: %s\n", httpc.FormatSpeed(result.AverageSpeed))

// Download with progress tracking
client, _ := httpc.New()
defer client.Close()

opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.Overwrite = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s",
        percentage,
        httpc.FormatSpeed(speed),
    )
}

result, err := client.DownloadWithOptions(
    "https://example.com/large-file.zip",
    opts,
    httpc.WithTimeout(10*time.Minute),
)

// Resume download
opts.ResumeDownload = true
opts.Overwrite = false
result, err = client.DownloadWithOptions(url, opts)
if result.Resumed {
    fmt.Println("Download resumed")
}
```

### File Upload
```go
// Single file upload
fileContent, err := os.ReadFile("document.pdf")
if err != nil {
    log.Fatal(err)
}

resp, err := client.Post("https://api.example.com/upload",
    httpc.WithFile("file", "document.pdf", fileContent),
    httpc.WithBearerToken("your-token"),
)

// Multiple file upload + form fields
formData := &httpc.FormData{
    Fields: map[string]string{
        "title":       "My Document",
        "description": "Important File",
        "category":    "reports",
    },
    Files: map[string]*httpc.FileData{
        "document": {
            Filename: "report.pdf",
            Content:  pdfContent,
        },
        "thumbnail": {
            Filename: "preview.jpg",
            Content:  jpgContent,
        },
    },
}

resp, err = client.Post("https://api.example.com/upload",
    httpc.WithFormData(formData),
    httpc.WithBearerToken("token"),
)
```

### Save Response to File
```go
resp, err := client.Get("https://api.example.com/data.json")
if err != nil {
    log.Fatal(err)
}

// Save response body to file
err = resp.SaveToFile("data.json")
if err != nil {
    log.Fatal(err)
}
```

## 🚨 Error Handling

### Basic Error Handling
```go
resp, err := client.Get(url)
if err != nil {
    // Check HTTP errors
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP error %d: %s\n", httpErr.StatusCode, httpErr.Status)
        fmt.Printf("URL: %s\n", httpErr.URL)
        fmt.Printf("Method: %s\n", httpErr.Method)
        return err
    }

    // Check timeout errors
    if strings.Contains(err.Error(), "timeout") {
        fmt.Println("Request timeout")
        return err
    }

    // Check network errors
    if strings.Contains(err.Error(), "connection refused") {
        fmt.Println("Connection refused")
        return err
    }

    return err
}

// Check response status
if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}
```

### Retry and Circuit Breaker
```go
// Circuit breaker automatically handles consecutive failures
resp, err := client.Get(url)
if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
    // Service temporarily unavailable, use fallback
    return getFallbackData()
}

// Configure retry behavior
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.RetryDelay = 1 * time.Second
config.BackoffFactor = 2.0

client, err := httpc.New(config)
```

### Context Cancellation
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Cancel request from another goroutine
go func() {
    time.Sleep(5 * time.Second)
    cancel() // Cancel after 5 seconds
}()

resp, err := client.Get(url, httpc.WithContext(ctx))
if err != nil {
    if errors.Is(err, context.Canceled) {
        fmt.Println("Request was canceled")
    } else if errors.Is(err, context.DeadlineExceeded) {
        fmt.Println("Request timed out")
    }
    return err
}
```

## 🎯 Advanced Features

### Concurrent Requests
```go
// Send multiple requests concurrently
urls := []string{
    "https://api.example.com/users/1",
    "https://api.example.com/users/2",
    "https://api.example.com/users/3",
}

var wg sync.WaitGroup
results := make(chan *httpc.Response, len(urls))

client, _ := httpc.New()
defer client.Close()

for _, url := range urls {
    wg.Add(1)
    go func(u string) {
        defer wg.Done()
        resp, err := client.Get(u,
            httpc.WithBearerToken("token"),
            httpc.WithTimeout(10*time.Second),
        )
        if err != nil {
            log.Printf("Request failed %s: %v", u, err)
            return
        }
        results <- resp
    }(url)
}

wg.Wait()
close(results)

// Process results
for resp := range results {
    fmt.Printf("Status: %d, Duration: %v\n", resp.StatusCode, resp.Duration)
}
```

### Custom Transport Layer
```go
config := httpc.DefaultConfig()

// Custom TLS configuration
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    MaxVersion: tls.VersionTLS13,
}

// Proxy configuration
config.ProxyURL = "http://proxy.example.com:8080"

client, err := httpc.New(config)
```

### Cookie Management
```go
// Automatic Cookie management (enabled by default)
client, err := httpc.New()

// First request sets cookies
resp1, _ := client.Post("https://example.com/login",
    httpc.WithForm(map[string]string{
        "username": "john",
        "password": "password123",
    }),
)

// Subsequent requests automatically include cookies
resp2, _ := client.Get("https://example.com/profile")
```

## 💡 Best Practices

### 1. Client Lifecycle Management
```go
// ✅ Recommended: Create and reuse client
func NewAPIClient() *APIClient {
    client, err := httpc.New()
    if err != nil {
        log.Fatal(err)
    }
    
    return &APIClient{client: client}
}

func (c *APIClient) Close() error {
    return c.client.Close()
}

// ❌ Not recommended: Create new client for each request
func badExample() {
    client, _ := httpc.New()
    resp, _ := client.Get(url)
    client.Close()
}
```

### 2. Error Handling Pattern
```go
// ✅ Recommended: Complete error handling
func fetchUser(id int) (*User, error) {
    resp, err := client.Get(fmt.Sprintf("/users/%d", id),
        httpc.WithBearerToken(token),
        httpc.WithTimeout(10*time.Second),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to fetch user: %w", err)
    }

    if !resp.IsSuccess() {
        return nil, fmt.Errorf("API error: %d %s", resp.StatusCode, resp.Status)
    }

    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    return &user, nil
}
```

### 3. Configuration Selection
```go
// Development environment
client, _ := httpc.New() // Use default configuration

// Production environment
client, _ := httpc.New(httpc.DefaultConfig())

// High security environment
client, _ := httpc.New(httpc.SecureConfig())
```

### 4. Timeout Settings
```go
// Different timeout settings for different scenarios
healthCheck := 2 * time.Second    // Health checks
userRequest := 5 * time.Second    // User requests
criticalOp := 30 * time.Second    // Critical operations
backgroundJob := 2 * time.Minute  // Background tasks

resp, err := client.Get(url, httpc.WithTimeout(userRequest))
```

### 5. API Client Wrapper
```go
type APIClient struct {
    client  httpc.Client
    baseURL string
    token   string
}

func NewAPIClient(baseURL, token string) (*APIClient, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    
    return &APIClient{
        client:  client,
        baseURL: baseURL,
        token:   token,
    }, nil
}

func (c *APIClient) GetUser(ctx context.Context, id int) (*User, error) {
    url := fmt.Sprintf("%s/users/%d", c.baseURL, id)
    
    resp, err := c.client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithBearerToken(c.token),
        httpc.WithTimeout(10*time.Second),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("API returned error: %d", resp.StatusCode)
    }
    
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    return &user, nil
}

func (c *APIClient) Close() error {
    return c.client.Close()
}
```

## ❓ Common Issues

### Q: How to handle large file downloads?
```go
// Use streaming download to avoid excessive memory usage
opts := httpc.DefaultDownloadOptions("large-file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    fmt.Printf("\rProgress: %.1f%%", float64(downloaded)/float64(total)*100)
}

result, err := client.DownloadWithOptions(url, opts,
    httpc.WithTimeout(30*time.Minute),
)
```

### Q: How to set up a proxy?
```go
config := httpc.DefaultConfig()
config.ProxyURL = "http://proxy.example.com:8080"
client, err := httpc.New(config)
```

### Q: How to skip TLS verification (testing only)?
```go
config := httpc.DefaultConfig()
config.InsecureSkipVerify = true // ⚠️ For testing only!
client, err := httpc.New(config)
```

### Q: How to handle redirects?
```go
// Automatically follows redirects by default
// To disable:
config := httpc.DefaultConfig()
config.FollowRedirects = false
client, err := httpc.New(config)
```

### Q: How to limit concurrent requests?
```go
config := httpc.DefaultConfig()
config.MaxConcurrentRequests = 100 // Limit maximum concurrency
client, err := httpc.New(config)
```

### Q: How to handle API rate limiting?
```go
// Use retry mechanism to handle 429 status codes
resp, err := client.Get(url,
    httpc.WithMaxRetries(3),
    httpc.WithTimeout(30*time.Second),
)

if err != nil {
    if strings.Contains(err.Error(), "429") {
        // Handle rate limiting
        time.Sleep(time.Minute)
        // Retry request
    }
}
```

## 🔗 Related Resources

- [Complete API Documentation](README.md)
- [Example Code](examples/)
- [Security Guide](docs/security.md)
- [Best Practices](docs/best-practices.md)
- [Configuration Guide](docs/configuration.md)

---

This usage guide covers all major features and best practices of the httpc library. If you have any questions or need more examples, please refer to the example code or submit an issue.