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
- MinTLSVersion: TLS 1.2
- MaxTLSVersion: TLS 1.3
- HTTP/2: Enabled
- FollowRedirects: true
- MaxRedirects: 10
- EnableCookies: false
- AllowPrivateIPs: true
- StrictContentLength: true
- UserAgent: "httpc/1.0"

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
- TLS: 1.2-1.3 (InsecureSkipVerify: true)
- Timeout: 30 seconds
- MaxRetries: 1
- RetryDelay: 100 milliseconds
- MaxIdleConns: 10
- MaxConnsPerHost: 5
- MaxResponseBodySize: 10 MB
- AllowPrivateIPs: true
- HTTP/2: Disabled
- EnableCookies: true
- UserAgent: "httpc-test/1.0"

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
- Timeout: 30 seconds
- MaxRetries: 3
- RetryDelay: 1 second
- BackoffFactor: 2.0
- MaxIdleConns: 50
- MaxConnsPerHost: 10
- MaxResponseBodySize: 10 MB
- AllowPrivateIPs: true
- HTTP/2: Enabled
- FollowRedirects: true
- MaxRedirects: 10
- EnableCookies: false

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
- Timeout: 15 seconds
- MaxRetries: 1
- RetryDelay: 2 seconds
- MaxIdleConns: 20
- MaxConnsPerHost: 5
- MaxResponseBodySize: 5 MB
- AllowPrivateIPs: false
- HTTP/2: Enabled
- FollowRedirects: false
- EnableCookies: false

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
    Timeout:         30 * time.Second,
    MaxIdleConns:    100,
    MaxConnsPerHost: 20,

    // Security settings
    MinTLSVersion:       tls.VersionTLS12,
    MaxTLSVersion:       tls.VersionTLS13,
    InsecureSkipVerify:  false,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
    AllowPrivateIPs:     false,
    StrictContentLength: true,

    // Retry settings
    MaxRetries:    3,
    RetryDelay:    1 * time.Second,
    BackoffFactor: 2.0,

    // Headers and features
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    EnableHTTP2:     true,
    EnableCookies:   false,
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
| `Timeout`               | `time.Duration` | 30s     | Overall request timeout          |
| `MaxIdleConns`          | `int`           | 50      | Max idle connections (all hosts) |
| `MaxConnsPerHost`       | `int`           | 10      | Max connections per host         |

**Note:** Advanced timeout settings (DialTimeout, KeepAlive, TLSHandshakeTimeout, etc.) are managed internally and not exposed in the public Config.

### Security Settings

| Field                   | Type          | Default | Description                        |
|-------------------------|---------------|---------|------------------------------------|
| `MinTLSVersion`         | `uint16`      | TLS 1.2 | Minimum TLS version                |
| `MaxTLSVersion`         | `uint16`      | TLS 1.3 | Maximum TLS version                |
| `InsecureSkipVerify`    | `bool`        | false   | Skip TLS verification (dangerous)  |
| `MaxResponseBodySize`   | `int64`       | 10 MB   | Max response body size             |
| `AllowPrivateIPs`       | `bool`        | false   | Allow private IP addresses         |
| `StrictContentLength`   | `bool`        | true    | Enforce Content-Length validation  |
| `TLSConfig`             | `*tls.Config` | nil     | Custom TLS configuration           |

**Note:** URL and header validation are always enabled internally for security.

### Retry Settings

| Field           | Type            | Default | Description                |
|-----------------|-----------------|---------|----------------------------|
| `MaxRetries`    | `int`           | 3       | Maximum retry attempts     |
| `RetryDelay`    | `time.Duration` | 1s      | Initial retry delay        |
| `BackoffFactor` | `float64`       | 2.0     | Exponential backoff factor |

**Note:** MaxRetryDelay and Jitter are calculated internally based on RetryDelay and BackoffFactor.

### Feature Settings

| Field             | Type                | Default     | Description                      |
|-------------------|---------------------|-------------|----------------------------------|
| `UserAgent`       | `string`            | "httpc/1.0" | User-Agent header                |
| `FollowRedirects` | `bool`              | true        | Follow HTTP redirects            |
| `MaxRedirects`    | `int`               | 10          | Maximum redirects to follow      |
| `EnableHTTP2`     | `bool`              | true        | Enable HTTP/2                    |
| `EnableCookies`   | `bool`              | false       | Enable automatic cookie jar      |
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

---

