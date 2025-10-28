# HTTPC Troubleshooting Guide

## 🎯 Overview

This guide helps you diagnose and resolve common issues encountered when using the httpc library. It provides detailed solutions and preventive measures organized by problem type.

## 📋 目录

1. [连接问题](#连接问题)
2. [超时问题](#超时问题)
3. [认证问题](#认证问题)
4. [TLS/SSL 问题](#tlsssl-问题)
5. [性能问题](#性能问题)
6. [内存问题](#内存问题)
7. [并发问题](#并发问题)
8. [文件操作问题](#文件操作问题)
9. [配置问题](#配置问题)
10. [调试技巧](#调试技巧)

## 🔌 连接问题

### 问题：connection refused

**错误信息：**
```
Get "http://localhost:8080": dial tcp [::1]:8080: connect: connection refused
```

**可能原因：**
- 目标服务器未运行
- 端口号错误
- 防火墙阻止连接
- 网络配置问题

**解决方案：**

1. **检查服务器状态**
```bash
# 检查端口是否开放
netstat -an | grep 8080
# 或使用 telnet 测试连接
telnet localhost 8080
```

2. **验证 URL 正确性**
```go
// 使用公共测试服务验证客户端工作正常
resp, err := client.Get("https://httpbin.org/get")
if err != nil {
    fmt.Println("客户端配置有问题")
} else {
    fmt.Println("客户端工作正常，检查目标服务器")
}
```

3. **检查网络配置**
```go
// 尝试不同的地址格式
urls := []string{
    "http://localhost:8080",
    "http://127.0.0.1:8080",
    "http://0.0.0.0:8080",
}

for _, url := range urls {
    resp, err := client.Get(url, httpc.WithTimeout(5*time.Second))
    if err == nil {
        fmt.Printf("成功连接: %s\n", url)
        break
    }
    fmt.Printf("连接失败 %s: %v\n", url, err)
}
```

### 问题：DNS 解析失败

**错误信息：**
```
Get "https://nonexistent.example.com": dial tcp: lookup nonexistent.example.com: no such host
```

**解决方案：**

1. **验证域名**
```bash
# 使用 nslookup 检查域名解析
nslookup api.example.com

# 使用 dig 获取更详细信息
dig api.example.com
```

2. **使用 IP 地址测试**
```go
// 如果域名解析有问题，尝试直接使用 IP
resp, err := client.Get("http://192.168.1.100:8080/api")
```

3. **配置自定义 DNS**
```go
config := httpc.DefaultConfig()
// 注意：httpc 使用系统 DNS 设置
// 需要在系统级别配置 DNS 或使用 hosts 文件
```

## ⏰ 超时问题

### 问题：请求超时

**错误信息：**
```
context deadline exceeded
```

**解决方案：**

1. **增加超时时间**
```go
// 方法1：使用 WithTimeout
resp, err := client.Get(url,
    httpc.WithTimeout(60*time.Second), // 增加到60秒
)

// 方法2：使用 Context
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()
resp, err := client.Get(url, httpc.WithContext(ctx))

// 方法3：配置默认超时
config := httpc.DefaultConfig()
config.Timeout = 120 * time.Second
client, err := httpc.New(config)
```

2. **分析超时原因**
```go
func analyzeTimeout(url string) {
    start := time.Now()
    
    resp, err := client.Get(url,
        httpc.WithTimeout(30*time.Second),
    )
    
    duration := time.Since(start)
    
    if err != nil {
        if strings.Contains(err.Error(), "timeout") {
            fmt.Printf("请求超时，耗时: %v\n", duration)
            if duration < 5*time.Second {
                fmt.Println("可能是连接超时")
            } else {
                fmt.Println("可能是响应超时")
            }
        }
    } else {
        fmt.Printf("请求成功，耗时: %v\n", duration)
    }
}
```

3. **使用分段超时**
```go
// 为不同阶段设置不同超时
config := httpc.DefaultConfig()
config.Timeout = 60 * time.Second           // 总超时
// 注意：httpc 内部会自动设置合理的分段超时
```

### 问题：超时冲突

**问题描述：**
同时设置了 `WithTimeout` 和 `WithContext`，不确定哪个生效。

**解决方案：**
```go
// 优先级：Context > WithTimeout > 配置默认超时

// 1. 只使用 Context（推荐）
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
resp, err := client.Get(url, httpc.WithContext(ctx))

// 2. 只使用 WithTimeout
resp, err := client.Get(url, httpc.WithTimeout(30*time.Second))

// 3. 避免混合使用
// ❌ 不推荐
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
resp, err := client.Get(url,
    httpc.WithContext(ctx),
    httpc.WithTimeout(30*time.Second), // 这个会被忽略
)
```

## 🔐 认证问题

### 问题：401 Unauthorized

**解决方案：**

1. **检查 Token 格式**
```go
func validateToken(token string) error {
    if len(token) == 0 {
        return fmt.Errorf("token 不能为空")
    }
    
    if len(token) > 1000 {
        return fmt.Errorf("token 过长")
    }
    
    // JWT token 通常包含两个点
    if strings.HasPrefix(token, "eyJ") && strings.Count(token, ".") == 2 {
        fmt.Println("看起来是有效的 JWT token")
    }
    
    return nil
}

// 使用
token := "your-jwt-token"
if err := validateToken(token); err != nil {
    log.Fatal(err)
}

resp, err := client.Get(url, httpc.WithBearerToken(token))
```

2. **检查认证头部**
```go
// 使用 httpbin.org 检查发送的头部
resp, err := client.Get("https://httpbin.org/headers",
    httpc.WithBearerToken("test-token"),
)
if err == nil {
    fmt.Println("发送的头部:", resp.Body)
}
```

3. **Basic 认证问题**
```go
// 确保用户名密码正确编码
username := "user@example.com"
password := "password123"

resp, err := client.Get(url,
    httpc.WithBasicAuth(username, password),
)

// 手动检查编码
auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
fmt.Printf("Basic auth: %s\n", auth)
```

### 问题：API Key 认证失败

**解决方案：**
```go
// 检查 API Key 头部名称
apiKey := "your-api-key"

// 常见的 API Key 头部名称
headers := map[string]string{
    "X-API-Key":       apiKey,
    "X-API-TOKEN":     apiKey,
    "Authorization":   "ApiKey " + apiKey,
    "X-Auth-Token":    apiKey,
}

for headerName, headerValue := range headers {
    resp, err := client.Get(url,
        httpc.WithHeader(headerName, headerValue),
    )
    
    if err == nil && resp.IsSuccess() {
        fmt.Printf("成功的头部: %s\n", headerName)
        break
    }
}
```

## 🔒 TLS/SSL 问题

### 问题：证书验证失败

**错误信息：**
```
x509: certificate signed by unknown authority
```

**解决方案：**

1. **临时跳过验证（仅测试环境）**
```go
// ⚠️ 仅用于测试环境！
config := httpc.DefaultConfig()
config.InsecureSkipVerify = true
client, err := httpc.New(config)
```

2. **添加自定义 CA 证书**
```go
// 加载自定义 CA 证书
caCert, err := os.ReadFile("ca-cert.pem")
if err != nil {
    log.Fatal(err)
}

caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    RootCAs: caCertPool,
}

client, err := httpc.New(config)
```

3. **使用系统证书 + 自定义证书**
```go
// 获取系统证书池
systemCerts, err := x509.SystemCertPool()
if err != nil {
    systemCerts = x509.NewCertPool()
}

// 添加自定义证书
customCert, err := os.ReadFile("custom-ca.pem")
if err == nil {
    systemCerts.AppendCertsFromPEM(customCert)
}

config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    RootCAs: systemCerts,
}
```

### 问题：TLS 版本不兼容

**错误信息：**
```
tls: protocol version not supported
```

**解决方案：**
```go
config := httpc.DefaultConfig()
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS10, // 降低最低版本（不推荐）
    MaxVersion: tls.VersionTLS13,
}

// 或者检查服务器支持的 TLS 版本
func checkTLSVersions(hostname string) {
    versions := []uint16{
        tls.VersionTLS10,
        tls.VersionTLS11,
        tls.VersionTLS12,
        tls.VersionTLS13,
    }
    
    for _, version := range versions {
        conn, err := tls.Dial("tcp", hostname+":443", &tls.Config{
            MinVersion: version,
            MaxVersion: version,
        })
        
        if err == nil {
            fmt.Printf("支持 TLS %x\n", version)
            conn.Close()
        }
    }
}
```

## 🚀 性能问题

### 问题：请求速度慢

**诊断步骤：**

1. **测量各阶段耗时**
```go
func measureRequestTime(url string) {
    start := time.Now()
    
    resp, err := client.Get(url)
    
    total := time.Since(start)
    
    if err != nil {
        fmt.Printf("请求失败，总耗时: %v\n", total)
        return
    }
    
    fmt.Printf("请求成功:\n")
    fmt.Printf("  总耗时: %v\n", total)
    fmt.Printf("  状态码: %d\n", resp.StatusCode)
    fmt.Printf("  响应大小: %d bytes\n", len(resp.RawBody))
    fmt.Printf("  重试次数: %d\n", resp.Attempts)
    
    // 分析可能的瓶颈
    if total > 5*time.Second {
        fmt.Println("⚠️ 请求耗时过长，可能原因:")
        fmt.Println("  - 网络延迟高")
        fmt.Println("  - 服务器响应慢")
        fmt.Println("  - 响应体过大")
    }
}
```

2. **优化连接池**
```go
// 高并发场景优化
config := httpc.DefaultConfig()
config.MaxIdleConns = 200
config.MaxConnsPerHost = 50

client, err := httpc.New(config)
```

3. **并发请求测试**
```go
func benchmarkConcurrentRequests(url string, concurrency int) {
    var wg sync.WaitGroup
    start := time.Now()
    
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            resp, err := client.Get(url)
            if err != nil {
                fmt.Printf("请求失败: %v\n", err)
            } else {
                fmt.Printf("状态: %d\n", resp.StatusCode)
            }
        }()
    }
    
    wg.Wait()
    duration := time.Since(start)
    
    fmt.Printf("并发 %d 个请求，总耗时: %v\n", concurrency, duration)
    fmt.Printf("平均每个请求: %v\n", duration/time.Duration(concurrency))
}
```

### 问题：内存使用过高

**解决方案：**

1. **限制响应体大小**
```go
config := httpc.DefaultConfig()
config.MaxResponseBodySize = 10 * 1024 * 1024 // 10MB
client, err := httpc.New(config)
```

2. **及时释放资源**
```go
func processLargeResponse(url string) error {
    resp, err := client.Get(url)
    if err != nil {
        return err
    }
    
    // 立即处理大响应体
    if len(resp.RawBody) > 1024*1024 {
        // 处理数据
        processData(resp.RawBody)
        
        // 帮助 GC 回收
        resp.RawBody = nil
        resp.Body = ""
    }
    
    return nil
}
```

3. **监控内存使用**
```go
func monitorMemory() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    fmt.Printf("内存使用:\n")
    fmt.Printf("  分配的内存: %d KB\n", m.Alloc/1024)
    fmt.Printf("  总分配: %d KB\n", m.TotalAlloc/1024)
    fmt.Printf("  系统内存: %d KB\n", m.Sys/1024)
    fmt.Printf("  GC 次数: %d\n", m.NumGC)
}

// 在请求前后调用
monitorMemory()
resp, err := client.Get(url)
monitorMemory()
```

## 🔄 并发问题

### 问题：并发请求失败

**错误信息：**
```
too many open files
```

**解决方案：**

1. **限制并发数**
```go
// 使用 semaphore 控制并发
sem := make(chan struct{}, 10) // 最多10个并发

var wg sync.WaitGroup
for _, url := range urls {
    wg.Add(1)
    go func(u string) {
        defer wg.Done()
        
        sem <- struct{}{}        // 获取许可
        defer func() { <-sem }() // 释放许可
        
        resp, err := client.Get(u)
        // 处理响应...
    }(url)
}
wg.Wait()
```

2. **优化系统限制**
```bash
# 检查文件描述符限制
ulimit -n

# 临时增加限制
ulimit -n 65536

# 永久修改 /etc/security/limits.conf
* soft nofile 65536
* hard nofile 65536
```

3. **使用连接池**
```go
config := httpc.DefaultConfig()
config.MaxIdleConns = 100
config.MaxConnsPerHost = 20
client, err := httpc.New(config)
```

### 问题：竞态条件

**解决方案：**
```go
// 使用互斥锁保护共享状态
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *SafeCounter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}

// 或使用原子操作
var counter int64

func incrementCounter() {
    atomic.AddInt64(&counter, 1)
}

func getCounter() int64 {
    return atomic.LoadInt64(&counter)
}
```

## 📁 文件操作问题

### 问题：文件下载失败

**解决方案：**

1. **检查磁盘空间**
```go
func checkDiskSpace(path string) error {
    stat, err := os.Stat(filepath.Dir(path))
    if err != nil {
        return err
    }
    
    // 检查是否有写权限
    testFile := filepath.Join(filepath.Dir(path), ".test")
    f, err := os.Create(testFile)
    if err != nil {
        return fmt.Errorf("没有写权限: %w", err)
    }
    f.Close()
    os.Remove(testFile)
    
    return nil
}
```

2. **处理大文件下载**
```go
func downloadLargeFile(url, filePath string) error {
    opts := httpc.DefaultDownloadOptions(filePath)
    opts.ProgressCallback = func(downloaded, total int64, speed float64) {
        if total > 0 {
            percentage := float64(downloaded) / float64(total) * 100
            fmt.Printf("\r进度: %.1f%% (%s/%s) - %s",
                percentage,
                httpc.FormatBytes(downloaded),
                httpc.FormatBytes(total),
                httpc.FormatSpeed(speed),
            )
        }
    }
    
    result, err := client.DownloadWithOptions(url, opts,
        httpc.WithTimeout(30*time.Minute), // 长超时
    )
    
    if err != nil {
        return fmt.Errorf("下载失败: %w", err)
    }
    
    fmt.Printf("\n下载完成: %s\n", httpc.FormatBytes(result.BytesWritten))
    return nil
}
```

3. **断点续传**
```go
func resumableDownload(url, filePath string) error {
    opts := httpc.DefaultDownloadOptions(filePath)
    opts.ResumeDownload = true
    opts.Overwrite = false
    
    // 检查已存在的文件
    if info, err := os.Stat(filePath); err == nil {
        fmt.Printf("发现已存在文件，大小: %s\n", httpc.FormatBytes(info.Size()))
    }
    
    result, err := client.DownloadWithOptions(url, opts)
    if err != nil {
        return err
    }
    
    if result.Resumed {
        fmt.Println("下载已恢复")
    }
    
    return nil
}
```

### 问题：文件上传失败

**解决方案：**

1. **检查文件大小限制**
```go
func uploadFile(filePath string) error {
    info, err := os.Stat(filePath)
    if err != nil {
        return err
    }
    
    // 检查文件大小（httpc 默认限制 50MB）
    maxSize := int64(50 * 1024 * 1024)
    if info.Size() > maxSize {
        return fmt.Errorf("文件过大: %s (最大 %s)",
            httpc.FormatBytes(info.Size()),
            httpc.FormatBytes(maxSize),
        )
    }
    
    content, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }
    
    resp, err := client.Post("https://api.example.com/upload",
        httpc.WithFile("file", filepath.Base(filePath), content),
        httpc.WithTimeout(5*time.Minute),
    )
    
    return err
}
```

2. **分块上传大文件**
```go
func uploadLargeFile(filePath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()
    
    info, _ := file.Stat()
    chunkSize := 5 * 1024 * 1024 // 5MB chunks
    totalChunks := int(math.Ceil(float64(info.Size()) / float64(chunkSize)))
    
    for i := 0; i < totalChunks; i++ {
        chunk := make([]byte, chunkSize)
        n, err := file.Read(chunk)
        if err != nil && err != io.EOF {
            return err
        }
        
        chunk = chunk[:n]
        
        resp, err := client.Post("https://api.example.com/upload-chunk",
            httpc.WithBinary(chunk, "application/octet-stream"),
            httpc.WithHeader("X-Chunk-Number", strconv.Itoa(i)),
            httpc.WithHeader("X-Total-Chunks", strconv.Itoa(totalChunks)),
            httpc.WithTimeout(2*time.Minute),
        )
        
        if err != nil {
            return fmt.Errorf("上传块 %d 失败: %w", i, err)
        }
        
        if !resp.IsSuccess() {
            return fmt.Errorf("上传块 %d 失败: %d", i, resp.StatusCode)
        }
        
        fmt.Printf("已上传块 %d/%d\n", i+1, totalChunks)
    }
    
    return nil
}
```

## ⚙️ 配置问题

### 问题：配置验证失败

**解决方案：**
```go
func validateAndFixConfig(config *httpc.Config) (*httpc.Config, error) {
    if config == nil {
        return httpc.DefaultConfig(), nil
    }
    
    // 验证并修复配置
    if config.Timeout < 0 {
        fmt.Println("⚠️ 超时时间不能为负数，使用默认值")
        config.Timeout = 60 * time.Second
    }
    
    if config.MaxRetries < 0 {
        fmt.Println("⚠️ 重试次数不能为负数，设置为0")
        config.MaxRetries = 0
    }
    
    if config.MaxRetries > 10 {
        fmt.Println("⚠️ 重试次数过多，限制为10")
        config.MaxRetries = 10
    }
    
    if config.MaxIdleConns <= 0 {
        fmt.Println("⚠️ 连接池大小无效，使用默认值")
        config.MaxIdleConns = 100
    }
    
    // 使用 httpc 的验证函数
    if err := httpc.ValidateConfig(config); err != nil {
        return nil, fmt.Errorf("配置验证失败: %w", err)
    }
    
    return config, nil
}

// 使用
config := &httpc.Config{
    Timeout:    -1, // 无效值
    MaxRetries: 20, // 过大值
}

validConfig, err := validateAndFixConfig(config)
if err != nil {
    log.Fatal(err)
}

client, err := httpc.New(validConfig)
```

## 🔍 调试技巧

### 1. 启用详细日志

```go
type DebugClient struct {
    client httpc.Client
    logger *log.Logger
}

func NewDebugClient() (*DebugClient, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    
    logger := log.New(os.Stdout, "[HTTPC] ", log.LstdFlags|log.Lshortfile)
    
    return &DebugClient{
        client: client,
        logger: logger,
    }, nil
}

func (d *DebugClient) Get(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    d.logger.Printf("发起 GET 请求: %s", url)
    
    start := time.Now()
    resp, err := d.client.Get(url, opts...)
    duration := time.Since(start)
    
    if err != nil {
        d.logger.Printf("请求失败: %v (耗时: %v)", err, duration)
    } else {
        d.logger.Printf("请求成功: %d %s (耗时: %v, 大小: %d bytes)",
            resp.StatusCode, resp.Status, duration, len(resp.RawBody))
    }
    
    return resp, err
}
```

### 2. 使用 httpbin.org 调试

```go
func debugRequest() {
    // 检查发送的头部
    resp, err := client.Get("https://httpbin.org/headers",
        httpc.WithHeader("X-Custom", "test"),
        httpc.WithBearerToken("debug-token"),
    )
    if err == nil {
        fmt.Println("发送的头部:", resp.Body)
    }
    
    // 检查发送的数据
    testData := map[string]string{"key": "value"}
    resp, err = client.Post("https://httpbin.org/post",
        httpc.WithJSON(testData),
    )
    if err == nil {
        fmt.Println("发送的数据:", resp.Body)
    }
    
    // 测试不同的状态码
    statusCodes := []int{200, 404, 500}
    for _, code := range statusCodes {
        url := fmt.Sprintf("https://httpbin.org/status/%d", code)
        resp, err := client.Get(url)
        fmt.Printf("状态码 %d: 错误=%v, 响应=%v\n", code, err, resp != nil)
    }
}
```

### 3. 网络抓包分析

```bash
# 使用 tcpdump 抓包
sudo tcpdump -i any -w capture.pcap host api.example.com

# 使用 Wireshark 分析
wireshark capture.pcap

# 或使用 curl 对比
curl -v -H "Authorization: Bearer token" https://api.example.com/users
```

### 4. 性能分析

```go
func profileRequest(url string) {
    // CPU 性能分析
    f, err := os.Create("cpu.prof")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()
    
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()
    
    // 执行请求
    for i := 0; i < 100; i++ {
        resp, err := client.Get(url)
        if err != nil {
            fmt.Printf("请求 %d 失败: %v\n", i, err)
        }
    }
    
    // 内存分析
    f2, err := os.Create("mem.prof")
    if err != nil {
        log.Fatal(err)
    }
    defer f2.Close()
    
    runtime.GC()
    pprof.WriteHeapProfile(f2)
}

// 分析结果
// go tool pprof cpu.prof
// go tool pprof mem.prof
```

## 📞 获取帮助

如果以上解决方案都无法解决您的问题：

1. **检查版本兼容性**
```bash
go version
go list -m github.com/cybergodev/httpc
```

2. **创建最小复现示例**
```go
package main

import (
    "fmt"
    "github.com/cybergodev/httpc"
)

func main() {
    client, err := httpc.New()
    if err != nil {
        fmt.Printf("创建客户端失败: %v\n", err)
        return
    }
    defer client.Close()
    
    resp, err := client.Get("https://httpbin.org/get")
    if err != nil {
        fmt.Printf("请求失败: %v\n", err)
        return
    }
    
    fmt.Printf("状态: %d\n", resp.StatusCode)
}
```

3. **收集环境信息**
```bash
# 操作系统
uname -a

# Go 版本
go version

# 网络配置
ifconfig
cat /etc/resolv.conf

# 防火墙状态
sudo iptables -L
```

4. **提交 Issue**
   - 包含完整的错误信息
   - 提供最小复现代码
   - 说明环境信息
   - 描述期望的行为

---

💡 **提示**: 大多数问题都可以通过仔细阅读错误信息和检查配置来解决。如果问题持续存在，请不要犹豫寻求帮助。