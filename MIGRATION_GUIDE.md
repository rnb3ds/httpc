# HTTPC 迁移指南

## 🎯 概述

本指南帮助您从其他 HTTP 客户端库迁移到 httpc，提供详细的对比和迁移步骤。

## 📋 目录

1. [从 net/http 迁移](#从-nethttp-迁移)
2. [从 resty 迁移](#从-resty-迁移)
3. [从 fasthttp 迁移](#从-fasthttp-迁移)
4. [从 go-resty 迁移](#从-go-resty-迁移)
5. [配置映射表](#配置映射表)
6. [常见迁移问题](#常见迁移问题)
7. [迁移检查清单](#迁移检查清单)

## 🌐 从 net/http 迁移

### 基本请求迁移

#### 原代码 (net/http)
```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

func main() {
    // 创建客户端
    client := &http.Client{
        Timeout: 30 * time.Second,
    }
    
    // GET 请求
    resp, err := client.Get("https://api.example.com/users")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", string(body))
    
    // POST JSON 请求
    user := map[string]interface{}{
        "name":  "John Doe",
        "email": "john@example.com",
    }
    
    jsonData, _ := json.Marshal(user)
    req, _ := http.NewRequest("POST", "https://api.example.com/users", bytes.NewBuffer(jsonData))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer token123")
    
    resp, err = client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    
    body, _ = io.ReadAll(resp.Body)
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", string(body))
}
```

#### 迁移后 (httpc)
```go
package main

import (
    "fmt"
    "time"
    "github.com/cybergodev/httpc"
)

func main() {
    // 创建客户端
    client, err := httpc.New()
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    // GET 请求
    resp, err := client.Get("https://api.example.com/users",
        httpc.WithTimeout(30*time.Second),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body) // 自动读取
    
    // POST JSON 请求
    user := map[string]interface{}{
        "name":  "John Doe",
        "email": "john@example.com",
    }
    
    resp, err = client.Post("https://api.example.com/users",
        httpc.WithJSON(user),                    // 自动序列化
        httpc.WithBearerToken("token123"),       // 便捷认证
        httpc.WithTimeout(30*time.Second),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)
}
```

### 高级功能迁移

#### 自定义传输层 (net/http → httpc)

**原代码:**
```go
transport := &http.Transport{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
    IdleConnTimeout:     90 * time.Second,
    TLSHandshakeTimeout: 10 * time.Second,
}

client := &http.Client{
    Transport: transport,
    Timeout:   30 * time.Second,
}
```

**迁移后:**
```go
config := httpc.DefaultConfig()
config.MaxIdleConns = 100
config.MaxConnsPerHost = 10
config.Timeout = 30 * time.Second

client, err := httpc.New(config)
```

#### Cookie 处理 (net/http → httpc)

**原代码:**
```go
jar, _ := cookiejar.New(nil)
client := &http.Client{
    Jar: jar,
}

// 手动添加 Cookie
req, _ := http.NewRequest("GET", url, nil)
req.AddCookie(&http.Cookie{
    Name:  "session",
    Value: "abc123",
})
```

**迁移后:**
```go
// 自动 Cookie 管理（默认启用）
client, _ := httpc.New()

// 手动添加 Cookie
resp, err := client.Get(url,
    httpc.WithCookieValue("session", "abc123"),
)
```

## 🚀 从 resty 迁移

### 基本用法迁移

#### 原代码 (resty)
```go
package main

import (
    "fmt"
    "github.com/go-resty/resty/v2"
)

func main() {
    client := resty.New()
    
    // GET 请求
    resp, err := client.R().
        SetHeader("Accept", "application/json").
        SetAuthToken("token123").
        Get("https://api.example.com/users")
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode())
    fmt.Printf("Body: %s\n", resp.String())
    
    // POST JSON 请求
    user := map[string]interface{}{
        "name":  "John Doe",
        "email": "john@example.com",
    }
    
    resp, err = client.R().
        SetHeader("Content-Type", "application/json").
        SetBody(user).
        SetAuthToken("token123").
        Post("https://api.example.com/users")
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode())
    fmt.Printf("Body: %s\n", resp.String())
}
```

#### 迁移后 (httpc)
```go
package main

import (
    "fmt"
    "github.com/cybergodev/httpc"
)

func main() {
    client, err := httpc.New()
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    // GET 请求
    resp, err := client.Get("https://api.example.com/users",
        httpc.WithJSONAccept(),              // 等同于 SetHeader("Accept", "application/json")
        httpc.WithBearerToken("token123"),   // 等同于 SetAuthToken
    )
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)
    
    // POST JSON 请求
    user := map[string]interface{}{
        "name":  "John Doe",
        "email": "john@example.com",
    }
    
    resp, err = client.Post("https://api.example.com/users",
        httpc.WithJSON(user),                // 等同于 SetBody + Content-Type
        httpc.WithBearerToken("token123"),
    )
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)
}
```

### 高级功能迁移

#### 重试配置 (resty → httpc)

**原代码:**
```go
client := resty.New()
client.SetRetryCount(3).
    SetRetryWaitTime(5 * time.Second).
    SetRetryMaxWaitTime(20 * time.Second)

resp, err := client.R().Get(url)
```

**迁移后:**
```go
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.RetryDelay = 5 * time.Second
// httpc 自动计算最大延迟

client, err := httpc.New(config)

resp, err := client.Get(url)
```

#### 文件上传 (resty → httpc)

**原代码:**
```go
resp, err := client.R().
    SetFile("file", "/path/to/file.pdf").
    SetFormData(map[string]string{
        "title": "My Document",
    }).
    Post("https://api.example.com/upload")
```

**迁移后:**
```go
fileContent, _ := os.ReadFile("/path/to/file.pdf")

formData := &httpc.FormData{
    Fields: map[string]string{
        "title": "My Document",
    },
    Files: map[string]*httpc.FileData{
        "file": {
            Filename: "file.pdf",
            Content:  fileContent,
        },
    },
}

resp, err := client.Post("https://api.example.com/upload",
    httpc.WithFormData(formData),
)
```

## ⚡ 从 fasthttp 迁移

### 基本请求迁移

#### 原代码 (fasthttp)
```go
package main

import (
    "fmt"
    "github.com/valyala/fasthttp"
)

func main() {
    // GET 请求
    req := fasthttp.AcquireRequest()
    resp := fasthttp.AcquireResponse()
    defer fasthttp.ReleaseRequest(req)
    defer fasthttp.ReleaseResponse(resp)
    
    req.SetRequestURI("https://api.example.com/users")
    req.Header.SetMethod("GET")
    req.Header.Set("Authorization", "Bearer token123")
    
    err := fasthttp.Do(req, resp)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode())
    fmt.Printf("Body: %s\n", resp.Body())
}
```

#### 迁移后 (httpc)
```go
package main

import (
    "fmt"
    "github.com/cybergodev/httpc"
)

func main() {
    client, err := httpc.New()
    if err != nil {
        panic(err)
    }
    defer client.Close()
    
    // GET 请求
    resp, err := client.Get("https://api.example.com/users",
        httpc.WithBearerToken("token123"),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
    fmt.Printf("Body: %s\n", resp.Body)
}
```

### 性能优化迁移

#### 连接池配置 (fasthttp → httpc)

**原代码:**
```go
client := &fasthttp.Client{
    MaxConnsPerHost:     1000,
    MaxIdleConnDuration: 10 * time.Second,
    ReadTimeout:         5 * time.Second,
    WriteTimeout:        5 * time.Second,
}
```

**迁移后:**
```go
config := httpc.DefaultConfig()
config.MaxConnsPerHost = 100  // httpc 使用更保守的默认值
config.Timeout = 10 * time.Second

client, err := httpc.New(config)
```

## 📊 配置映射表

### net/http → httpc

| net/http | httpc | 说明 |
|----------|-------|------|
| `http.Client.Timeout` | `Config.Timeout` | 请求超时 |
| `Transport.MaxIdleConns` | `Config.MaxIdleConns` | 最大空闲连接 |
| `Transport.MaxIdleConnsPerHost` | `Config.MaxConnsPerHost` | 每主机最大连接 |
| `Transport.TLSClientConfig` | `Config.TLSConfig` | TLS 配置 |
| `Transport.DisableKeepAlives` | 无直接对应 | httpc 默认启用 |

### resty → httpc

| resty | httpc | 说明 |
|-------|-------|------|
| `SetTimeout()` | `WithTimeout()` | 请求超时 |
| `SetRetryCount()` | `Config.MaxRetries` | 重试次数 |
| `SetAuthToken()` | `WithBearerToken()` | Bearer 认证 |
| `SetHeader()` | `WithHeader()` | 设置头部 |
| `SetBody()` | `WithJSON()` | JSON 请求体 |
| `SetFormData()` | `WithForm()` | 表单数据 |
| `SetFile()` | `WithFile()` | 文件上传 |

### fasthttp → httpc

| fasthttp | httpc | 说明 |
|----------|-------|------|
| `Client.MaxConnsPerHost` | `Config.MaxConnsPerHost` | 每主机连接数 |
| `Client.ReadTimeout` | `Config.Timeout` | 读取超时 |
| `Client.WriteTimeout` | 包含在 `Timeout` 中 | 写入超时 |
| `Request.SetRequestURI()` | 方法参数 | 请求 URL |
| `Request.Header.SetMethod()` | 方法名 | HTTP 方法 |

## ❓ 常见迁移问题

### 1. 响应体处理差异

**问题:** 在 net/http 中需要手动读取和关闭响应体

**解决方案:**
```go
// net/http (需要手动处理)
resp, err := client.Get(url)
defer resp.Body.Close()
body, err := io.ReadAll(resp.Body)

// httpc (自动处理)
resp, err := client.Get(url)
// resp.Body 已经是字符串
// resp.RawBody 是原始字节
```

### 2. 错误处理差异

**问题:** 不同库的错误类型不同

**解决方案:**
```go
// 统一的错误处理
resp, err := client.Get(url)
if err != nil {
    // 检查 httpc 特定错误
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP 错误: %d", httpErr.StatusCode)
    }
    return err
}

// 检查响应状态
if !resp.IsSuccess() {
    return fmt.Errorf("请求失败: %d", resp.StatusCode)
}
```

### 3. 配置迁移

**问题:** 配置选项名称和结构不同

**解决方案:**
```go
// 创建迁移辅助函数
func migrateFromNetHTTP(oldClient *http.Client) (httpc.Client, error) {
    config := httpc.DefaultConfig()
    
    if oldClient.Timeout > 0 {
        config.Timeout = oldClient.Timeout
    }
    
    if transport, ok := oldClient.Transport.(*http.Transport); ok {
        config.MaxIdleConns = transport.MaxIdleConns
        config.MaxConnsPerHost = transport.MaxConnsPerHost
        config.TLSConfig = transport.TLSClientConfig
    }
    
    return httpc.New(config)
}
```

### 4. 中间件迁移

**问题:** 原有的中间件需要重写

**解决方案:**
```go
// 创建 httpc 兼容的中间件包装器
type MiddlewareWrapper struct {
    client httpc.Client
    middleware func(http.RoundTripper) http.RoundTripper
}

func WrapMiddleware(client httpc.Client, mw func(http.RoundTripper) http.RoundTripper) *MiddlewareWrapper {
    return &MiddlewareWrapper{
        client: client,
        middleware: mw,
    }
}

// 或者重写为 httpc 风格的中间件
func loggingMiddleware(next func(string, ...httpc.RequestOption) (*httpc.Response, error)) func(string, ...httpc.RequestOption) (*httpc.Response, error) {
    return func(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
        start := time.Now()
        resp, err := next(url, opts...)
        duration := time.Since(start)
        
        log.Printf("请求 %s 耗时 %v", url, duration)
        return resp, err
    }
}
```

## 📋 迁移检查清单

### 准备阶段
- [ ] 分析现有代码中的 HTTP 客户端使用
- [ ] 识别自定义配置和中间件
- [ ] 准备测试用例验证迁移结果
- [ ] 备份原始代码

### 迁移阶段
- [ ] 安装 httpc 库：`go get -u github.com/cybergodev/httpc`
- [ ] 替换导入语句
- [ ] 迁移客户端创建代码
- [ ] 迁移请求方法调用
- [ ] 迁移配置选项
- [ ] 迁移错误处理逻辑

### 验证阶段
- [ ] 运行现有测试确保功能正常
- [ ] 进行性能测试对比
- [ ] 检查内存使用情况
- [ ] 验证错误处理行为
- [ ] 测试边界条件

### 优化阶段
- [ ] 利用 httpc 特有功能优化代码
- [ ] 调整配置以获得最佳性能
- [ ] 添加 httpc 特定的错误处理
- [ ] 更新文档和注释

## 🔧 迁移工具

### 自动化迁移脚本

```bash
#!/bin/bash
# migrate_to_httpc.sh

echo "开始迁移到 httpc..."

# 1. 安装 httpc
go get -u github.com/cybergodev/httpc

# 2. 替换常见的导入
find . -name "*.go" -exec sed -i 's|"net/http"|"github.com/cybergodev/httpc"|g' {} \;
find . -name "*.go" -exec sed -i 's|"github.com/go-resty/resty/v2"|"github.com/cybergodev/httpc"|g' {} \;

# 3. 替换常见的方法调用
find . -name "*.go" -exec sed -i 's|http\.Get|httpc.Get|g' {} \;
find . -name "*.go" -exec sed -i 's|http\.Post|httpc.Post|g' {} \;

echo "自动迁移完成，请手动检查和调整代码"
```

### 迁移验证工具

```go
package main

import (
    "fmt"
    "net/http"
    "time"
    "github.com/cybergodev/httpc"
)

// 对比测试工具
func compareClients(url string) {
    // 测试 net/http
    start := time.Now()
    httpResp, httpErr := http.Get(url)
    httpDuration := time.Since(start)
    
    if httpResp != nil {
        httpResp.Body.Close()
    }
    
    // 测试 httpc
    client, _ := httpc.New()
    defer client.Close()
    
    start = time.Now()
    httpcResp, httpcErr := client.Get(url)
    httpcDuration := time.Since(start)
    
    // 对比结果
    fmt.Printf("URL: %s\n", url)
    fmt.Printf("net/http: 耗时=%v, 错误=%v\n", httpDuration, httpErr != nil)
    if httpResp != nil {
        fmt.Printf("  状态码: %d\n", httpResp.StatusCode)
    }
    
    fmt.Printf("httpc: 耗时=%v, 错误=%v\n", httpcDuration, httpcErr != nil)
    if httpcResp != nil {
        fmt.Printf("  状态码: %d\n", httpcResp.StatusCode)
    }
    
    fmt.Println("---")
}

func main() {
    urls := []string{
        "https://httpbin.org/get",
        "https://api.github.com",
        "https://www.google.com",
    }
    
    for _, url := range urls {
        compareClients(url)
    }
}
```

## 📚 迁移后的优势

迁移到 httpc 后，您将获得：

1. **更简洁的 API** - 减少样板代码
2. **更好的安全性** - 内置安全验证和防护
3. **更高的性能** - 优化的连接池和内存管理
4. **更强的功能** - 内置重试、熔断器等高级功能
5. **更好的错误处理** - 结构化的错误类型
6. **更完善的文档** - 详细的使用指南和示例

## 🆘 获取帮助

如果在迁移过程中遇到问题：

1. **查看文档** - [完整使用指南](USAGE_GUIDE.md)
2. **参考示例** - [示例代码](examples/)
3. **故障排除** - [故障排除指南](TROUBLESHOOTING.md)
4. **提交 Issue** - 在 GitHub 上报告问题
5. **社区讨论** - 参与社区讨论获取帮助

---

💡 **提示**: 迁移是一个渐进的过程。建议先在非关键路径上测试，确认无问题后再全面迁移。