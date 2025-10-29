# Product Overview

HTTPC is a modern, high-performance HTTP client library for Go designed for production-grade applications. It provides enterprise-level security, intelligent concurrency control, and thread-safe operations.

## Key Features

- **Security-first design** - TLS 1.2+, input validation, CRLF protection, SSRF prevention
- **High performance** - Zero-allocation buffer pooling, intelligent connection reuse, goroutine-safe operations
- **Massive concurrency** - Handles thousands of concurrent requests with adaptive semaphore control
- **Built-in resilience** - Circuit breaker, intelligent retry with exponential backoff, graceful degradation
- **Developer-friendly** - Simple API with rich options and comprehensive error handling
- **Automatic cookie management** - Simple and convenient automatic management of cookies

## Target Use Cases

- REST API clients
- File download/upload operations
- High-throughput web scraping
- Microservice communication
- Production web applications requiring reliability and security

## API Design Philosophy

The library follows a dual-pattern approach:
- **Request Methods** (`Get`, `Post`, etc.) - specify what HTTP operation to perform
- **Option Methods** (`WithJSON`, `WithTimeout`, etc.) - specify how to customize the request

This design promotes clean, readable code while maintaining flexibility.