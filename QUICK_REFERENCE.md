# HTTPC 快速参考指南

## 🚀 快速开始

```go
// 安装
go get -u github.com/cybergodev/httpc

// 基本使用
client, err := httpc.New()
defer client.Close()

resp, err := client.Get("https://api.example.com/users")
```

## 📋 API 速查表

### 客户端创建
| 方法 | 说明 | 示例 |
|------|------|------|
| `httpc.New()` | 默认配置 | `client, err := httpc.New()` |
| `httpc.New(config)` | 自定义配置 | `client, err := httpc.New(myConfig)` |
| `httpc.SecureConfig()` | 安全预设配置 | `config := httpc.SecureConfig()` |
| `httpc.PerformanceConfig()` | 性能预设配置 | `config := httpc.PerformanceConfig()` |

### HTTP 方法
| 方法 | 说明 | 示例 |
|------|------|------|
| `Get(url, opts...)` | GET 请求 | `client.Get(url, httpc.WithQuery("page", 1))` |
| `Post(url, opts...)` | POST 请求 | `client.Post(url, httpc.WithJSON(data))` |
| `Put(url, opts...)` | PUT 请求 | `client.Put(url, httpc.WithJSON(data))` |
| `Patch(url, opts...)` | PATCH 请求 | `client.Patch(url, httpc.WithJSON(updates))` |
| `Delete(url, opts...)` | DELETE 请求 | `client.Delete(url, httpc.WithBearerToken(token))` |
| `Head(url, opts...)` | HEAD 请求 | `client.Head(url)` |
| `Options(url, opts...)` | OPTIONS 请求 | `client.Options(url)` |

### 包级别函数
| 方法 | 说明 | 示例 |
|------|------|------|
| `httpc.Get()` | 包级别 GET | `httpc.Get(url, opts...)` |
| `httpc.Post()` | 包级别 POST | `httpc.Post(url, opts...)` |
| `httpc.Put()` | 包级别 PUT | `httpc.Put(url, opts...)` |
| `httpc.Delete()` | 包级别 DELETE | `httpc.Delete(url, opts...)` |

## ⚙️ 请求选项

### 头部设置
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithHeader(k, v)` | 设置单个头部 | `WithHeader("X-API-Key", "key")` |
| `WithHeaderMap(map)` | 批量设置头部 | `WithHeaderMap(headers)` |
| `WithUserAgent(ua)` | 设置 User-Agent | `WithUserAgent("MyApp/1.0")` |
| `WithContentType(ct)` | 设置 Content-Type | `WithContentType("application/json")` |
| `WithAccept(accept)` | 设置 Accept | `WithAccept("application/json")` |
| `WithJSONAccept()` | 设置 JSON Accept | `WithJSONAccept()` |

### 认证
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithBearerToken(token)` | Bearer 认证 | `WithBearerToken("jwt-token")` |
| `WithBasicAuth(u, p)` | Basic 认证 | `WithBasicAuth("user", "pass")` |

### 查询参数
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithQuery(k, v)` | 单个参数 | `WithQuery("page", 1)` |
| `WithQueryMap(map)` | 批量参数 | `WithQueryMap(params)` |

### 请求体
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithJSON(data)` | JSON 数据 | `WithJSON(user)` |
| `WithForm(data)` | 表单数据 | `WithForm(formData)` |
| `WithText(text)` | 纯文本 | `WithText("Hello")` |
| `WithBody(data)` | 原始数据 | `WithBody(rawData)` |
| `WithBinary(data, ct)` | 二进制数据 | `WithBinary(bytes, "image/png")` |

### 文件操作
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithFile(field, name, content)` | 单文件上传 | `WithFile("file", "doc.pdf", content)` |
| `WithFormData(data)` | 多文件上传 | `WithFormData(formData)` |

### 超时和重试
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithTimeout(duration)` | 设置超时 | `WithTimeout(30*time.Second)` |
| `WithMaxRetries(n)` | 最大重试次数 | `WithMaxRetries(3)` |
| `WithContext(ctx)` | 使用上下文 | `WithContext(ctx)` |

### Cookie
| 选项 | 说明 | 示例 |
|------|------|------|
| `WithCookie(cookie)` | 添加 Cookie | `WithCookie(cookie)` |
| `WithCookies(cookies)` | 批量 Cookie | `WithCookies(cookies)` |
| `WithCookieValue(n, v)` | 简单 Cookie | `WithCookieValue("session", "id")` |

## 📦 响应处理

### 响应属性
| 属性 | 类型 | 说明 |
|------|------|------|
| `StatusCode` | `int` | HTTP 状态码 |
| `Status` | `string` | 状态文本 |
| `Headers` | `http.Header` | 响应头部 |
| `Body` | `string` | 响应体字符串 |
| `RawBody` | `[]byte` | 响应体字节 |
| `Duration` | `time.Duration` | 请求耗时 |
| `Attempts` | `int` | 重试次数 |
| `Cookies` | `[]*http.Cookie` | 响应 Cookie |

### 响应方法
| 方法 | 说明 | 示例 |
|------|------|------|
| `IsSuccess()` | 是否成功 (2xx) | `if resp.IsSuccess() { ... }` |
| `IsRedirect()` | 是否重定向 (3xx) | `if resp.IsRedirect() { ... }` |
| `IsClientError()` | 客户端错误 (4xx) | `if resp.IsClientError() { ... }` |
| `IsServerError()` | 服务器错误 (5xx) | `if resp.IsServerError() { ... }` |
| `JSON(v)` | 解析 JSON | `resp.JSON(&user)` |
| `GetCookie(name)` | 获取 Cookie | `resp.GetCookie("session")` |
| `HasCookie(name)` | 检查 Cookie | `resp.HasCookie("auth")` |
| `SaveToFile(path)` | 保存到文件 | `resp.SaveToFile("data.json")` |

## 📁 文件操作

### 下载
| 方法 | 说明 | 示例 |
|------|------|------|
| `DownloadFile(url, path, opts...)` | 简单下载 | `DownloadFile(url, "file.zip")` |
| `DownloadWithOptions(url, opts, ...)` | 高级下载 | `DownloadWithOptions(url, opts)` |

### 下载选项
| 选项 | 说明 | 示例 |
|------|------|------|
| `DefaultDownloadOptions(path)` | 默认选项 | `opts := DefaultDownloadOptions("file.zip")` |
| `opts.Overwrite` | 覆盖文件 | `opts.Overwrite = true` |
| `opts.ResumeDownload` | 断点续传 | `opts.ResumeDownload = true` |
| `opts.ProgressCallback` | 进度回调 | `opts.ProgressCallback = func(...) { ... }` |

## 🔧 配置选项

### 基本配置
| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Timeout` | `time.Duration` | `60s` | 请求超时 |
| `MaxIdleConns` | `int` | `100` | 最大空闲连接 |
| `MaxConnsPerHost` | `int` | `20` | 每主机最大连接 |
| `MaxRetries` | `int` | `2` | 最大重试次数 |
| `RetryDelay` | `time.Duration` | `2s` | 重试延迟 |
| `BackoffFactor` | `float64` | `2.0` | 退避因子 |

### 安全配置
| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `InsecureSkipVerify` | `bool` | `false` | 跳过 TLS 验证 |
| `MaxResponseBodySize` | `int64` | `50MB` | 最大响应体 |
| `AllowPrivateIPs` | `bool` | `false` | 允许私有 IP |

### 功能配置
| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `UserAgent` | `string` | `"httpc/1.0"` | 用户代理 |
| `FollowRedirects` | `bool` | `true` | 跟随重定向 |
| `EnableHTTP2` | `bool` | `true` | 启用 HTTP/2 |
| `EnableCookies` | `bool` | `true` | 启用 Cookie |

## 🛡️ 配置预设

| 预设函数 | 说明 | 适用场景 |
|------|------|----------|
| `DefaultConfig()` | 平衡模式（默认） | 大多数应用 |
| `SecureConfig()` | 安全模式 | 高安全要求 |
| `PerformanceConfig()` | 性能模式 | 高吞吐量场景 |
| `TestingConfig()` | 测试模式 | 开发测试 |

```go
// 使用预设
config := httpc.SecureConfig()
client, err := httpc.New(config)
```

## 🚨 错误处理

### 错误类型
| 类型 | 说明 | 检查方法 |
|------|------|----------|
| `*httpc.HTTPError` | HTTP 错误 | `errors.As(err, &httpErr)` |
| `context.DeadlineExceeded` | 超时 | `errors.Is(err, context.DeadlineExceeded)` |
| `context.Canceled` | 取消 | `errors.Is(err, context.Canceled)` |

### 常见错误处理
```go
resp, err := client.Get(url)
if err != nil {
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        fmt.Printf("HTTP %d: %s", httpErr.StatusCode, httpErr.Status)
    }
    return err
}

if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

## 💡 常用模式

### API 客户端模式
```go
type APIClient struct {
    client  httpc.Client
    baseURL string
    token   string
}

func (c *APIClient) GetUser(id int) (*User, error) {
    resp, err := c.client.Get(c.baseURL+"/users/"+strconv.Itoa(id),
        httpc.WithBearerToken(c.token),
        httpc.WithTimeout(10*time.Second),
    )
    if err != nil {
        return nil, err
    }
    
    var user User
    return &user, resp.JSON(&user)
}
```

### 并发请求模式
```go
var wg sync.WaitGroup
results := make(chan *httpc.Response, len(urls))

for _, url := range urls {
    wg.Add(1)
    go func(u string) {
        defer wg.Done()
        resp, err := client.Get(u)
        if err == nil {
            results <- resp
        }
    }(url)
}

wg.Wait()
close(results)
```

### 重试模式
```go
resp, err := client.Get(url,
    httpc.WithMaxRetries(3),
    httpc.WithTimeout(30*time.Second),
)
```

### 文件上传模式
```go
resp, err := client.Post(uploadURL,
    httpc.WithFile("file", "document.pdf", fileContent),
    httpc.WithBearerToken(token),
)
```

### 进度下载模式
```go
opts := httpc.DefaultDownloadOptions("file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    fmt.Printf("\r%.1f%% - %s", 
        float64(downloaded)/float64(total)*100,
        httpc.FormatSpeed(speed))
}

result, err := client.DownloadWithOptions(url, opts)
```

## 🔗 实用函数

| 函数 | 说明 | 示例 |
|------|------|------|
| `httpc.FormatBytes(bytes)` | 格式化字节数 | `FormatBytes(1024)` → `"1.00 KB"` |
| `httpc.FormatSpeed(bps)` | 格式化速度 | `FormatSpeed(1024)` → `"1.00 KB/s"` |
| `httpc.DefaultConfig()` | 默认配置 | `config := httpc.DefaultConfig()` |
| `httpc.ValidateConfig(cfg)` | 验证配置 | `err := httpc.ValidateConfig(config)` |

## 📚 更多资源

- [完整使用指南](USAGE_GUIDE.md)
- [示例代码](examples/)
- [API 文档](README.md)
- [最佳实践](docs/best-practices.md)

---

💡 **提示**: 这个快速参考涵盖了最常用的 API。更详细的信息请参考完整文档。