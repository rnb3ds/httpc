# Advanced Usage Examples

**Time to complete: 15 minutes**

This directory demonstrates advanced features for production-grade HTTP client usage.

## What You'll Learn

- Timeout and retry strategies
- Context usage for cancellation and timeouts
- File uploads (single and multiple)
- File downloads with progress tracking
- Production-grade resilience patterns


## Examples Overview

### 1. Timeout and Retry (`timeout_retry.go`)

Master timeout and retry strategies for resilient applications:

- **Basic Timeout**: Simple request timeouts
- **Context Timeout**: Context-based timeouts
- **Retry Configuration**: Configure retry behavior
- **Combined Strategies**: Timeout + retry for resilience
- **Disable Retries**: When retries aren't appropriate

**Key Patterns:**
- Health checks: Short timeout, no retry
- Critical operations: Long timeout, multiple retries
- User-facing requests: Moderate timeout, few retries
- Background jobs: Very long timeout, many retries

**Quick Example:**
```go
// Basic timeout
resp, err := client.Get(url,
    httpc.WithTimeout(10*time.Second),
)

// With retry
resp, err := client.Post(url,
    httpc.WithJSON(data),
    httpc.WithTimeout(10*time.Second),
    httpc.WithMaxRetries(3),
)

// Context-based timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Get(url,
    httpc.WithContext(ctx),
)
```

### 2. File Upload (`file_upload.go`)

Handle file uploads efficiently:

- **Single File**: Upload one file
- **Multiple Files**: Upload multiple files at once
- **File with Fields**: Combine files and form data
- **Large Files**: Handle large uploads with proper timeouts

**Common Patterns:**
- Avatar/profile picture uploads
- Document uploads with metadata
- Batch file uploads
- Large file handling with extended timeouts

**Quick Example:**
```go
// Single file upload
fileContent := []byte("file content")
resp, err := client.Post(url,
    httpc.WithFile("file", "document.pdf", fileContent),
)

// Multiple files with metadata
formData := &httpc.FormData{
    Fields: map[string]string{
        "title": "My Document",
        "tags":  "important",
    },
    Files: map[string]*httpc.FileData{
        "document": {
            Filename: "doc.pdf",
            Content:  pdfContent,
        },
        "thumbnail": {
            Filename: "thumb.jpg",
            Content:  jpgContent,
        },
    },
}

resp, err := client.Post(url,
    httpc.WithFormData(formData),
    httpc.WithTimeout(60*time.Second),
)
```

### 3. File Download (`file_download.go`)

Learn how to download files efficiently with comprehensive examples:

- **Simple Download**: Basic file download with `DownloadFile()`
- **Progress Tracking**: Real-time download progress with callbacks
- **Large Files**: Optimized for large file downloads with streaming
- **Resume Downloads**: Resume interrupted downloads using Range requests
- **Save Response**: Alternative method using `Response.SaveToFile()`
- **Authenticated Downloads**: Download protected files with auth headers

**Key Features:**
- Streaming downloads (memory efficient)
- Progress callbacks with speed tracking
- Automatic directory creation
- Resume support with Range requests
- Overwrite protection
- Custom buffer sizes

**Quick Example:**
```go
// Simple download
result, err := client.DownloadFile(
    "https://example.com/file.zip",
    "downloads/file.zip",
)

// With progress tracking
opts := &httpc.DownloadOptions{
    FilePath:  "downloads/file.zip",
    Overwrite: true,
    ProgressCallback: func(downloaded, total int64, speed float64) {
        percentage := float64(downloaded) / float64(total) * 100
        fmt.Printf("\rProgress: %.1f%% - %s",
            percentage,
            httpc.FormatSpeed(speed),
        )
    },
}
result, err := client.DownloadWithOptions(url, opts)

// Resume interrupted download
opts.ResumeDownload = true
result, err := client.DownloadWithOptions(url, opts)
```

## Client Configuration

The httpc library provides flexible configuration options. While these examples focus on request-level options, you can also configure the client globally.

### Creating Clients

```go
// Default configuration (recommended for most use cases)
client, err := httpc.New()

// Secure client with enhanced security settings
client, err := httpc.NewSecure()

// Performance-optimized client
client, err := httpc.NewPerformance()

// Custom configuration
config := httpc.DefaultConfig()
config.Timeout = 30 * time.Second
config.MaxRetries = 3
client, err := httpc.New(config)
```

For detailed configuration options, see the main [USAGE_GUIDE.md](../../USAGE_GUIDE.md).

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

