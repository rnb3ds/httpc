# HTTPC Examples

Comprehensive, production-ready examples for the httpc library. Learn by doing!

## 📁 Structure

```
examples/
├── 01_quickstart/          # Start here! (5 minutes)
├── 02_core_features/       # Essential features (15 minutes)
└── 03_advanced/            # Advanced patterns (30 minutes)
```

## 🚀 Quick Start (5 minutes)

**[01_quickstart/basic_usage.go](01_quickstart/basic_usage.go)**
- Simplest GET/POST/PUT/DELETE requests
- Package-level functions vs client instances
- Basic JSON handling
- When to use what

```go
// Simplest possible request
resp, err := httpc.Get("https://api.example.com/data")

// With JSON
resp, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
)
```

## 🎯 Core Features (15 minutes)

### Request Options
**[02_core_features/request_options.go](02_core_features/request_options.go)**
- All body formats: JSON, Form, XML, Binary, Text
- Headers and authentication (Bearer, Basic, API Key)
- Query parameters (single, map, special characters)

### Response Handling
**[02_core_features/response_handling.go](02_core_features/response_handling.go)**
- Result API structure (Request/Response/Meta)
- JSON/XML parsing
- Status code checking
- Header access
- Response metadata

### Error Handling
**[02_core_features/error_handling.go](02_core_features/error_handling.go)**
- Basic error patterns
- HTTP status errors (4xx, 5xx)
- Timeout and context cancellation
- Parsing errors
- Comprehensive error handling pattern

### Compression
**[02_core_features/compression.go](02_core_features/compression.go)**
- Automatic gzip/deflate decompression
- Accept-Encoding headers

## 🔧 Advanced Patterns (30 minutes)

### Client Configuration
**[03_advanced/client_configuration.go](03_advanced/client_configuration.go)**
- Default, Secure, Performance presets
- Custom configuration
- Configuration comparison for different scenarios

### HTTP Methods
**[03_advanced/http_methods.go](03_advanced/http_methods.go)**
- GET, POST, PUT, PATCH, DELETE
- HEAD, OPTIONS
- Use cases for each method

### Timeout & Retry
**[03_advanced/timeout_retry.go](03_advanced/timeout_retry.go)**
- Basic timeout
- Context with timeout
- Retry configuration
- Combined timeout and retry

### Redirects
**[03_advanced/redirects.go](03_advanced/redirects.go)**
- Automatic redirect following
- Disable redirects
- Limit maximum redirects
- Per-request control
- Track redirect chain

### Concurrent Requests
**[03_advanced/concurrent_requests.go](03_advanced/concurrent_requests.go)**
- Parallel requests
- Worker pool pattern
- Error handling in concurrent requests
- Rate-limited requests

### File Operations
**[03_advanced/file_operations.go](03_advanced/file_operations.go)**
- File upload (single, multiple, with metadata)
- File download (simple, with progress, resume)
- Large file handling

### Advanced Cookies
**[03_advanced/cookies_advanced.go](03_advanced/cookies_advanced.go)**
- Request cookies (simple, with attributes, multiple)
- Response cookies (read, iterate, check)
- Cookie Jar (automatic management)
- Cookie string parsing
- Real-world scenarios

### Domain Client
**[03_advanced/domain_client.go](03_advanced/domain_client.go)**
- Automatic state management
- Persistent headers and cookies
- URL matching
- State clearing

### REST API Client
**[03_advanced/rest_api_client.go](03_advanced/rest_api_client.go)**
- Complete REST API client pattern
- CRUD operations
- Error handling
- Context management

## 🏃 Running Examples

### Run a specific example:
```bash
go run -tags examples examples/01_quickstart/basic_usage.go
go run -tags examples examples/02_core_features/request_options.go
go run -tags examples examples/03_advanced/concurrent_requests.go
```

### Run all examples in a directory:
```bash
# Quick start
go run -tags examples examples/01_quickstart/*.go

# Core features
go run -tags examples examples/02_core_features/request_options.go
go run -tags examples examples/02_core_features/response_handling.go
go run -tags examples examples/02_core_features/error_handling.go

# Advanced
go run -tags examples examples/03_advanced/client_configuration.go
go run -tags examples examples/03_advanced/concurrent_requests.go
```

## 📚 Learning Path

### Beginner
1. ✅ **01_quickstart/basic_usage.go** - Learn the basics
2. ✅ **02_core_features/request_options.go** - Master request options
3. ✅ **02_core_features/response_handling.go** - Handle responses
4. ✅ **02_core_features/error_handling.go** - Handle errors properly

### Intermediate
5. ✅ **03_advanced/client_configuration.go** - Configure clients
6. ✅ **03_advanced/timeout_retry.go** - Resilient requests
7. ✅ **03_advanced/file_operations.go** - File handling
8. ✅ **03_advanced/cookies_advanced.go** - Cookie management

### Advanced
9. ✅ **03_advanced/concurrent_requests.go** - Parallel processing
10. ✅ **03_advanced/domain_client.go** - State management
11. ✅ **03_advanced/rest_api_client.go** - Production patterns

## 💡 Tips

- **Start with 01_quickstart** - Don't skip the basics!
- **Run examples** - See them in action
- **Modify examples** - Best way to learn
- **Check comments** - Detailed explanations included
- **Use reliable endpoints** - Examples use httpbin.org and echo.hoppscotch.io

## 🔗 Additional Resources

- **[Main README](../README.md)** - Library overview
- **[Getting Started Guide](../docs/getting-started.md)** - Detailed tutorial
- **[API Documentation](../docs)** - Complete reference
- **[Configuration Guide](../docs/configuration.md)** - Advanced configuration

---

**Happy coding! 🚀**

