# HTTPC - Go 语言生产级 HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Zero Deps](https://img.shields.io/badge/deps-zero-brightgreen.svg)](go.mod)

一款高性能的 Go 语言 HTTP 客户端库，具备企业级安全、零外部依赖和生产级默认配置。

**[English Document](README.md)**

---

## ✨ 特性

| 特性 | 描述 |
|------|------|
| 🔒 **默认安全** | TLS 1.2+、SSRF 防护、CRLF 注入防护 |
| ⚡ **高性能** | 连接池、HTTP/2、goroutine 安全、sync.Pool 优化 |
| 🔄 **内置弹性** | 智能重试、指数退避和抖动 |
| 🛠️ **开发友好** | 清晰的 API、直观的选项模式、完善的文档 |
| 📦 **零依赖** | 纯 Go 标准库，无外部包 |
| ✅ **生产就绪** | 经过实战检验的默认配置、广泛的测试覆盖 |

---

## 📦 安装

```bash
go get -u github.com/cybergodev/httpc
```

---

## 🚀 快速开始（5 分钟上手）

### 简单 GET 请求

```go
package main

import (
    "fmt"
    "log"

    "github.com/cybergodev/httpc"
)

func main() {
    // 包级别函数 - 便捷使用
    result, err := httpc.Get("https://httpbin.org/get")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("状态码: %d, 耗时: %v\n", result.StatusCode(), result.Meta.Duration)
}
```

### POST JSON 数据和认证

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
    fmt.Printf("响应: %s\n", result.Body())
}
```

---

## 📖 HTTP 方法

```go
// GET 带查询参数
result, _ := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQueryMap(map[string]any{"limit": 20}),
)

// POST JSON 请求体
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
// 单个请求头
httpc.WithHeader("Authorization", "Bearer token")

// 多个请求头
httpc.WithHeaderMap(map[string]string{
    "X-Custom": "value",
    "X-Request-ID": "123",
})

// User-Agent
httpc.WithUserAgent("my-app/1.0")
```

### 认证

```go
// Bearer 令牌
httpc.WithBearerToken("your-jwt-token")

// 基础认证
httpc.WithBasicAuth("username", "password")
```

### 查询参数

```go
// 单个参数
httpc.WithQuery("page", 1)

// 多个参数
httpc.WithQueryMap(map[string]any{"page": 1, "limit": 20})
```

### 请求体

```go
// JSON
httpc.WithJSON(data)

// XML
httpc.WithXML(data)

// 表单数据 (application/x-www-form-urlencoded)
httpc.WithForm(map[string]string{"key": "value"})

// Multipart 表单数据 (文件上传)
httpc.WithFormData(formData)
httpc.WithFile("file", "document.pdf", fileBytes)

// 原始数据
httpc.WithBody([]byte("raw data"))
httpc.WithBinary(binaryData, "application/pdf")
```

### Cookies

```go
// 单个 Cookie
httpc.WithCookie(http.Cookie{Name: "session", Value: "abc123"})

// 从 Map 设置多个 Cookies
httpc.WithCookieMap(map[string]string{
    "session_id": "abc123",
    "user_pref":  "dark_mode",
})

// Cookie 字符串
httpc.WithCookieString("session=abc123; token=xyz")
```

### 请求控制

```go
// 用于取消的 Context
httpc.WithContext(ctx)

// 超时
httpc.WithTimeout(30 * time.Second)

// 重试配置
httpc.WithMaxRetries(3)

// 重定向控制
httpc.WithFollowRedirects(false)
httpc.WithMaxRedirects(5)
```

### 回调函数

```go
// 请求前
httpc.WithOnRequest(func(req httpc.RequestMutator) error {
    log.Printf("发送 %s %s", req.Method(), req.URL())
    return nil
})

// 响应后
httpc.WithOnResponse(func(resp httpc.ResponseMutator) error {
    log.Printf("收到 %d", resp.StatusCode())
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
| **Cookies** | `WithCookie(cookie)`, `WithCookieMap(map)`, `WithCookieString("a=1; b=2")`, `WithSecureCookie(config)` |
| **控制** | `WithTimeout(dur)`, `WithMaxRetries(n)`, `WithContext(ctx)` |
| **重定向** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **回调** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

---

## 📥 响应处理

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// 快速访问
fmt.Println(result.StatusCode())     // 200
fmt.Println(result.Proto())          // "HTTP/1.1" 或 "HTTP/2.0"
fmt.Println(result.RawBody())        // 响应体（[]byte）
fmt.Println(result.Body())           // 响应体（字符串）

// 状态检查
if result.IsSuccess() { }            // 2xx
if result.IsRedirect() { }           // 3xx
if result.IsClientError() { }        // 4xx
if result.IsServerError() { }        // 5xx

// 解析 JSON 响应
var data map[string]interface{}
result.Unmarshal(&data)

// Cookie 访问
cookie := result.GetCookie("session")
if result.HasCookie("session") { }

// 发送的请求 Cookie
reqCookie := result.GetRequestCookie("token")

// 保存响应到文件
result.SaveToFile("response.json")

// 元信息
fmt.Println(result.Meta.Duration)      // 请求耗时
fmt.Println(result.Meta.Attempts)      // 重试次数
fmt.Println(result.Meta.RedirectCount) // 重定向次数
fmt.Println(result.Meta.RedirectChain) // 重定向 URL 链

// 字符串表示（安全用于日志）
fmt.Println(result.String())
```

---

## ⏱️ Context 和取消

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := httpc.Get("https://api.example.com",
    httpc.WithContext(ctx),
)
if errors.Is(err, context.DeadlineExceeded) {
    fmt.Println("请求超时")
}
```

---

## 📁 文件下载

### 简单下载

文件下载包含内置的安全保护功能：
- **UNC 路径阻止** - 防止访问 Windows 网络路径
- **系统路径保护** - 阻止写入关键系统目录
- **路径遍历检测** - 防止目录逃逸攻击
- **恢复支持** - 自动恢复中断的下载

```go
result, _ := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("已下载: %s，速度 %s/s\n",
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

### 断点续传

```go
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ResumeDownload = true
result, _ := httpc.DownloadWithOptions(url, opts)
if result.Resumed {
    fmt.Println("已从上次位置恢复下载")
}
```

---

## 🌐 域名客户端（会话管理）

用于向同一域发送多个请求，自动管理 Cookie 和 Header：

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// 登录 - 服务器设置 cookies
client.Post("/login", httpc.WithJSON(credentials))

// 设置持久 header（所有请求使用）
client.SetHeader("Authorization", "Bearer "+token)

// 后续请求自动包含 cookies + headers
profile, _ := client.Get("/profile")
data, _ := client.Get("/data")

// Cookie 管理
client.SetCookie(&http.Cookie{Name: "session", Value: "abc"})
client.GetCookie("session")
client.DeleteCookie("session")
client.ClearCookies()

// Header 管理
client.SetHeaders(map[string]string{"X-App": "v1"})
client.GetHeaders()
client.DeleteHeader("X-Old")
client.ClearHeaders()
```

---

## ⚠️ 错误处理

```go
result, err := httpc.Get(url)
if err != nil {
    var clientErr *httpc.ClientError
    if errors.As(err, &clientErr) {
        fmt.Printf("错误: %s (代码: %s)\n", clientErr.Message, clientErr.Code())
        fmt.Printf("可重试: %v\n", clientErr.IsRetryable())
    }
    return err
}

// 检查响应状态
if !result.IsSuccess() {
    return fmt.Errorf("意外的状态码: %d", result.StatusCode())
}
```

### 错误类型

```go
const (
    ErrorTypeUnknown        // 未知或未分类错误
    ErrorTypeNetwork        // 网络级错误（连接被拒绝、DNS 失败）
    ErrorTypeTimeout        // 请求超时
    ErrorTypeContextCanceled // Context 被取消
    ErrorTypeResponseRead   // 读取响应体错误
    ErrorTypeTransport      // HTTP 传输错误
    ErrorTypeRetryExhausted // 所有重试已用尽
    ErrorTypeTLS            // TLS 握手错误
    ErrorTypeCertificate    // 证书验证错误
    ErrorTypeDNS            // DNS 解析错误
    ErrorTypeValidation     // 请求验证错误
    ErrorTypeHTTP           // HTTP 级错误（4xx, 5xx）
)
```

---

## ⚙️ 配置

### 预设配置

```go
// 生产级默认配置（推荐）
client, _ := httpc.New(httpc.DefaultConfig())

// 最大安全性（启用 SSRF 防护）
client, _ := httpc.New(httpc.SecureConfig())

// 高吞吐量
client, _ := httpc.New(httpc.PerformanceConfig())

// 轻量级（无重试）
client, _ := httpc.New(httpc.MinimalConfig())

// 仅测试用 - 禁用安全功能！
client, _ := httpc.New(httpc.TestingConfig())
```

### 自定义配置

```go
config := &httpc.Config{
    // 超时
    Timeout:               30 * time.Second,
    DialTimeout:           10 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    IdleConnTimeout:       90 * time.Second,

    // 连接
    MaxIdleConns:      100,
    MaxConnsPerHost:   20,
    EnableHTTP2:       true,
    EnableCookies:     false,

    // 安全
    MinTLSVersion:       tls.VersionTLS12,
    MaxTLSVersion:       tls.VersionTLS13,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
    AllowPrivateIPs:     true,

    // 重试
    MaxRetries:    3,
    RetryDelay:    1 * time.Second,
    BackoffFactor: 2.0,
    EnableJitter:  true,

    // 其他
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    MaxRedirects:    10,
}
client, _ := httpc.New(config)
```

### 配置选项

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| **超时** ||||
| `Timeout` | `time.Duration` | `30s` | 整体请求超时 |
| `DialTimeout` | `time.Duration` | `10s` | TCP 连接超时 |
| `TLSHandshakeTimeout` | `time.Duration` | `10s` | TLS 握手超时 |
| `ResponseHeaderTimeout` | `time.Duration` | `30s` | 响应头超时 |
| `IdleConnTimeout` | `time.Duration` | `90s` | 空闲连接超时 |
| **连接** ||||
| `MaxIdleConns` | `int` | `50` | 最大空闲连接数 |
| `MaxConnsPerHost` | `int` | `10` | 每主机最大连接数 |
| `ProxyURL` | `string` | `""` | 代理 URL (http/socks5) |
| `EnableSystemProxy` | `bool` | `false` | 自动检测系统代理 |
| `EnableHTTP2` | `bool` | `true` | 启用 HTTP/2 |
| `EnableCookies` | `bool` | `false` | 启用 cookie jar |
| `EnableDoH` | `bool` | `false` | 启用 DNS-over-HTTPS |
| `DoHCacheTTL` | `time.Duration` | `5m` | DoH 缓存时间 |
| **安全** ||||
| `TLSConfig` | `*tls.Config` | `nil` | 自定义 TLS 配置 |
| `MinTLSVersion` | `uint16` | `TLS 1.2` | 最低 TLS 版本 |
| `MaxTLSVersion` | `uint16` | `TLS 1.3` | 最高 TLS 版本 |
| `InsecureSkipVerify` | `bool` | `false` | 跳过 TLS 验证（仅测试！） |
| `MaxResponseBodySize` | `int64` | `10MB` | 最大响应体大小 |
| `AllowPrivateIPs` | `bool` | `true` | 允许私有 IP（SSRF） |
| `ValidateURL` | `bool` | `true` | 启用 URL 验证 |
| `ValidateHeaders` | `bool` | `true` | 启用 Header 验证 |
| `StrictContentLength` | `bool` | `true` | 严格 Content-Length 检查 |
| `RedirectWhitelist` | `[]string` | `nil` | 允许重定向的域名 |
| **重试** ||||
| `MaxRetries` | `int` | `3` | 最大重试次数 |
| `RetryDelay` | `time.Duration` | `1s` | 初始重试延迟 |
| `BackoffFactor` | `float64` | `2.0` | 退避乘数 |
| `EnableJitter` | `bool` | `true` | 重试添加抖动 |
| `CustomRetryPolicy` | `RetryPolicy` | `nil` | 自定义重试逻辑 |
| **其他** ||||
| `Middlewares` | `[]MiddlewareFunc` | `nil` | 中间件链 |
| `UserAgent` | `string` | `"httpc/1.0"` | 默认 User-Agent |
| `Headers` | `map[string]string` | `{}` | 默认 Headers |
| `FollowRedirects` | `bool` | `true` | 跟随重定向 |
| `MaxRedirects` | `int` | `10` | 最大重定向次数 |

---

## 🔌 中间件

### 内置中间件

```go
// 请求日志
httpc.LoggingMiddleware(log.Printf)

// Panic 恢复
httpc.RecoveryMiddleware()

// 请求 ID
httpc.RequestIDMiddleware("X-Request-ID", nil)

// 超时强制
httpc.TimeoutMiddleware(30*time.Second)

// 静态 Headers
httpc.HeaderMiddleware(map[string]string{
    "X-App-Version": "1.0.0",
})

// 指标收集
httpc.MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {
    metrics.Record(method, url, statusCode, duration)
})

// 安全审计
httpc.AuditMiddleware(func(a httpc.AuditEvent) {
    log.Printf("[审计] %s %s -> %d (%v)", a.Method, a.URL, a.StatusCode, a.Duration)
})
```

### 链式组合多个中间件

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
            // 请求前
            req.SetHeader("X-Custom", "value")

            // 调用下一个处理器
            resp, err := next(ctx, req)

            // 响应后
            return resp, err
        }
    }
}
```

---

## 🔀 代理配置

```go
// 手动代理
config := &httpc.Config{
    ProxyURL: "http://127.0.0.1:8080",
    // 或 SOCKS5: "socks5://127.0.0.1:1080"
}

// 系统代理自动检测 (Windows/macOS/Linux)
config := &httpc.Config{
    EnableSystemProxy: true,  // 从环境变量和系统设置读取
}
```

---

## 🔒 安全特性

| 特性 | 描述 |
|------|------|
| **TLS 1.2+** | 默认使用现代加密标准 |
| **SSRF 防护** | DNS 验证阻止私有 IP |
| **CRLF 注入防护** | Header 和 URL 验证 |
| **路径遍历防护** | 安全的文件操作 |
| **域名白名单** | 限制重定向到允许的域名 |
| **响应大小限制** | 可配置限制防止内存耗尽 |

### 重定向域名白名单

```go
config := &httpc.Config{
    RedirectWhitelist: []string{"api.example.com", "secure.example.com"},
}
```

### SSRF 防护

默认情况下，`AllowPrivateIPs` 为 `true` 以保证兼容性。当向用户提供的 URL 发送请求时，应启用 SSRF 防护：

```go
// 启用 SSRF 防护
cfg := httpc.DefaultConfig()
cfg.AllowPrivateIPs = false
client, _ := httpc.New(cfg)

// 或使用安全预设
client, _ := httpc.New(httpc.SecureConfig())
```

---

## 🔄 并发安全

HTTPC 从设计上就是 goroutine 安全的：

```go
client, _ := httpc.New()
defer client.Close()

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        result, _ := client.Get("https://api.example.com")
        // 处理响应...
    }()
}
wg.Wait()
```

### 性能优化

```go
// 使用完后将 Result 释放回池中（减少 GC 压力）
result, _ := httpc.Get(url)
defer httpc.ReleaseResult(result)
```

**线程安全保证：**
- 所有 `Client` 方法并发使用安全
- 包级别函数安全地使用共享默认客户端
- 响应对象可从多个 goroutine 安全读取
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
| [01_quickstart](examples/01_quickstart) | 基本用法 |
| [02_core_features](examples/02_core_features) | Headers、认证、body 格式 |
| [03_advanced](examples/03_advanced) | 文件上传、下载、重试、中间件 |

---

## 📄 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

如果这个项目对你有帮助，请给一个 Star! ⭐
