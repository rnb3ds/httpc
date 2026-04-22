# Configuration

This guide covers all configuration options for HTTPC clients.

> **Prerequisite**: This guide assumes you understand the [Client Setup pattern](01_getting-started.md#common-patterns) from the Getting Started guide.

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

Timeouts:
- Timeouts.Request: 30 seconds
- Timeouts.Dial: 10 seconds
- Timeouts.TLSHandshake: 10 seconds
- Timeouts.ResponseHeader: 30 seconds
- Timeouts.IdleConn: 90 seconds

Connection:
- Connection.MaxIdleConns: 50
- Connection.MaxConnsPerHost: 10
- Connection.EnableHTTP2: true
- Connection.EnableCookies: false

Security:
- Security.MinTLSVersion: TLS 1.2
- Security.MaxTLSVersion: TLS 1.3
- Security.MaxResponseBodySize: 10 MB
- Security.AllowPrivateIPs: true
- Security.StrictContentLength: true

Retry:
- Retry.MaxRetries: 3
- Retry.Delay: 1 second
- Retry.BackoffFactor: 2.0

Middleware:
- Middleware.UserAgent: "httpc/1.0"
- Middleware.FollowRedirects: true
- Middleware.MaxRedirects: 10

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
- TLS: 1.2-1.3 (Security.InsecureSkipVerify: true)
- Timeouts.Request: 30 seconds
- Timeouts.Dial: 5 seconds
- Timeouts.TLSHandshake: 5 seconds
- Timeouts.ResponseHeader: 10 seconds
- Timeouts.IdleConn: 30 seconds
- Retry.MaxRetries: 1
- Retry.Delay: 100 milliseconds
- Retry.EnableJitter: false
- Connection.MaxIdleConns: 10
- Connection.MaxConnsPerHost: 5
- Security.MaxResponseBodySize: 10 MB
- Security.AllowPrivateIPs: true
- Security.ValidateURL: false
- Security.ValidateHeaders: false
- Connection.EnableHTTP2: false
- Connection.EnableCookies: true
- Middleware.UserAgent: "httpc-test/1.0"

**Use Cases:**
- Development environments
- Testing with localhost/127.0.0.1
- Local development
- Testing with self-signed certificates

**⚠️ WARNING:** This config disables security features and should NEVER be used in production.

### Balanced (Default)

For most applications:

```go
client, err := httpc.New(httpc.DefaultConfig())
// Or simply:
client, err := httpc.New()  // Uses balanced by default
```

**Settings:**
- TLS: 1.2-1.3 (modern security)
- Timeouts.Request: 30 seconds
- Retry.MaxRetries: 3
- Retry.Delay: 1 second
- Retry.BackoffFactor: 2.0
- Connection.MaxIdleConns: 50
- Connection.MaxConnsPerHost: 10
- Security.MaxResponseBodySize: 10 MB
- Security.AllowPrivateIPs: true
- Connection.EnableHTTP2: true
- Middleware.FollowRedirects: true
- Middleware.MaxRedirects: 10
- Connection.EnableCookies: false

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
- TLS: 1.2-1.3 (modern security)
- Timeouts.Request: 15 seconds
- Timeouts.Dial: 5 seconds
- Timeouts.TLSHandshake: 5 seconds
- Timeouts.ResponseHeader: 10 seconds
- Timeouts.IdleConn: 30 seconds
- Retry.MaxRetries: 1
- Retry.Delay: 2 seconds
- Retry.EnableJitter: true
- Connection.MaxIdleConns: 20
- Connection.MaxConnsPerHost: 5
- Security.MaxResponseBodySize: 5 MB
- Security.AllowPrivateIPs: false
- Security.ValidateURL: true
- Security.ValidateHeaders: true
- Connection.EnableHTTP2: true
- Middleware.FollowRedirects: false
- Connection.EnableCookies: false

**Use Cases:**
- Financial services (PCI DSS compliance)
- Healthcare (HIPAA compliance)
- Government systems (FIPS compliance)
- High-security environments
- Payment processing

**🔒 Security:** Maximum security with minimal attack surface.

## Custom Configuration

For fine-grained control, create a custom configuration:

```go
config := &httpc.Config{
    // Network settings
    Timeouts: httpc.TimeoutConfig{
        Request: 30 * time.Second,
    },
    Connection: httpc.ConnectionConfig{
        MaxIdleConns:    100,
        MaxConnsPerHost: 20,
        EnableHTTP2:     true,
        EnableCookies:   false,
    },

    // Security settings
    Security: httpc.SecurityConfig{
        MinTLSVersion:       tls.VersionTLS12,
        MaxTLSVersion:       tls.VersionTLS13,
        InsecureSkipVerify:  false,
        MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
        AllowPrivateIPs:     false,
        StrictContentLength: true,
    },

    // Retry settings
    Retry: httpc.RetryConfig{
        MaxRetries:    3,
        Delay:         1 * time.Second,
        BackoffFactor: 2.0,
    },

    // Headers and features
    Middleware: httpc.MiddlewareConfig{
        UserAgent:       "MyApp/1.0",
        FollowRedirects: true,
        Headers: map[string]string{
            "Accept": "application/json",
        },
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
config.Timeouts.Request = 30 * time.Second
config.Retry.MaxRetries = 3
config.Middleware.UserAgent = "MyApp/1.0"

client, err := httpc.New(config)
```

## TLS Configuration

### Basic TLS Configuration

```go
import "crypto/tls"

config := httpc.DefaultConfig()
config.Security.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    MaxVersion: tls.VersionTLS13,
}

client, err := httpc.New(config)
```

### Custom Cipher Suites

```go
config := httpc.DefaultConfig()
config.Security.TLSConfig = &tls.Config{
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
config.Security.TLSConfig = &tls.Config{
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
config.Security.TLSConfig = &tls.Config{
    RootCAs: caCertPool,
}

client, err := httpc.New(config)
```

### Skip TLS Verification (Testing Only)

```go
// WARNING: Only for testing! Never use in production!
config := httpc.DefaultConfig()
config.Security.InsecureSkipVerify = true

client, err := httpc.New(config)
```

## Configuration Reference

### Timeouts

| Field                        | Type            | Default | Description                      |
|------------------------------|-----------------|---------|----------------------------------|
| `Timeouts.Request`           | `time.Duration` | 30s     | Overall request timeout          |
| `Timeouts.Dial`              | `time.Duration` | 10s     | Dial timeout                     |
| `Timeouts.TLSHandshake`     | `time.Duration` | 10s     | TLS handshake timeout            |
| `Timeouts.ResponseHeader`    | `time.Duration` | 30s     | Response header timeout          |
| `Timeouts.IdleConn`          | `time.Duration` | 90s     | Idle connection timeout          |

### Connection

| Field                          | Type            | Default | Description                      |
|--------------------------------|-----------------|---------|----------------------------------|
| `Connection.MaxIdleConns`      | `int`           | 50      | Max idle connections (all hosts) |
| `Connection.MaxConnsPerHost`   | `int`           | 10      | Max connections per host         |
| `Connection.ProxyURL`          | `string`        | ""      | Proxy server URL                 |
| `Connection.EnableSystemProxy` | `bool`          | false   | Use system proxy settings        |
| `Connection.EnableHTTP2`       | `bool`          | true    | Enable HTTP/2                    |
| `Connection.EnableCookies`     | `bool`          | false   | Enable automatic cookie jar      |
| `Connection.EnableDoH`         | `bool`          | false   | Enable DNS-over-HTTPS resolution |
| `Connection.DoHCacheTTL`       | `time.Duration` | 5m      | DoH DNS cache TTL                |

### Security

| Field                           | Type          | Default | Description                        |
|---------------------------------|---------------|---------|------------------------------------|
| `Security.MinTLSVersion`        | `uint16`      | TLS 1.2 | Minimum TLS version                |
| `Security.MaxTLSVersion`        | `uint16`      | TLS 1.3 | Maximum TLS version                |
| `Security.InsecureSkipVerify`   | `bool`        | false   | Skip TLS verification (dangerous)  |
| `Security.MaxResponseBodySize`  | `int64`       | 10 MB   | Max response body size             |
| `Security.AllowPrivateIPs`      | `bool`        | true    | Allow private IP addresses         |
| `Security.StrictContentLength`  | `bool`        | true    | Enforce Content-Length validation  |
| `Security.TLSConfig`            | `*tls.Config` | nil     | Custom TLS configuration           |
| `Security.ValidateURL`          | `bool`        | true    | Enable URL validation              |
| `Security.ValidateHeaders`      | `bool`        | true    | Enable header validation (CRLF prevention) |
| `Security.CookieSecurity`       | `*validation.CookieSecurityConfig` | nil | Cookie security attribute validation |
| `Security.RedirectWhitelist`    | `[]string`    | nil     | Allowed domains for redirects      |

**Note:** URL and header validation are enabled by default for security.

### Retry

| Field                    | Type            | Default | Description                |
|--------------------------|-----------------|---------|----------------------------|
| `Retry.MaxRetries`       | `int`           | 3       | Maximum retry attempts     |
| `Retry.Delay`            | `time.Duration` | 1s      | Initial retry delay        |
| `Retry.BackoffFactor`    | `float64`       | 2.0     | Exponential backoff factor |
| `Retry.EnableJitter`     | `bool`          | true    | Enable jitter in retry delay |
| `Retry.CustomPolicy`     | `RetryPolicy`   | nil     | Custom retry logic override |

**Note:** MaxRetryDelay is calculated internally as `min(Delay * BackoffFactor * 3, 30s)`.

### Middleware

| Field                        | Type                | Default     | Description                      |
|------------------------------|---------------------|-------------|----------------------------------|
| `Middleware.Middlewares`     | `[]MiddlewareFunc`  | nil         | Middleware chain for request/response interception |
| `Middleware.UserAgent`       | `string`            | "httpc/1.0" | User-Agent header                |
| `Middleware.FollowRedirects` | `bool`              | true        | Follow HTTP redirects            |
| `Middleware.MaxRedirects`    | `int`               | 10          | Maximum redirects to follow      |
| `Middleware.Headers`         | `map[string]string` | nil         | Default headers for all requests |

## Best Practices

### DO

- Use `DefaultConfig()` for production
- Use `SecureConfig()` for compliance requirements
- Set appropriate timeouts for your use case
- Enable TLS 1.2+ in production
- Validate URLs and headers
- Use custom User-Agent to identify your app

### DON'T

- Use `Security.InsecureSkipVerify` in production
- Set very long timeouts without good reason
- Disable header validation
- Use `TestingConfig()` for external APIs
- Set excessive connection limits without testing

---

