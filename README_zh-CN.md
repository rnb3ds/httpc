# HTTPC - 生产级 Go HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Zero Deps](https://img.shields.io/badge/deps-zero-brightgreen.svg)](go.mod)

一个高性能的 Go HTTP 客户端库，具备企业级安全性、零外部依赖，以及生产就绪的默认配置。

**[English document](README.md)**

---

## ✨ 特性

| 特性 | 描述 |
|------|------|
| 🔒 **默认安全** | TLS 1.2+、SSRF 防护、CRLF 注入防护、路径遍历阻断 |
| ⚡ **高性能** | 连接池、HTTP/2、goroutine 安全、sync.Pool 优化 |
| 🔄 **内置弹性** | 智能重试，支持指数退避和抖动 |
| 🛠️ **开发者友好** | 简洁的 API、直观的选项模式、完善的文档 |
| 📦 **零依赖** | 纯 Go 标准库，无外部包 |
| ✅ **生产就绪** | 经过实战检验的默认配置，广泛的测试覆盖 |
| 🍪 **Cookie 管理** | 完整的 Cookie Jar 支持，带安全验证 |
| 📁 **文件操作** | 安全的文件下载，支持进度跟踪和断点续传 |

---

## 📦 安装

```bash
go get -u github.com/cybergodev/httpc
```

---

## 🚀 快速开始 (5 分钟)

### 简单 GET 请求

```go
package main

import (
    "fmt"
    "log"

    "github.com/cybergodev/httpc"
)

func main() {
    // Package-level function - convenient for simple requests
    result, err := httpc.Get("https://httpbin.org/get")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d, Duration: %v\n", result.StatusCode(), result.Meta.Duration)
}
```

### 带认证的 POST JSON 请求

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/cybergodev/httpc"
)

func main() {
    user := map[string]string{"name": "John", "email": "john@example.com"}
    result, err := httpc.Post("https://httpbin.org/post",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
        httpc.WithTimeout(30*time.Second),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Response: %s\n", result.Body())
}
```

---

## 📖 HTTP 方法

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
```

---

## 🔧 请求选项

### 请求头

```go
// Single header
httpc.WithHeader("Authorization", "Bearer token")

// Multiple headers
httpc.WithHeaderMap(map[string]string{
    "X-Custom": "value",
    "X-Request-ID": "123",
})

// User-Agent
httpc.WithUserAgent("my-app/1.0")
```

### 认证

```go
// Bearer token
httpc.WithBearerToken("your-jwt-token")

// Basic auth
httpc.WithBasicAuth("username", "password")
```

### 查询参数

```go
// Single parameter
httpc.WithQuery("page", 1)

// Multiple parameters
httpc.WithQueryMap(map[string]any{"page": 1, "limit": 20})
```

### 请求体

```go
// JSON
httpc.WithJSON(data)

// XML
httpc.WithXML(data)

// Form data (application/x-www-form-urlencoded)
httpc.WithForm(map[string]string{"key": "value"})

// Multipart form data (file upload)
httpc.WithFormData(formData)
httpc.WithFile("file", "document.pdf", fileBytes)

// Raw body
httpc.WithBody([]byte("raw data"))
httpc.WithBinary(binaryData, "application/pdf")
```

### Cookie

```go
// Single cookie
httpc.WithCookie(http.Cookie{Name: "session", Value: "abc123"})

// Multiple cookies from map
httpc.WithCookieMap(map[string]string{
    "session_id": "abc123",
    "user_pref":  "dark_mode",
})

// Cookie string
httpc.WithCookieString("session=abc123; token=xyz")
```

### 请求控制

```go
// Context for cancellation
httpc.WithContext(ctx)

// Timeout
httpc.WithTimeout(30 * time.Second)

// Retry configuration
httpc.WithMaxRetries(3)

// Redirect control
httpc.WithFollowRedirects(false)
httpc.WithMaxRedirects(5)
```

### 回调函数

```go
// Before request
httpc.WithOnRequest(func(req httpc.RequestMutator) error {
    log.Printf("Sending %s %s", req.Method(), req.URL())
    return nil
})

// After response
httpc.WithOnResponse(func(resp httpc.ResponseMutator) error {
    log.Printf("Received %d", resp.StatusCode())
    return nil
})
```

### 完整选项参考

| 类别 | 选项 |
|------|------|
| **请求头** | `WithHeader(key, value)`, `WithHeaderMap(map)`, `WithUserAgent(ua)` |
| **认证** | `WithBearerToken(token)`, `WithBasicAuth(user, pass)` |
| **查询参数** | `WithQuery(key, value)`, `WithQueryMap(map)` |
| **请求体** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithFormData(form)`, `WithFile(field, filename, content)`, `WithBody([]byte)`, `WithBinary([]byte, contentType?)` |
| **Cookie** | `WithCookie(cookie)`, `WithCookieMap(map)`, `WithCookieString("a=1; b=2")`, `WithSecureCookie(config)` |
| **控制** | `WithTimeout(dur)`, `WithMaxRetries(n)`, `WithContext(ctx)` |
| **重定向** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **回调** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

---

## 📥 响应处理

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// Quick access
fmt.Println(result.StatusCode())     // 200
fmt.Println(result.Proto())          // "HTTP/1.1" or "HTTP/2.0"
fmt.Println(result.RawBody())        // Response body ([]byte)
fmt.Println(result.Body())           // Response body (string)

// Status checks
if result.IsSuccess() { }            // 2xx
if result.IsRedirect() { }           // 3xx
if result.IsClientError() { }        // 4xx
if result.IsServerError() { }        // 5xx

// Parse JSON response
var data map[string]interface{}
result.Unmarshal(&data)

// Cookie access
cookie := result.GetCookie("session")
if result.HasCookie("session") { }

// Request cookies sent
reqCookie := result.GetRequestCookie("token")

// Save response to file
result.SaveToFile("response.json")

// Metadata
fmt.Println(result.Meta.Duration)      // Request duration
fmt.Println(result.Meta.Attempts)      // Retry count
fmt.Println(result.Meta.RedirectCount) // Redirect count
fmt.Println(result.Meta.RedirectChain) // Redirect URLs

// String representation (safe for logging - masks sensitive headers)
fmt.Println(result.String())
```

---

## ⏱️ Context 与取消

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := httpc.Get("https://api.example.com",
    httpc.WithContext(ctx),
)
if errors.Is(err, context.DeadlineExceeded) {
    fmt.Println("Request timed out")
}
```

---

## 📁 文件下载

### 简单下载

文件下载内置安全防护：
- **UNC 路径阻断** - 防止访问 Windows 网络路径
- **系统路径保护** - 禁止写入关键系统目录
- **路径遍历检测** - 防止目录逃逸攻击
- **断点续传支持** - 自动恢复中断的下载

```go
result, _ := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("Downloaded: %s at %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))
```

### 带进度的下载

```go
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    pct := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r%.1f%% - %s/s", pct, httpc.FormatSpeed(speed))
}
result, _ := httpc.DownloadWithOptions(url, opts)
```

### 断点续传下载

```go
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ResumeDownload = true
result, _ := httpc.DownloadWithOptions(url, opts)
if result.Resumed {
    fmt.Println("Download resumed from previous position")
}
```

---

## 🌐 域名客户端 (会话管理)

用于对同一域名发起多次请求，自动管理 Cookie 和请求头：

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// Login - server sets cookies
client.Post("/login", httpc.WithJSON(credentials))

// Set persistent header (used for all requests)
client.SetHeader("Authorization", "Bearer "+token)

// Subsequent requests include cookies + headers automatically
profile, _ := client.Get("/profile")
data, _ := client.Get("/data")

// Cookie management
client.SetCookie(&http.Cookie{Name: "session", Value: "abc"})
client.GetCookie("session")
client.DeleteCookie("session")
client.ClearCookies()

// Header management
client.SetHeaders(map[string]string{"X-App": "v1"})
client.GetHeaders()
client.DeleteHeader("X-Old")
client.ClearHeaders()
```

---

## ⚙️ 配置

### 预设配置

```go
// Production-ready defaults (recommended)
client, _ := httpc.New(httpc.DefaultConfig())

// Maximum security (SSRF protection enabled)
client, _ := httpc.New(httpc.SecureConfig())

// High throughput
client, _ := httpc.New(httpc.PerformanceConfig())

// Lightweight (no retries)
client, _ := httpc.New(httpc.MinimalConfig())

// Testing only - disables security features!
client, _ := httpc.New(httpc.TestingConfig())
```

### 自定义配置

```go
config := &httpc.Config{
    // Timeouts
    Timeout:               30 * time.Second,
    DialTimeout:           10 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    IdleConnTimeout:       90 * time.Second,

    // Connection
    MaxIdleConns:      100,
    MaxConnsPerHost:   20,
    EnableHTTP2:       true,
    EnableCookies:     false,

    // Security
    MinTLSVersion:       tls.VersionTLS12,
    MaxTLSVersion:       tls.VersionTLS13,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
    AllowPrivateIPs:     true,

    // Retry
    MaxRetries:    3,
    RetryDelay:    1 * time.Second,
    BackoffFactor: 2.0,
    EnableJitter:  true,

    // Other
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    MaxRedirects:    10,
}
client, _ := httpc.New(config)
```

### 配置选项

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| **超时设置** ||||
| `Timeout` | `time.Duration` | `30s` | 整体请求超时 |
| `DialTimeout` | `time.Duration` | `10s` | TCP 连接超时 |
| `TLSHandshakeTimeout` | `time.Duration` | `10s` | TLS 握手超时 |
| `ResponseHeaderTimeout` | `time.Duration` | `30s` | 响应头超时 |
| `IdleConnTimeout` | `time.Duration` | `90s` | 空闲连接超时 |
| **连接设置** ||||
| `MaxIdleConns` | `int` | `50` | 最大空闲连接数 |
| `MaxConnsPerHost` | `int` | `10` | 每个主机最大连接数 |
| `ProxyURL` | `string` | `""` | 代理 URL (http/socks5) |
| `EnableSystemProxy` | `bool` | `false` | 自动检测系统代理 |
| `EnableHTTP2` | `bool` | `true` | 启用 HTTP/2 |
| `EnableCookies` | `bool` | `false` | 启用 Cookie Jar |
| `EnableDoH` | `bool` | `false` | 启用 DNS-over-HTTPS |
| `DoHCacheTTL` | `time.Duration` | `5m` | DoH 缓存时长 |
| **安全设置** ||||
| `TLSConfig` | `*tls.Config` | `nil` | 自定义 TLS 配置 |
| `MinTLSVersion` | `uint16` | `TLS 1.2` | 最低 TLS 版本 |
| `MaxTLSVersion` | `uint16` | `TLS 1.3` | 最高 TLS 版本 |
| `InsecureSkipVerify` | `bool` | `false` | 跳过 TLS 验证 (仅限测试！) |
| `MaxResponseBodySize` | `int64` | `10MB` | 最大响应体大小 |
| `AllowPrivateIPs` | `bool` | `true` | 允许私有 IP (SSRF) |
| `ValidateURL` | `bool` | `true` | 启用 URL 验证 |
| `ValidateHeaders` | `bool` | `true` | 启用请求头验证 |
| `StrictContentLength` | `bool` | `true` | 严格 Content-Length 检查 |
| `RedirectWhitelist` | `[]string` | `nil` | 允许的重定向域名 |
| **重试设置** ||||
| `MaxRetries` | `int` | `3` | 最大重试次数 |
| `RetryDelay` | `time.Duration` | `1s` | 初始重试延迟 |
| `BackoffFactor` | `float64` | `2.0` | 退避乘数 |
| `EnableJitter` | `bool` | `true` | 重试添加抖动 |
| `CustomRetryPolicy` | `RetryPolicy` | `nil` | 自定义重试逻辑 |
| **其他** ||||
| `Middlewares` | `[]MiddlewareFunc` | `nil` | 中间件链 |
| `UserAgent` | `string` | `"httpc/1.0"` | 默认 User-Agent |
| `Headers` | `map[string]string` | `{}` | 默认请求头 |
| `FollowRedirects` | `bool` | `true` | 跟随重定向 |
| `MaxRedirects` | `int` | `10` | 最大重定向次数 |

---

## 🔌 中间件

### 内置中间件

```go
// Request logging
httpc.LoggingMiddleware(log.Printf)

// Panic recovery
httpc.RecoveryMiddleware()

// Request ID
httpc.RequestIDMiddleware("X-Request-ID", nil)

// Timeout enforcement
httpc.TimeoutMiddleware(30*time.Second)

// Static headers
httpc.HeaderMiddleware(map[string]string{
    "X-App-Version": "1.0.0",
})

// Metrics collection
httpc.MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {
    metrics.Record(method, url, statusCode, duration)
})

// Security audit
httpc.AuditMiddleware(func(a httpc.AuditEvent) {
    log.Printf("[AUDIT] %s %s -> %d (%v)", a.Method, a.URL, a.StatusCode, a.Duration)
})
```

### 链式中间件

```go
chainedMiddleware := httpc.Chain(
    httpc.RecoveryMiddleware(),
    httpc.LoggingMiddleware(log.Printf),
    httpc.RequestIDMiddleware("X-Request-ID", nil),
    httpc.HeaderMiddleware(map[string]string{"X-App": "v1"}),
)
config.Middlewares = []httpc.MiddlewareFunc{chainedMiddleware}
```

### 自定义中间件

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

---

## 🔀 代理配置

```go
// Manual proxy
config := &httpc.Config{
    ProxyURL: "http://127.0.0.1:8080",
    // Or SOCKS5: "socks5://127.0.0.1:1080"
}

// System proxy auto-detection (Windows/macOS/Linux)
config := &httpc.Config{
    EnableSystemProxy: true,  // Reads from environment and system settings
}
```

---

## 🔒 安全特性

| 特性 | 描述 |
|------|------|
| **TLS 1.2+** | 默认使用现代加密标准 |
| **SSRF 防护** | 双层 DNS 验证阻断私有 IP |
| **CRLF 注入防护** | 请求头和 URL 验证 |
| **路径遍历防护** | 安全的文件操作 |
| **域名白名单** | 限制重定向到允许的域名 |
| **响应大小限制** | 可配置限制，防止内存耗尽 |

### 重定向域名白名单

```go
config := &httpc.Config{
    RedirectWhitelist: []string{"api.example.com", "secure.example.com"},
}
```

### SSRF 防护

默认情况下，`AllowPrivateIPs` 为 `true` 以保持兼容性。当请求用户提供的 URL 时，应启用 SSRF 防护：

```go
// Enable SSRF protection
cfg := httpc.DefaultConfig()
cfg.AllowPrivateIPs = false
client, _ := httpc.New(cfg)

// Or use the secure preset
client, _ := httpc.New(httpc.SecureConfig())
```

---

## ⚠️ 错误处理

```go
result, err := httpc.Get(url)
if err != nil {
    var clientErr *httpc.ClientError
    if errors.As(err, &clientErr) {
        fmt.Printf("Error: %s (code: %s)\n", clientErr.Message, clientErr.Code())
        fmt.Printf("Retryable: %v\n", clientErr.IsRetryable())
    }
    return err
}

// Check response status
if !result.IsSuccess() {
    return fmt.Errorf("unexpected status code: %d", result.StatusCode())
}
```

### 错误类型

```go
const (
    ErrorTypeUnknown        // 未知或未分类错误
    ErrorTypeNetwork        // 网络级错误 (连接被拒绝, DNS 失败)
    ErrorTypeTimeout        // 请求超时
    ErrorTypeContextCanceled // Context 被取消
    ErrorTypeResponseRead   // 读取响应体错误
    ErrorTypeTransport      // HTTP 传输错误
    ErrorTypeRetryExhausted // 所有重试已耗尽
    ErrorTypeTLS            // TLS 握手错误
    ErrorTypeCertificate    // 证书验证错误
    ErrorTypeDNS            // DNS 解析错误
    ErrorTypeValidation     // 请求验证错误
    ErrorTypeHTTP           // HTTP 级错误 (4xx, 5xx)
)
```

---

## 🔄 并发安全

HTTPC 设计为 goroutine 安全：

```go
client, _ := httpc.New()
defer client.Close()

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        result, _ := client.Get("https://api.example.com")
        // Process response...
    }()
}
wg.Wait()
```

### 性能优化

```go
// Release Result back to pool after use (reduces GC pressure)
result, _ := httpc.Get(url)
defer httpc.ReleaseResult(result)
```

**线程安全保证：**
- 所有 `Client` 方法均可安全并发使用
- 包级函数安全使用共享的默认客户端
- 响应对象可安全地从多个 goroutine 读取
- 内部指标使用原子操作

---

## 📚 文档

| 资源 | 描述 |
|------|------|
| [入门指南](docs/getting-started.md) | 安装和第一步 |
| [配置](docs/configuration.md) | 客户端配置和预设 |
| [请求选项](docs/request-options.md) | 完整选项参考 |
| [错误处理](docs/error-handling.md) | 错误处理模式 |
| [文件下载](docs/file-download.md) | 带进度的文件下载 |
| [HTTP 重定向](docs/redirects.md) | 重定向处理和跟踪 |
| [Cookie API](docs/cookie-api-reference.md) | Cookie 管理 |
| [安全](SECURITY.md) | 安全特性和最佳实践 |

### 示例代码

| 目录 | 描述 |
|------|------|
| [01_quickstart](examples/01_quickstart) | 基础用法 |
| [02_core_features](examples/02_core_features) | 请求头、认证、请求体格式 |
| [03_advanced](examples/03_advanced) | 文件上传、下载、重试、中间件 |

---

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件。

---

如果这个项目对你有帮助，请给一个 Star！ ⭐
