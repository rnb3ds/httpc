# File Download Guide

This guide covers all aspects of downloading files using HTTPC, from simple downloads to advanced features like progress tracking and resume support.

## Table of Contents

- [Quick Start](#quick-start)
- [Basic Download](#basic-download)
- [Progress Tracking](#progress-tracking)
- [Resume Downloads](#resume-downloads)
- [Large Files](#large-files)
- [Authentication](#authentication)
- [Advanced Options](#advanced-options)
- [Best Practices](#best-practices)

## Quick Start

### Simple Download

The easiest way to download a file:

```go
package main

import (
    "log"
    "github.com/cybergodev/httpc"
)

func main() {
    // Download a file
    result, err := httpc.DownloadFile(
        "https://example.com/file.zip",
        "downloads/file.zip",
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Downloaded %s in %v",
        httpc.FormatBytes(result.BytesWritten),
        result.Duration,
    )
}
```

## Basic Download

### Using Client Instance

For reusable clients, create a client instance:

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Download file
result, err := client.DownloadFile(
    "https://example.com/document.pdf",
    "downloads/document.pdf",
)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("File: %s\n", result.FilePath)
fmt.Printf("Size: %s\n", httpc.FormatBytes(result.BytesWritten))
fmt.Printf("Speed: %s\n", httpc.FormatSpeed(result.AverageSpeed))
```

### With Request Options

Combine download with standard request options:

```go
result, err := client.DownloadFile(
    "https://api.example.com/files/report.pdf",
    "downloads/report.pdf",
    httpc.WithBearerToken("your-token"),
    httpc.WithTimeout(5*time.Minute),
    httpc.WithMaxRetries(3),
)
```

## Progress Tracking

### Real-time Progress

Track download progress in real-time:

```go
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.Overwrite = true
opts.ProgressInterval = 500 * time.Millisecond

opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    if total > 0 {
        percentage := float64(downloaded) / float64(total) * 100
        fmt.Printf("\r[%.1f%%] %s / %s - %s    ",
            percentage,
            httpc.FormatBytes(downloaded),
            httpc.FormatBytes(total),
            httpc.FormatSpeed(speed),
        )
    } else {
        // Total size unknown
        fmt.Printf("\rDownloaded: %s - %s    ",
            httpc.FormatBytes(downloaded),
            httpc.FormatSpeed(speed),
        )
    }
}

result, err := client.DownloadFileWithOptions(url, opts)
fmt.Println() // New line after progress
```

### Progress Bar Example

Create a simple progress bar:

```go
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    if total > 0 {
        percentage := float64(downloaded) / float64(total)
        barWidth := 50
        filled := int(percentage * float64(barWidth))
        
        bar := strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled)
        fmt.Printf("\r[%s] %.1f%% - %s",
            bar,
            percentage*100,
            httpc.FormatSpeed(speed),
        )
    }
}
```

## Resume Downloads

### Basic Resume

Resume interrupted downloads automatically:

```go
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.ResumeDownload = true  // Enable resume
opts.Overwrite = false      // Don't overwrite existing file

result, err := client.DownloadFileWithOptions(url, opts)
if err != nil {
    log.Fatal(err)
}

if result.Resumed {
    fmt.Println("✓ Download resumed from previous position")
} else {
    fmt.Println("✓ Download completed (server doesn't support resume)")
}
```

### Resume with Progress

Combine resume with progress tracking:

```go
opts := httpc.DefaultDownloadOptions("downloads/movie.mp4")
opts.ResumeDownload = true
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    fmt.Printf("\rResuming: %.1f%% - %s", percentage, httpc.FormatSpeed(speed))
}

result, err := client.DownloadFileWithOptions(url, opts)
```

### Retry Failed Downloads

Automatically retry with resume:

```go
const maxAttempts = 3
var result *httpc.DownloadResult
var err error

for attempt := 1; attempt <= maxAttempts; attempt++ {
    opts := httpc.DefaultDownloadOptions("downloads/file.zip")
    opts.ResumeDownload = true
    
    result, err = client.DownloadFileWithOptions(
        url,
        opts,
        httpc.WithTimeout(10*time.Minute),
    )
    
    if err == nil {
        break
    }
    
    log.Printf("Attempt %d failed: %v", attempt, err)
    time.Sleep(time.Second * time.Duration(attempt))
}
```

## Large Files

### Optimized for Large Files

Configure for optimal large file downloads:

```go
opts := httpc.DefaultDownloadOptions("downloads/large-video.mp4")
opts.BufferSize = 128 * 1024  // 128KB buffer for better performance
opts.Overwrite = true
opts.ProgressInterval = 1 * time.Second  // Update less frequently

opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    percentage := float64(downloaded) / float64(total) * 100
    eta := time.Duration(float64(total-downloaded)/speed) * time.Second
    
    fmt.Printf("\r%.1f%% - %s/%s - %s - ETA: %v    ",
        percentage,
        httpc.FormatBytes(downloaded),
        httpc.FormatBytes(total),
        httpc.FormatSpeed(speed),
        eta.Round(time.Second),
    )
}

result, err := client.DownloadFileWithOptions(
    url,
    opts,
    httpc.WithTimeout(30*time.Minute),  // Longer timeout
    httpc.WithMaxRetries(5),            // More retries
)
```

## Authentication

### Bearer Token

Download protected files with authentication:

```go
result, err := client.DownloadFile(
    "https://api.example.com/files/private.zip",
    "downloads/private.zip",
    httpc.WithBearerToken("your-api-token"),
)
```

### Basic Auth

```go
result, err := client.DownloadFile(
    "https://secure.example.com/file.zip",
    "downloads/file.zip",
    httpc.WithBasicAuth("username", "password"),
)
```

### Custom Headers

```go
result, err := client.DownloadFile(
    url,
    filePath,
    httpc.WithHeader("X-API-Key", "your-api-key"),
    httpc.WithHeader("X-Client-ID", "client-123"),
)
```

## Advanced Options

### All Download Options

```go
opts := &httpc.DownloadOptions{
    // Required
    FilePath: "downloads/file.zip",
    
    // File handling
    Overwrite:      true,      // Overwrite existing files
    ResumeDownload: false,     // Resume partial downloads
    CreateDirs:     true,      // Create parent directories
    FileMode:       0644,      // File permissions (Unix)
    
    // Performance
    BufferSize: 32 * 1024,     // 32KB buffer (default)
    
    // Progress tracking
    ProgressInterval: 500 * time.Millisecond,
    ProgressCallback: func(downloaded, total int64, speed float64) {
        // Your progress handler
    },
}

result, err := client.DownloadFileWithOptions(url, opts)
```

### Save Response to File

Alternative method for small files:

```go
// Make a regular GET request
resp, err := client.Get("https://example.com/data.json")
if err != nil {
    log.Fatal(err)
}

// Save response to file
err = resp.SaveToFile("data.json")
if err != nil {
    log.Fatal(err)
}
```

## Best Practices

### 1. Always Close the Client

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()  // Important!
```

### 2. Use Appropriate Timeouts

```go
// Small files (< 10MB)
httpc.WithTimeout(1 * time.Minute)

// Medium files (10-100MB)
httpc.WithTimeout(5 * time.Minute)

// Large files (> 100MB)
httpc.WithTimeout(30 * time.Minute)
```

### 3. Enable Resume for Large Files

```go
opts := httpc.DefaultDownloadOptions(filePath)
opts.ResumeDownload = true  // Always enable for large files
```

### 4. Handle Errors Gracefully

```go
result, err := client.DownloadFile(url, filePath)
if err != nil {
    // Check if it's a partial download
    if fileInfo, statErr := os.Stat(filePath); statErr == nil {
        log.Printf("Partial download: %s", httpc.FormatBytes(fileInfo.Size()))
        log.Printf("Use ResumeDownload to continue")
    }
    return err
}
```

### 5. Verify Downloads

```go
result, err := client.DownloadFile(url, filePath)
if err != nil {
    log.Fatal(err)
}

// Verify file size
fileInfo, _ := os.Stat(filePath)
if fileInfo.Size() != result.BytesWritten {
    log.Fatal("File size mismatch")
}

// Verify checksum (if available)
if expectedChecksum != "" {
    actualChecksum := calculateChecksum(filePath)
    if actualChecksum != expectedChecksum {
        log.Fatal("Checksum mismatch")
    }
}
```

### 6. Use Progress for User Feedback

```go
opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    // Update UI, log progress, or send to monitoring system
    metrics.RecordDownloadProgress(downloaded, total, speed)
}
```

## Helper Functions

### Format Bytes

```go
size := httpc.FormatBytes(1048576)  // "1.00 MB"
```

### Format Speed

```go
speed := httpc.FormatSpeed(1048576)  // "1.00 MB/s"
```

## Examples

See the [file_download.go](../examples/03_advanced/file_download.go) example for complete working code.

## Related Documentation

- [Getting Started](getting-started.md) - Basic usage
- [Request Options](request-options.md) - Authentication and timeout options
- [Error Handling](error-handling.md) - Handling download errors

---
