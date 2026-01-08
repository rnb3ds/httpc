# HTTPC - 生产级 Go HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/cybergodev/httpc.svg)](https://pkg.go.dev/github.com/cybergodev/httpc)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Performance](https://img.shields.io/badge/performance-high%20performance-green.svg)](https://github.com/cybergodev/json)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](https://github.com/cybergodev/json)

一个高性能的 Go HTTP 客户端库，具有企业级安全性、零外部依赖和生产就绪的默认配置。专为需要可靠性、安全性和性能的应用程序而构建。

**[📖 English Documentation](README.md)** | **[📚 完整文档](docs)**

---

## 为什么选择 HTTPC？

- 🛡️ **默认安全** - TLS 1.2+、SSRF 防护、CRLF 注入防护
- ⚡ **高性能** - 连接池、HTTP/2、协程安全操作
- 📊 **内置弹性** - 智能重试，支持指数退避和抖动
- 🎯 **开发者友好** - 简洁的 API、函数式选项、全面的错误处理
- 🔧 **零依赖** - 纯 Go 标准库，无外部依赖
- 🚀 **生产就绪** - 经过实战检验的默认配置，广泛的测试覆盖


## 快速开始

```bash
go get -u github.com/cybergodev/httpc
```

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
    fmt.Printf("状态码: %d\n", result.StatusCode())

    // 带 JSON 和认证的 POST 请求
    user := map[string]string{"name": "张三", "email": "zhangsan@example.com"}
    result, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("已创建: %s\n", result.Body())
}
```

**[📖 查看更多示例](examples)** | **[🚀 入门指南](docs/getting-started.md)**

## 核心功能

### HTTP 方法

所有标准 HTTP 方法，API 简洁直观：

```go
// GET - 获取数据
result, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithBearerToken("token"),
)

// POST - 创建资源
result, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
    httpc.WithBearerToken("token"),
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

// 同时支持 HEAD、OPTIONS 和自定义方法
```

### 请求选项

使用函数式选项自定义请求（所有选项都以 `With` 开头）：

```go
// 请求头和认证
httpc.WithHeader("X-API-Key", "key")
httpc.WithBearerToken("token")
httpc.WithBasicAuth("user", "pass")

// 查询参数
httpc.WithQuery("page", 1)
httpc.WithQueryMap(map[string]interface{}{"page": 1, "limit": 20})

// 请求体
httpc.WithJSON(data)              // JSON 格式
httpc.WithXML(data)               // XML 格式
httpc.WithForm(formData)          // 表单数据
httpc.WithText("content")         // 纯文本
httpc.WithBinary(data, "image/png")  // 二进制数据及内容类型
httpc.WithFile("file", "doc.pdf", content)  // 文件上传

// Cookie 设置
httpc.WithCookieString("session=abc123; token=xyz789")  // 解析 Cookie 字符串
httpc.WithCookieValue("name", "value")                  // 单个 Cookie
httpc.WithCookie(cookie)                                // http.Cookie 对象
httpc.WithCookies(cookies)                              // 多个 Cookie

// 重定向控制
httpc.WithFollowRedirects(false)  // 禁用自动重定向跟随
httpc.WithMaxRedirects(5)         // 限制最大重定向次数 (0-50)

// 超时和重试
httpc.WithTimeout(30*time.Second)
httpc.WithMaxRetries(3)
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
body := result.Body()                // 响应体字符串
rawBody := result.RawBody()          // 响应体字节数组

// 详细响应信息
response := result.Response
fmt.Printf("状态: %d %s\n", response.StatusCode, response.Status)
fmt.Printf("内容长度: %d\n", response.ContentLength)
fmt.Printf("响应头: %v\n", response.Headers)
fmt.Printf("Cookie: %v\n", response.Cookies)

// 请求信息
request := result.Request
fmt.Printf("方法: %s\n", request.Method)
fmt.Printf("URL: %s\n", request.URL)
fmt.Printf("请求头: %v\n", request.Headers)

// 元数据
meta := result.Meta
fmt.Printf("耗时: %v\n", meta.Duration)
fmt.Printf("尝试次数: %d\n", meta.Attempts)
fmt.Printf("重定向次数: %d\n", meta.RedirectCount)
```

### 响应处理

```go
result, err := httpc.Get(url)
if err != nil {
    log.Fatal(err)
}

// 状态检查
if result.IsSuccess() {        // 2xx
    fmt.Println("成功！")
}

// 解析 JSON 响应
var data map[string]interface{}
if err := result.JSON(&data); err != nil {
    log.Fatal(err)
}

// 访问响应数据
fmt.Printf("状态码: %d\n", result.StatusCode())
fmt.Printf("响应体: %s\n", result.Body())
fmt.Printf("耗时: %v\n", result.Meta.Duration)
fmt.Printf("尝试次数: %d\n", result.Meta.Attempts)

// 处理 Cookie
cookie := result.GetCookie("session_id")
if result.HasCookie("session_id") {
    fmt.Println("找到会话 Cookie")
}

// 访问请求 Cookie
requestCookies := result.RequestCookies()
requestCookie := result.GetRequestCookie("auth_token")

// 访问详细响应信息
fmt.Printf("内容长度: %d\n", result.Response.ContentLength)
fmt.Printf("响应头: %v\n", result.Response.Headers)
fmt.Printf("请求头: %v\n", result.Request.Headers)

// 保存响应到文件
err = result.SaveToFile("response.html")
```

### 自动响应解压缩

HTTPC 自动检测并解压缩 HTTP 响应：

```go
// 请求压缩响应
result, err := httpc.Get("https://api.example.com/data",
    httpc.WithHeader("Accept-Encoding", "gzip, deflate"),
)

// 响应自动解压缩
fmt.Printf("解压后的内容: %s\n", result.Body())
fmt.Printf("原始编码: %s\n", result.Response.Headers.Get("Content-Encoding"))
```

**支持的编码：**
- ✅ **gzip** - 完全支持 (compress/gzip)
- ✅ **deflate** - 完全支持 (compress/flate)

**注意：** 当服务器发送 `Content-Encoding` 头时，解压缩是自动的。库会透明地处理这一过程，因此您始终收到解压后的内容。

### 文件下载

```go
// 简单下载
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
fmt.Printf("已下载: %s，平均速度 %s\n", 
    httpc.FormatBytes(result.BytesWritten),
    httpc.FormatSpeed(result.AverageSpeed))

// 带进度跟踪的下载
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r进度: %.1f%% - %s", percentage, httpc.FormatSpeed(speed))
}
result, err := httpc.DownloadWithOptions(url, opts)

// 断点续传
opts.ResumeDownload = true
result, err := httpc.DownloadWithOptions(url, opts)

// 带认证的下载
result, err := httpc.DownloadFile(url, "file.zip",
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(5*time.Minute),
)
```

**[📖 文件下载指南](docs/file-download.md)**

## 配置

### 使用预设快速开始

```go
// Default - 生产环境均衡配置（推荐）
client, err := httpc.New()

// Secure - 最大安全性（严格验证，最少重试）
client, err := httpc.NewSecure()

// Performance - 高吞吐量优化
client, err := httpc.NewPerformance()

// Minimal - 轻量级简单请求
client, err := httpc.NewMinimal()

// Testing - 开发环境宽松配置（切勿在生产环境使用）
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
// 创建可复用的客户端
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()  // 始终关闭以释放资源

// 或使用包级函数（自动管理）
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

// 或针对单个请求
result, err := httpc.Get(url, httpc.WithMaxRetries(5))
```

### Context 支持

```go
// 超时控制
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
result, err := client.Get(url, httpc.WithContext(ctx))

// 取消控制
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

// 禁用特定请求的重定向
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
// 自动 Cookie 处理
config := httpc.DefaultConfig()
config.EnableCookies = true
client, err := httpc.New(config)

// 登录设置 Cookie
client.Post("https://example.com/login", httpc.WithForm(credentials))

// 后续请求自动包含 Cookie
client.Get("https://example.com/profile")

// 手动 Cookie 设置
// 解析 Cookie 字符串（来自浏览器开发者工具或服务器响应）
result, err := httpc.Get("https://api.example.com/data",
    httpc.WithCookieString("PSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc"),
)

// 设置单个 Cookie
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

**[📖 Cookie API 参考](docs/cookie-api-reference.md)**

### 域客户端 - 自动状态管理

对于需要向同一域发起多个请求的应用，`DomainClient` 提供自动的 Cookie 和 Header 管理：

```go
// 创建域专用客户端
client, err := httpc.NewDomain("https://api.example.com")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// 第一个请求 - 服务器设置 Cookie
resp1, err := client.Get("/login",
    httpc.WithJSON(credentials),
)

// resp1 中的 Cookie 自动保存并在后续请求中发送
resp2, err := client.Get("/profile")  // Cookie 自动包含

// 设置持久化 Header（所有请求都会发送）
client.SetHeader("Authorization", "Bearer "+token)
client.SetHeader("X-API-Key", "your-api-key")

// 所有后续请求都包含这些 Header
resp3, err := client.Get("/data")  // Header + Cookie 自动包含

// 按请求覆盖（不影响持久化状态）
resp4, err := client.Get("/special",
    httpc.WithHeader("Accept", "application/xml"),  // 仅此请求覆盖
)

// 手动 Cookie 管理
client.SetCookie(&http.Cookie{Name: "session", Value: "abc123"})
client.SetCookies([]*http.Cookie{
    {Name: "pref", Value: "dark"},
    {Name: "lang", Value: "zh"},
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

**真实场景示例 - 登录流程：**

```go
client, _ := httpc.NewDomain("https://api.example.com")
defer client.Close()

// 步骤 1：登录（服务器设置会话 Cookie）
loginResp, _ := client.Post("/auth/login",
    httpc.WithJSON(map[string]string{
        "username": "user@example.com",
        "password": "secret",
    }),
)

// 步骤 2：提取令牌并设置为持久化 Header
var loginData map[string]string
loginResp.JSON(&loginData)
client.SetHeader("Authorization", "Bearer "+loginData["token"])

// 步骤 3：进行 API 调用（Cookie + 认证 Header 自动发送）
profileResp, _ := client.Get("/api/user/profile")
dataResp, _ := client.Get("/api/user/data")
settingsResp, _ := client.Put("/api/user/settings",
    httpc.WithJSON(newSettings),
)

// 所有请求自动包含：
// - 登录响应中的会话 Cookie
// - Authorization Header
// - 任何其他持久化的 Header/Cookie
```

**主要特性：**
- **自动 Cookie 持久化** - 响应中的 Cookie 被保存并在后续请求中发送
- **自动 Header 持久化** - 设置一次 Header，在所有请求中使用
- **按请求覆盖** - 使用 `WithCookies()` 和 `WithHeaderMap()` 覆盖特定请求
- **线程安全** - 所有操作都是协程安全的
- **手动控制** - 完整的 API 用于检查和修改状态

**[📖 查看完整示例](examples/domain_client_example.go)**

## 安全性与性能

### 安全特性
- **默认 TLS 1.2+** - 现代加密标准
- **SSRF 防护** - DNS 解析前后验证，阻止私有 IP
- **CRLF 注入防护** - 请求头和 URL 验证
- **输入验证** - 全面验证所有用户输入
- **路径遍历防护** - 安全的文件操作
- **可配置限制** - 响应大小、超时、连接数限制

### 性能优化
- **连接池** - 高效的连接复用，支持每主机连接数限制
- **HTTP/2 支持** - 多路复用提升性能
- **协程安全** - 所有操作线程安全，使用原子操作
- **智能重试** - 指数退避加抖动，减少服务器负载
- **内存高效** - 可配置限制防止内存耗尽

### 并发安全

HTTPC 从设计之初就考虑了并发使用：

```go
// ✅ 安全：在多个协程间共享单个客户端
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
- ✅ 所有 `Client` 方法都可安全并发使用
- ✅ 包级函数（`Get`、`Post` 等）安全地使用共享的默认客户端
- ✅ 响应对象在返回后可从多个协程读取
- ✅ 内部指标和连接池使用原子操作
- ✅ Config 在客户端创建时深拷贝，防止修改问题

**最佳实践：**
- 创建一个客户端并在整个应用中复用
- 不要在传递给 `New()` 后修改 `Config`
- 响应对象可安全读取，但不应并发修改

**测试：** 运行 `make test-race` 验证代码中的无竞态操作。

### 性能基准测试

HTTPC 专为高性能设计，最小化内存分配：

```bash
# 运行基准测试
go test -bench=. -benchmem ./...

# 示例结果（实际结果可能有所不同）：
BenchmarkClient_Get-8           5000    250000 ns/op    1024 B/op    8 allocs/op
BenchmarkClient_Post-8          4000    300000 ns/op    1536 B/op   12 allocs/op
BenchmarkClient_Concurrent-8   10000    150000 ns/op     512 B/op    4 allocs/op
```

**性能特性：**
- **零拷贝操作** - 尽可能避免数据复制
- **连接池** - 可配置限制的连接复用
- **热路径优化** - 最小化内存分配
- **原子操作** - 线程安全的计数器
- **高效字符串操作** - 预分配缓冲区

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
- **[安全性](SECURITY.md)** - 安全特性和最佳实践

### 示例
- **[快速开始](examples/01_quickstart)** - 基本用法
- **[核心功能](examples/02_core_features)** - 请求头、认证、请求体格式
- **[高级功能](examples/03_advanced)** - 文件上传、下载、重试
- **[实战案例](examples/04_real_world)** - 完整的 API 客户端

## 贡献

欢迎贡献！对于重大更改，请先开启 issue 讨论或联系我们。

## 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

**由 CyberGoDev 团队用 ❤️ 打造**