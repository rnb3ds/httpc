# HTTPC - 现代化的 Go HTTP 客户端

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](docs/security.md)
[![Performance](https://img.shields.io/badge/performance-high%20performance-green.svg)](https://github.com/cybergodev/json)
[![Thread Safe](https://img.shields.io/badge/thread%20safe-yes-brightgreen.svg)](https://github.com/cybergodev/json)

一个优雅、高性能的 Go HTTP 客户端库，专为生产级应用而设计。具备企业级安全性、智能并发控制与协程安全操作、零分配缓冲池以及自适应连接管理。旨在处理数千个并发请求，同时保持内存效率和所有操作的线程安全。

#### **[📖 English Docs](README.md)** - User guide

---

## ✨ 为什么选择 HTTPC？

- 🛡️ **默认安全** - TLS 1.2+、输入验证、CRLF 保护、SSRF 防护
- ⚡ **高性能** - 协程安全操作、零分配缓冲池（减少 90% GC 压力）、智能连接复用
- 🚀 **大规模并发** - 通过自适应信号量控制和每主机连接限制，适用于高并发请求
- 🔒 **线程安全** - 所有操作都是协程安全的，采用无锁原子计数器和同步状态管理
- 🔄 **内置弹性** - 熔断器、带指数退避的智能重试、优雅降级
- 🎯 **开发者友好** - 简洁的 API、丰富的选项、全面的错误处理
- 📊 **可观测性** - 实时指标、结构化日志、健康检查
- 🔧 **零配置** - 安全的默认设置，开箱即用

## 📋 快速参考

- **[快速参考指南](QUICK_REFERENCE.md)** - 常见任务速查表

---

## 📑 目录

- [快速开始](#-快速开始)
- [HTTP 请求方法](#-http-请求方法)
- [请求选项说明](#-请求选项说明)
- [响应处理](#-响应处理)
- [文件下载](#-文件下载)
- [配置](#-配置)
- [错误处理](#-错误处理)
- [高级特性](#-高级特性)
- [性能](#-性能)


## 🚀 快速开始

### 安装

```bash
go get -u github.com/cybergodev/httpc
```

### 5 分钟教程

```go
package main

import (
    "fmt"
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // 发起一个简单的 GET 请求
    resp, err := httpc.Get("https://api.example.com/users")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("状态码: %d\n", resp.StatusCode)
    fmt.Printf("响应体: %s\n", resp.Body)

    // POST JSON 数据
    user := map[string]string{
        "name":  "张三",
        "email": "zhangsan@example.com",
    }

    // 发起一个简单的 POST 请求
    resp, err = httpc.Post("https://api.example.com/users",
        httpc.WithJSON(user),
        httpc.WithBearerToken("your-token"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("状态码: %d\n", resp.StatusCode)
    fmt.Printf("响应体: %s\n", resp.Body)
}
```

## 🌐 HTTP 请求方法

### 💡 核心概念：请求方法 vs 选项方法

在使用 HTTPC 之前，理解这两个概念很重要：

<table>
<tr>
<th width="50%">🎯 请求方法</th>
<th width="50%">⚙️ 选项方法</th>
</tr>
<tr>
<td>

**用途**：指定 HTTP 请求类型（"做什么"）

**特点**：
- Client 对象的方法
- 决定 HTTP 动词
- 第一个参数是 URL

**示例**：
```go
client.Get(url, ...)
client.Post(url, ...)
client.Put(url, ...)
client.Delete(url, ...)
```

</td>
<td>

**用途**：自定义请求参数（"如何做"）

**特点**：
- 以 `With` 开头的函数
- 用于配置请求细节
- 作为可变参数传递

**示例**：
```go
httpc.WithJSON(data)
httpc.WithQuery("key", "val")
httpc.WithBearerToken("token")
httpc.WithTimeout(30*time.Second)
```

</td>
</tr>
</table>

#### 📝 使用模式

```go
// 基本语法
resp, err := httpc.请求方法(url, 选项1, 选项2, ...)

// 实际示例：POST 请求 + JSON 数据 + 认证 + 超时
resp, err := httpc.Post("https://api.example.com/users",   // ← 请求方法
    httpc.WithJSON(userData),                              // ← 选项方法
    httpc.WithBearerToken("token"),                        // ← 选项方法
    httpc.WithTimeout(30*time.Second),                     // ← 选项方法
)
```

---

### 📋 请求方法快速参考

| 请求方法                                 | HTTP 动词 | 用途           | 常见使用场景         |
|------------------------------------------|-----------|----------------|----------------------|
| `Get(url, opts...)`                      | GET       | 获取资源       | 查询列表、获取详情   |
| `Post(url, opts...)`                     | POST      | 创建资源       | 提交表单、创建记录   |
| `Put(url, opts...)`                      | PUT       | 完整更新       | 替换整个资源         |
| `Patch(url, opts...)`                    | PATCH     | 部分更新       | 更新特定字段         |
| `Delete(url, opts...)`                   | DELETE    | 删除资源       | 删除记录             |
| `Head(url, opts...)`                     | HEAD      | 仅获取头部     | 检查资源是否存在     |
| `Options(url, opts...)`                  | OPTIONS   | 获取支持的方法 | CORS 预检            |
| `Request(ctx, method, url, opts...)`     | 自定义    | 自定义方法     | 特殊需求             |

---

### GET - 获取资源

**用途**：从服务器获取数据，不修改服务器状态。

```go
// 1. 最简单的 GET 请求（无选项）
resp, err := httpc.Get("https://api.example.com/users")

// 2. 带查询参数（使用 WithQuery 选项）
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),        // ← 选项：添加 ?page=1
    httpc.WithQuery("limit", 10),      // ← 选项：添加 &limit=10
)
// 实际请求：GET /users?page=1&limit=10

// 3. 带认证和头部（使用 WithBearerToken 和 WithHeader 选项）
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithBearerToken("your-token"),            // ← 选项：添加认证
    httpc.WithHeader("Accept", "application/json"), // ← 选项：设置头部
)
```

### POST - 创建资源

**用途**：向服务器提交数据，通常用于创建新资源。

```go
// 1. POST JSON 数据（使用 WithJSON 选项）
user := map[string]interface{}{
    "name":  "张三",
    "email": "zhangsan@example.com",
}
resp, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),  // ← 选项：设置 JSON 请求体
)

// 2. POST 表单数据（使用 WithForm 选项）
resp, err := httpc.Post("https://api.example.com/login",
    httpc.WithForm(map[string]string{  // ← 选项：设置表单数据
        "username": "zhangsan",
        "password": "secret",
    }),
)

// 3. POST 文件上传（使用 WithFile 选项）
resp, err := httpc.Post("https://api.example.com/upload",
    httpc.WithFile("file", "document.pdf", fileContent),  // ← 选项：上传文件
    httpc.WithBearerToken("your-token"),                  // ← 选项：添加认证
)
```

### PUT - 完整资源更新

**用途**：完全替换服务器上的资源。

```go
// PUT 更新整个用户对象（使用 WithJSON 和 WithBearerToken 选项）
updatedUser := map[string]interface{}{
    "name":  "李四",
    "email": "lisi@example.com",
    "age":   30,
}
resp, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updatedUser),         // ← 选项：设置 JSON 数据
    httpc.WithBearerToken("your-token"), // ← 选项：添加认证
)
```

### PATCH - 部分资源更新

**用途**：仅更新资源的特定字段。

```go
// PATCH 仅更新邮箱字段（使用 WithJSON 选项）
updates := map[string]interface{}{
    "email": "newemail@example.com",
}
resp, err := httpc.Patch("https://api.example.com/users/123",
    httpc.WithJSON(updates),             // ← 选项：设置要更新的字段
    httpc.WithBearerToken("your-token"), // ← 选项：添加认证
)
```

### DELETE - 删除资源

**用途**：从服务器删除资源。

```go
// 1. 删除特定资源（使用 WithBearerToken 选项）
resp, err := httpc.Delete("https://api.example.com/users/123",
    httpc.WithBearerToken("your-token"),  // ← 选项：添加认证
)

// 2. 带查询参数的删除（使用 WithQuery 选项）
resp, err := httpc.Delete("https://api.example.com/cache",
    httpc.WithQuery("key", "session-123"),  // ← 选项：指定要删除的键
    httpc.WithBearerToken("your-token"),    // ← 选项：添加认证
)
```

### HEAD - 仅获取头部

**用途**：检查资源是否存在，不获取响应体。

```go
// 检查资源是否存在（通常不需要选项）
resp, err := httpc.Head("https://api.example.com/users/123")
if err == nil && resp.StatusCode == 200 {
    fmt.Println("资源存在")
    fmt.Printf("内容长度: %d\n", resp.ContentLength)
}
```

### OPTIONS - 获取支持的方法

**用途**：查询服务器支持的 HTTP 方法。

```go
// 查询 API 端点支持的方法（通常不需要选项）
resp, err := httpc.Options("https://api.example.com/users")
allowedMethods := resp.Headers.Get("Allow")
fmt.Println("支持的方法:", allowedMethods)  // 例如：GET, POST, PUT, DELETE
```

### Request - 通用请求方法

**用途**：发送自定义 HTTP 方法的请求。

```go
// 使用自定义 HTTP 方法（带选项方法）
ctx := context.Background()
resp, err := httpc.Request(ctx, "CUSTOM", "https://api.example.com/resource",
    httpc.WithJSON(data),                // ← 选项：设置数据
    httpc.WithHeader("X-Custom", "val"), // ← 选项：自定义头部
)
```

## ⚙️ 请求选项说明

选项方法用于自定义请求的各个方面。所有选项方法都以 `With` 开头，可以自由组合。

### 📋 选项方法分类快速参考

| 分类                                        | 用途                 | 选项数量 |
|---------------------------------------------|----------------------|----------|
| [头部选项](#1️⃣-头部选项)                   | 设置 HTTP 请求头     | 7        |
| [认证选项](#2️⃣-认证选项)                   | 添加认证信息         | 2        |
| [查询参数选项](#3️⃣-查询参数选项)           | 添加 URL 查询参数    | 2        |
| [请求体选项](#4️⃣-请求体选项)               | 设置请求体内容       | 7        |
| [文件上传选项](#5️⃣-文件上传选项)           | 上传文件             | 1        |
| [超时和重试选项](#6️⃣-超时和重试选项)       | 控制超时和重试       | 3        |
| [Cookie 选项](#7️⃣-cookie-选项)             | 管理 Cookie          | 3        |

---

### 1️⃣ 头部选项

用于设置 HTTP 请求头。

**完整选项列表**：
- `WithHeader(key, value)` - 设置单个头部
- `WithHeaderMap(headers)` - 设置多个头部
- `WithUserAgent(ua)` - 设置 User-Agent
- `WithContentType(ct)` - 设置 Content-Type
- `WithAccept(accept)` - 设置 Accept
- `WithJSONAccept()` - 设置 Accept 为 application/json
- `WithXMLAccept()` - 设置 Accept 为 application/xml

```go
// 设置单个头部
httpc.Get(url,
    httpc.WithHeader("X-Custom-Header", "value"),
)

// 设置多个头部
httpc.Get(url,
    httpc.WithHeaderMap(map[string]string{
        "X-API-Version": "v1",
        "X-Client-ID":   "client-123",
    }),
)

// 常用头部的便捷方法
httpc.Get(url,
    httpc.WithUserAgent("MyApp/1.0"),              // User-Agent
    httpc.WithContentType("application/json"),     // Content-Type
    httpc.WithAccept("application/json"),          // Accept
    httpc.WithJSONAccept(),                        // Accept: application/json
    httpc.WithXMLAccept(),                         // Accept: application/xml
)
```

---

### 2️⃣ 认证选项

用于添加认证信息。

**完整选项列表**：
- `WithBearerToken(token)` - Bearer Token 认证
- `WithBasicAuth(username, password)` - Basic 认证

```go
// Bearer Token 认证（JWT）
httpc.Get(url,
    httpc.WithBearerToken("your-jwt-token"),
)

// Basic 认证
httpc.Get(url,
    httpc.WithBasicAuth("username", "password"),
)

// API Key 认证（使用 WithHeader）
httpc.Get(url,
    httpc.WithHeader("X-API-Key", "your-api-key"),
)
```

---

### 3️⃣ 查询参数选项

用于添加 URL 查询参数（`?key=value&...`）。

**完整选项列表**：
- `WithQuery(key, value)` - 添加单个查询参数
- `WithQueryMap(params)` - 添加多个查询参数

```go
// 添加单个查询参数
httpc.Get(url,
    httpc.WithQuery("page", 1),
    httpc.WithQuery("filter", "active"),
)
// 结果：GET /api?page=1&filter=active

// 添加多个查询参数
httpc.Get(url,
    httpc.WithQueryMap(map[string]interface{}{
        "page":   1,
        "limit":  20,
        "sort":   "created_at",
        "order":  "desc",
    }),
)
// 结果：GET /api?page=1&limit=20&sort=created_at&order=desc
```

---

### 4️⃣ 请求体选项

用于设置请求体内容，支持多种格式。

**完整选项列表**：
- `WithJSON(data)` - JSON 格式请求体
- `WithXML(data)` - XML 格式请求体
- `WithForm(data)` - 表单格式请求体
- `WithText(text)` - 纯文本请求体
- `WithBody(data)` - 原始请求体
- `WithFormData(formData)` - Multipart 表单数据
- `WithBinary(data, contentType)` - 二进制数据

```go
// JSON 格式（最常用）
httpc.Post(url,
    httpc.WithJSON(map[string]interface{}{
        "name": "张三",
        "age":  30,
    }),
)
// Content-Type: application/json

// XML 格式
httpc.Post(url,
    httpc.WithXML(struct {
        Name string `xml:"name"`
        Age  int    `xml:"age"`
    }{Name: "张三", Age: 30}),
)
// Content-Type: application/xml

// 表单格式（application/x-www-form-urlencoded）
httpc.Post(url,
    httpc.WithForm(map[string]string{
        "username": "zhangsan",
        "password": "secret",
    }),
)
// Content-Type: application/x-www-form-urlencoded

// 纯文本
httpc.Post(url,
    httpc.WithText("你好，世界！"),
)
// Content-Type: text/plain

// 二进制数据
httpc.Post(url,
    httpc.WithBinary([]byte{0x89, 0x50, 0x4E, 0x47}, "image/png"),
)
// Content-Type: image/png

// 原始数据 + 自定义 Content-Type
httpc.Post(url,
    httpc.WithBody(customData),
    httpc.WithContentType("application/vnd.api+json"),
)

// Multipart 表单数据（用于文件上传）
httpc.Post(url,
    httpc.WithFormData(formData),
)
// Content-Type: multipart/form-data
```

---

### 5️⃣ 文件上传选项

用于上传文件到服务器。

**完整选项列表**：
- `WithFile(fieldName, filename, content)` - 上传单个文件（便捷方法）

```go
// 简单的单文件上传
httpc.Post(url,
    httpc.WithFile("file", "document.pdf", fileContent),
)

// 多文件 + 表单字段（使用请求体选项中的 WithFormData）
formData := &httpc.FormData{
    Fields: map[string]string{
        "title":       "我的文档",
        "description": "重要文件",
        "category":    "报告",
    },
    Files: map[string]*httpc.FileData{
        "document": {
            Filename:    "report.pdf",
            Content:     pdfContent,
            ContentType: "application/pdf",
        },
        "thumbnail": {
            Filename:    "preview.jpg",
            Content:     jpgContent,
            ContentType: "image/jpeg",
        },
    },
}
httpc.Post(url,
    httpc.WithFormData(formData),
    httpc.WithBearerToken("token"),  // 可以与其他选项组合
)
```

---

### 6️⃣ 超时和重试选项

用于控制请求超时和重试行为。

**完整选项列表**：
- `WithTimeout(duration)` - 设置请求超时
- `WithMaxRetries(n)` - 设置最大重试次数
- `WithContext(ctx)` - 使用 Context 进行控制

```go
// 设置请求超时
httpc.Get(url,
    httpc.WithTimeout(30 * time.Second),
)

// 设置最大重试次数
httpc.Get(url,
    httpc.WithMaxRetries(3),
)

// 使用 Context 控制超时和取消
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()
httpc.Get(url,
    httpc.WithContext(ctx),
)

// 组合使用
httpc.Post(url,
    httpc.WithJSON(data),
    httpc.WithTimeout(30 * time.Second),
    httpc.WithMaxRetries(2),
)
```

---

### 7️⃣ Cookie 选项

用于向请求添加 Cookie。

**完整选项列表**：
- `WithCookie(cookie)` - 添加完整的 Cookie
- `WithCookies(cookies)` - 添加多个 Cookie
- `WithCookieValue(name, value)` - 添加简单的 Cookie

```go
// 简单的 Cookie（仅名称和值）
httpc.Get(url,
    httpc.WithCookieValue("session_id", "abc123"),
)

// 完整的 Cookie（带属性）
httpc.Get(url,
    httpc.WithCookie(&http.Cookie{
        Name:     "session",
        Value:    "xyz789",
        Path:     "/",
        Domain:   "example.com",
        Secure:   true,
        HttpOnly: true,
    }),
)

// 多个 Cookie
httpc.Get(url,
    httpc.WithCookies([]*http.Cookie{
        {Name: "cookie1", Value: "value1"},
        {Name: "cookie2", Value: "value2"},
    }),
)
```

---

### 💡 选项方法组合示例

选项方法可以自由组合以满足各种复杂需求：

```go
// 示例 1：完整的 API 请求
resp, err := httpc.Post("https://api.example.com/users",
    // 请求体
    httpc.WithJSON(userData),
    // 认证
    httpc.WithBearerToken("your-token"),
    // 头部
    httpc.WithHeader("X-Request-ID", "req-123"),
    httpc.WithUserAgent("MyApp/1.0"),
    // 超时和重试
    httpc.WithTimeout(30*time.Second),
    httpc.WithMaxRetries(2),
)

// 示例 2：文件上传 + 认证 + 超时
resp, err := httpc.Post("https://api.example.com/upload",
    httpc.WithFile("file", "report.pdf", fileContent),
    httpc.WithBearerToken("token"),
    httpc.WithTimeout(60*time.Second),
)

// 示例 3：查询 + 认证 + 自定义头部
resp, err := httpc.Get("https://api.example.com/users",
    httpc.WithQuery("page", 1),
    httpc.WithQuery("limit", 20),
    httpc.WithBearerToken("token"),
    httpc.WithHeader("X-API-Version", "v2"),
)
```

## 📦 响应处理

Response 对象提供了便捷的方法来处理 HTTP 响应。

### 响应结构

```go
type Response struct {
    StatusCode    int            // HTTP 状态码
    Status        string         // HTTP 状态文本
    Headers       http.Header    // 响应头
    Body          string         // 响应体（字符串）
    RawBody       []byte         // 响应体（字节）
    ContentLength int64          // 内容长度
    Proto         string         // HTTP 协议版本
    Duration      time.Duration  // 请求耗时
    Attempts      int            // 重试次数
    Cookies       []*http.Cookie // 响应 Cookie
}
```

### 状态检查

```go
resp, err := client.Get(url)

// 检查成功（2xx）
if resp.IsSuccess() {
    fmt.Println("请求成功")
}

// 检查重定向（3xx）
if resp.IsRedirect() {
    fmt.Println("已重定向")
}

// 检查客户端错误（4xx）
if resp.IsClientError() {
    fmt.Println("客户端错误")
}

// 检查服务器错误（5xx）
if resp.IsServerError() {
    fmt.Println("服务器错误")
}
```

### 解析响应体

```go
// 解析 JSON
var result map[string]interface{}
err := resp.JSON(&result)

// 解析 XML
var data XMLStruct
err := resp.XML(&data)

// 访问原始响应体
bodyString := resp.Body
bodyBytes := resp.RawBody
```

### 处理 Cookie

```go
// 获取特定的 Cookie
cookie := resp.GetCookie("session_id")
if cookie != nil {
    fmt.Println("会话:", cookie.Value)
}

// 检查 Cookie 是否存在
if resp.HasCookie("auth_token") {
    fmt.Println("已认证")
}

// 获取所有 Cookie
for _, cookie := range resp.Cookies {
    fmt.Printf("%s: %s\n", cookie.Name, cookie.Value)
}
```

### 响应元数据

```go
// 请求耗时
fmt.Printf("请求耗时: %v\n", resp.Duration)

// 重试次数
fmt.Printf("尝试次数: %d\n", resp.Attempts)

// 内容长度
fmt.Printf("大小: %d 字节\n", resp.ContentLength)

// 协议版本
fmt.Printf("协议: %s\n", resp.Proto)
```

## 📥 文件下载

HTTPC 提供强大的文件下载功能，支持进度跟踪、断点续传和大文件流式传输。

### 简单文件下载

```go
// 下载文件到磁盘
result, err := httpc.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("已下载: %s\n", httpc.FormatBytes(result.BytesWritten))
fmt.Printf("速度: %s\n", httpc.FormatSpeed(result.AverageSpeed))
```

### 带进度跟踪的下载

```go
client, _ := httpc.New()
defer client.Close()

// 配置下载选项
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.Overwrite = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\r进度: %.1f%% - %s",
        percentage,
        httpc.FormatSpeed(speed),
    )
}

// 带进度的下载
result, err := client.DownloadFileWithOptions(
    "https://example.com/large-file.zip",
    opts,
    httpc.WithTimeout(10*time.Minute),
)
```

### 断点续传

```go
// 启用中断下载的续传
opts := httpc.DefaultDownloadOptions("downloads/file.zip")
opts.ResumeDownload = true  // 从中断处继续
opts.Overwrite = false      // 不覆盖，而是追加

result, err := client.DownloadFileWithOptions(url, opts)
if result.Resumed {
    fmt.Println("下载已成功续传")
}
```

### 保存响应到文件

```go
// 替代方案：保存任何响应到文件
resp, err := client.Get("https://example.com/data.json")
if err != nil {
    log.Fatal(err)
}

// 保存响应体到文件
err = resp.SaveToFile("data.json")
```

### 下载选项

```go
opts := &httpc.DownloadOptions{
    FilePath:         "downloads/file.zip",  // 必需：目标路径
    Overwrite:        true,                  // 覆盖已存在的文件
    ResumeDownload:   false,                 // 续传部分下载
    CreateDirs:       true,                  // 创建父目录
    BufferSize:       32 * 1024,             // 缓冲区大小（默认 32KB）
    ProgressInterval: 500 * time.Millisecond, // 进度更新频率
    ProgressCallback: progressFunc,          // 进度回调函数
    FileMode:         0644,                  // 文件权限
}

result, err := client.DownloadFileWithOptions(url, opts)
```

### 带认证的下载

```go
// 下载受保护的文件
result, err := client.DownloadFile(
    "https://api.example.com/files/protected.zip",
    "downloads/protected.zip",
    httpc.WithBearerToken("your-token"),
    httpc.WithTimeout(5*time.Minute),
)
```

## 🔧 配置

### 默认配置

```go
// 使用安全的默认设置
client, err := httpc.New()
```

**默认设置：**
- 超时：60 秒
- 最大重试次数：2
- TLS：1.2-1.3
- HTTP/2：已启用
- 连接池：已启用
- 最大并发请求：500
- 最大响应体大小：50 MB

### 安全预设

```go
// 宽松（开发/测试）
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelPermissive))

// 平衡（生产 - 默认）
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelBalanced))

// 严格（高安全性）
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelStrict))
```

### 自定义配置

```go
config := &httpc.Config{
    // 网络设置
    Timeout:               30 * time.Second,
    DialTimeout:           10 * time.Second,
    KeepAlive:             30 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 20 * time.Second,
    IdleConnTimeout:       60 * time.Second,

    // 连接池
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
    MaxConnsPerHost:     20,

    // 安全设置
    MinTLSVersion:         tls.VersionTLS12,
    MaxTLSVersion:         tls.VersionTLS13,
    InsecureSkipVerify:    false,
    MaxResponseBodySize:   50 * 1024 * 1024, // 50 MB
    MaxConcurrentRequests: 500,
    ValidateURL:           true,
    ValidateHeaders:       true,
    AllowPrivateIPs:       false,

    // 重试设置
    MaxRetries:    2,
    RetryDelay:    2 * time.Second,
    MaxRetryDelay: 60 * time.Second,
    BackoffFactor: 2.0,
    Jitter:        true,

    // 头部和功能
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    EnableHTTP2:     true,
    EnableCookies:   true,
    Headers: map[string]string{
        "Accept": "application/json",
    },
}

client, err := httpc.New(config)
```

## 🚨 错误处理

### 智能错误处理

```go
resp, err := httpc.Get(url)
if err != nil {
    // 检查 HTTP 错误
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Status)
        fmt.Printf("URL: %s\n", httpErr.URL)
        fmt.Printf("方法: %s\n", httpErr.Method)
    }

    // 检查熔断器
    if strings.Contains(err.Error(), "circuit breaker is open") {
        // 服务宕机，使用降级方案
        return fallbackData, nil
    }

    // 检查超时
    if strings.Contains(err.Error(), "timeout") {
        // 处理超时
        return nil, fmt.Errorf("请求超时")
    }

    return err
}

// 检查响应状态
if !resp.IsSuccess() {
    return fmt.Errorf("意外的状态码: %d", resp.StatusCode)
}
```

### 错误类型

- **HTTPError**：HTTP 错误响应（4xx、5xx）
- **超时错误**：请求超时
- **熔断器错误**：服务暂时不可用
- **验证错误**：无效的 URL 或头部
- **网络错误**：连接失败

## 🎯 高级特性

### 熔断器

自动防止级联故障，通过临时阻止对失败服务的请求。

```go
// 熔断器默认启用
// 在连续失败后打开，恢复后关闭
client, err := httpc.New()

resp, err := client.Get(url)
if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
    // 使用降级方案或缓存数据
    return getCachedData()
}
```

### 自动重试

带指数退避和抖动的智能重试机制。

```go
// 配置重试行为
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.RetryDelay = 1 * time.Second
config.MaxRetryDelay = 30 * time.Second
config.BackoffFactor = 2.0
config.Jitter = true

client, err := httpc.New(config)

// 单个请求的重试覆盖
resp, err := client.Get(url,
    httpc.WithMaxRetries(5),
)
```

### 连接池

高效的连接复用以获得更好的性能。

```go
config := httpc.DefaultConfig()
config.MaxIdleConns = 100        // 总空闲连接数
config.MaxIdleConnsPerHost = 10  // 每个主机的空闲连接数
config.MaxConnsPerHost = 20      // 每个主机的最大连接数
config.IdleConnTimeout = 90 * time.Second

client, err := httpc.New(config)
```

### Cookie 管理

支持 Cookie Jar 的自动 Cookie 处理。

```go
// 自动 Cookie 管理（默认启用）
client, err := httpc.New()

// 第一个请求设置 Cookie
resp1, _ := client.Get("https://example.com/login",
    httpc.WithForm(map[string]string{
        "username": "zhangsan",
        "password": "secret",
    }),
)

// 后续请求自动包含 Cookie
resp2, _ := client.Get("https://example.com/profile")

// 自定义 Cookie Jar
jar, _ := httpc.NewCookieJar()
config := httpc.DefaultConfig()
config.CookieJar = jar
client, err := httpc.New(config)
```

### Context 支持

完整的 Context 支持，用于取消和截止时间。

```go
// 带超时的 Context
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Request(ctx, "GET", url)

// 带取消的 Context
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel() // 5 秒后取消
}()

resp, err := client.Get(url, httpc.WithContext(ctx))
```

## 📊 性能

### 并发与线程安全
- **大规模并发**：通过自适应信号量限流处理适用于高并发请求
- **协程安全**：所有操作使用原子计数器和同步状态管理
- **无锁指标**：实时性能跟踪，无竞争
- **每主机限制**：智能连接分配防止主机过载

### 内存优化
- **零分配池化**：可重用缓冲池减少 90% GC 压力
- **智能缓冲区大小**：基于响应模式的自适应缓冲区分配
- **内存边界**：可配置的限制防止内存耗尽
- **高效清理**：使用 sync.Pool 自动资源回收

### 网络性能
- **连接池**：智能连接复用，带每主机跟踪
- **HTTP/2 多路复用**：单个连接上的多个并发流
- **Keep-Alive 优化**：带可配置超时的持久连接
- **低延迟**：优化的请求/响应处理管道

### 可靠性
- **Panic 恢复**：全面的错误处理防止崩溃
- **熔断器**：自动故障检测和恢复
- **优雅降级**：在部分故障下继续运行
- **资源限制**：通过可配置边界防止资源耗尽

## 📖 文档

### 📚 完整文档

- **[📖 文档](docs)** - 完整文档中心
- **[🚀 入门指南](docs/getting-started.md)** - 安装和第一步
- **[⚙️ 配置](docs/configuration.md)** - 客户端配置和预设
- **[🔧 请求选项](docs/request-options.md)** - 自定义 HTTP 请求
- **[❗ 错误处理](docs/error-handling.md)** - 全面的错误处理
- **[📥 文件下载](docs/file-download.md)** - 带进度的文件下载
- **[🔄 熔断器](docs/circuit-breaker.md)** - 自动故障保护
- **[✅ 最佳实践](docs/best-practices.md)** - 推荐的使用模式
- **[🔒 安全性](docs/security.md)** - 安全功能和合规性
- **[💡 示例](examples)** - 代码示例和教程

### 💻 代码示例

- **[快速开始](examples/01_quickstart)** - 基本使用示例
- **[核心功能](examples/02_core_features)** - 头部、认证、请求体格式、Cookie
- **[高级功能](examples/03_advanced)** - 文件上传、下载、超时、重试
- **[实际应用](examples/04_real_world)** - 完整的 REST API 客户端实现

---

## 📄 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

---

## 🤝 贡献

欢迎贡献！请随时提交 Pull Request。对于重大更改，请先开启一个 issue 讨论您想要更改的内容。


## 🌟 Star 历史

如果您觉得这个项目有用，请考虑给它一个 star！⭐

---

**由 CyberGoDev 团队用 ❤️ 制作**

