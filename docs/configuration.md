# Configuration

This guide covers all configuration options for HTTPC clients.

## Table of Contents

- [Default Configuration](#default-configuration)
- [Security Presets](#security-presets)
- [Custom Configuration](#custom-configuration)
- [TLS Configuration](#tls-configuration)
- [Configuration Reference](#configuration-reference)

## Default Configuration

The library provides secure and optimized defaults that work for most use cases:

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**Default values:**
- Timeout: 30 seconds
- MaxRetries: 3
- RetryDelay: 1 second
- BackoffFactor: 2.0
- MaxIdleConns: 50
- MaxConnsPerHost: 10
- MaxResponseBodySize: 10 MB
- TLS: 1.2-1.3 (via TLSConfig if set)
- HTTP/2: Enabled
- FollowRedirects: true
- EnableCookies: false
- AllowPrivateIPs: false
- StrictContentLength: true

## Security Presets

Choose the right security level for your use case.

### Permissive (Development/Testing)

For development environments and internal APIs:

```go
client, err := httpc.New(httpc.TestingConfig())
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**Settings:**
- TLS: 1.0+ (allows older versions)
- Timeout: 120 seconds
- MaxRetries: 5
- MaxConcurrentRequests: 1000
- MaxResponseBodySize: 100 MB
- AllowPrivateIPs: true
- ValidateHeaders: false

**Use Cases:**
- Development environments
- Internal APIs with legacy systems
- High-throughput testing scenarios
- Local development

### Balanced (Default)

For most applications:

```go
client, err := httpc.New(httpc.DefaultConfig())
// Or simply:
client, err := httpc.New()  // Uses balanced by default
```

**Settings:**
- TLS: 1.2-1.3 (modern security)
- Timeout: 60 seconds
- MaxRetries: 2
- MaxConcurrentRequests: 500
- MaxResponseBodySize: 50 MB
- AllowPrivateIPs: false
- ValidateHeaders: true

**Use Cases:**
- Most applications
- Public APIs
- Standard web services
- Microservices communication

**✅ Recommended:** This is the default and works for 90% of use cases.

### Strict (High Security)

For high-security environments and compliance:

```go
client, err := httpc.New(httpc.SecureConfig())
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**Settings:**
- TLS: 1.3 only (maximum security)
- Timeout: 30 seconds
- MaxRetries: 1
- MaxConcurrentRequests: 100
- MaxResponseBodySize: 10 MB
- AllowPrivateIPs: false
- ValidateHeaders: true
- FollowRedirects: false

**Use Cases:**
- Financial services (PCI DSS compliance)
- Healthcare (HIPAA compliance)
- Government systems (FIPS compliance)
- High-security environments
- Payment processing

**🔒 Security:** Maximum security with minimal attack surface.

### Comparison Table

| Setting                 | Permissive | Balanced  | Strict    |
|-------------------------|------------|-----------|-----------|
| **TLS Version**         | 1.0+       | 1.2-1.3   | 1.3 only  |
| **Timeout**             | 120s       | 60s       | 30s       |
| **Max Retries**         | 5          | 2         | 1         |
| **Max Body Size**       | 100 MB     | 50 MB     | 10 MB     |
| **Concurrent Requests** | 1000       | 500       | 100       |
| **Private IPs**         | ✅ Allowed  | ❌ Blocked | ❌ Blocked |
| **Header Validation**   | ❌ Disabled | ✅ Enabled | ✅ Enabled |
| **Follow Redirects**    | ✅ Yes      | ✅ Yes     | ❌ No      |
| **HTTP/2**              | ✅ Yes      | ✅ Yes     | ✅ Yes     |

## Custom Configuration

For fine-grained control, create a custom configuration:

```go
config := &httpc.Config{
    // Network settings
    Timeout:               30 * time.Second,
    DialTimeout:           15 * time.Second,
    KeepAlive:             30 * time.Second,
    TLSHandshakeTimeout:   15 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    IdleConnTimeout:       90 * time.Second,
    MaxIdleConns:          100,
    MaxIdleConnsPerHost:   10,
    MaxConnsPerHost:       20,

    // Security settings
    MinTLSVersion:         tls.VersionTLS12,
    MaxTLSVersion:         tls.VersionTLS13,
    InsecureSkipVerify:    false,
    MaxResponseBodySize:   50 * 1024 * 1024, // 50 MB
    MaxConcurrentRequests: 500,
    ValidateURL:           true,
    ValidateHeaders:       true,

    // Retry settings
    MaxRetries:    2,
    RetryDelay:    2 * time.Second,
    MaxRetryDelay: 60 * time.Second,
    BackoffFactor: 2.0,
    Jitter:        true,

    // Headers and features
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    EnableHTTP2:     true,
    Headers: map[string]string{
        "Accept": "application/json",
    },
}

client, err := httpc.New(config)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Modifying Presets

Start with a preset and customize:

```go
config := httpc.DefaultConfig()
config.Timeout = 30 * time.Second
config.MaxRetries = 3
config.UserAgent = "MyApp/1.0"

client, err := httpc.New(config)
```

## TLS Configuration

### Basic TLS Configuration

```go
import "crypto/tls"

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    MaxVersion: tls.VersionTLS13,
}

client, err := httpc.New(config)
```

### Custom Cipher Suites

```go
config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    CipherSuites: []uint16{
        tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
    },
}

client, err := httpc.New(config)
```

### Client Certificates

```go
cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    Certificates: []tls.Certificate{cert},
}

client, err := httpc.New(config)
```

### Custom CA Certificates

```go
caCert, err := os.ReadFile("ca.crt")
if err != nil {
    log.Fatal(err)
}

caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    RootCAs: caCertPool,
}

client, err := httpc.New(config)
```

### Skip TLS Verification (Testing Only)

```go
// ⚠️ WARNING: Only for testing! Never use in production!
config := httpc.DefaultConfig()
config.InsecureSkipVerify = true

client, err := httpc.New(config)
```

## Configuration Reference

### Network Settings

| Field                   | Type            | Default | Description                      |
|-------------------------|-----------------|---------|----------------------------------|
| `Timeout`               | `time.Duration` | 60s     | Overall request timeout          |
| `DialTimeout`           | `time.Duration` | 30s     | TCP connection timeout           |
| `KeepAlive`             | `time.Duration` | 30s     | Keep-alive probe interval        |
| `TLSHandshakeTimeout`   | `time.Duration` | 10s     | TLS handshake timeout            |
| `ResponseHeaderTimeout` | `time.Duration` | 30s     | Response header timeout          |
| `IdleConnTimeout`       | `time.Duration` | 90s     | Idle connection timeout          |
| `MaxIdleConns`          | `int`           | 100     | Max idle connections (all hosts) |
| `MaxIdleConnsPerHost`   | `int`           | 10      | Max idle connections per host    |
| `MaxConnsPerHost`       | `int`           | 20      | Max connections per host         |

### Security Settings

| Field                   | Type          | Default | Description                          |
|-------------------------|---------------|---------|--------------------------------------|
| `MinTLSVersion`         | `uint16`      | TLS 1.2 | Minimum TLS version                  |
| `MaxTLSVersion`         | `uint16`      | TLS 1.3 | Maximum TLS version                  |
| `InsecureSkipVerify`    | `bool`        | false   | Skip TLS verification (⚠️ dangerous) |
| `MaxResponseBodySize`   | `int64`       | 50 MB   | Max response body size               |
| `MaxConcurrentRequests` | `int`         | 500     | Max concurrent requests              |
| `ValidateURL`           | `bool`        | true    | Validate URL format                  |
| `ValidateHeaders`       | `bool`        | true    | Validate headers (CRLF protection)   |
| `TLSConfig`             | `*tls.Config` | nil     | Custom TLS configuration             |

### Retry Settings

| Field           | Type            | Default | Description                |
|-----------------|-----------------|---------|----------------------------|
| `MaxRetries`    | `int`           | 2       | Maximum retry attempts     |
| `RetryDelay`    | `time.Duration` | 2s      | Initial retry delay        |
| `MaxRetryDelay` | `time.Duration` | 60s     | Maximum retry delay        |
| `BackoffFactor` | `float64`       | 2.0     | Exponential backoff factor |
| `Jitter`        | `bool`          | true    | Add jitter to retry delays |

### Feature Settings

| Field             | Type                | Default     | Description                      |
|-------------------|---------------------|-------------|----------------------------------|
| `UserAgent`       | `string`            | "httpc/1.0" | User-Agent header                |
| `FollowRedirects` | `bool`              | true        | Follow HTTP redirects            |
| `EnableHTTP2`     | `bool`              | true        | Enable HTTP/2                    |
| `Headers`         | `map[string]string` | nil         | Default headers for all requests |

## Best Practices

### ✅ DO

- Use `DefaultConfig()` for production
- Use `SecureConfig()` for compliance requirements
- Set appropriate timeouts for your use case
- Enable TLS 1.2+ in production
- Validate URLs and headers
- Use custom User-Agent to identify your app

### ❌ DON'T

- Use `InsecureSkipVerify` in production
- Set very long timeouts without good reason
- Disable header validation
- Use `TestingConfig()` for external APIs
- Set excessive connection limits without testing

## Examples

### Example 1: API Client with Custom Timeout

```go
config := httpc.DefaultConfig()
config.Timeout = 30 * time.Second
config.UserAgent = "MyApp/1.0"

client, err := httpc.New(config)
```

### Example 2: High-Throughput Client

```go
config := httpc.DefaultConfig()
config.MaxConcurrentRequests = 1000
config.MaxIdleConnsPerHost = 50
config.MaxConnsPerHost = 100

client, err := httpc.New(config)
```

### Example 3: Strict Security Client

```go
config := httpc.SecureConfig()
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS13,
    CipherSuites: []uint16{
        tls.TLS_AES_256_GCM_SHA384,
        tls.TLS_CHACHA20_POLY1305_SHA256,
    },
}

client, err := httpc.New(config)
```

## Related Documentation

- [Getting Started](getting-started.md) - Basic usage and first steps
- [Request Options](request-options.md) - Customizing requests
- [Security Guide](security.md) - Security features and TLS configuration
- [Best Practices](best-practices.md) - Production patterns

---

