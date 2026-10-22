# Middleware

How to use httpc's middleware chain: built-ins, security auditing, chaining,
and custom middleware. For a quick introduction see the README section
"Middleware".

## Table of Contents

- [Pattern](#pattern)
- [Built-in Middleware](#built-in-middleware)
- [AuditEvent](#auditevent)
- [Chaining](#chaining)
- [Custom Middleware](#custom-middleware)

## Pattern

Every configurable middleware follows the same pattern: a `XxxConfig` struct,
a `DefaultXxxConfig()` constructor, and a `XxxMiddleware(cfg)` factory.
Pass `nil` to accept defaults. Register middleware through
`Config.Middleware.Middlewares`.

## Built-in Middleware

```go
// Request logging
httpc.LoggingMiddleware(&httpc.LoggingConfig{LogFunc: log.Printf})

// Panic recovery (no config needed)
httpc.RecoveryMiddleware()

// Request ID (nil config = defaults: "X-Request-ID" header, crypto/rand generator)
httpc.RequestIDMiddleware(&httpc.RequestIDConfig{HeaderName: "X-Request-ID"})

// Timeout enforcement
httpc.TimeoutMiddleware(&httpc.TimeoutMiddlewareConfig{Duration: 30 * time.Second})

// Static headers
httpc.HeaderMiddleware(&httpc.HeaderConfig{
    Headers: map[string]string{"X-App-Version": "1.0.0"},
})

// Metrics collection
httpc.MetricsMiddleware(&httpc.MetricsConfig{
    OnMetrics: func(method, url string, statusCode int, duration time.Duration, err error) {
        metrics.Record(method, url, statusCode, duration)
    },
})

// Security audit
auditCfg := httpc.DefaultAuditConfig()
auditCfg.OnAudit = func(a httpc.AuditEvent) {
    log.Printf("[AUDIT] %s %s -> %d (%v)", a.Method, a.URL, a.StatusCode, a.Duration)
}
httpc.AuditMiddleware(auditCfg)

// Audit with custom config (include request/response headers in the event)
auditCfgJSON := httpc.DefaultAuditConfig()
auditCfgJSON.OnAudit = func(a httpc.AuditEvent) { log.Printf("[AUDIT] %v", a) }
auditCfgJSON.IncludeHeaders = true
httpc.AuditMiddleware(auditCfgJSON)
```

## AuditEvent

| Field | Type | Description |
|-------|------|-------------|
| `Timestamp` | `time.Time` | Request timestamp |
| `Method` | `string` | HTTP method |
| `URL` | `string` | Request URL |
| `StatusCode` | `int` | Response status code |
| `Duration` | `time.Duration` | Request duration |
| `Attempts` | `int` | Retry attempts |
| `Error` | `error` | Error if any |
| `SourceIP` | `string` | Source IP (set via context key `SourceIPKey`) |
| `UserID` | `string` | User ID (set via context key `UserIDKey`) |
| `RedirectChain` | `[]string` | Redirect URLs |
| `ReqHeaders` | `map[string][]string` | Request headers (when `IncludeHeaders: true`) |
| `RespHeaders` | `map[string][]string` | Response headers (when `IncludeHeaders: true`) |

`AuditEvent` supports JSON serialization via `MarshalJSON()`.

## Chaining

```go
chainedMiddleware := httpc.Chain(
    httpc.RecoveryMiddleware(),
    httpc.LoggingMiddleware(&httpc.LoggingConfig{LogFunc: log.Printf}),
    httpc.RequestIDMiddleware(nil),
    httpc.HeaderMiddleware(&httpc.HeaderConfig{Headers: map[string]string{"X-App": "v1"}}),
)
config.Middleware.Middlewares = []httpc.MiddlewareFunc{chainedMiddleware}
```

## Custom Middleware

A middleware is a function wrapping the `httpc.Handler` — it may inspect or
mutate the request before the call and the response after it:

```go
func CustomMiddleware() httpc.MiddlewareFunc {
    return func(next httpc.Handler) httpc.Handler {
        return func(ctx context.Context, req httpc.RequestMutator) (httpc.ResponseMutator, error) {
            // Before request
            req.SetHeader("X-Custom", "value")

            // Call next handler
            resp, err := next(ctx, req)

            // After response
            return resp, err
        }
    }
}
```
