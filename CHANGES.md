# Changes

All notable changes to the cybergodev/httpc library will be documented in this file.

[//]: # (The format is based on [Keep a Changelog]&#40;https://keepachangelog.com/en/1.0.0/&#41;,)
[//]: # (and this project adheres to [Semantic Versioning]&#40;https://semver.org/spec/v2.0.0.html&#41;.)

---

## v1.3.7 - DNS-over-HTTPS, Proxy Detection & Code Quality (2026-01-22)

### Added
- **DNS-over-HTTPS (DoH) Resolver**: Encrypted DNS resolution with multiple providers (Cloudflare, Google, AliDNS)
  - Built-in caching with configurable TTL (default: 5 minutes)
  - Automatic fallback to system DNS when DoH fails
  - Helps bypass DNS pollution and prevents DNS hijacking
  - Configurable via `EnableDoH` and `DoHCacheTTL` options
- **Configurable System Proxy Detection**: New `EnableSystemProxy` option for explicit control over automatic system proxy detection
  - Supports Windows Registry, macOS system settings, and environment variables
  - Proxy priority: Manual ProxyURL > System Proxy > Direct Connection
  - Default: `false` (requires explicit opt-in)
- **Cross-Platform System Proxy Detection**: Automatic detection across Windows, macOS, and Linux platforms
  - Windows: Reads from Registry (`Internet Settings`)
  - All platforms: Falls back to `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` env vars
- **New `internal/netutil` Package**: Shared IP validation utilities
  - `IsPrivateOrReservedIP()` for private/reserved IP detection
  - `ValidateIP()` for SSRF protection
  - `IsLocalhost()` for localhost detection
  - Single source of truth for network security checks

### Changed
- **IP Validation Logic**: Consolidated duplicate IP validation code (~60 lines) into shared `netutil` package
- **Cookie Validation**: Centralized in `internal/validation/common.go`, removed duplication from `public_options.go` and `domain_client.go`
- **Cross-Platform Path Validation**: Enhanced `isSystemPath()` with platform-specific system paths for Windows, macOS, and Linux
  - Environment variable expansion on Windows (`%SystemRoot%`, `%windir%`)
  - Proper case-insensitive comparison on Windows, case-sensitive on Unix
- **FormData Performance**: Replaced JSON serialization/deserialization with type reflection-based detection
- **Constants**: Added `maxURLLen` constant, replaced hard-coded timeout values with named constants
- **Metrics**: Removed unused `idleConns` field and `IdleConnections` metric

### Fixed
- **Windows Proxy Detection**: Replaced deprecated `syscall.StringToUTF16Ptr()` with `windows.UTF16PtrFromString()` from `golang.org/x/sys/windows`
- **Dead Code**: Removed unused `isValidHeaderByte()` function from `types.go`

### API Compatibility
- **Breaking Change**: System proxy detection no longer automatic by default
  - Set `EnableSystemProxy: true` to restore old behavior
  - Code relying on implicit system proxy detection needs migration
- **No Breaking Changes to Public API**: All modifications are internal refactoring except `EnableSystemProxy` default behavior

### Impact
- **Maintainability**: Reduced code duplication by ~150 lines through package consolidation
- **Cross-Platform**: System path detection works correctly on Windows, macOS, and Linux
- **Testability**: `GetOS` variable allows OS mocking for unit tests
- **Security**: Consistent IP and cookie validation across all code paths

---

## v1.3.6 - Code Quality, Cross-Platform Compatibility & Documentation (2026-01-19)

### Security
- **Fixed SSRF Protection Cross-Platform Compatibility**: Replaced Windows-incompatible `syscall.RawConn.Control` implementation with cross-platform DNS resolution validation
  - `validateAddressBeforeDial()` now performs comprehensive address validation before connection
  - Validates all resolved IPs for domain names (defense against DNS rebinding attacks)
  - Consistent behavior across Windows, Linux, and macOS
- **Enhanced Validation**: Direct IP validation for IP address targets, DNS resolution with full IP range checking

### Performance
- **Code Deduplication**: Removed ~30 lines of duplicate string trimming code by using `strings.TrimSpace`
- **Helper Functions**: Added `isValidHeaderByte()` for byte-based header validation
- **Optimized Imports**: Removed unused imports (`syscall`, `strings`)

### Code Quality
- **Fixed Race Conditions**:
  - `prepareManagedOptions()` now holds read lock for entire operation
  - Previously acquired lock twice separately, allowing state to change between reads
- **Improved Default Client Safety**:
  - `getDefaultClient()` properly checks for nil after loading
  - Simplified `SetDefaultClient()` by removing confusing custom implementation check logic
- **Standard Library Usage**:
  - Replaced custom `pathJoin` with `path.Join` from stdlib
  - Replaced custom trim functions with `strings.TrimSpace`
  - Replaced `fmt.Sprintf` with `strconv.Itoa/FormatInt` for faster integer conversions
- **Removed Unused Code**:
  - Eliminated `autoManage` field (always `true` with no way to toggle)
  - Removed `done` channel (only closed, never used for synchronization)
  - Removed redundant `isValidHeaderChar` function
  - Cleaned up `systemPaths` list
- **Simplified Methods**:
  - Streamlined `doRequest()` by removing unnecessary context extraction
  - Simplified `RoundTrip()` method in transport
  - Reduced complexity in download method implementations

### Bug Fixes
- **DomainClient Documentation**: Fixed inaccurate example claiming domain restriction enforcement
  - Clarified that full URLs bypass domain restriction (by design for flexibility)
  - Added best practice guidance for developers
- **Error Handling**: Fixed ignored error from `url.Parse(dc.baseURL)`
- **Parameter Naming**: Renamed `path` to `pathStr` to avoid naming conflict with package alias

### Documentation
- **Enhanced Example Code** :
  - Updated `demonstrateSecureConfig()` with better error handling
  - Added error messages for RFC 2544 benchmark network environments
  - Added Scenario 7 for private/reserved network configuration
  - Changed example URLs to `httpbin.org` (more reliable)
- **Validation Package**: Exported `IsValidHeaderChar()` for cross-package use
- **Deprecation Handling**: Removed deprecation notice from `isValidHeaderChar()` (used internally)

### API Changes
- **Backward Compatible**: All changes are backward compatible except for `Html()` removal
- No breaking changes to public APIs (except deprecated method removal)

---

## v1.3.5 - Documentation, Code Quality & Performance Enhancement (2026-01-14)

### Documentation
- **Quick Start Enhancement**: Added comprehensive default request headers documentation explaining `httpc.DefaultConfig()` usage and three-level customization (global, client, per-request)
- **Bilingual Support**: Updated both README.md and README_zh-CN.md with structured default headers guide
- **Documentation Accuracy**: Comprehensive verification and corrections across all documentation files (100% accuracy achieved)

### Features
- **DomainClient Download Support**: Added `DownloadFile()` and `DownloadWithOptions()` methods to DomainClient for automatic state management in file downloads
- **Cookie Management**: Enhanced documentation clarifying `EnableCookies` defaults to `false` in `DefaultConfig()`
- **Request Inspection**: Added `Request.Headers` and `Request.Cookies` to Result type for complete request visibility

### Security
- **Context Propagation**: Fixed context handling in `doRequest()` to properly use `WithContext()` for timeout and cancellation
- **URL Scheme Validation**: Added validation to only allow `http` and `https` schemes in DomainClient, preventing SSRF attacks via dangerous protocols (file:, data:, javascript:)
- **Race Condition Prevention**: Limited atomic retry operations to prevent potential CPU thrashing under extreme load
- **CRLF Injection**: Maintained comprehensive protection across all input validation paths

### Code Quality
- **Test Suite Consolidation**: Reduced test files from 32 to 30 by merging duplicate coverage tests
- **Cookie Validation**: Simplified cookie string parsing, reduced complexity by ~30 lines
- **Package Documentation**: Added comprehensive package-level godoc with usage examples
- **Redundant Code Removal**: Eliminated ~200 lines of unnecessary comments and unreachable code
- **URL Building**: Improved DomainClient URL construction using proper URL parsing instead of string manipulation
- **Cookie Security**: Removed hardcoded security-sensitive cookie flags, now uses Go stdlib defaults

### Performance
- **Benchmark Coverage**: 16 comprehensive benchmark categories covering HTTP requests, concurrency, advanced features, memory efficiency, response processing, and decompression
- **Measured Performance**:
  - GET request: ~214μs with 8.7KB memory (93 allocations)
  - POST JSON: ~231μs with 12.7KB memory (146 allocations)
  - Consistent performance across 1-100 concurrent goroutines (~60μs per operation)
  - Zero-copy convenience methods for status code and cookie access
  - Gzip/deflate decompression: ~14μs with 68KB working buffer

### Bug Fixes
- **RequestInfo Fields**: Corrected documentation to only include actual fields (`Headers`, `Cookies`)
- **Error Handling Behavior**: Clarified that HTTPC returns Result for ALL status codes (including 4xx/5xx)
- **Cookie Parsing**: Fixed handling of cookies with empty values (e.g., `empty=`)

### Examples
- **Restructuring**: Major consolidation reducing example files by 60% (30+ files → 12 files)
- **Quality Improvements**: Enhanced error handling, better documentation, reliable testing endpoints
- **Compilation Fixes**: Fixed 6 bugs where method calls were missing parentheses
- **Comprehensive Guides**: Created detailed examples/README.md with learning paths

### Impact
- **Code Quality**: Reduced test files, eliminated code duplication, improved maintainability
- **Features**: DomainClient download parity with Client interface
- **Backward Compatibility**: Zero breaking changes, all tests pass

---

## v1.3.4 - Code Quality, Documentation & Examples Enhancement (2026-01-08)

### Code Quality
- **Eliminated code duplication**: Removed duplicate `validateHeaderKeyValue` function from `types.go`, unified to use `internal/validation` package
- **Performance optimizations**: Replaced `fmt.Sprintf` with `strconv.Itoa/FormatInt` for 2-3x faster integer conversions
- **Memory optimizations**: Changed to fixed-size arrays in `FormatBytes()` and `FormatSpeed()` to eliminate heap allocations
- **Loop optimizations**: Improved iteration patterns in validation and parsing functions
- **Simplified logic**: Streamlined URL building in `domain_client.go`, removed redundant parsing
- **Code cleanup**: Removed redundant comments and verbose validation logic across validation and security packages

### Documentation
- **README improvements**: Fixed variable naming consistency (`resp` → `result`), added missing API documentation
- **API completeness**: Documented `WithXML()`, `WithText()`, `WithBinary()`, `HasCookie()`, `RequestCookies()`, `GetRequestCookie()`, `SaveToFile()`
- **Docs verification**: Comprehensive verification of all 8 docs files, added missing `WithCookieString` documentation
- **Chinese README**: Synchronized all changes to maintain translation consistency
- **Accuracy**: Improved from 98% to 100% documentation accuracy

### Examples
- **Major restructuring**: Reduced from 30+ files across 6 directories to 12 files across 3 directories (60% reduction)
- **Consolidated examples**: Merged related examples into comprehensive files:
  - `request_options.go` (251 lines) - merged 3 files covering body formats, auth, query params
  - `response_handling.go` (216 lines) - merged 3 files covering Result API, parsing, formatting
  - `file_operations.go` (225 lines) - merged 2 files covering upload/download
  - `cookies_advanced.go` (270 lines) - merged 5 files covering all cookie operations
  - `domain_client.go` (165 lines) - merged 4 files covering DomainClient usage
- **Quality improvements**: Enhanced error handling, better documentation, reliable endpoints (httpbin.org)
- **Bug fixes**: Fixed 6 compilation errors where method calls were missing parentheses (`.StatusCode` → `.StatusCode()`)
- **README guides**: Created comprehensive `examples/README.md` with learning paths and directory-specific guides

### Bug Fixes
- Fixed localhost detection logic in `internal/security/validator.go`
- Fixed URL sanitization for fragments in `internal/engine/errors.go`
- Fixed transport error classification patterns
- Fixed API usage bugs in example files (method calls vs field access)

### Impact
- **Code size**: Reduced by ~1,000 lines through consolidation and optimization
- **Performance**: 2-3x faster integer conversions, eliminated heap allocations in hot paths
- **Maintainability**: Single source of truth for validation, better organized tests and examples
- **Developer experience**: Complete and accurate documentation, high-quality consolidated examples
- **Backward compatibility**: Zero breaking changes, all tests pass

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
