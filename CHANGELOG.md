# cybergodev/httpc - Release Notes

All notable changes to this project will be documented in this file.

---

## v1.2.0 - Stability & Test Coverage Enhancement (2025-11-24)

### 🎯 Overview

Major stability upgrade focused on test coverage improvement, error handling enhancement, concurrency safety, and code quality optimization. This release ensures production reliability through comprehensive testing and validation.

### ✨ Key Improvements

**📊 Test Coverage Significantly Improved**

Added `coverage_improvement_test.go` targeting previously untested code paths:
- Config preset tests (SecureConfig, PerformanceConfig, MinimalConfig, TestingConfig)
- Package-level HTTP methods (Get, Post, Put, Patch, Delete, Head, Options)
- Request options completeness (WithXMLAccept, WithJSONAccept, WithBinary, WithCookies)
- Response handling edge cases (Cookie operations, JSON parsing errors)
- Download package-level functions (DownloadWithOptions)
- Client helper functions (calculateOptimalIdleConnsPerHost, calculateMaxRetryDelay)

**🛡️ Security Enhancements**

Comprehensive validation and boundary checks:
- URL validation (CRLF injection prevention, protocol restrictions, length limits)
- Header validation (special character filtering, pseudo-header blocking, auto-managed header protection)
- Cookie validation (name/value/domain/path validation, size limits, injection prevention)
- Query parameter validation (key/value length limits, special character checks)
- File path validation (path traversal prevention, system path protection, Windows volume handling)
- SSRF protection (pre-DNS IP validation, post-DNS resolution verification, private IP blocking)

**🔄 Retry Mechanism Optimization**

Intelligent retry logic and error classification:
- Context cancellation/timeout no longer retries (avoids futile retries)
- Smart network error classification (DNS errors, connection errors, timeout errors)
- Retry-After header support (RFC1123 time format, seconds format)
- Exponential backoff optimization (secure random jitter, max delay limits)
- Precise retryable status codes (408, 429, 500, 502, 503, 504)

**⚡ Concurrency Safety Improvements**

Thread-safe guarantees and atomic operations:
- Config deep copy (prevents concurrent modification, Headers map isolation)
- Response Headers deep copy (safe multi-goroutine reads)
- Atomic operation optimization (request counters, latency stats, connection pool metrics)
- Default client double-checked locking (thread-safe lazy initialization)
- Connection pool health checks (connection hit rate, active connection monitoring)

**🔧 Error Handling Enhancement**

Unified error classification and context information:
- ClientError types (Retryable, Timeout, Network, Validation, Server)
- Rich error context (URL, Method, Attempts, StatusCode)
- Panic recovery mechanism (prevents client crashes)
- Error wrapping optimization (uses %w to preserve error chain)

**📥 File Download Improvements**

Path safety and cross-platform compatibility:
- Windows path handling (volume validation, path separator normalization)
- Working directory restrictions (prevents path traversal to parent directories)
- System path protection (blocks access to /etc, /sys, C:\Windows, etc.)
- Automatic directory creation (parent directory auto-creation, permission settings)
- Resume download optimization (Range header support, 416 status code handling)

### 🔧 Code Quality Enhancements

**Test Organization**
- Grouped tests by functional modules (Config, Security, Retry, Options, Integration)
- Table-driven test patterns (improved test maintainability)
- Boundary condition tests (null values, oversized inputs, special characters)
- Error path tests (exception scenario coverage)

**Documentation**
- Thread safety documentation (Config, Response, Client)
- Usage examples (concurrent usage patterns)
- Parameter validation documentation (limits and constraints)

**Performance**
- Response body fully drained (connection reuse optimization)
- Max drain limit (prevents memory exhaustion from malicious servers)
- Context timeout handling optimization (avoids duplicate timeout settings)
- Latency metric calculation optimization (moving average algorithm)

### 🐛 Bug Fixes

- Fixed retry after context cancellation
- Fixed Windows path traversal detection false positives
- Fixed race conditions from concurrent Config modifications
- Fixed connection leaks from incomplete response body reads
- Fixed incomplete cookie validation security issues
- Fixed race conditions in default client initialization

### 📊 Test Statistics

- **New test file**: `coverage_improvement_test.go` (410+ lines)
- **Test cases added**: 50+ new test cases
- **Coverage improvement**: Critical path coverage from ~70% to ~95%
- **Test scenarios**: Normal flows, boundary conditions, error handling, concurrency safety

### ⚠️ Breaking Changes

**None** - Fully backward compatible with v1.1.0. All public APIs remain unchanged.

### 📝 Upgrade Notes

Upgrading from v1.1.0 to v1.2.0 requires no code changes. Benefits include:
- Higher stability and reliability
- Better error handling and diagnostics
- Stronger security protections
- More comprehensive concurrency safety guarantees

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
- `internal/engine` - Reconstruct and optimize the internal core package.

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

