# Security Policy

## Security Architecture

HTTPC implements defense-in-depth security with multiple layers of protection:

### 1. Transport Layer Security (TLS)

**Default Configuration:**
- Minimum TLS version: **TLS 1.2**
- Maximum TLS version: **TLS 1.3**
- Certificate verification: **Enabled**
- Secure cipher suites only
- Session ticket caching for performance
- TLS renegotiation: **Disabled** (prevents renegotiation attacks)

**Cipher Suites (Priority Order):**
```
TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305
TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305
```

**Elliptic Curves:**
- X25519 (preferred)
- P-256
- P-384

### 2. SSRF (Server-Side Request Forgery) Protection

HTTPC implements **two-layer SSRF protection**:

#### Layer 1: Pre-DNS Validation
Located in `internal/security/validator.go`:
- Blocks obvious localhost patterns (`localhost`, `127.0.0.1`, `::1`)
- Validates direct IP addresses before DNS resolution
- Rejects invalid URL schemes (only `http` and `https` allowed)

#### Layer 2: Post-DNS Validation (Critical Security Boundary)
Located in `internal/connection/pool.go`:
- Validates **resolved IP addresses** after DNS lookup but **before connection**
- Prevents DNS rebinding attacks
- Blocks connections to:
  - Loopback addresses (`127.0.0.0/8`, `::1`)
  - Private IP ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`)
  - Link-local addresses (`169.254.0.0/16`, `fe80::/10`)
  - Multicast addresses
  - Reserved IP ranges (Class E: `240.0.0.0/4`, `0.0.0.0/8`)
  - Unspecified addresses (`0.0.0.0`, `::`)

**Configuration:**
```go
config := httpc.DefaultConfig()
config.AllowPrivateIPs = false  // Default: blocks private IPs (production)
// Set to true only for development/testing environments
```

**Attack Scenarios Prevented:**
- Cloud metadata service access (`169.254.169.254`)
- Internal network scanning
- DNS rebinding attacks
- Time-of-check-time-of-use (TOCTOU) vulnerabilities

### 3. CRLF Injection Prevention

Comprehensive validation prevents HTTP header injection attacks:

**Protected Components:**
- HTTP headers (keys and values)
- User-Agent strings
- Cookie names, values, domains, and paths
- Basic authentication credentials
- Bearer tokens
- Query parameters
- Form field names and filenames

**Validation Rules:**
- Rejects `\r` (carriage return), `\n` (line feed), `\x00` (null byte)
- Enforces size limits to prevent DoS
- Validates character sets for header keys
- Blocks pseudo-headers (`:authority`, `:method`, etc.)
- Prevents automatic header override (`Content-Length`, `Transfer-Encoding`, `Connection`, `Upgrade`)

**Implementation:**
```go
// All user inputs are validated
if strings.ContainsAny(value, "\r\n\x00") {
    return fmt.Errorf("invalid characters detected")
}
```

### 4. Path Traversal Protection

File download operations include robust path validation:

**Protection Mechanisms:**
- Path normalization using `filepath.Clean()`
- Absolute path resolution and validation
- Working directory boundary enforcement
- System path access prevention
- Volume-aware validation (Windows compatibility)

**Blocked Paths:**
- `/etc/`, `/sys/`, `/proc/`, `/dev/`, `/boot/`, `/root/`
- `/usr/bin/`, `/usr/sbin/`, `/bin/`, `/sbin/`
- `C:\Windows\`, `C:\System32\`, `C:\Program Files\`
- `/Library/`, `/System/`, `/Applications/` (macOS)

**Implementation:**
```go
// Validates file paths before write operations
if err := prepareFilePath(filePath); err != nil {
    return fmt.Errorf("path validation failed: %w", err)
}
```

### 5. Resource Exhaustion Protection

**Response Size Limits:**
- Default: 10 MB per response
- Configurable via `MaxResponseBodySize`
- JSON parsing limited to 50 MB
- Prevents memory exhaustion attacks

**Connection Limits:**
- `MaxIdleConns`: 50 (default)
- `MaxConnsPerHost`: 10 (default)
- `MaxIdleConnsPerHost`: Calculated optimally
- Prevents connection pool exhaustion

**Timeout Protection:**
- Default request timeout: 30 seconds
- Dial timeout: 10 seconds
- TLS handshake timeout: 10 seconds
- Response header timeout: 30 seconds
- Idle connection timeout: 90 seconds
- Prevents slowloris and slow-read attacks

**Size Validation:**
- URL length: max 2048 characters
- Header key: max 256 characters
- Header value: max 8 KB
- Query key: max 256 characters
- Query value: max 8 KB
- Cookie name: max 256 characters
- Cookie value: max 4 KB
- User-Agent: max 512 characters

### 6. Input Validation

**URL Validation:**
- Required scheme (`http` or `https` only)
- Required host component
- Length limits (max 2048 characters)
- Format validation via `url.Parse()`
- Prevents protocol smuggling

**Header Validation:**
- Character set validation (alphanumeric and hyphens only for keys)
- CRLF injection prevention
- Size limits enforcement
- Pseudo-header blocking
- Automatic header protection

**Cookie Validation:**
- Name and value character validation
- Domain and path validation
- Size limits enforcement
- Secure defaults (`HttpOnly`, `SameSite=Lax`)

### 7. Retry Security

**Exponential Backoff with Jitter:**
- Prevents thundering herd problem
- Configurable retry limits (default: 3, max: 10)
- Exponential delay calculation
- Random jitter to distribute load
- Maximum retry delay cap (30 seconds)

**Retryable Conditions:**
- Network errors (connection refused, timeout)
- Transient HTTP errors (429, 500, 502, 503, 504)
- Non-retryable: client errors (4xx except 429)

## Security Presets

### SecureConfig() - Maximum Security
```go
client, err := httpc.NewSecure()
```
- TLS 1.2+ enforced
- Certificate verification required
- Private IPs blocked
- Strict validation enabled
- Conservative resource limits
- **Use for:** Production, external APIs, financial services, healthcare

### DefaultConfig() - Balanced Security
```go
client, err := httpc.New()
```
- TLS 1.2-1.3 supported
- Certificate verification enabled
- Private IPs blocked
- Full validation enabled
- Reasonable resource limits
- **Use for:** Most applications, microservices, web services

### TestingConfig() - Development Only
```go
// Internal use only - not exposed in public API
```
- Relaxed validation
- Private IPs allowed
- Insecure TLS verification
- **⚠️ NEVER use in production**

## Compliance

### PCI DSS (Payment Card Industry Data Security Standard)
- ✅ TLS 1.2+ enforced by default
- ✅ Strong cipher suites only
- ✅ Certificate verification required
- ✅ No weak cryptography
- ✅ Secure key management support (via custom `TLSConfig`)

### HIPAA (Health Insurance Portability and Accountability Act)
- ✅ Encryption in transit (TLS 1.2+)
- ✅ Certificate validation
- ✅ Audit trail support (via custom logging)
- ✅ Access controls (via authentication options)

### GDPR (General Data Protection Regulation)
- ✅ Data protection in transit
- ✅ Secure communication channels
- ✅ No unnecessary data logging
- ⚠️ Application-level data minimization required

### OWASP Top 10 Protection

| Risk                                 | Protection                                          |
|--------------------------------------|-----------------------------------------------------|
| A01:2021 - Broken Access Control     | SSRF protection, private IP blocking                |
| A02:2021 - Cryptographic Failures    | TLS 1.2+, strong ciphers, cert verification         |
| A03:2021 - Injection                 | CRLF prevention, input validation                   |
| A04:2021 - Insecure Design           | Secure defaults, defense-in-depth                   |
| A05:2021 - Security Misconfiguration | Secure presets, validation enforcement              |
| A06:2021 - Vulnerable Components     | Zero external dependencies                          |
| A07:2021 - Authentication Failures   | Secure auth helpers, credential validation          |
| A08:2021 - Software/Data Integrity   | TLS verification, no insecure defaults              |
| A09:2021 - Logging Failures          | Error classification, structured errors             |
| A10:2021 - SSRF                      | Two-layer SSRF protection, DNS rebinding prevention |

## Security Best Practices

### 1. Always Use TLS in Production
```go
// ✅ Good
resp, err := client.Get("https://api.example.com")

// ❌ Bad (unless necessary)
resp, err := client.Get("http://api.example.com")
```

### 2. Never Disable Certificate Verification
```go
// ❌ NEVER do this in production
config := httpc.DefaultConfig()
config.InsecureSkipVerify = true  // Vulnerable to MITM attacks
```

### 3. Block Private IPs for External APIs
```go
// ✅ Good for production
config := httpc.DefaultConfig()
config.AllowPrivateIPs = false  // Default

// ⚠️ Only for development/testing
config.AllowPrivateIPs = true
```

### 4. Set Appropriate Timeouts
```go
// ✅ Good - prevents hanging requests
resp, err := client.Get(url, 
    httpc.WithTimeout(30*time.Second),
)

// ❌ Bad - no timeout protection
resp, err := client.Get(url)  // Uses default, but be explicit
```

### 5. Limit Response Sizes
```go
// ✅ Good - prevents memory exhaustion
config := httpc.DefaultConfig()
config.MaxResponseBodySize = 10 * 1024 * 1024  // 10 MB
```

### 6. Validate User-Provided URLs
```go
// ✅ Good - additional application-level validation
if !strings.HasPrefix(userURL, "https://trusted-domain.com") {
    return errors.New("untrusted URL")
}
resp, err := client.Get(userURL)
```

### 7. Use Secure Authentication
```go
// ✅ Good - Bearer token
resp, err := client.Get(url,
    httpc.WithBearerToken(token),
)

// ✅ Good - Basic auth over HTTPS only
resp, err := client.Get("https://api.example.com",
    httpc.WithBasicAuth(username, password),
)

// ❌ Bad - credentials in URL
resp, err := client.Get("https://user:pass@api.example.com")
```

### 8. Handle Errors Securely
```go
// ✅ Good - don't expose sensitive details
if err != nil {
    log.Printf("Request failed: %v", err)
    return errors.New("service unavailable")  // Generic message to user
}

// ❌ Bad - exposes internal details
if err != nil {
    return fmt.Errorf("failed to connect to %s: %v", internalURL, err)
}
```

### 9. Use Connection Pooling Wisely
```go
// ✅ Good - reuse client instances
var httpClient httpc.Client

func init() {
    httpClient, _ = httpc.New()
}

// ❌ Bad - creates new client per request
func makeRequest() {
    client, _ := httpc.New()
    defer client.Close()
    // ...
}
```

### 10. Implement Rate Limiting
```go
// ✅ Good - limit concurrent requests
config := httpc.DefaultConfig()
config.MaxConnsPerHost = 10  // Limit per host
config.MaxIdleConns = 50     // Total pool limit
```

## Security Testing

### Automated Security Tests

HTTPC includes comprehensive security tests:
- SSRF protection validation
- CRLF injection prevention
- TLS configuration verification
- Input validation testing
- Path traversal prevention
- Resource limit enforcement

Run security tests:
```bash
go test -v -run TestSecurity ./...
```

### Manual Security Review Checklist

- [ ] TLS 1.2+ enforced
- [ ] Certificate verification enabled
- [ ] Private IPs blocked (production)
- [ ] URL validation enabled
- [ ] Header validation enabled
- [ ] Response size limits configured
- [ ] Timeouts set appropriately
- [ ] Connection limits configured
- [ ] No hardcoded credentials
- [ ] Error messages sanitized
- [ ] Logging doesn't expose sensitive data
- [ ] Authentication uses secure methods
- [ ] HTTPS used for all external APIs

## Additional Resources

- [OWASP HTTP Security Headers](https://owasp.org/www-project-secure-headers/)
- [Mozilla TLS Configuration](https://wiki.mozilla.org/Security/Server_Side_TLS)
- [NIST TLS Guidelines](https://csrc.nist.gov/publications/detail/sp/800-52/rev-2/final)
- [CWE-918: SSRF](https://cwe.mitre.org/data/definitions/918.html)
- [CWE-93: CRLF Injection](https://cwe.mitre.org/data/definitions/93.html)

---

**Security Contact**: cybergodev@gmail.com
