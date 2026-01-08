# Changes

All notable changes to the cybergodev/httpc library will be documented in this file.

[//]: # (The format is based on [Keep a Changelog]&#40;https://keepachangelog.com/en/1.0.0/&#41;,)
[//]: # (and this project adheres to [Semantic Versioning]&#40;https://semver.org/spec/v2.0.0.html&#41;.)

---

## v1.3.3 - Code Quality & Security Enhancement (2025-12-31)

### Security
- Enhanced SSRF protection with secure defaults (blocks private IP ranges by default)
- Maintained backward compatibility for existing `AllowPrivateIPs: true` configurations
- Improved default security posture for new users

### Code Quality
- **NEW**: Created `internal/validation/common.go` package consolidating all validation logic
- **REMOVED**: ~150 lines of duplicate validation code from `public_options.go`
- **CONSOLIDATED**: 8 validation functions into reusable, well-tested utilities
- Enhanced test coverage with 50+ validation test cases
- Unified error message formats across all validators

### Performance
- Optimized retry logic with early returns and priority ordering
- Enhanced file path validation with single-pass character checking
- Improved cookie parsing with better bounds checking
- Reduced allocations in hot paths through optimized string operations

### Changed
- All validation logic centralized in single package for better maintainability
- Consistent CRLF injection prevention and input sanitization
- Zero breaking changes - all public APIs remain unchanged
- Code duplication reduced by ~25% through consolidation

---

## v1.3.1 - Security and Performance Optimization (2025-12-12)

### Security
- Enhanced input validation with CRLF injection prevention
- Strengthened SSRF protection with additional IP range checks
- Improved file path validation with comprehensive security checks
- Enhanced cookie parsing with strict validation rules

### Performance
- Optimized hot path functions for minimal allocations
- Improved connection pool metrics with lock-free operations
- Optimized string operations and trimming functions
- Enhanced domain client URL building and state management

### Error Handling
- Comprehensive error classification improvements with priority-based matching
- Added Unix error code support (ECONNREFUSED, ETIMEDOUT, EHOSTUNREACH, ENETUNREACH)
- Enhanced network error pattern detection
- Fixed HTTP/2 header validation error messages
- Added Connection header value validation (keep-alive, close, upgrade)

### Code Quality
- Added comprehensive comments for security-critical functions
- Improved error context and classification accuracy
- Enhanced test coverage for edge cases and ambiguous patterns
- Optimized error classification with early returns for better performance

---

## v1.3.0 - Performance and Quality Improvements (2025-12-09)

> **⚠️ BREAKING CHANGES**: This version includes internal optimizations that may affect behavior in edge cases. While the public API remains unchanged, applications relying on specific internal timing or nil-handling behavior should test thoroughly before upgrading.

### ⚠️ Migration Notes
- **Nil Response Handling**: Removed redundant nil checks in hot paths. Ensure your code doesn't pass nil Response/Result objects to methods.
- **Validation Changes**: Stricter input validation may reject previously accepted (but invalid) inputs. Review error handling for header/cookie/URL validation.
- **Decompression**: Responses with `Content-Encoding: gzip/deflate` are now automatically decompressed. If you were manually handling decompression, remove that code.
- **Redirect Tracking**: New redirect chain tracking is enabled by default. Use `WithFollowRedirects(false)` to disable if needed.

### Performance
- Optimized hot path execution by removing redundant nil checks in Result and Response methods
- Improved status code checks with local variable caching
- Enhanced localhost detection using switch statement for better performance
- Optimized URL building in DomainClient (prioritize https:// check)
- Accelerated file path validation by moving UNC path check earlier
- Reduced allocations in cookie parsing with custom trim functions
- Added integer lookup table (0-99) for common conversions
- Optimized String() methods with pre-allocated strings.Builder

### Code Quality
- Consolidated duplicate cookie parsing logic between cookie_utils.go and public_options.go
- Added shared trimSpace helper functions to eliminate code duplication
- Unified character validation patterns (control char checks: c < 0x20 || c == 0x7F)
- Simplified validation logic across all input validators
- Enhanced prepareFilePath with single-pass validation
- Improved whitespace-only header key validation
- Optimized option processing in Request method
- Enhanced code readability and maintainability

### Added
- Automatic decompression for gzip and deflate encoded responses
- Redirect chain tracking (RedirectChain, RedirectCount in Result)
- Per-request redirect control (WithFollowRedirects, WithMaxRedirects)
- WithCookieString for parsing cookie strings from browser/server
- WithCookieValue for simple cookie creation
- DomainClient for automatic cookie/header management per domain

### Security
- Enhanced header validation to detect CRLF injection
- Comprehensive URL validation with scheme checking
- Strengthened cookie validation with improved error messages
- Added size limits for all user inputs

### Changed
- All optimizations maintain thread safety and existing functionality
- Zero breaking changes to public API
- Test coverage maintained at >72%
- Backward compatibility preserved with deprecated Response type

---


## v1.2.2 - Response and Cookie Enhancement (2025-12-04)

### Added
- String() method for Response type providing concise text representation
- Html() method as alias for Response.Body property
- `WithCookieString()` method for automatic cookie string parsing
- Support for browser-style cookie strings (e.g., "name1=value1; name2=value2")
- Comprehensive cookie string validation and error handling
- Cookie methods documentation in request options sections
- Enhanced Cookie Management examples in both English and Chinese README
- Thread-safe formatted output with nil response handling

### Changed
- Updated documentation with manual cookie setting examples
- Expanded cookie usage examples for better developer experience

---

## v1.2.1 - Code Quality, Security & Performance (2025-12-01)

### Security
- Fixed header injection vulnerability by rejecting all control characters (0x00-0x1F, 0x7F)
- Fixed path traversal validation using filepath.Rel() for robust cross-platform checking
- Fixed silent cookie jar error handling with proper error propagation
- Fixed file close error handling to prevent silent data loss
- Enhanced control character validation across all input paths

### Performance
- Fixed race condition in latency metrics using atomic.CompareAndSwap
- Optimized retry jitter generation (~10x faster using math/rand)
- Optimized header validation with fast path for managed headers
- Optimized localhost detection with fast path for common cases
- Simplified response body drain logic for better connection reuse
- Optimized header copying using shallow copy (reduced allocations)
- Removed unnecessary HEAD request in download operations (~30-50% faster)
- Reduced validation overhead by caching string length calculations

### Code Quality
- Modernized to Go 1.22+ syntax (range over int in loops)
- Consolidated validation logic into reusable helper functions
- Eliminated ~250 lines of duplicate validation code
- Extracted magic numbers to named constants across all files
- Refactored config presets to build from DefaultConfig() base
- Removed obsolete closure capture patterns
- Enhanced error messages with better context
- Improved thread-safety documentation for Response and Config types
- Removed unused HTTP2MaxStreams config field

### Changed
- Test coverage improved to 77.2% (main), 88.1% (security)
- Reduced validation code by 25% through consolidation
- Reduced config preset initialization by ~50%
- Binary size reduced through code deduplication
- All validation functions now use consistent error formats

### Fixed
- Race condition in concurrent latency metric updates
- Silent failures in cookie jar creation
- Incomplete response body draining causing connection leaks
- Redundant string operations in error classification
- Inconsistent validation behavior across duplicate code paths

---

## v1.2.0 - Optimize the core logic (2025-11-24)

### Added
- 50+ new test cases targeting previously untested code paths
- Config preset tests (SecureConfig, PerformanceConfig, MinimalConfig, TestingConfig)
- Package-level HTTP methods tests
- Request options completeness tests
- Response handling edge case tests
- Download package-level function tests
- Comprehensive validation (URL, header, cookie, query, file path)
- SSRF protection with pre/post-DNS validation
- Retry-After header support
- Panic recovery mechanism
- Thread safety documentation

### Changed
- Context cancellation/timeout no longer retries
- Smart network error classification
- Exponential backoff with secure random jitter
- Precise retryable status codes (408, 429, 500, 502, 503, 504)
- Config and Response deep copy for thread safety
- Atomic operation optimization
- Default client double-checked locking
- Connection pool health checks
- Rich error context with wrapping
- Windows path handling improvements
- Test coverage improved from ~70% to ~95%

### Fixed
- Retry after context cancellation
- Windows path traversal detection false positives
- Race conditions from concurrent Config modifications
- Connection leaks from incomplete response body reads
- Incomplete cookie validation security issues
- Race conditions in default client initialization

---

## v1.1.0 - Optimization and Enhancement (2025-11-02)

### Added
- internal/engine core engine package
- internal/monitoring package for real-time metrics
- Adaptive semaphore control with request queuing
- Three-tier buffer pool system (4KB/32KB/256KB)
- Thread-safe LRU response cache with TTL
- Per-host connection pool statistics
- HTTP/2 optimization
- Enhanced input validation with CRLF protection
- Token bucket rate limiting
- USAGE_GUIDE.md documentation

### Changed
- Optimized request/response processing pipeline
- Improved retry logic and error handling
- Memory allocations reduced by ~90%
- Average latency improved by ~30%
- Connection reuse efficiency improved by ~40%

### Fixed
- Connection pool memory leaks
- Concurrent race conditions
- Context cancellation propagation issues
- Resource cleanup edge cases

---

## v1.0.0 - Initial Release (2025-10-02)

### Added
- Full HTTP methods support
- 25+ request options
- Rich response handling with JSON/XML parsing
- Advanced file upload/download with progress tracking
- Automatic cookie management
- TLS 1.2-1.3 with configurable cipher suites
- Three security presets
- Connection pooling with HTTP/2 support
- Circuit breaker for fault protection
- Intelligent retry with exponential backoff
- Input validation and CRLF protection
- SSRF prevention
- Buffer pooling
- Secure defaults
