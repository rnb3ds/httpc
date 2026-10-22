# HTTPC - 安全的 Go HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://golang.org)
[![Release](https://img.shields.io/github/v/release/cybergodev/httpc)](https://github.com/cybergodev/httpc/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Dependencies](https://img.shields.io/badge/deps-minimal-brightgreen.svg)](go.mod)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-%E2%9C%93-brightgreen.svg)](docs/09_concurrency-safety.md)

一个快速、安全的 Go HTTP 客户端库，具备合理的默认配置、精简的依赖集和内置弹性机制。

**[English Documentation](README.md)** | **[www.cybergo.dev/httpc](https://www.cybergo.dev/httpc)**

---

## 目录

- [特性](#特性)
- [安装](#安装)
- [快速开始](#快速开始)
- [HTTP 方法](#http-方法)
- [请求选项](#请求选项)
- [响应处理](#响应处理)
- [Context 与取消](#context-与取消)
- [文件下载](#文件下载)
- [流式传输](#流式传输)
- [域名客户端 (会话管理)](#域名客户端-会话管理)
- [配置](#配置)
- [中间件](#中间件)
- [代理配置](#代理配置)
- [安全特性](#安全特性)
- [浏览器传输指纹](#浏览器传输指纹)
- [错误处理](#错误处理)
- [并发安全](#并发安全)
- [接口](#接口)
- [文档](#文档)
- [许可证](#许可证)

---

## 特性

| 特性 | 描述 |
|------|------|
| **默认安全** | TLS 1.2+、SSRF 防护、CRLF 注入防护、路径遍历阻断 |
| **高性能** | 连接池、HTTP/2、goroutine 安全、`sync.Pool` 优化 |
| **内置弹性** | 智能重试，支持指数退避和抖动 —— 非幂等请求（POST/PATCH）默认仅执行一次 |
| **自动解压缩** | 透明的 gzip/deflate 处理，带 Zip 炸弹防护 |
| **开发者友好** | 简洁的 API、直观的选项模式、完善的文档 |
| **依赖** | 7 个直接模块 —— 标准路径用 `golang.org/x/net` + `golang.org/x/sys`；brotli / compress / uTLS / tls-client / fhttp 仅供 `Security.TLSFingerprint` 使用 |
| **Cookie 管理** | 完整的 Cookie Jar 支持，带安全验证 |
| **文件操作** | 安全的文件下载，支持进度跟踪和断点续传 |
| **流式传输** | `io.Reader` 请求体、零拷贝 `io.Pipe` 上传、磁盘流式下载 |

---

## 安装

```bash
go get -u github.com/cybergodev/httpc
```

```go
import "github.com/cybergodev/httpc"
```

**环境要求：** Go 1.27+

---

## 快速开始

```go
// 包级函数 —— 无需创建客户端
result, err := httpc.Get("https://httpbin.org/get")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("状态码: %d, 耗时: %v\n", result.StatusCode(), result.Meta.Duration)
```

超出一次性请求的场景，请创建可复用的客户端（推荐）：

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

完整可运行程序：[examples/01_basic_usage.go](examples/01_basic_usage.go) ·
[docs/01_getting-started.md](docs/01_getting-started.md)。

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

// HEAD / OPTIONS
result, _ := httpc.Head(url)
result, _ := httpc.Options(url)

// 自定义方法的通用请求
result, _ := httpc.Request(ctx, "PROPFIND", url)
```

---

## 请求选项

每个请求都接受 `With*` 选项 —— 请求头、认证、查询参数、请求体、Cookie、
超时、重试、重定向、SSRF 覆盖、回调，以及包级的代理/指纹路由：

| 类别 | 选项 |
|------|------|
| **请求头** | `WithHeader(key, value)`, `WithHeaderMap(map)`, `WithUserAgent(ua)` |
| **认证** | `WithBearerToken(token)`, `WithBasicAuth(user, pass)` |
| **查询参数** | `WithQuery(key, value)`, `WithQueryMap(map)` |
| **请求体** | `WithJSON(data)`, `WithXML(data)`, `WithForm(map)`, `WithFormData(*FormData)`, `WithFile(field, filename, content)`, `WithBody(data, ...BodyKind)`, `WithBinary([]byte, ...contentType)`, `WithStreamBody(bool)` |
| **Cookie** | `WithCookies([]http.Cookie)`, `WithCookieMap(map)`, `WithCookieString("a=1; b=2")`, `WithSecureCookie(config)` |
| **控制** | `WithTimeout(dur)`, `WithNoTimeout()`, `WithMaxRetries(n)`, `WithRetryNonIdempotent(bool)`, `WithContext(ctx)`, `WithAllowPrivateIPs(bool)` |
| **重定向** | `WithFollowRedirects(bool)`, `WithMaxRedirects(n)` |
| **路由** | `WithProxy(url)`, `WithSystemProxy()`, `WithTLSFingerprint(preset)` — 仅包级函数 |
| **回调** | `WithOnRequest(fn)`, `WithOnResponse(fn)` |

每个选项的详细说明与示例：
[docs/03_request-options.md](docs/03_request-options.md)。

> **包级路由**：`WithProxy` / `WithSystemProxy` / `WithTLSFingerprint` 把单次
> 包级调用路由到一个缓存的专用客户端（隔离连接池，继承默认客户端的安全
> 默认）；用 `httpc.CloseRoutedClients()` 统一关闭。在客户端实例上使用会返回
> 错误 —— 请改用 `Config.Connection.*` / `Security.TLSFingerprint`。

---

## 响应处理

```go
result, _ := httpc.Get("https://api.example.com/users/123")

// Result.Request / Result.Response / Result.Meta 携带完整的请求、响应
// 与元数据（耗时、尝试次数、代理、重定向链）。

fmt.Println(result.StatusCode()) // 200
fmt.Println(result.Body())       // 响应体 (string)

if result.IsSuccess() { }        // 2xx；另有 IsRedirect / IsClientError / IsServerError

var data map[string]any
if err := result.Unmarshal(&data); err != nil { // 解析 JSON
    log.Fatal(err)
}

fmt.Println(result.Meta.ProxyURL) // 使用的代理（"" = 直连；系统代理不记录）
fmt.Println(result.String())      // 可安全打日志 —— 敏感请求头已遮蔽
```

### 自动解压缩

`gzip` / `deflate` 响应会被透明解压（解压缩炸弹防护将解压后大小限制在
`Security.MaxDecompressedBodySize`，默认 100 MB）。其他任何
`Content-Encoding`（如 `br`、`zstd` 或 `gzip, br` 列表）都会直接返回错误，
而不是把原始压缩字节当正文返回 —— 通常仅在手动设置 `Accept-Encoding` 时发生。

完整检查 API（Cookie、`SaveToFile`、重定向链、请求元数据）：
[docs/08_request-inspection.md](docs/08_request-inspection.md)。

---

## Context 与取消

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := httpc.Get("https://api.example.com", httpc.WithContext(ctx))
if errors.Is(err, context.DeadlineExceeded) {
    fmt.Println("请求超时")
}
```

---

## 文件下载

`Download` 流式写入磁盘，内置防护（UNC/系统路径阻断、路径遍历检测），
并支持进度、断点续传与校验和验证：

```go
opts := httpc.DefaultDownloadConfig()
opts.FilePath = "downloads/large.zip"
opts.ResumeDownload = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    fmt.Printf("\r%.1f%% - %s/s", float64(downloaded)/float64(total)*100, httpc.FormatSpeed(speed))
}
result, err := httpc.Download(ctx, "https://example.com/large.zip", opts)
```

`Download` 直接接收 `context.Context`，也接受请求选项：
`httpc.Download(ctx, url, opts, httpc.WithBearerToken(...))`。配置与结果字段：
[docs/07_file-download.md](docs/07_file-download.md)。

---

## 流式传输

**上传** —— `WithBody` 接受任意 `io.Reader`（需显式设置 Content-Type；
`io.Pipe` 实现零拷贝流式）：

```go
result, err := httpc.Post("https://example.com/upload",
    httpc.WithBody(file),
    httpc.WithHeader("Content-Type", "application/octet-stream"),
)
```

> **安全提示：** `WithBody(io.Reader)` 绕过请求体大小验证 ——
> 不受信任的 Reader 请用 `io.LimitReader` 包裹。

**下载** —— `Download()` 将响应流式写入磁盘，从不缓冲。常规 `Get` / `Post`
始终缓冲完整响应；对其传入 `WithStreamBody(true)` 会返回
`ErrStreamBodyRequiresDownload`。

详细说明：[docs/03_request-options.md](docs/03_request-options.md) § 流式请求体 ·
[docs/07_file-download.md](docs/07_file-download.md)。

---

## 域名客户端 (会话管理)

对同一域名发起多次请求，自动管理 Cookie 和请求头：

```go
client, _ := httpc.NewDomainDefault("https://api.example.com")
defer client.Close()

client.Post("/login", httpc.WithJSON(credentials)) // 服务器设置 Cookie
client.SetHeader("Authorization", "Bearer "+token) // 持久请求头

profile, _ := client.Get("/profile") // 自动携带 Cookie 和请求头
```

`Client` 的所有方法都支持相对路径（含 `Download`）。底层 `SessionManager`
（线程安全的 Cookie/请求头存储）也可独立使用：
[docs/06_cookie-api.md](docs/06_cookie-api.md)。

---

## 配置

```go
// 推荐的默认配置
client, _ := httpc.NewDefault()

// 最高安全性 (启用 SSRF 防护)
client, _ := httpc.New(httpc.SecureConfig())

// 高吞吐量 / 轻量级 / 仅限测试
client, _ := httpc.New(httpc.PerformanceConfig())
client, _ := httpc.New(httpc.MinimalConfig())
client, _ := httpc.New(httpc.TestingConfig()) // 禁用安全特性！
```

```go
config := httpc.DefaultConfig()
config.Timeouts.Request = 30 * time.Second
config.Connection.MaxIdleConns = 100
config.Security.MaxResponseBodySize = 50 * 1024 * 1024
config.Retry.MaxRetries = 3
if err := config.Validate(); err != nil { // New() 内部也会自动验证
    log.Fatal(err)
}
client, _ := httpc.New(config)
fmt.Println(config.String()) // 敏感值自动遮蔽
```

`Timeouts` / `Connection` / `Security` / `Retry` / `Middleware` / `Defaults`
全部字段 —— 类型、默认值与行为说明（代理池、DoH、指纹、证书固定、SSRF
豁免、重定向上限）：[docs/02_configuration.md](docs/02_configuration.md)。

---

## 中间件

可组合的请求中间件：日志、Panic 恢复、请求 ID、超时、静态请求头、指标
收集与安全审计：

```go
config := httpc.DefaultConfig()
config.Middleware.Middlewares = []httpc.MiddlewareFunc{
    httpc.Chain(
        httpc.RecoveryMiddleware(),
        httpc.LoggingMiddleware(&httpc.LoggingConfig{LogFunc: log.Printf}),
        httpc.RequestIDMiddleware(nil),
        httpc.AuditMiddleware(nil), // nil = 默认审计配置
    ),
}
```

每个可配置中间件遵循相同模式（`XxxConfig` + `DefaultXxxConfig()` +
`XxxMiddleware(cfg)`，传 `nil` 用默认值）；自定义中间件是对 `httpc.Handler`
的包装函数。完整指南（含 `AuditEvent` 字段）：
[docs/11_middleware.md](docs/11_middleware.md)。

---

## 代理配置

```go
config := httpc.DefaultConfig()
config.Connection.ProxyURL = "http://127.0.0.1:8080" // http/https/socks5/socks5h
// 或：config.Connection.EnableSystemProxy = true    // 环境变量 + 平台设置
```

代理池提供轮换、熔断与基于状态码的轮换：

```go
config.Connection.ProxyPool = []string{
    "http://proxy1.example.com:7070",
    "socks5://proxy2.example.com:1080",
}
config.Connection.ProxyRotateOnStatus = []int{403} // 403 时换代理 IP 重试
config.Retry.MaxRetries = 3
```

每个 `Result` 都报告产生最终响应的代理（`result.Meta.ProxyURL`，空字符串 =
直连 —— `EnableSystemProxy` 动态检测的系统代理不会记录在此字段）。
**优先级：** `ProxyURL` > `ProxyPool` > `EnableSystemProxy` > 直连。
代理池、策略、熔断、逐请求轮换、每代理 HTTP/2（`EnableProxyPoolHTTP2`）：
[docs/12_proxy.md](docs/12_proxy.md)。

---

## 安全特性

| 特性 | 描述 |
|------|------|
| **TLS 1.2+** | 默认使用现代加密标准 |
| **证书固定** | 即使 CA 被攻破也能防御中间人攻击 (MITM) |
| **SSRF 防护** | 双层 DNS 验证阻断私有 IP |
| **CRLF 注入防护** | 请求头和 URL 验证 |
| **路径遍历防护** | 安全的文件操作 |
| **域名白名单** | 限制重定向到允许的域名 |
| **响应大小限制** | 可配置限制，防止内存耗尽 |

### SSRF 防护

私有/保留 IP 默认被阻断（`Security.AllowPrivateIPs` 为 `false`）。可按粒度放宽：

```go
cfg := httpc.DefaultConfig()
cfg.Security.SSRFExemptCIDRs = []string{"10.0.0.0/8"} // 豁免 CIDR（如 VPN/VPC）
client, _ := httpc.New(cfg)

// 或仅对单个受信任的内部 URL 放行：
result, err := httpc.Get("http://localhost:8080/health",
    httpc.WithAllowPrivateIPs(true))
```

经代理时，httpc 会在请求进入代理前本地解析并校验目标主机名。两个注意点：
**本地 DNS 失败即拒绝**（本进程无法解析的主机名会被拒绝，即使代理能解析 ——
请用 `SSRFExemptCIDRs` 豁免）；**无 DNS 重绑定保证**（代理会做自己的第二次解析）。

### 证书固定

在服务器未提供已固定公钥时拒绝 TLS 握手 —— 即使受信任 CA 被攻破也能防御
中间人攻击：

```go
pinner, err := httpc.NewSPKIHashPinner(
    "YLh1dUR9y6Kja30RrAn7JKnbQG/uEtLMkBgFF2fuihg=", // 当前密钥
    "C5+lpZ7tcVwmwQIMcRtPbsQtWLABXhQzejna0wHFr8M=", // 备用密钥（轮换）
)
cfg := httpc.DefaultConfig()
cfg.Security.CertificatePinner = pinner
client, err := httpc.New(cfg)
```

证书固定与 `Security.TLSFingerprint` 兼容；设置 `InsecureSkipVerify` 时，pinner 成为唯一校验门槛（固定叶子证书）。

### 安全警告输出

不安全配置（`TestingConfig()`、`InsecureSkipVerify: true`）默认会在 stderr
打印警告，可重定向或抑制：`httpc.SetSecurityWarnOutput(io.Discard)`。

---

## 浏览器传输指纹

做指纹识别的站点检测的远不止 User-Agent——TLS ClientHello（JA3/JA4）、HTTP/2
分帧与有序 header 块。设置 `Security.TLSFingerprint` 后，httpc 的传输层被整体
替换为浏览器仿真，走完全指纹化的 HTTP/2（优先）与 HTTP/1.1（回退）；重定向、
重试、中间件、流式下载与 Cookie 处理仍是 httpc 的语义：

```go
cfg := httpc.DefaultConfig()
cfg.Security.TLSFingerprint = "chrome-latest" // 或 "chrome-windows"、"firefox-latest" 等
client, err := httpc.New(cfg)
```

**预设**：`chrome` / `chrome-latest` / `chrome-155` 与 `firefox` /
`firefox-latest` / `firefox-158`，均有 `*-windows` / `*-macos` / `*-linux`
桌面变体与 `*-android` / `*-ios` 移动变体（如 `chrome-windows`、`firefox-ios`），
另有 `safari-ios`（= `ios-safari-latest`）与旧版本固定别名 `chrome-152` /
`firefox-148`。暂无桌面 Safari 档案（无已验证数据）。`TLSFingerprint` 留空即
保持标准协议栈，不引入额外依赖。

要点行为——完整说明见 [docs/10_tls-fingerprinting.md](docs/10_tls-fingerprinting.md)：

- **头部集权威**：预设替换同名头（含 User-Agent）；自定义头原样透传。
- **ALPN 决定协议**：服务器支持就走 HTTP/2，否则 HTTP/1.1，从不重放；不尝试 HTTP/3（QUIC 会绕过证书固定）。
- **代理语义保留**：代理池、轮换、熔断（HTTP/HTTPS/SOCKS5）；HTTPS 代理与任何 TLS 对端一样做证书校验。
- **证书固定可用**：每次指纹化握手都运行 `CertificatePinner`；`Security.TLSConfig` 与指纹互斥。
- **SSRF 防护**：直连保持与标准传输层同级的拨号层保证。
- **能力边界**：仅覆盖被动传输观测——不含浏览器 JavaScript 或行为信号。预设已端到端实测（`internal/fingerprint/e2e_network_test.go`）；
  每周 CI 任务监测档案漂移。

---

## 错误处理

```go
result, err := httpc.Get(url)
if err != nil {
    var clientErr *httpc.ClientError
    if errors.As(err, &clientErr) {
        fmt.Printf("错误: %s (代码: %s)，可重试: %v\n",
            clientErr.Message, clientErr.Code(), clientErr.IsRetryable())
    }
    return err
}
```

`ClientError` 携带 `Type` / `Message` / `Cause`（用 `%w` 解包）/ `URL` /
`Method` / `Attempts` / `StatusCode`。常见类型：`ErrorTypeNetwork`、
`ErrorTypeTimeout`、`ErrorTypeTLS`、`ErrorTypeDNS`、`ErrorTypeHTTP`；重试耗尽时
保留最后一次错误的类型 —— 请用 `Attempts` / `IsRetryable()` 判断。供
`errors.Is` 使用的哨兵包括 `ErrClientClosed`、`ErrFileExists`、
`ErrStreamBodyRequiresDownload`（请用 `Download`）、`ErrProxyConnectionFailed`
及配置校验的 `ErrInvalid*` 系列。完整参考：
[docs/04_error-handling.md](docs/04_error-handling.md)。

---

## 并发安全

httpc 客户端可安全并发使用：

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

- 所有 `Client` 方法均可安全并发使用；包级函数共享默认客户端
  （`SetDefaultClient` 自定义、`CloseDefaultClient` 重置）
- `Result` 对象返回后不可变 —— 并发读取安全
- `WithProxy` / `WithSystemProxy` / `WithTLSFingerprint` 创建独立路由客户端，
  用 `CloseRoutedClients()` 统一回收

详细说明：[docs/09_concurrency-safety.md](docs/09_concurrency-safety.md)。

---

## 接口

用于测试和扩展的接口 —— 需要 Mock HTTP 调用时使用：

```go
type Doer interface {
    Request(ctx context.Context, method, url string, options ...RequestOption) (*Result, error)
}
```

`Client` 扩展 `Doer`，增加逐方法便捷函数、`Download` 与 `Close()`；
`DomainClienter` 再增加域级会话管理。嵌入接口可获得前向兼容的 Mock：

```go
type MockClient struct {
    httpc.Client // 嵌入以保持前向兼容性
}

func (m *MockClient) Get(url string, options ...httpc.RequestOption) (*httpc.Result, error) {
    return &httpc.Result{
        Response: &httpc.ResponseInfo{StatusCode: 200, Body: `{"ok": true}`},
    }, nil
}
```

---

## 文档

| 资源 | 描述 |
|------|------|
| [入门指南](docs/01_getting-started.md) | 安装和第一步 |
| [配置](docs/02_configuration.md) | 客户端配置和预设 |
| [请求选项](docs/03_request-options.md) | 完整选项参考 |
| [错误处理](docs/04_error-handling.md) | 错误处理模式 |
| [HTTP 重定向](docs/05_redirects.md) | 重定向处理和跟踪 |
| [Cookie API](docs/06_cookie-api.md) | Cookie 管理 |
| [文件下载](docs/07_file-download.md) | 带进度的文件下载 |
| [请求检查](docs/08_request-inspection.md) | 请求/响应检查 |
| [并发安全](docs/09_concurrency-safety.md) | 线程安全保证 |
| [浏览器传输指纹](docs/10_tls-fingerprinting.md) | 浏览器传输指纹详解 |
| [中间件](docs/11_middleware.md) | 内置与自定义中间件 |
| [代理](docs/12_proxy.md) | 代理池、轮换、熔断 |
| [安全](SECURITY.md) | 安全特性和最佳实践 |

### 示例代码

24 个可运行示例，按从基础到高级排序。每个都是带 `//go:build examples` 标签的
独立 `package main`（不进入常规构建），需逐个运行：

```bash
go run -tags examples examples/01_basic_usage.go
```

| 类别 | 示例 |
|------|------|
| **基础** | [01_basic_usage](examples/01_basic_usage.go), [02_http_methods](examples/02_http_methods.go), [03_response_handling](examples/03_response_handling.go), [04_compression](examples/04_compression.go) |
| **核心功能** | [05_request_options](examples/05_request_options.go), [06_error_handling](examples/06_error_handling.go), [07_timeout_retry](examples/07_timeout_retry.go), [08_client_configuration](examples/08_client_configuration.go), [09_redirects](examples/09_redirects.go), [10_cookies_advanced](examples/10_cookies_advanced.go) |
| **有状态客户端** | [11_session](examples/11_session.go), [12_domain_client](examples/12_domain_client.go), [13_proxy_configuration](examples/13_proxy_configuration.go), [14_doh](examples/14_doh.go) |
| **高级** | [15_middleware](examples/15_middleware.go), [16_concurrent_requests](examples/16_concurrent_requests.go), [17_file_operations](examples/17_file_operations.go), [18_rest_api_client](examples/18_rest_api_client.go), [19_advanced_patterns](examples/19_advanced_patterns.go) |
| **安全** | [20_certificate_pinning](examples/20_certificate_pinning.go), [21_ssrf_protection](examples/21_ssrf_protection.go) |
| **流式传输** | [22_streaming_bodies](examples/22_streaming_bodies.go) |
| **指纹** | [23_tls_fingerprint](examples/23_tls_fingerprint.go) |
| **包级路由** | [24_package_routing](examples/24_package_routing.go) |

---

## 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件。

---

如果这个项目对你有帮助，请给一个 Star！
