# cybergodev/httpc - Release Notes

All notable changes to this project will be documented in this file.

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

