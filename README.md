# HTTPC - Secure HTTP Client for Go

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://golang.org)
[![Release](https://img.shields.io/github/v/release/cybergodev/httpc)](https://github.com/cybergodev/httpc/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Dependencies](https://img.shields.io/badge/deps-minimal-brightgreen.svg)](go.mod)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-%E2%9C%93-brightgreen.svg)](docs/09_concurrency-safety.md)

A fast, secure HTTP client library for Go with sensible defaults, a lean dependency set, and built-in resilience.

**[中文文档](README_zh-CN.md)** | **[www.cybergo.dev/httpc](https://www.cybergo.dev/httpc)**

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [HTTP Methods](#http-methods)
- [Request Options](#request-options)
- [Response Handling](#response-handling)
- [Context & Cancellation](#context--cancellation)
- [File Download](#file-download)
- [Streaming Bodies](#streaming-bodies)
- [Domain Client (Session Management)](#domain-client-session-management)
- [Configuration](#configuration)
- [Middleware](#middleware)
- [Proxy Configuration](#proxy-configuration)
- [Security Features](#security-features)
- [Browser Transport Fingerprinting](#browser-transport-fingerprinting)
- [Error Handling](#error-handling)
- [Concurrency Safety](#concurrency-safety)
- [Interfaces](#interfaces)
- [Documentation](#documentation)
- [License](#license)

---

## Features

| Feature | Description |
|---------|-------------|
| **Secure by Default** | TLS 1.2+, SSRF protection, CRLF injection prevention, path traversal blocking |
| **High Performance** | Connection pooling, HTTP/2, goroutine-safe, `sync.Pool` optimization |
| **Built-in Resilience** | Smart retry with exponential backoff and jitter — non-idempotent requests (POST/PATCH) execute exactly once by default |
| **Automatic Decompression** | Transparent gzip/deflate handling with zip-bomb protection |
| **Developer Friendly** | Clean API, intuitive options pattern, comprehensive documentation |
| **Dependencies** | 7 direct modules — `golang.org/x/net` + `golang.org/x/sys` for the standard path, plus brotli / compress / uTLS / tls-client / fhttp pulled in only by `Security.TLSFingerprint` |
| **Cookie Management** | Full cookie jar support with security validation |
| **File Operations** | Secure file download with progress tracking and resume support |
| **Streaming** | `io.Reader` request bodies, zero-copy `io.Pipe` uploads, disk-streaming downloads |

---

## Installation

```bash
go get -u github.com/cybergodev/httpc
```

```go
import "github.com/cybergodev/httpc"
```

**Requirements:** Go 1.27+

---

## Quick Start

```go
// Package-level function — no client setup needed
result, err := httpc.Get("https://httpbin.org/get")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Status: %d, Duration: %v\n", result.StatusCode(), result.Meta.Duration)
```

For anything beyond one-off requests, create a reusable client (recommended):

```go
client, err := httpc.NewDefault()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

result, err := client.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQuery("limit", 20),
)
```

Full runnable programs: [examples/01_basic_usage.go](examples/01_basic_usage.go) ·
[docs/01_getting-started.md](docs/01_getting-started.md).

---

## HTTP Methods

```go
// GET with query parameters
result, _ := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQueryMap(map[string]any{"limit": 20}),
)

// POST JSON body
result, _ := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(map[string]string{"name": "John"}),
)

// PUT / PATCH / DELETE
result, _ := httpc.Put(url, httpc.WithJSON(data))
result, _ := httpc.Patch(url, httpc.WithJSON(partialData))
result, _ := httpc.Delete(url)

// HEAD / OPTIONS
result, _ := httpc.Head(url)
result, _ := httpc.Options(url)

// Generic request with custom method
result, _ := httpc.Request(ctx, "PROPFIND", url)
```

---

## Request Options

Every request accepts `With*` options — headers, auth, query, body, cookies,
timeouts, retries, redirects, SSRF overrides, callbacks, and package-level
proxy/fingerprint routing:

| Category | Options |
|----------|---------|
| **Headers** | `WithHeader(key, value)`, `WithHeaderMap(map)`, `WithUserAgent(ua)` |
| **Auth** | `WithBearerToken(token)`, `WithBasicAuth(user, pass)` |
| **Query** | `WithQuery(key, value)`, `WithQueryMap(map)` |
| **Body** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithFormData(*FormData)`, `WithFile(field, filename, content)`, `WithBody(data, ...BodyKind)`, `WithBinary([]byte, ...contentType)`, `WithStreamBody(bool)` |
| **Cookies** | `WithCookies([]http.Cookie)`, `WithCookieMap(map)`, `WithCookieString("a=1; b=2")`, `WithSecureCookie(config)` |
| **Control** | `WithTimeout(dur)`, `WithNoTimeout()`, `WithMaxRetries(n)`, `WithRetryNonIdempotent(bool)`, `WithContext(ctx)`, `WithAllowPrivateIPs(bool)` |
| **Redirects** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **Routing** | `WithProxy(url)`, `WithSystemProxy()`, `WithTLSFingerprint(preset)` — package-level functions only |
| **Callbacks** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

Details and examples for every option:
[docs/03_request-options.md](docs/03_request-options.md).

> **Package-level routing**: `WithProxy` / `WithSystemProxy` /
> `WithTLSFingerprint` route a single package-level call to a dedicated cached
> client (isolated pools, inheriting the default client's secure defaults);
> close them all with `httpc.CloseRoutedClients()`. On a client instance they
> return an error — use `Config.Connection.*` / `Security.TLSFingerprint` there.

---

## Response Handling

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// Result.Request / Result.Response / Result.Meta carry the full request,
// response, and metadata (duration, attempts, proxy, redirect chain).

fmt.Println(result.StatusCode()) // 200
fmt.Println(result.Body())       // response body (string)

if result.IsSuccess() { }        // 2xx; also IsRedirect / IsClientError / IsServerError

var data map[string]any
if err := result.Unmarshal(&data); err != nil { // parse JSON
    log.Fatal(err)
}

fmt.Println(result.Meta.ProxyURL) // serving proxy; "" = direct (system proxy not recorded)
fmt.Println(result.String())      // safe for logging — masks sensitive headers
```

### Automatic Decompression

`gzip` / `deflate` responses are decompressed transparently (a
decompression-bomb guard caps the size at `Security.MaxDecompressedBodySize`,
default 100 MB). Any other `Content-Encoding` (e.g. `br`, `zstd`, or a list
like `gzip, br`) is rejected with an error rather than returned as raw bytes —
normally only reachable by setting `Accept-Encoding` manually.

Full inspection API (cookies, `SaveToFile`, redirect chain, request
metadata): [docs/08_request-inspection.md](docs/08_request-inspection.md).

---

## Context & Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := httpc.Get("https://api.example.com", httpc.WithContext(ctx))
if errors.Is(err, context.DeadlineExceeded) {
    fmt.Println("Request timed out")
}
```

---

## File Download

`Download` streams to disk with built-in protections (UNC/system path
blocking, path traversal detection) plus progress, resume, and checksum
verification:

```go
opts := httpc.DefaultDownloadConfig()
opts.FilePath = "downloads/large.zip"
opts.ResumeDownload = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    fmt.Printf("\r%.1f%% - %s/s", float64(downloaded)/float64(total)*100, httpc.FormatSpeed(speed))
}
result, err := httpc.Download(ctx, "https://example.com/large.zip", opts)
```

`Download` takes a `context.Context` directly and also accepts request options:
`httpc.Download(ctx, url, opts, httpc.WithBearerToken(...))`. Config and
result fields: [docs/07_file-download.md](docs/07_file-download.md).

---

## Streaming Bodies

**Upload** — `WithBody` accepts any `io.Reader` (set Content-Type explicitly;
`io.Pipe` gives zero-copy streaming):

```go
result, err := httpc.Post("https://example.com/upload",
    httpc.WithBody(file),
    httpc.WithHeader("Content-Type", "application/octet-stream"),
)
```

> **Security:** `WithBody(io.Reader)` bypasses request-body-size validation —
> wrap untrusted readers with `io.LimitReader`.

**Download** — `Download()` streams the response to disk and never buffers it.
Regular `Get` / `Post` always buffer the full response; passing
`WithStreamBody(true)` to them returns `ErrStreamBodyRequiresDownload`.

Details: [docs/03_request-options.md](docs/03_request-options.md) § Streaming
Request Bodies · [docs/07_file-download.md](docs/07_file-download.md).

---

## Domain Client (Session Management)

Multiple requests to the same domain with automatic cookie and header
management:

```go
client, _ := httpc.NewDomainDefault("https://api.example.com")
defer client.Close()

client.Post("/login", httpc.WithJSON(credentials)) // server sets cookies
client.SetHeader("Authorization", "Bearer "+token) // persistent header

profile, _ := client.Get("/profile") // cookies + headers sent automatically
```

All `Client` methods work with relative paths, including `Download`. The
underlying `SessionManager` (thread-safe cookie/header store) is also available
standalone: [docs/06_cookie-api.md](docs/06_cookie-api.md).

---

## Configuration

```go
// Recommended defaults
client, _ := httpc.NewDefault()

// Maximum security (SSRF protection enabled)
client, _ := httpc.New(httpc.SecureConfig())

// High throughput / lightweight / testing only
client, _ := httpc.New(httpc.PerformanceConfig())
client, _ := httpc.New(httpc.MinimalConfig())
client, _ := httpc.New(httpc.TestingConfig()) // disables security features!
```

```go
config := httpc.DefaultConfig()
config.Timeouts.Request = 30 * time.Second
config.Connection.MaxIdleConns = 100
config.Security.MaxResponseBodySize = 50 * 1024 * 1024
config.Retry.MaxRetries = 3
if err := config.Validate(); err != nil { // New() also validates internally
    log.Fatal(err)
}
client, _ := httpc.New(config)
fmt.Println(config.String()) // sensitive values masked
```

Every field of `Timeouts` / `Connection` / `Security` / `Retry` /
`Middleware` / `Defaults` — types, defaults, behavior notes (proxy pool, DoH,
fingerprint, pinning, SSRF exemptions, redirect limits):
[docs/02_configuration.md](docs/02_configuration.md).

---

## Middleware

Composable request middleware: logging, panic recovery, request IDs, timeouts,
static headers, metrics, and security auditing:

```go
config := httpc.DefaultConfig()
config.Middleware.Middlewares = []httpc.MiddlewareFunc{
    httpc.Chain(
        httpc.RecoveryMiddleware(),
        httpc.LoggingMiddleware(&httpc.LoggingConfig{LogFunc: log.Printf}),
        httpc.RequestIDMiddleware(nil),
        httpc.AuditMiddleware(nil), // nil = default audit config
    ),
}
```

Every configurable middleware follows the same pattern (`XxxConfig` +
`DefaultXxxConfig()` + `XxxMiddleware(cfg)`; pass `nil` for defaults); custom
middleware is a function over `httpc.Handler`. Full guide incl. `AuditEvent`
fields: [docs/11_middleware.md](docs/11_middleware.md).

---

## Proxy Configuration

```go
config := httpc.DefaultConfig()
config.Connection.ProxyURL = "http://127.0.0.1:8080" // http/https/socks5/socks5h
// or: config.Connection.EnableSystemProxy = true    // env vars + platform settings
```

Proxy pools add rotation, circuit breaking, and status-based rotation:

```go
config.Connection.ProxyPool = []string{
    "http://proxy1.example.com:7070",
    "socks5://proxy2.example.com:1080",
}
config.Connection.ProxyRotateOnStatus = []int{403} // retry 403s via a different proxy
config.Retry.MaxRetries = 3
```

Each `Result` reports the proxy that served it (`result.Meta.ProxyURL`; empty
string = direct — the dynamic system proxy from `EnableSystemProxy` is not
recorded). **Priority:** `ProxyURL` > `ProxyPool` > `EnableSystemProxy` >
direct. Pools, strategies, circuit breaking, per-request rotation, and
per-proxy HTTP/2 (`EnableProxyPoolHTTP2`):
[docs/12_proxy.md](docs/12_proxy.md).

---

## Security Features

| Feature | Description |
|---------|-------------|
| **TLS 1.2+** | Modern encryption standards by default |
| **Certificate Pinning** | Defense against MITM even with a compromised CA |
| **SSRF Protection** | Two-layer DNS validation blocks private IPs |
| **CRLF Injection Prevention** | Header and URL validation |
| **Path Traversal Protection** | Safe file operations |
| **Domain Whitelist** | Restrict redirects to allowed domains |
| **Response Size Limit** | Configurable limit to prevent memory exhaustion |

### SSRF Protection

Private/reserved IPs are blocked by default (`Security.AllowPrivateIPs` is
`false`). Widen access granularly:

```go
cfg := httpc.DefaultConfig()
cfg.Security.SSRFExemptCIDRs = []string{"10.0.0.0/8"} // exempt CIDRs (e.g. VPN/VPC)
client, _ := httpc.New(cfg)

// Or opt in for a single trusted internal URL:
result, err := httpc.Get("http://localhost:8080/health",
    httpc.WithAllowPrivateIPs(true))
```

Through a proxy, httpc resolves and validates the target hostname locally
before the proxy carries the request. Two caveats: it is **fail-closed on
local DNS** (unresolvable hostnames are rejected even if the proxy could
resolve them — exempt them via `SSRFExemptCIDRs`) and carries **no
DNS-rebinding guarantee** (the proxy performs its own second resolution).

### Certificate Pinning

Rejects the TLS handshake unless the server presents a pinned public key —
MITM defense that holds even when a trusted CA is compromised:

```go
pinner, err := httpc.NewSPKIHashPinner(
    "YLh1dUR9y6Kja30RrAn7JKnbQG/uEtLMkBgFF2fuihg=", // current key
    "C5+lpZ7tcVwmwQIMcRtPbsQtWLABXhQzejna0wHFr8M=", // backup key (rotation)
)
cfg := httpc.DefaultConfig()
cfg.Security.CertificatePinner = pinner
client, err := httpc.New(cfg)
```

Pinning also works with `Security.TLSFingerprint`; with `InsecureSkipVerify`
set, the pinner becomes the only verification gate (pins the leaf certificate).

### Security Warning Output

Insecure configurations (`TestingConfig()`, `InsecureSkipVerify: true`) print
a warning to stderr by default. Redirect or suppress it:
`httpc.SetSecurityWarnOutput(io.Discard)`.

---

## Browser Transport Fingerprinting

Some sites fingerprint more than the User-Agent — the TLS ClientHello (JA3/JA4),
HTTP/2 framing, and the ordered header block. Setting `Security.TLSFingerprint`
replaces httpc's transport with a browser emulation covering all of them at once,
over fully fingerprinted HTTP/2 (preferred) and HTTP/1.1 (fallback); redirects,
retries, middleware, streaming downloads, and cookies keep their httpc semantics:

```go
cfg := httpc.DefaultConfig()
cfg.Security.TLSFingerprint = "chrome-latest" // or "chrome-windows", "firefox-latest", ...
client, err := httpc.New(cfg)
```

**Presets**: `chrome` / `chrome-latest` / `chrome-155` and `firefox` /
`firefox-latest` / `firefox-158`, each with `*-windows` / `*-macos` / `*-linux`
desktop and `*-android` / `*-ios` mobile variants (e.g. `chrome-windows`,
`firefox-ios`), plus `safari-ios` (= `ios-safari-latest`) and the retired-major
pins `chrome-152` / `firefox-148`. No desktop-Safari profile exists (no verified
data yet). Leave `TLSFingerprint` empty for the standard stack with no extra
dependencies.

Key behaviors — full details in [docs/10_tls-fingerprinting.md](docs/10_tls-fingerprinting.md):

- **Header set is authoritative**: the preset replaces same-named headers
  (User-Agent included); custom headers pass through untouched.
- **ALPN decides the protocol**: HTTP/2 when the server speaks it, HTTP/1.1
  otherwise, never replayed; HTTP/3 is not attempted (QUIC would bypass
  certificate pinning).
- **Proxies keep working**: pools, rotation, and circuit breaking
  (HTTP/HTTPS/SOCKS5); HTTPS proxies are certificate-verified like any TLS peer.
- **Certificate pinning works** on every fingerprinted handshake;
  `Security.TLSConfig` is mutually exclusive with fingerprinting.
- **SSRF protection** keeps the same dial-layer guarantee on direct connections.
- **Scope**: passive transport observations only — no browser JavaScript or
  behavioral signals. Presets are verified end-to-end
  (`internal/fingerprint/e2e_network_test.go`); a weekly CI job watches for drift.

---

## Error Handling

```go
result, err := httpc.Get(url)
if err != nil {
    var clientErr *httpc.ClientError
    if errors.As(err, &clientErr) {
        fmt.Printf("Error: %s (code: %s), retryable: %v\n",
            clientErr.Message, clientErr.Code(), clientErr.IsRetryable())
    }
    return err
}
```

`ClientError` carries `Type` / `Message` / `Cause` (unwrap with `%w`) / `URL` /
`Method` / `Attempts` / `StatusCode`. Common types: `ErrorTypeNetwork`,
`ErrorTypeTimeout`, `ErrorTypeTLS`, `ErrorTypeDNS`, `ErrorTypeHTTP`; retries
exhaust with the last error's type kept — check `Attempts` / `IsRetryable()`.
Sentinels for `errors.Is` include `ErrClientClosed`, `ErrFileExists`,
`ErrStreamBodyRequiresDownload` (use `Download`), `ErrProxyConnectionFailed`,
and the `ErrInvalid*` config-validation family. Full reference:
[docs/04_error-handling.md](docs/04_error-handling.md).

---

## Concurrency Safety

httpc clients are safe for concurrent use:

```go
client, _ := httpc.NewDefault()
defer client.Close()

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        result, _ := client.Get("https://api.example.com")
        // ...
    }()
}
wg.Wait()
```

- All `Client` methods are safe for concurrent use; package-level functions
  share a default client (customize with `SetDefaultClient`, reset with
  `CloseDefaultClient`)
- `Result` objects are immutable once returned — concurrent reads are safe
- `WithProxy` / `WithSystemProxy` / `WithTLSFingerprint` create independent
  routed clients, reclaimed via `CloseRoutedClients()`

Details: [docs/09_concurrency-safety.md](docs/09_concurrency-safety.md).

---

## Interfaces

Interfaces for testing and extensibility — use them to mock HTTP calls:

```go
type Doer interface {
    Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error)
}
```

`Client` extends `Doer` with the per-method convenience functions plus
`Download` and `Close`; `DomainClienter` adds the domain-session surface.
Embed for forward-compatible mocks:

```go
type MockClient struct {
    httpc.Client // embed for forward compatibility
}

func (m *MockClient) Get(url string, options ...httpc.RequestOption) (*httpc.Result, error) {
    return &httpc.Result{
        Response: &httpc.ResponseInfo{StatusCode: 200, Body: `{"ok": true}`},
    }, nil
}
```

---

## Documentation

| Resource | Description |
|----------|-------------|
| [Getting Started](docs/01_getting-started.md) | Installation and first steps |
| [Configuration](docs/02_configuration.md) | Client configuration and presets |
| [Request Options](docs/03_request-options.md) | Complete options reference |
| [Error Handling](docs/04_error-handling.md) | Error handling patterns |
| [HTTP Redirects](docs/05_redirects.md) | Redirect handling and tracking |
| [Cookie API](docs/06_cookie-api.md) | Cookie management |
| [File Download](docs/07_file-download.md) | File download with progress |
| [Request Inspection](docs/08_request-inspection.md) | Request/response inspection |
| [Concurrency Safety](docs/09_concurrency-safety.md) | Thread safety guarantees |
| [TLS Fingerprinting](docs/10_tls-fingerprinting.md) | Browser transport fingerprinting in depth |
| [Middleware](docs/11_middleware.md) | Built-in and custom middleware |
| [Proxy](docs/12_proxy.md) | Pools, rotation, circuit breaking |
| [Security](SECURITY.md) | Security features and best practices |

### Example Code

24 runnable examples, ordered from basic to advanced. Each is a standalone
`package main` behind a `//go:build examples` tag (kept out of the normal
build); run them one at a time:

```bash
go run -tags examples examples/01_basic_usage.go
```

| Category | Examples |
|----------|----------|
| **Basics** | [01_basic_usage](examples/01_basic_usage.go), [02_http_methods](examples/02_http_methods.go), [03_response_handling](examples/03_response_handling.go), [04_compression](examples/04_compression.go) |
| **Core Features** | [05_request_options](examples/05_request_options.go), [06_error_handling](examples/06_error_handling.go), [07_timeout_retry](examples/07_timeout_retry.go), [08_client_configuration](examples/08_client_configuration.go), [09_redirects](examples/09_redirects.go), [10_cookies_advanced](examples/10_cookies_advanced.go) |
| **Stateful Clients** | [11_session](examples/11_session.go), [12_domain_client](examples/12_domain_client.go), [13_proxy_configuration](examples/13_proxy_configuration.go), [14_doh](examples/14_doh.go) |
| **Advanced** | [15_middleware](examples/15_middleware.go), [16_concurrent_requests](examples/16_concurrent_requests.go), [17_file_operations](examples/17_file_operations.go), [18_rest_api_client](examples/18_rest_api_client.go), [19_advanced_patterns](examples/19_advanced_patterns.go) |
| **Security** | [20_certificate_pinning](examples/20_certificate_pinning.go), [21_ssrf_protection](examples/21_ssrf_protection.go) |
| **Streaming** | [22_streaming_bodies](examples/22_streaming_bodies.go) |
| **Fingerprinting** | [23_tls_fingerprint](examples/23_tls_fingerprint.go) |
| **Package-Level Routing** | [24_package_routing](examples/24_package_routing.go) |

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

If this project helps you, please give it a Star!
