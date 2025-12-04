# HTTPC - 生产级 Go HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](docs/security.md)
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
    resp, err := httpc.Get("https://api.example.com/users")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("状态码: %d\n", resp.StatusCode)

    // 带 JSON 和认证的 POST 请求
    user := map[string]string{"name": "张三", "email": "zhangsan@example.com"}
    resp, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("已创建: %s\n", resp.Body)
}
```

**[📖 查看更多示例](examples)** | **[🚀 入门指南](docs/getting-started.md)**

## 核心功能

### HTTP 方法

所有标准 HTTP 方法，API 简洁直观：

```go
// GET - 获取数据
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithBearerToken("token"),
)

// POST - 创建资源
resp, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
    httpc.WithBearerToken("token"),
)

// PUT - 完整更新
resp, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),
)

// PATCH - 部分更新
resp, err := httpc.Patch("https://api.example.com/users/123",
    httpc.WithJSON(map[string]string{"email": "new@example.com"}),
)

// DELETE - 删除资源
resp, err := httpc.Delete("https://api.example.com/users/123")

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
httpc.WithForm(formData)          // 表单数据
httpc.WithFile("file", "doc.pdf", content)  // 文件上传

// Cookie 设置
httpc.WithCookieString("session=abc123; token=xyz789")  // 解析 Cookie 字符串
httpc.WithCookieValue("name", "value")                  // 单个 Cookie
httpc.WithCookie(cookie)                                // http.Cookie 对象
httpc.WithCookies(cookies)                              // 多个 Cookie

// 超时和重试
httpc.WithTimeout(30*time.Second)
httpc.WithMaxRetries(3)
httpc.WithContext(ctx)

// 组合多个选项
resp, err := httpc.Post(url,
    httpc.WithJSON(data),
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(30*time.Second),
    httpc.WithMaxRetries(2),
)
```

**[📖 完整选项参考](docs/request-options.md)**

### 响应处理

```go
resp, err := httpc.Get(url)
if err != nil {
    log.Fatal(err)
}

// 状态检查
if resp.IsSuccess() {        // 2xx
    fmt.Println("成功！")
}

// 解析 JSON 响应
var result map[string]interface{}
if err := resp.JSON(&result); err != nil {
    log.Fatal(err)
}

// 访问响应数据
fmt.Printf("状态码: %d\n", resp.StatusCode)
fmt.Printf("响应体: %s\n", resp.Body)
fmt.Printf("耗时: %v\n", resp.Duration)
fmt.Printf("尝试次数: %d\n", resp.Attempts)

// 处理 Cookie
cookie := resp.GetCookie("session_id")
```

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
resp, err := httpc.Get(url)
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
if !resp.IsSuccess() {
    return fmt.Errorf("意外的状态码: %d", resp.StatusCode)
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
resp, err := httpc.Get(url)
```

### 自动重试

```go
// 在客户端级别配置
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.BackoffFactor = 2.0
client, err := httpc.New(config)

// 或针对单个请求
resp, err := httpc.Get(url, httpc.WithMaxRetries(5))
```

### Context 支持

```go
// 超时控制
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
resp, err := client.Get(url, httpc.WithContext(ctx))

// 取消控制
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel()
}()
resp, err := client.Get(url, httpc.WithContext(ctx))
```

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
resp, err := httpc.Get("https://api.example.com/data",
    httpc.WithCookieString("PSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc"),
)

// 设置单个 Cookie
resp, err = httpc.Get("https://api.example.com/data",
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
resp, err = httpc.Get("https://api.example.com/data", httpc.WithCookie(cookie))
```

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
        resp, _ := client.Get("https://api.example.com")
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

**[📖 安全指南](docs/security.md)**

## 文档

### 指南
- **[入门指南](docs/getting-started.md)** - 安装和第一步
- **[配置](docs/configuration.md)** - 客户端配置和预设
- **[请求选项](docs/request-options.md)** - 完整选项参考
- **[错误处理](docs/error-handling.md)** - 错误处理模式
- **[文件下载](docs/file-download.md)** - 带进度的文件下载
- **[安全性](docs/security.md)** - 安全特性和最佳实践

### 示例
- **[快速开始](examples/01_quickstart)** - 基本用法
- **[核心功能](examples/02_core_features)** - 请求头、认证、请求体格式
- **[高级功能](examples/03_advanced)** - 文件上传、下载、重试
- **[实战案例](examples/04_real_world)** - 完整的 API 客户端

## 贡献

欢迎贡献！对于重大更改，请先开启 issue 讨论。

## 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

**由 CyberGoDev 团队用 ❤️ 打造**
