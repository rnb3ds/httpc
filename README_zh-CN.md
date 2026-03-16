# HTTPC - Go 语言生产级 HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Zero Deps](https://img.shields.io/badge/deps-zero-brightgreen.svg)](go.mod)

一款高性能的 Go 语言 HTTP 客户端库，具备企业级安全、零外部依赖和生产级默认配置。

**[English Document](README.md)**

---

## 特性

| 特性 | 描述 |
|------|------|
| **默认安全** | TLS 1.2+、SSRF 防护、CRLF 注入防护 |
| **高性能** | 连接池、HTTP/2、goroutine 安全、sync.Pool 优化 |
| **内置弹性** | 智能重试、指数退避和抖动 |
| **开发友好** | 清晰的 API、直观的选项模式、完善的文档 |
| **零依赖** | 纯 Go 标准库，无外部包 |
| **生产就绪** | 经过实战检验的默认配置、广泛的测试覆盖 |

---

## 安装

```bash
go get -u github.com/cybergodev/httpc
```

---

## 快速开始（5 分钟上手）

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
```

---

## HTTP 方法

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
```

---

## 请求选项

| 类别 | 选项 |
|------|------|
| **请求头** | `WithHeader(key, value)`, `WithHeaderMap(map)`, `WithUserAgent(ua)` |
| **认证** | `WithBearerToken(token)`, `WithBasicAuth(user, pass)` |
| **查询参数** | `WithQuery(key, value)`, `WithQueryMap(map)` |
| **请求体** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithBinary([]byte)` |
| **文件** | `WithFile(field, filename, content)`, `WithFormData(form)` |
| **Cookies** | `WithCookie(cookie)`, `WithCookieString("a=1; b=2")` |
| **控制** | `WithTimeout(dur)`, `WithMaxRetries(n)`, `WithContext(ctx)` |
| **重定向** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **回调** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

---

## 响应处理

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// 快速访问
fmt.Println(result.StatusCode())     // 200
fmt.Println(result.RawBody())        // 响应体（[]byte）
fmt.Println(result.Body())           // 响应体（字符串）

// 状态检查
if result.IsSuccess() { }            // 2xx
if result.IsClientError() { }        // 4xx
if result.IsServerError() { }        // 5xx

// 解析响应 JSON 数据
var data map[string]interface{}
result.Unmarshal(&data)

// 元信息
fmt.Println(result.Meta.Duration)      // 请求耗时
fmt.Println(result.Meta.Attempts)      // 重试次数
fmt.Println(result.Meta.RedirectCount) // 重定向次数
```

---

## Context 和取消

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

## 文件下载

```go
// 简单下载
result, _ := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("已下载: %s，速度 %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// 带进度的下载
opts := httpc.DefaultDownloadOptions("downloads/large.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    pct := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r%.1f%% - %s/s", pct, httpc.FormatSpeed(speed))
}
result, _ := httpc.DownloadWithOptions(url, opts)
```

---

## Domain Client（会话管理）

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
```

---

## 错误处理

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

---

## 配置

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
    Timeout:             30 * time.Second,
    MaxRetries:          3,
    MaxIdleConns:        100,
    MaxConnsPerHost:     20,
    MinTLSVersion:       tls.VersionTLS12,
    MaxResponseBodySize: 50 * 1024 * 1024, // 50 MB
    UserAgent:           "MyApp/1.0",
    EnableHTTP2:         true,
}
client, _ := httpc.New(config)
```

### 配置选项

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `Timeout` | `time.Duration` | `30s` | 整体请求超时 |
| `DialTimeout` | `time.Duration` | `10s` | TCP 连接超时 |
| `MaxIdleConns` | `int` | `50` | 最大空闲连接数 |
| `MaxConnsPerHost` | `int` | `10` | 每主机最大连接数 |
| `MaxRetries` | `int` | `3` | 最大重试次数 |
| `RetryDelay` | `time.Duration` | `1s` | 初始重试延迟 |
| `BackoffFactor` | `float64` | `2.0` | 退避乘数 |
| `EnableJitter` | `bool` | `true` | 重试添加抖动 |
| `ProxyURL` | `string` | `""` | 代理 URL (http/socks5) |
| `EnableSystemProxy` | `bool` | `false` | 自动检测系统代理 |
| `EnableHTTP2` | `bool` | `true` | 启用 HTTP/2 |
| `EnableCookies` | `bool` | `false` | 启用 cookie jar |
| `MinTLSVersion` | `uint16` | `TLS 1.2` | 最低 TLS 版本 |
| `MaxResponseBodySize` | `int64` | `10MB` | 最大响应体大小 |
| `UserAgent` | `string` | `"httpc/1.0"` | 默认 User-Agent |
| `FollowRedirects` | `bool` | `true` | 跟随重定向 |
| `MaxRedirects` | `int` | `10` | 最大重定向次数 |
| `AllowPrivateIPs` | `bool` | `true` | 允许私有 IP（SSRF） |

---

## 中间件

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

// 指标收集
httpc.MetricsMiddleware(func(method, url string, statusCode int, duration time.Duration, err error) {})

// 安全审计
httpc.AuditMiddleware(func(a httpc.AuditEvent) {})
```

### 链式组合多个中间件

```go
chainedMiddleware := httpc.Chain(
    httpc.RecoveryMiddleware(),
    httpc.LoggingMiddleware(log.Printf),
    httpc.RequestIDMiddleware("X-Request-ID", nil),
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

## 代理配置

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

## 安全特性

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

## 并发安全

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

## 文档

| 资源 | 描述 |
|------|------|
| [入门指南](docs/getting-started.md) | 安装和第一步 |
| [配置](docs/configuration.md) | 客户端配置和预设 |
| [请求选项](docs/request-options.md) | 完整选项参考 |
| [错误处理](docs/error-handling.md) | 错误处理模式 |
| [文件下载](docs/file-download.md) | 带进度的文件下载 |
| [HTTP 重定向](docs/redirects.md) | 重定向处理和跟踪 |
| [安全](SECURITY.md) | 安全特性和最佳实践 |

### 示例代码

| 目录 | 描述 |
|------|------|
| [01_quickstart](examples/01_quickstart) | 基本用法 |
| [02_core_features](examples/02_core_features) | Headers、认证、body 格式 |
| [03_advanced](examples/03_advanced) | 文件上传、下载、重试、中间件 |

---

## 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

如果这个项目对你有帮助，请给一个 Star! ⭐
