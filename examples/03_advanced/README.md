# Advanced Usage Examples

**Time to complete: 15 minutes**

This directory demonstrates advanced features for production-grade HTTP client usage.

## What You'll Learn

- Client configuration and customization
- Timeout and retry strategies
- Context usage and cancellation
- File uploads (single and multiple)
- File downloads with progress tracking
- Concurrent request handling
- Performance optimization


## Examples Overview

### 1. Client Configuration (`client_config.go`)

Learn how to configure the HTTP client for different scenarios:

- **Default Configuration**: Production-ready defaults
- **Custom Configuration**: Tailor settings to your needs
- **Secure Client**: Enhanced security settings
- **TLS Configuration**: Custom TLS settings
- **Connection Pooling**: Optimize connection reuse

**Key Concepts:**
- Timeout settings (dial, TLS handshake, response header, idle)
- Connection limits (max idle, per host)
- Security settings (TLS version, certificate validation)
- Retry configuration (max retries, backoff, jitter)
- HTTP/2 support

### 2. Timeout and Retry (`timeout_retry.go`)

Master timeout and retry strategies:

- **Basic Timeout**: Simple request timeouts
- **Context Timeout**: Context-based timeouts
- **Retry Configuration**: Configure retry behavior
- **Combined Strategies**: Timeout + retry for resilience
- **Disable Retries**: When retries aren't appropriate

**Patterns:**
- Health checks: Short timeout, no retry
- Critical operations: Long timeout, multiple retries
- User-facing requests: Moderate timeout, few retries
- Background jobs: Very long timeout, many retries

### 3. Context Usage (`context_usage.go`)

Leverage Go contexts effectively:

- **Timeout Context**: Automatic cancellation after timeout
- **Cancellation Context**: Manual cancellation
- **Deadline Context**: Cancel at specific time
- **Value Context**: Pass request-scoped values
- **Parent-Child Contexts**: Hierarchical cancellation

**Use Cases:**
- Request cancellation when user navigates away
- Timeout enforcement across multiple operations
- Graceful shutdown
- Request tracing and correlation IDs

### 4. File Upload (`file_upload.go`)

Handle file uploads efficiently:

- **Single File**: Upload one file
- **Multiple Files**: Upload multiple files at once
- **File with Fields**: Combine files and form data
- **Large Files**: Handle large uploads with proper timeouts
- **Progress Tracking**: Monitor upload progress

**Patterns:**
- Avatar uploads
- Document uploads with metadata
- Batch file uploads
- Large file handling

### 5. File Download (`file_download.go`)

Learn how to download files efficiently:

- **Simple Download**: Basic file download
- **Progress Tracking**: Real-time download progress
- **Large Files**: Optimized for large file downloads
- **Resume Downloads**: Resume interrupted downloads
- **Save Response**: Alternative method to save responses
- **Authenticated Downloads**: Download protected files

**Features:**
- Streaming downloads (memory efficient)
- Progress callbacks with speed tracking
- Automatic directory creation
- Resume support with Range requests
- Overwrite protection
- Custom buffer sizes

**Quick Example:**
```go
// Simple download
result, err := httpc.Download(
    "https://example.com/file.zip",
    "downloads/file.zip",
)

// With progress tracking
opts := httpc.DefaultDownloadOptions("downloads/file.zip")
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rProgress: %.1f%% - %s",
        percentage,
        httpc.FormatSpeed(speed),
    )
}
result, err := client.DownloadFileWithOptions(url, opts)
```

### 6. Concurrent Requests (`concurrent.go`)

Make multiple requests efficiently:

- **Parallel Requests**: Execute requests concurrently
- **Worker Pool**: Limit concurrent requests
- **Error Handling**: Handle errors in concurrent scenarios
- **Result Aggregation**: Collect and process results
- **Rate Limiting**: Control request rate

**Patterns:**
- Batch API calls
- Fan-out/fan-in
- Parallel data fetching
- Concurrent uploads/downloads

## Configuration Reference

### Default Configuration

```go
config := httpc.DefaultConfig()
// Timeout: 60s
// MaxRetries: 2
// RetryDelay: 2s
// MaxIdleConns: 100
// MaxIdleConnsPerHost: 10
// MaxConnsPerHost: 20
// MaxConcurrentRequests: 500
// TLS: 1.2+
// HTTP/2: Enabled
```

### Custom Configuration

```go
config := &httpc.Config{
    // Network settings
    Timeout:               30 * time.Second,
    DialTimeout:           15 * time.Second,
    KeepAlive:             30 * time.Second,
    TLSHandshakeTimeout:   15 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
    IdleConnTimeout:       90 * time.Second,
    MaxIdleConns:          100,
    MaxIdleConnsPerHost:   10,
    MaxConnsPerHost:       20,

    // Security settings
    MinTLSVersion:         tls.VersionTLS12,
    MaxTLSVersion:         tls.VersionTLS13,
    InsecureSkipVerify:    false,
    MaxResponseBodySize:   50 * 1024 * 1024, // 50 MB
    MaxConcurrentRequests: 500,
    ValidateURL:           true,
    ValidateHeaders:       true,

    // Retry settings
    MaxRetries:    2,
    RetryDelay:    2 * time.Second,
    MaxRetryDelay: 60 * time.Second,
    BackoffFactor: 2.0,
    Jitter:        true,

    // Headers and features
    UserAgent:       "MyApp/1.0",
    FollowRedirects: true,
    EnableHTTP2:     true,
}

client, err := httpc.New(config)
```

### Secure Client

```go
client, err := httpc.NewSecureClient()
// TLS 1.2-1.3, 15s timeout, 2 max retries
```

## Timeout Strategies

### Quick Operations (< 5s)
```go
httpc.WithTimeout(2 * time.Second)
httpc.WithMaxRetries(0)
```

### Standard Operations (5-15s)
```go
httpc.WithTimeout(10 * time.Second)
httpc.WithMaxRetries(2)
```

### Long Operations (15-60s)
```go
httpc.WithTimeout(30 * time.Second)
httpc.WithMaxRetries(3)
```

### Background Jobs (> 60s)
```go
httpc.WithTimeout(120 * time.Second)
httpc.WithMaxRetries(5)
```

## Context Patterns

### Request Timeout
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Get(url, httpc.WithContext(ctx))
```

### Manual Cancellation
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Cancel from another goroutine when needed
go func() {
    <-stopChan
    cancel()
}()

resp, err := client.Get(url, httpc.WithContext(ctx))
```

### Deadline
```go
deadline := time.Now().Add(1 * time.Hour)
ctx, cancel := context.WithDeadline(context.Background(), deadline)
defer cancel()

resp, err := client.Get(url, httpc.WithContext(ctx))
```

## File Upload Patterns

### Single File
```go
fileContent := []byte("file content")
resp, err := client.Post(url,
    httpc.WithFile("file", "document.pdf", fileContent),
)
```

### Multiple Files with Metadata
```go
formData := &httpc.FormData{
    Fields: map[string]string{
        "title": "My Document",
        "tags":  "important,urgent",
    },
    Files: map[string]*httpc.FileData{
        "document": {
            Filename:    "doc.pdf",
            Content:     pdfContent,
            ContentType: "application/pdf",
        },
        "thumbnail": {
            Filename:    "thumb.jpg",
            Content:     jpgContent,
            ContentType: "image/jpeg",
        },
    },
}

resp, err := client.Post(url,
    httpc.WithFormData(formData),
    httpc.WithTimeout(60*time.Second),
)
```

## Concurrent Request Patterns

### Parallel Requests
```go
urls := []string{url1, url2, url3}
results := make(chan *httpc.Response, len(urls))

for _, url := range urls {
    go func(u string) {
        resp, err := client.Get(u)
        if err == nil {
            results <- resp
        }
    }(url)
}

// Collect results
for i := 0; i < len(urls); i++ {
    resp := <-results
    // Process response
}
```

### Worker Pool
```go
const workers = 10
jobs := make(chan string, 100)
results := make(chan *httpc.Response, 100)

// Start workers
for w := 0; w < workers; w++ {
    go func() {
        for url := range jobs {
            resp, err := client.Get(url)
            if err == nil {
                results <- resp
            }
        }
    }()
}

// Send jobs
for _, url := range urls {
    jobs <- url
}
close(jobs)
```

## Best Practices

1. **Configure for your use case**: Don't use default config blindly
2. **Set appropriate timeouts**: Match timeout to operation type
3. **Use contexts**: Enable cancellation and timeout control
4. **Handle retries wisely**: Not all operations should retry
5. **Limit concurrency**: Use worker pools for many requests
6. **Monitor performance**: Track duration and attempts
7. **Secure by default**: Use TLS 1.2+, validate certificates
8. **Connection pooling**: Reuse clients, don't create per-request

## Performance Tips

- **Reuse clients**: Create once, use many times
- **Connection pooling**: Configured automatically
- **HTTP/2**: Enabled by default for multiplexing
- **Concurrent requests**: Use goroutines with worker pools
- **Timeouts**: Prevent hanging requests
- **Retries**: Use exponential backoff with jitter

## Next Steps

After mastering advanced usage:
- **[Real-World Examples](../04_real_world)** - See complete implementations

## Tips

- Start with default configuration and adjust as needed
- Use `NewSecureClient()` for enhanced security
- Always set timeouts for production code
- Use contexts for cancellation support
- Monitor `resp.Duration` and `resp.Attempts` for performance insights
- Configure connection limits based on your infrastructure

