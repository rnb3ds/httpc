# HTTPC Examples

Practical examples for the httpc library. Learn by doing!

---

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
- Default, Secure, Performance, Testing presets
- Custom configuration
- Configuration comparison for different scenarios

### HTTP Methods
**[03_advanced/http_methods.go](03_advanced/http_methods.go)**
- HEAD (headers only)
- OPTIONS (allowed methods)
- PATCH (partial updates)
- Method comparison table

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

### Middleware
**[03_advanced/middleware.go](03_advanced/middleware.go)**
- Logging middleware
- Request ID middleware
- Metrics collection
- Recovery middleware
- Audit middleware
- Middleware chaining

### Proxy Configuration
**[03_advanced/proxy_configuration.go](03_advanced/proxy_configuration.go)**
- Direct connection (default)
- System proxy detection
- Manual proxy configuration
- Proxy priority rules

### Advanced Patterns
**[03_advanced/advanced_patterns.go](03_advanced/advanced_patterns.go)**
- Request/Response callbacks
- Result pool optimization (ReleaseResult)
- Testing configuration
- Default client management
- Memory optimization techniques

### Session Management
**[03_advanced/session.go](03_advanced/session.go)**
- SessionManager creation and usage
- Persistent headers and cookies
- Session with DomainClient
- Session state lifecycle

## 🏃 Running Examples

### Run a specific example:
```bash
go run -tags examples examples/01_quickstart/basic_usage.go
go run -tags examples examples/02_core_features/request_options.go
go run -tags examples examples/03_advanced/concurrent_requests.go
go run -tags examples examples/03_advanced/advanced_patterns.go
```

### Run multiple examples (one at a time):
```bash
# Quick start
go run -tags examples examples/01_quickstart/basic_usage.go

# Core features
go run -tags examples examples/02_core_features/request_options.go
go run -tags examples examples/02_core_features/response_handling.go
go run -tags examples examples/02_core_features/error_handling.go
go run -tags examples examples/02_core_features/compression.go

# Advanced
go run -tags examples examples/03_advanced/client_configuration.go
go run -tags examples examples/03_advanced/http_methods.go
go run -tags examples examples/03_advanced/timeout_retry.go
go run -tags examples examples/03_advanced/redirects.go
go run -tags examples examples/03_advanced/concurrent_requests.go
go run -tags examples examples/03_advanced/file_operations.go
go run -tags examples examples/03_advanced/cookies_advanced.go
go run -tags examples examples/03_advanced/domain_client.go
go run -tags examples examples/03_advanced/rest_api_client.go
go run -tags examples examples/03_advanced/middleware.go
go run -tags examples examples/03_advanced/proxy_configuration.go
go run -tags examples examples/03_advanced/advanced_patterns.go
go run -tags examples examples/03_advanced/session.go
```

> **Note**: Each example file has its own `main()` function and must be run individually.

## 📚 Learning Path

### Beginner
1. ✅ **01_quickstart/basic_usage.go** - Learn the basics
2. ✅ **02_core_features/request_options.go** - Master request options
3. ✅ **02_core_features/response_handling.go** - Handle responses
4. ✅ **02_core_features/error_handling.go** - Handle errors properly

### Intermediate
5. ✅ **03_advanced/client_configuration.go** - Configure clients
6. ✅ **03_advanced/http_methods.go** - HEAD, OPTIONS, PATCH
7. ✅ **03_advanced/timeout_retry.go** - Resilient requests
8. ✅ **03_advanced/file_operations.go** - File handling
9. ✅ **03_advanced/cookies_advanced.go** - Cookie management
10. ✅ **03_advanced/redirects.go** - Redirect handling

### Advanced
11. ✅ **03_advanced/concurrent_requests.go** - Parallel processing
12. ✅ **03_advanced/domain_client.go** - State management
13. ✅ **03_advanced/rest_api_client.go** - Production patterns
14. ✅ **03_advanced/middleware.go** - Request middleware
15. ✅ **03_advanced/proxy_configuration.go** - Proxy setup
16. ✅ **03_advanced/advanced_patterns.go** - Callbacks & optimization
17. ✅ **03_advanced/session.go** - Session management

## 🔗 Additional Resources

- **[Main README](../README.md)** - Library overview
- **[Getting Started Guide](../docs/getting-started.md)** - Detailed tutorial
- **[API Documentation](../docs)** - Complete reference
- **[Configuration Guide](../docs/configuration.md)** - Advanced configuration

---


## 💡 Disclaimer & Acknowledgments

### Third-Party Testing Services

The examples in this directory use publicly available testing endpoints provided by open-source projects for demonstration purposes only:

#### 1. Hoppscotch
- **Testing Endpoint**: `https://echo.hoppscotch.io`
- **Project**: [Hoppscotch - Open-source API development ecosystem](https://hoppscotch.io)
- **Repository**: [github.com/hoppscotch/hoppscotch](https://github.com/hoppscotch/hoppscotch)
- **License**: [Hoppscotch License](https://github.com/hoppscotch/hoppscotch/blob/main/LICENSE)

**Usage in Examples**:
- Client configuration demonstrations
- Request/response testing
- HTTP method examples

#### 2. Postman httpbin
- **Testing Endpoint**: `https://httpbin.org`
- **Project**: [httpbin - HTTP Request & Response Service](https://httpbin.org)
- **Repository**: [github.com/postmanlabs/httpbin](https://github.com/postmanlabs/httpbin)
- **License**: [https://github.com/postmanlabs/httpbin/blob/master/LICENSE](https://github.com/postmanlabs/httpbin/blob/master/LICENSE)

**Usage in Examples**:
- Request testing and validation
- Response inspection
- Header and cookie handling
- Status code testing

### Important Notes

**For Learners and Developers:**
- ✅ These endpoints are provided free of charge by their respective projects
- ✅ They are publicly available for testing and educational purposes
- ⚠️ **Rate Limits**: Both services may impose rate limits or availability restrictions
- ⚠️ **Not for Production**: Replace with your own API endpoints in production applications
- ⚠️ **Service Availability**: These services may experience downtime or changes

**Legal Disclaimer:**
- These testing services are used solely for demonstrating httpc library capabilities
- Examples are provided "as is" without warranty of any kind

**Our Gratitude:**
We extend our sincere thanks to the **Hoppscotch** and **Postman httpbin** teams for providing these valuable testing resources to the developer community. Their contributions have significantly simplified the API testing and training for developers.

---

**Happy coding! 🚀**
