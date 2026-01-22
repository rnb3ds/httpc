# HTTPC - Go 语言生产级 HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Performance](https://img.shields.io/badge/performance-high%20performance-green.svg)](https://github.com/cybergodev/json)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](https://github.com/cybergodev/json)

一款高性能的 Go 语言 HTTP 客户端库，具备企业级安全、零外部依赖和生产级默认配置。为追求可靠性、安全性和性能的应用而构建。

**[📖 English Documentation](README.md)** | **[📚 完整文档](docs)**

---

## ✨ 核心特性

- 🛡️ **默认安全** - TLS 1.2+、SSRF 防护、CRLF 注入防护
- ⚡ **高性能** - 连接池、HTTP/2、goroutine 安全操作
- 📊 **内置弹性** - 智能重试、指数退避和抖动
- 🎯 **开发友好** - 清晰的 API、函数选项、全面的错误处理
- 🔧 **零依赖** - 纯 Go 标准库，无外部包
- 🚀 **生产就绪** - 经过实战检验的默认配置、广泛的测试覆盖

## 📦 安装

```bash
go get -u github.com/cybergodev/httpc
```

## 🚀 快速开始

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // 简单的 GET 请求
    result, err := httpc.Get("https://api.example.com/users")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %d\n", result.StatusCode())

    // POST JSON 数据和认证
    user := map[string]string{"name": "John", "email": "john@example.com"}
    result, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created: %s\n", result.Body())
}
```

> **默认请求头**: 使用 `httpc.DefaultConfig()` 默认配置，默认使用 `User-Agent: httpc/1.0` 请求头。可自定义设置默认请求头：
> - **User-Agent**: 设置 `config.UserAgent` 或使用 `httpc.WithUserAgent("your-custom-agent")`
> - **自定义请求头**: 在创建客户端时设置 `config.Headers` 映射以添加客户端级别的默认请求头
> - **每次请求**: 使用 `httpc.WithHeader()` 或 `httpc.WithHeaderMap()` 为特定请求覆盖默认值

**[📖 查看更多示例](examples)** | **[🚀 入门指南](docs/getting-started.md)**

## 📖 核心功能

### HTTP 方法

所有标准 HTTP 方法，提供清晰直观的 API：

```go
// GET - 获取数据
result, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithBearerToken("token"),
)

// POST - 创建资源
result, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
    httpc.WithBearerToken("your-token"),
)

// PUT - 完整更新
result, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),
)

// PATCH - 部分更新
result, err := httpc.Patch("https://api.example.com/users/123",
    httpc.WithJSON(map[string]string{"email": "new@example.com"}),
)

// DELETE - 删除资源
result, err := httpc.Delete("https://api.example.com/users/123")

// HEAD、OPTIONS 和自定义方法也支持
```

### 请求选项

使用函数选项自定义请求（所有选项以 `With` 开头）：

```go
// Headers 和认证
httpc.WithHeader("x-api-key", "key")
httpc.WithBearerToken("token")
httpc.WithBasicAuth("user", "pass")

// 查询参数
httpc.WithQuery("page", 1)
httpc.WithQueryMap(map[string]interface{}{"page": 1, "limit": 20})

// 请求体
httpc.WithJSON(data)              // JSON 请求体
httpc.WithXML(data)               // XML 请求体
httpc.WithForm(formData)          // 表单数据（URL 编码）
httpc.WithFormData(data)          // 多部分表单数据（用于文件上传）
httpc.WithText("content")         // 纯文本
httpc.WithBinary(data, "image/png")  // 二进制数据（带内容类型）
httpc.WithFile("file", "doc.pdf", content)  // 单个文件上传

// Cookies
httpc.WithCookieString("session=abc123; token=xyz789")  // 解析 cookie 字符串
httpc.WithCookieValue("name", "value")                  // 单个 cookie
httpc.WithCookie(cookie)                                // http.Cookie 对象
httpc.WithCookies(cookies)                              // 多个 cookies

// 重定向
httpc.WithFollowRedirects(false)  // 禁用自动跟随重定向
httpc.WithMaxRedirects(5)         // 限制最大重定向次数（0-50）

// 超时和重试
httpc.WithTimeout(30*time.Second)
httpc.WithMaxRetries(3)         // 允许 0-10 次重试
httpc.WithContext(ctx)

// 组合多个选项
result, err := httpc.Post(url,
    httpc.WithJSON(data),
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(30*time.Second),
    httpc.WithMaxRetries(2),
)
```

**[📖 完整选项参考](docs/request-options.md)**

### 响应数据访问

HTTPC 返回一个 `Result` 对象，提供对请求和响应信息的结构化访问：

```go
result, err := httpc.Get("https://api.example.com/users/123")
if err != nil {
    log.Fatal(err)
}

// 快速访问方法
statusCode := result.StatusCode()    // HTTP 状态码
body := result.Body()                // 响应体（字符串）
rawBody := result.RawBody()          // 响应体（[]byte）

// 详细响应信息
response := result.Response
fmt.Printf("Status: %d %s\n", response.StatusCode, response.Status)
fmt.Printf("Content-Length: %d\n", response.ContentLength)
fmt.Printf("Headers: %v\n", response.Headers)
fmt.Printf("Cookies: %v\n", response.Cookies)

// 请求信息
request := result.Request
fmt.Printf("Request Headers: %v\n", request.Headers)
fmt.Printf("Request Cookies: %v\n", request.Cookies)

// 元数据
meta := result.Meta
fmt.Printf("Duration: %v\n", meta.Duration)
fmt.Printf("Attempts: %d\n", meta.Attempts)
fmt.Printf("Redirects: %d\n", meta.RedirectCount)
```

### 响应处理

```go
result, err := httpc.Get(url)
if err != nil {
    log.Fatal(err)
}

// 状态检查
if result.IsSuccess() {        // 2xx
    fmt.Println("Success!")
}

// 解析 JSON 响应
var data map[string]interface{}
if err := result.JSON(&data); err != nil {
    log.Fatal(err)
}

// 访问响应数据
fmt.Printf("Status: %d\n", result.StatusCode())
fmt.Printf("Body: %s\n", result.Body())
fmt.Printf("Duration: %v\n", result.Meta.Duration)
fmt.Printf("Attempts: %d\n", result.Meta.Attempts)

// 使用响应 cookies
cookie := result.GetCookie("session_id")
if result.HasCookie("session_id") {
    fmt.Println("找到 Session cookie")
}
responseCookies := result.ResponseCookies()  // 获取所有响应 cookies

// 访问请求 cookies
requestCookies := result.RequestCookies()  // 获取所有请求 cookies
requestCookie := result.GetRequestCookie("auth_token")

// 结果的字符串表示
fmt.Println(result.String())

// 访问详细响应信息
fmt.Printf("Content-Length: %d\n", result.Response.ContentLength)
fmt.Printf("Response Headers: %v\n", result.Response.Headers)
fmt.Printf("Request Headers: %v\n", result.Request.Headers)

// 保存响应到文件
err = result.SaveToFile("response.html")
```

### 自动响应解压缩

HTTPC 自动检测并解压缩压缩的 HTTP 响应：

```go
// 请求压缩响应
result, err := httpc.Get("https://api.example.com/data",
    httpc.WithHeader("Accept-Encoding", "gzip, deflate"),
)

// 响应自动解压缩
fmt.Printf("解压缩后的内容: %s\n", result.Body())
fmt.Printf("原始编码: %s\n", result.Response.Headers.Get("Content-Encoding"))
```

**支持的编码：**
- ✅ **gzip** - 完全支持（compress/gzip）
- ✅ **deflate** - 完全支持（compress/flate）
- ❌ **br** (Brotli) - 不支持
- ❌ **compress** (LZW) - 不支持

**注意：** 当服务器发送 `Content-Encoding` 头时，解压缩是自动的。库透明地处理这个过程，因此您始终接收解压缩后的内容。

### 文件下载

文件下载包含内置的安全保护功能：
- **UNC 路径阻止** - 防止访问 Windows 网络路径
- **系统路径保护** - 阻止写入关键系统目录
- **路径遍历检测** - 防止目录逃逸攻击
- **恢复支持** - 自动恢复中断的下载

```go
// 简单下载
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("已下载: %s，速度 %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// 带进度跟踪的下载
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r进度: %.1f%% - %s", percentage, httpc.FormatSpeed(speed))
}
result, err := httpc.DownloadWithOptions(url, opts)

// 恢复中断的下载
opts.ResumeDownload = true
result, err := httpc.DownloadWithOptions(url, opts)

// 带认证的下载
result, err := httpc.DownloadFile(url, "file.zip",
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(5*time.Minute),
)
```

**[📖 文件下载指南](docs/file-download.md)**

## 🔧 配置

### 使用预设快速开始

```go
// Default - 生产环境平衡配置（推荐）
client, err := httpc.New()

// Secure - 最大安全性（严格验证、最少重试）
client, err := httpc.NewSecure()

// Performance - 高吞吐量优化
client, err := httpc.NewPerformance()

// Minimal - 轻量级简单请求
client, err := httpc.NewMinimal()

// Testing - 开发测试宽松配置（切勿用于生产环境）
// 警告：禁用 TLS 验证，降低安全性
client, err := httpc.New(httpc.TestingConfig())
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
client, err := httpc.New(config)
```

**[📖 配置指南](docs/configuration.md)**

## 错误处理

```go
result, err := httpc.Get(url)
if err != nil {
    // 检查特定错误类型
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Status)
    }

    // 检查超时
    if strings.Contains(err.Error(), "timeout") {
        return fmt.Errorf("请求超时")
    }

    return err
}

// 检查响应状态
// 注意：HTTPC 对所有状态码（包括 4xx 和 5xx）都返回 Result
// HTTPError 不会自动返回给非 2xx 状态码
if !result.IsSuccess() {
    return fmt.Errorf("意外的状态码: %d", result.StatusCode())
}

// 访问详细错误信息
if result.IsClientError() {
    fmt.Printf("客户端错误 (4xx): %d\n", result.StatusCode())
} else if result.IsServerError() {
    fmt.Printf("服务器错误 (5xx): %d\n", result.StatusCode())
}
```

**[📖 错误处理指南](docs/error-handling.md)**

## 高级功能

### 客户端生命周期管理

```go
// 创建可重用的客户端
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()  // 始终关闭以释放资源

// 或使用包级别函数（自动管理）
defer httpc.CloseDefaultClient()
result, err := httpc.Get(url)
```

### 自动重试

```go
// 在客户端级别配置
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.BackoffFactor = 2.0
client, err := httpc.New(config)

// 或每次请求配置
result, err := httpc.Get(url, httpc.WithMaxRetries(5))
```

### Context 支持

```go
// 超时
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
result, err := client.Get(url, httpc.WithContext(ctx))

// 取消
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel()
}()
result, err := client.Get(url, httpc.WithContext(ctx))
```

### HTTP 重定向

```go
// 自动跟随重定向（默认）
result, err := httpc.Get("https://example.com/redirect")
fmt.Printf("跟随了 %d 次重定向\n", result.Meta.RedirectCount)

// 为特定请求禁用重定向
result, err := httpc.Get(url, httpc.WithFollowRedirects(false))
if result.IsRedirect() {
    fmt.Printf("重定向到: %s\n", result.Response.Headers.Get("Location"))
}

// 限制重定向次数
result, err := httpc.Get(url, httpc.WithMaxRedirects(5))

// 跟踪重定向链
for i, url := range result.Meta.RedirectChain {
    fmt.Printf("%d. %s\n", i+1, url)
}
```

**[📖 重定向指南](docs/redirects.md)**

### Cookie 管理

```go
// 自动 cookie 处理
// 注意：DefaultConfig() 中 EnableCookies 默认为 false
config := httpc.DefaultConfig()
config.EnableCookies = true  // 必须显式启用以自动处理 cookies
client, err := httpc.New(config)

// Login 设置 cookies
client.Post("https://example.com/login", httpc.WithForm(credentials))

// 后续请求自动包含 cookies
client.Get("https://example.com/profile")

// 手动设置 cookie
// 解析 cookie 字符串（从浏览器开发工具或服务器响应）
result, err := httpc.Get("https://api.example.com/data",
    httpc.WithCookieString("PSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc"),
)

// 设置单个 cookies
result, err = httpc.Get("https://api.example.com/data",
    httpc.WithCookieValue("session", "abc123"),
    httpc.WithCookieValue("token", "xyz789"),
)

// 使用 http.Cookie 对象进行高级设置
cookie := &http.Cookie{
    Name:     "secure_session",
    Value:    "encrypted_value",
    Secure:   true,
    HttpOnly: true,
    SameSite: http.SameSiteStrictMode,
}
result, err = httpc.Get("https://api.example.com/data", httpc.WithCookie(cookie))
```

**注意：** 对于需要跨多个请求自动管理 cookie 状态的情况，建议使用 `DomainClient`，它会自动处理 cookie 持久化。

**[📖 Cookie API 参考](docs/cookie-api-reference.md)**

### Domain Client - 自动状态管理

对于向同一域发送多个请求的应用，`DomainClient` 提供自动的 Cookie 和 Header 管理：

```go
// 创建域特定客户端
client, err := httpc.NewDomain("https://api.example.com")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// 请求首页
resp0, err := client.Get("/")

// 第一个请求 - 服务器设置 cookies
resp1, err := client.Post("/login",
    httpc.WithJSON(credentials),
)

// Cookies 从 resp1 自动保存并在后续请求中发送
resp2, err := client.Get("/profile")  // Cookies 自动包含

// 设置持久 headers（用于所有请求）
client.SetHeader("Authorization", "Bearer "+token)
client.SetHeader("x-api-key", "your-api-key")

// 一次设置多个 headers
err = client.SetHeaders(map[string]string{
    "Authorization": "Bearer " + token,
    "x-api-key": "your-api-key",
})

// 所有后续请求包含这些 headers
resp3, err := client.Get("/data")  // Headers + Cookies 自动包含

// 每次请求覆盖（不影响持久状态）
resp4, err := client.Get("/special",
    httpc.WithHeader("Accept", "application/xml"),  // 仅对此请求覆盖
)

// 手动 cookie 管理
client.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})
client.SetCookies([]*http.Cookie{
    {Name: "pref", Value: "dark"},
    {Name: "lang", Value: "en"},
})

// 查询状态
cookies := client.GetCookies()
headers := client.GetHeaders()
sessionCookie := client.GetCookie("session")

// 清除状态
client.DeleteCookie("session")
client.DeleteHeader("X-API-Key")
client.ClearCookies()
client.ClearHeaders()
```

**真实世界示例 - 登录流程：**

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// 步骤 1：登录（服务器设置 session cookie）
loginResp, _ := client.Post("/auth/login",
    httpc.WithJSON(map[string]string{
        "username": "user@example.com",
        "password": "secret",
    }),
)

// 步骤 2：提取 token 并设置为持久 header
var loginData map[string]string
loginResp.JSON(&loginData)
client.SetHeader("Authorization", "Bearer "+loginData["token"])

// 步骤 3：进行 API 调用（cookies + auth header 自动发送）
profileResp, _ := client.Get("/api/user/profile")
dataResp, _ := client.Get("/api/user/data")
settingsResp, _ := client.Put("/api/user/settings",
    httpc.WithJSON(newSettings),
)

// 所有请求自动包含：
// - 登录响应中的 session cookies
// - Authorization header
// - 任何其他持久 headers/cookies
```

**使用 DomainClient 进行文件下载：**

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// 设置认证 header（用于所有请求，包括下载）
client.SetHeader("Authorization", "Bearer "+token)

// 带自动状态管理的简单下载
result, err := client.DownloadFile("/files/report.pdf", "downloads/report.pdf")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("已下载: %s，速度 %s/s\n",
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// 带进度跟踪的下载
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r进度: %.1f%% - %s", percentage, httpc.FormatSpeed(speed))
}
result, err = client.DownloadWithOptions("/files/large-file.zip", opts)
```

**核心特性：**
- **自动 Cookie 持久化** - 响应中的 cookies 被保存并在后续请求中发送
- **自动 Header 持久化** - 设置一次 headers，所有请求中使用
- **每次请求覆盖** - 使用 `WithCookies()` 和 `WithHeaderMap()` 为特定请求覆盖
- **线程安全** - 所有操作都是 goroutine 安全的
- **手动控制** - 完整的 API 用于检查和修改状态
- **文件下载支持** - 下载文件时自动状态管理（cookies/headers）
- **自动启用 Cookie** - `NewDomain()` 会自动启用 cookies，无论配置如何

**[📖 查看完整示例](examples/03_advanced/domain_client.go)**

### 代理配置

HTTPC 支持灵活的代理配置，提供三种模式：

#### 代理优先级

```
优先级 1: ProxyURL（手动代理）        - 最高优先级
优先级 2: EnableSystemProxy（自动检测系统代理）
优先级 3: 直连（无代理）              - 默认
```

#### 1. 手动代理（最高优先级）

直接指定代理 URL。此优先级高于所有其他代理设置。

```go
// 直接指定代理
config := &httpc.Config{
    ProxyURL: "http://127.0.0.1:1234",
    Timeout:  30 * time.Second,
}
client, err := httpc.New(config)

// SOCKS5 代理
config := &httpc.Config{
    ProxyURL: "socks5://127.0.0.1:1080",
}
client, err := httpc.New(config)

// 带认证的企业代理
config := &httpc.Config{
    ProxyURL: "http://user:pass@proxy.company.com:8080",
}
client, err := httpc.New(config)
```

#### 2. 系统代理检测

启用自动检测系统代理设置。包括：

- **Windows**: 从注册表读取 (`HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
- **macOS**: 从系统偏好设置读取
- **Linux**: 从系统设置读取
- **所有平台**: 回退到环境变量 (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)

```go
// 启用系统代理检测
config := &httpc.Config{
    EnableSystemProxy: true,
}
client, err := httpc.New(config)
// 如果配置了系统代理，将自动使用
```

**环境变量：**

```bash
# 通过环境变量设置代理
export HTTP_PROXY=http://127.0.0.1:1234
export HTTPS_PROXY=http://127.0.0.1:1234
export NO_PROXY=localhost,127.0.0.1,.local.com

# 然后在代码中启用系统代理检测
config := &httpc.Config{
    EnableSystemProxy: true,  // 将从环境变量读取
}
```

#### 3. 直连模式（默认）

当 `ProxyURL` 为空且 `EnableSystemProxy` 为 `false` 时，直接连接而不使用任何代理。

```go
// 默认行为 - 直连
client, err := httpc.New()

// 显式直连
config := &httpc.Config{
    // ProxyURL 为空（默认）
    // EnableSystemProxy 为 false（默认）
}
client, err := httpc.New(config)
```

**[📖 查看完整示例](examples/03_advanced/proxy_configuration.go)**

## 安全与性能

### 安全特性
- **TLS 1.2+ 默认** - 现代加密标准
- **SSRF 防护** - DNS 前后验证阻止私有 IP
- **CRLF 注入防护** - Header 和 URL 验证
- **输入验证** - 所有用户输入的全面验证
- **路径遍历防护** - 安全的文件操作
- **可配置限制** - 响应大小、超时、连接限制

### 性能优化
- **连接池** - 高效的连接重用，每主机限制
- **HTTP/2 支持** - 多路复用以提高性能
- **Goroutine 安全** - 所有操作线程安全，使用原子操作
- **智能重试** - 带抖动的指数退避减少服务器负载
- **内存高效** - 可配置限制防止内存耗尽

### 并发安全

HTTPC 从根本上设计用于并发使用：

```go
// ✅ 安全：在 goroutine 之间共享单个客户端
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

**线程安全保证：**
- ✅ 所有 `Client` 方法并发使用安全
- ✅ 包级别函数（`Get`、`Post` 等）安全地使用共享默认客户端
- ✅ 响应对象返回后可从多个 goroutine 读取
- ✅ 内部指标和连接池使用原子操作
- ✅ 配置在客户端创建时深拷贝以防止修改问题

**最佳实践：**
- 创建一个客户端并在整个应用中重用
- 不要在传递给 `New()` 后修改 `Config`
- 响应对象可以安全读取但不应该并发修改

**测试：** 运行 `make test-race` 验证代码中的无竞争操作。

**[📖 安全指南](SECURITY.md)**

## 文档

### 指南
- **[入门指南](docs/getting-started.md)** - 安装和第一步
- **[配置](docs/configuration.md)** - 客户端配置和预设
- **[请求选项](docs/request-options.md)** - 完整选项参考
- **[错误处理](docs/error-handling.md)** - 错误处理模式
- **[文件下载](docs/file-download.md)** - 带进度的文件下载
- **[HTTP 重定向](docs/redirects.md)** - 重定向处理和跟踪
- **[请求检查](docs/request-inspection.md)** - 检查请求详情
- **[安全](SECURITY.md)** - 安全特性和最佳实践

### 示例
- **[快速开始](examples/01_quickstart)** - 基本用法
- **[核心功能](examples/02_core_features)** - Headers、认证、body 格式
- **[高级](examples/03_advanced)** - 文件上传、下载、重试

## 🤝 贡献

欢迎贡献、问题报告和建议！

## 📄 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

**用心为 Go 社区打造** ❤️ | 如果这个项目对您有帮助，请给它一个 ⭐️ Star！
