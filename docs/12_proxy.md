# Proxy Configuration

Manual proxies, system-proxy detection, proxy pools with rotation and circuit
breaking, and per-request rotation. For a quick introduction see the README
section "Proxy Configuration".

## Table of Contents

- [Manual & System Proxy](#manual--system-proxy)
- [Proxy Pool (Rotation + Circuit Breaking)](#proxy-pool-rotation--circuit-breaking)
- [Per-Request Proxy Rotation](#per-request-proxy-rotation)
- [HTTP/2 with a Proxy Pool](#http2-with-a-proxy-pool)
- [Which Proxy Served a Request?](#which-proxy-served-a-request)

## Manual & System Proxy

```go
// Manual proxy
config := httpc.DefaultConfig()
config.Connection.ProxyURL = "http://127.0.0.1:8080"
// Or HTTPS proxy: "https://proxy.example.com:8443"
// Or SOCKS: "socks5://host:1080" / "socks5h://host:1080"

// System proxy auto-detection (Windows/macOS/Linux)
config := httpc.DefaultConfig()
config.Connection.EnableSystemProxy = true // reads env vars + platform settings
```

## Proxy Pool (Rotation + Circuit Breaking)

Distribute requests across multiple proxies with automatic failover and
status-based rotation:

```go
config := httpc.DefaultConfig()
config.Connection.ProxyPool = []string{
    "http://proxy1.example.com:7070",
    "http://proxy2.example.com:8080",
    "socks5://proxy3.example.com:1080",
}
// Strategy: ProxyStrategyRoundRobin (default) or ProxyStrategyRandom
config.Connection.ProxyPoolStrategy = httpc.ProxyStrategyRoundRobin

// Circuit breaking: after 5 consecutive connection failures, skip the proxy
// for 60s, then retry it (half-open probe). Defaults: threshold=3, cooldown=30s.
config.Connection.ProxyFailureThreshold = 5
config.Connection.ProxyCooldown = 60 * time.Second

// Rotate proxy on 403 (IP-based blocking). The request is retried
// through a different proxy IP. Requires Retry.MaxRetries > 0.
// Unlike connection failures, status-based rotation does NOT circuit-break
// the proxy — blocks are often target-specific.
config.Connection.ProxyRotateOnStatus = []int{403}
config.Retry.MaxRetries = 3
```

## Per-Request Proxy Rotation

Connection reuse normally makes consecutive requests to the same host reuse the
previous proxy tunnel, bypassing pool selection. `ProxyRotatePerRequest` closes
idle connections before each request so every `Get`/`Post` picks a fresh proxy:

```go
config.Connection.ProxyRotatePerRequest = true
```

Adds overhead (no connection reuse); essential for scraping and
fingerprint-rotation use cases. Requires `ProxyPool`; no effect with `ProxyURL`.

## HTTP/2 with a Proxy Pool

By default a multi-proxy pool runs on **HTTP/1**: the standard library's
HTTP/2 connection pool is keyed by target host only (not by which proxy
carried the tunnel), so with HTTP/2 enabled it would pin every request to
whichever proxy connected first — silently bypassing rotation and load
balancing.

`EnableProxyPoolHTTP2` opts into per-proxy transports instead: each proxy
gets its own transport and HTTP/2 pool behind a per-request dispatcher, so
rotation behaves exactly as on HTTP/1 while concurrent requests through the
**same** proxy multiplex over one HTTP/2 tunnel:

```go
config.Connection.ProxyPool = proxies
config.Connection.EnableProxyPoolHTTP2 = true // requires EnableHTTP2 (default)
```

When to enable it:

- **Stable pool + high concurrency to the same host** — the multiplexing win.
  Local benchmark (4 proxies, 50 concurrent requests, warm pools): p50
  1.9 ms → 0.6 ms, p99 6.1 ms → 2.3 ms, new tunnels per request
  0.12 → 0 (each avoided tunnel saves a CONNECT + TLS handshake ≈ 1+ RTT
  through a real proxy).
- Leave it **off** when requests must not be correlatable: requests routed
  to the same proxy share that proxy's connections, including concurrent
  ones. Note that `ProxyRotatePerRequest` does **not** prevent this —
  rotation indexes repeat every pool-size requests and its idle-connection
  drain only reaches connections that are no longer in flight.

Requires `EnableHTTP2` (the combination is rejected by `ValidateConfig`).
Connection limits (`MaxConnsPerHost`, `MaxIdleConnsPerHost`) apply per proxy
transport (process-wide ceiling = limit × pool size); `MaxTotalConns` still
caps all dials globally. Cold bursts after idle timeouts open ~one
connection per concurrent request in both modes — the win applies once
pools are warm. With `Security.TLSFingerprint` the fingerprint transport's
own per-proxy backends apply and this option is ignored.

## Which Proxy Served a Request?

Every `Result` reports the proxy that produced its final response (empty string
= direct, or a dynamically detected system proxy — `EnableSystemProxy` does
not record the proxy it used):

```go
result, _ := client.Get("https://httpbin.org/ip")
fmt.Println(result.Meta.ProxyURL) // e.g. "http://proxy2.example.com:8080"
```

With retries, `ProxyURL` reflects the attempt that returned the response
(`Meta.Attempts` holds the count).

**Priority:** `ProxyURL` > `ProxyPool` > `EnableSystemProxy` > direct.

For package-level per-call routing (`WithProxy` / `WithSystemProxy`) see
[03_request-options.md](03_request-options.md) § Package-Level Routing Options;
for the browser-fingerprint transport see
[10_tls-fingerprinting.md](10_tls-fingerprinting.md).
