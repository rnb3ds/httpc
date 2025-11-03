# cybergodev/httpc - Release Notes

All notable changes to this project will be documented in this file.

---

## v1.1.0 - Performance & Architecture Upgrade (2025-11-02)

### 🎯 Overview

Major architecture upgrade focused on performance optimization, concurrency control, health monitoring, and developer experience improvements. Adopts modular internal architecture while maintaining full backward compatibility.

### ✨ New Features

- **🏗️ Modular Architecture**: Introduced `internal/engine` core engine with clear separation between public API and internal implementation
- **📊 Health Monitoring**: Added `internal/monitoring` package for real-time tracking of request success rates, latency, timeout rates, and resource usage
- **🚀 Concurrency Management**: Adaptive semaphore control with request queuing and graceful degradation
- **💾 Memory Management**: Three-tier buffer pool system (4KB/32KB/256KB) reducing GC pressure by 90%
- **🔄 Response Caching**: Thread-safe LRU cache with TTL and automatic expiration cleanup
- **🔌 Connection Pool Enhancement**: Per-host statistics, HTTP/2 optimization, and connection lifecycle management
- **🛡️ Security Enhancement**: Enhanced input validation, CRLF injection protection, length limits, and special character filtering
- **📈 Rate Limiting**: Token bucket algorithm implementation with configurable RPS and burst capacity

### 🔧 Improvements

- **Performance Optimization**: Optimized request/response processing pipeline, retry logic, and error handling
- **Test Coverage**: Increased test coverage, including benchmarks and security validation
- **Code Quality**: Extensive use of atomic operations, lock-free design, and richer error context
- **Documentation**: Added `USAGE_GUIDE.md`, updated all examples and API documentation

### 🐛 Bug Fixes

- Fixed connection pool memory leaks and concurrent race conditions
- Improved context cancellation propagation and resource cleanup
- Fixed edge cases in error handling and retry logic

### 📦 Internal Changes

**New Internal Packages**:
- `internal/cache` - Response caching
- `internal/concurrency` - Concurrency control
- `internal/engine` - HTTP processing engine
- `internal/memory` - Memory management
- `internal/monitoring` - Health monitoring
- `internal/ratelimit` - Rate limiting

**Removed**: `internal/pool` (replaced by `internal/memory`)

### 📊 Performance Metrics

- Memory allocations reduced by ~90%
- Supports 500+ concurrent requests
- Average latency improved by ~30%
- Connection reuse efficiency improved by ~40%

### ⚠️ Breaking Changes

**None** - Fully backward compatible with v1.0.0. All public APIs remain unchanged.

---

## v1.0.0 - Initial version (2025-10-02)

### 🎉 Initial Release

Modern HTTP client library for Go with comprehensive security features, optimal performance, and developer-friendly APIs.

**Key Features:**
- 🛡️ **Secure by Default** - TLS 1.2+, input validation, CRLF protection, SSRF prevention
- ⚡ **High Performance** - Connection pooling, HTTP/2, buffer pooling (90% less allocations)
- 🔄 **Built-in Resilience** - Circuit breaker, intelligent retry, graceful degradation
- 🎯 **Developer Friendly** - Simple API, rich options, comprehensive error handling
- 📥 **File Download** - Progress tracking, resume support, streaming for large files

### What's Included

**Core Features:**
- Full HTTP methods support (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS)
- 25+ request options (headers, auth, query params, body formats, cookies, timeout, retry)
- Rich response handling with JSON/XML parsing
- Advanced file upload/download with progress tracking
- Automatic cookie management with cookie jar

**Security & Performance:**
- TLS 1.2-1.3 with configurable cipher suites
- Three security presets (Permissive, Balanced, Strict)
- Connection pooling with HTTP/2 support
- Circuit breaker for fault protection
- Intelligent retry with exponential backoff

**Configuration:**
- Secure defaults that work out of the box
- Timeout: 60s, Max Retries: 2, TLS: 1.2-1.3
- Max concurrent requests: 500
- Max response body size: 50 MB

