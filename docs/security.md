# Security Guide

This guide covers security features, best practices, and compliance considerations for HTTPC.

## Table of Contents

- [Overview](#overview)
- [TLS Configuration](#tls-configuration)
- [Input Validation](#input-validation)
- [Security Presets](#security-presets)
- [Attack Prevention](#attack-prevention)
- [Compliance](#compliance)
- [Best Practices](#best-practices)

## Overview

HTTPC provides comprehensive security features:

- ✅ **TLS 1.2+ by default** - Modern encryption standards
- ✅ **Input validation** - URL and header validation
- ✅ **SSRF prevention** - Blocks requests to private IPs
- ✅ **CRLF injection protection** - Header validation
- ✅ **Size limits** - Prevents memory exhaustion
- ✅ **Rate limiting** - Built-in concurrency limits

## TLS Configuration

### Default TLS Settings

```go
// Secure by default
client, err := httpc.New()
```

**Default TLS configuration:**
- Minimum version: TLS 1.2
- Maximum version: TLS 1.3
- Secure cipher suites only
- Certificate verification enabled

### TLS Version Control

```go
import "crypto/tls"

config := httpc.DefaultConfig()
config.MinTLSVersion = tls.VersionTLS12
config.MaxTLSVersion = tls.VersionTLS13

client, err := httpc.New(config)
```

**Recommended versions:**
- **Standard use**: TLS 1.2 minimum, TLS 1.3 preferred
- **High security**: TLS 1.3 only
- **Legacy systems**: TLS 1.0+ (use with caution)

### Custom Cipher Suites

```go
config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    CipherSuites: []uint16{
        // TLS 1.3 cipher suites (preferred)
        tls.TLS_AES_256_GCM_SHA384,
        tls.TLS_CHACHA20_POLY1305_SHA256,
        tls.TLS_AES_128_GCM_SHA256,
        
        // TLS 1.2 cipher suites (for compatibility)
        tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
    },
}

client, err := httpc.New(config)
```

### Client Certificates (mTLS)

```go
import "crypto/x509"

// Load client certificate
cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

// Load CA certificate
caCert, err := os.ReadFile("ca.crt")
if err != nil {
    log.Fatal(err)
}

caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    Certificates: []tls.Certificate{cert},
    RootCAs:      caCertPool,
    MinVersion:   tls.VersionTLS12,
}

client, err := httpc.New(config)
```

### Certificate Pinning

```go
import "crypto/sha256"

func verifyServerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
    // Expected certificate fingerprint
    expectedFingerprint := "abc123..."
    
    if len(rawCerts) == 0 {
        return fmt.Errorf("no certificates provided")
    }
    
    // Calculate fingerprint
    hash := sha256.Sum256(rawCerts[0])
    fingerprint := hex.EncodeToString(hash[:])
    
    if fingerprint != expectedFingerprint {
        return fmt.Errorf("certificate fingerprint mismatch")
    }
    
    return nil
}

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    VerifyPeerCertificate: verifyServerCertificate,
}

client, err := httpc.New(config)
```

## Input Validation

### URL Validation

HTTPC automatically validates URLs to prevent:
- SSRF attacks
- Invalid URL formats
- Malicious redirects

```go
// Enabled by default
config := httpc.DefaultConfig()
config.ValidateURL = true  // Default

client, err := httpc.New(config)
```

**Validation checks:**
- Valid URL format
- Allowed protocols (http, https)
- No private IP addresses (in production)
- No localhost access (in production)

### Header Validation

Prevents CRLF injection attacks:

```go
// Enabled by default
config := httpc.DefaultConfig()
config.ValidateHeaders = true  // Default

client, err := httpc.New(config)
```

**Validation checks:**
- No CRLF characters (\r\n)
- Valid header format
- No control characters

### Private IP Protection

```go
// Standard configuration
config := httpc.ConfigPreset(httpc.SecurityLevelBalanced)
config.AllowPrivateIPs = false  // Blocks 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16

// Development configuration
config := httpc.ConfigPreset(httpc.SecurityLevelPermissive)
config.AllowPrivateIPs = true  // Allows private IPs
```

## Security Presets

### Strict (Maximum Security)

```go
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelStrict))
```

**Settings:**
- TLS 1.3 only
- No redirects
- Strict validation
- Small response size limit (10 MB)
- Low concurrency limit (100)
- Short timeout (30s)

**Use cases:**
- Financial services (PCI DSS)
- Healthcare (HIPAA)
- Government systems (FIPS)
- Payment processing

### Balanced (Default)

```go
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelBalanced))
// Or simply:
client, err := httpc.New()
```

**Settings:**
- TLS 1.2-1.3
- Redirects enabled
- Full validation
- Medium response size limit (50 MB)
- Medium concurrency limit (500)
- Standard timeout (60s)

**Use cases:**
- Most applications
- Public APIs
- Web services
- Microservices

### Permissive (Development Only)

```go
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelPermissive))
```

**Settings:**
- TLS 1.0+
- Redirects enabled
- Relaxed validation
- Large response size limit (100 MB)
- High concurrency limit (1000)
- Long timeout (120s)

**Use cases:**
- Development environments
- Testing
- Legacy system integration
- Internal tools

**⚠️ Warning:** Not recommended for external APIs!

## Attack Prevention

### SSRF Prevention

```go
// Enabled by default in balanced preset
config := httpc.ConfigPreset(httpc.SecurityLevelBalanced)
config.AllowPrivateIPs = false

client, err := httpc.New(config)

// This will be blocked:
resp, err := client.Get("http://169.254.169.254/latest/meta-data/")
// Error: private IP addresses not allowed
```

### CRLF Injection Prevention

```go
// Automatically validated
resp, err := client.Get(url,
    httpc.WithHeader("X-Custom", "value\r\nInjected: header"),
)
// Error: invalid header value
```

### Response Size Limits

```go
config := httpc.DefaultConfig()
config.MaxResponseBodySize = 50 * 1024 * 1024  // 50 MB

client, err := httpc.New(config)

// Responses larger than 50 MB will be rejected
```

### Request Rate Limiting

```go
config := httpc.DefaultConfig()
config.MaxConcurrentRequests = 500  // Limit concurrent requests

client, err := httpc.New(config)
```

### Timeout Protection

```go
// Prevent hanging requests
resp, err := client.Get(url,
    httpc.WithTimeout(30*time.Second),
)
```

## Compliance

### PCI DSS Compliance

```go
// PCI DSS requires TLS 1.2+
config := httpc.ConfigPreset(httpc.SecurityLevelStrict)
config.MinTLSVersion = tls.VersionTLS12

// Disable weak cipher suites
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    CipherSuites: []uint16{
        tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
    },
}

client, err := httpc.New(config)
```

### HIPAA Compliance

```go
// HIPAA requires encryption in transit
config := httpc.ConfigPreset(httpc.SecurityLevelStrict)
config.MinTLSVersion = tls.VersionTLS12
config.InsecureSkipVerify = false  // Must verify certificates

// Enable audit logging
// (implement custom logging wrapper)

client, err := httpc.New(config)
```

### GDPR Compliance

```go
// GDPR requires data protection
config := httpc.ConfigPreset(httpc.SecurityLevelBalanced)

// Don't log sensitive data
// Implement data minimization
// Use encryption for personal data

client, err := httpc.New(config)
```

## Best Practices

### ✅ DO

1. **Use TLS 1.2+ for secure connections**
   ```go
   config.MinTLSVersion = tls.VersionTLS12
   ```

2. **Enable certificate verification**
   ```go
   config.InsecureSkipVerify = false  // Default
   ```

3. **Validate all inputs**
   ```go
   config.ValidateURL = true
   config.ValidateHeaders = true
   ```

4. **Set response size limits**
   ```go
   config.MaxResponseBodySize = 50 * 1024 * 1024
   ```

5. **Use appropriate security preset**
   ```go
   httpc.ConfigPreset(httpc.SecurityLevelBalanced)
   ```

6. **Implement timeout protection**
   ```go
   httpc.WithTimeout(30*time.Second)
   ```

7. **Block private IPs for external APIs**
   ```go
   config.AllowPrivateIPs = false
   ```

8. **Use strong cipher suites**
   ```go
   config.TLSConfig.CipherSuites = []uint16{...}
   ```

### ❌ DON'T

1. **Never skip TLS verification**
   ```go
   // Bad!
   config.InsecureSkipVerify = true
   ```

2. **Don't use permissive preset for external APIs**
   ```go
   // Bad for external APIs!
   httpc.ConfigPreset(httpc.SecurityLevelPermissive)
   ```

3. **Don't allow private IPs for external APIs**
   ```go
   // Bad for external APIs!
   config.AllowPrivateIPs = true
   ```

4. **Don't disable validation**
   ```go
   // Bad!
   config.ValidateURL = false
   config.ValidateHeaders = false
   ```

5. **Don't use weak TLS versions**
   ```go
   // Bad!
   config.MinTLSVersion = tls.VersionTLS10
   ```

6. **Don't log sensitive data**
   ```go
   // Bad!
   log.Printf("Token: %s", token)
   ```

7. **Don't hardcode credentials**
   ```go
   // Bad!
   httpc.WithBearerToken("hardcoded-token")

   // Good - use environment variables
   httpc.WithBearerToken(os.Getenv("API_TOKEN"))
   ```

## Security Checklist

### Deployment Checklist

- [ ] TLS 1.2+ enabled
- [ ] Certificate verification enabled
- [ ] URL validation enabled
- [ ] Header validation enabled
- [ ] Private IPs blocked (for external APIs)
- [ ] Response size limits set
- [ ] Timeouts configured
- [ ] Rate limiting enabled
- [ ] Appropriate security preset used
- [ ] No hardcoded credentials
- [ ] Sensitive data not logged
- [ ] Security headers set
- [ ] Error messages sanitized

### Compliance Requirements

- [ ] TLS version meets requirements
- [ ] Cipher suites approved
- [ ] Audit logging implemented
- [ ] Data encryption enabled
- [ ] Access controls in place
- [ ] Security monitoring active
- [ ] Incident response plan ready

## Additional Resources

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [TLS Best Practices](https://wiki.mozilla.org/Security/Server_Side_TLS)
- [PCI DSS Requirements](https://www.pcisecuritystandards.org/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)

---

