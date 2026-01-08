# Advanced Examples

**Time to complete**: 30 minutes

Master advanced patterns and production-ready techniques.

## What You'll Learn

- ✅ Client configuration and presets
- ✅ All HTTP methods and their use cases
- ✅ Timeout and retry strategies
- ✅ Redirect handling
- ✅ Concurrent request patterns
- ✅ File upload and download
- ✅ Advanced cookie management
- ✅ Domain-specific clients
- ✅ Complete REST API client pattern

## Examples

### 1. Client Configuration
**[client_configuration.go](client_configuration.go)** - Configure clients for different scenarios

**Covers:**
- Default, Secure, Performance presets
- Custom configuration
- Configuration comparison

### 2. HTTP Methods
**[http_methods.go](http_methods.go)** - All HTTP methods

**Covers:**
- GET, POST, PUT, PATCH, DELETE
- HEAD, OPTIONS
- Use cases for each method

### 3. Timeout & Retry
**[timeout_retry.go](timeout_retry.go)** - Resilient requests

**Covers:**
- Basic timeout
- Context with timeout
- Retry configuration
- Combined timeout and retry

### 4. Redirects
**[redirects.go](redirects.go)** - Redirect handling

**Covers:**
- Automatic redirect following
- Disable redirects
- Limit maximum redirects
- Track redirect chain

### 5. Concurrent Requests
**[concurrent_requests.go](concurrent_requests.go)** - Parallel processing

**Covers:**
- Parallel requests
- Worker pool pattern
- Error handling in concurrent requests
- Rate-limited requests

### 6. File Operations
**[file_operations.go](file_operations.go)** - File handling

**Covers:**
- File upload (single, multiple, with metadata)
- File download (simple, with progress, resume)
- Large file handling

### 7. Advanced Cookies
**[cookies_advanced.go](cookies_advanced.go)** - Cookie management

**Covers:**
- Request cookies (simple, with attributes, multiple)
- Response cookies (read, iterate, check)
- Cookie Jar (automatic management)
- Cookie string parsing
- Real-world scenarios

### 8. Domain Client
**[domain_client.go](domain_client.go)** - State management

**Covers:**
- Automatic state management
- Persistent headers and cookies
- URL matching
- State clearing

### 9. REST API Client
**[rest_api_client.go](rest_api_client.go)** - Production pattern

**Covers:**
- Complete REST API client
- CRUD operations
- Error handling
- Context management

## Running Examples

```bash
# Client configuration
go run -tags examples examples/03_advanced/client_configuration.go

# HTTP methods
go run -tags examples examples/03_advanced/http_methods.go

# Timeout & retry
go run -tags examples examples/03_advanced/timeout_retry.go

# Redirects
go run -tags examples examples/03_advanced/redirects.go

# Concurrent requests
go run -tags examples examples/03_advanced/concurrent_requests.go

# File operations
go run -tags examples examples/03_advanced/file_operations.go

# Advanced cookies
go run -tags examples examples/03_advanced/cookies_advanced.go

# Domain client
go run -tags examples examples/03_advanced/domain_client.go

# REST API client
go run -tags examples examples/03_advanced/rest_api_client.go
```

## Key Takeaways

1. **Configuration**: Choose the right preset or customize
2. **Resilience**: Use timeouts and retries for production
3. **Concurrency**: Handle multiple requests efficiently
4. **State Management**: Use DomainClient for session-based APIs
5. **Production Patterns**: Follow REST API client example

## Common Patterns

### Production Client Setup
```go
config := httpc.SecureConfig()
config.Timeout = 30 * time.Second
config.MaxRetries = 3
config.EnableCookies = true

client, err := httpc.New(config)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Concurrent Requests with Error Handling
```go
type result struct {
    data string
    err  error
}

results := make(chan result, len(urls))

for _, url := range urls {
    go func(u string) {
        resp, err := client.Get(u)
        if err != nil {
            results <- result{err: err}
            return
        }
        results <- result{data: resp.Body()}
    }(url)
}

for range urls {
    r := <-results
    if r.err != nil {
        log.Printf("Error: %v\n", r.err)
    }
}
```

## Next Steps

You've completed all examples! Now you can:
- Build production-ready HTTP clients
- Handle complex scenarios with confidence
- Optimize for performance and reliability

---

**Estimated time**: 30 minutes | **Difficulty**: Advanced

