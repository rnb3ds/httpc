# File Download Guide

This guide covers all aspects of downloading files using HTTPC, from simple downloads to advanced features like progress tracking and resume support.

## Table of Contents

- [Quick Start](#quick-start)
- [Basic Download](#basic-download)
- [Progress Tracking](#progress-tracking)
- [Large Files](#large-files)
- [Authentication](#authentication)
- [Advanced Options](#advanced-options)
- [Best Practices](#best-practices)

## Quick Start

### Simple Download (Package-Level Function)

The easiest way to download a file - no need to create a client:

```go
package main

import (
    "fmt"
    "log"
    "strings"
    "time"
    "github.com/cybergodev/httpc"
)

func main() {
    // Download a file using package-level function
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

For reusable clients or when making multiple requests, create a client instance:

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

Track download completion:

**Note**: The current implementation loads the entire response into memory before writing to disk, so the progress callback is called once at the end with final statistics. This is suitable for most files but may not provide real-time progress updates during the download.

```go
opts := httpc.DefaultDownloadOptions("downloads/large-file.zip")
opts.Overwrite = true

opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    if total > 0 {
        percentage := float64(downloaded) / float64(total) * 100
        fmt.Printf("\r[%.1f%%] %s / %s - %s/s    ",
            percentage,
            httpc.FormatBytes(downloaded),
            httpc.FormatBytes(total),
            httpc.FormatBytes(int64(speed)),
        )
    } else {
        // Total size unknown
        fmt.Printf("\rDownloaded: %s - %s/s    ",
            httpc.FormatBytes(downloaded),
            httpc.FormatBytes(int64(speed)),
        )
    }
}

result, err := client.DownloadWithOptions(url, opts)
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
        fmt.Printf("\r[%s] %.1f%% - %s/s",
            bar,
            percentage*100,
            httpc.FormatBytes(int64(speed)),
        )
    }
}
```

## Large Files

### Optimized for Large Files

Configure for optimal large file downloads:

```go
opts := httpc.DefaultDownloadOptions("downloads/large-video.mp4")
opts.Overwrite = true

opts.ProgressCallback = func(downloaded, total int64, speed float64) {
    if total > 0 {
        percentage := float64(downloaded) / float64(total) * 100
        remaining := total - downloaded
        var eta time.Duration
        if speed > 0 {
            eta = time.Duration(float64(remaining)/speed) * time.Second
        }
        
        fmt.Printf("\r%.1f%% - %s/%s - %s/s - ETA: %v    ",
            percentage,
            httpc.FormatBytes(downloaded),
            httpc.FormatBytes(total),
            httpc.FormatBytes(int64(speed)),
            eta.Round(time.Second),
        )
    }
}

result, err := client.DownloadWithOptions(
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
    
    // Progress tracking
    ProgressCallback: func(downloaded, total int64, speed float64) {
        // Your progress handler
    },
}

result, err := client.DownloadWithOptions(url, opts)
```

**Available Options:**
- `FilePath` (string) - Destination file path (required)
- `Overwrite` (bool) - Overwrite existing files (default: false)
- `ResumeDownload` (bool) - Resume partial downloads (default: false)
- `ProgressCallback` (func) - Progress tracking callback (optional)

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

### Manual Streaming (for very large files)

For maximum control over large file downloads:

```go
import (
    "io"
    "os"
)

// Make a regular GET request
resp, err := client.Get("https://example.com/very-large-file.bin")
if err != nil {
    log.Fatal(err)
}

// Create file
file, err := os.Create("large-file.bin")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Copy response body to file
// Note: This requires implementing custom streaming if needed
_, err = file.Write(resp.RawBody)
if err != nil {
    log.Fatal(err)
}
```

## Implementation Notes

### Current Behavior

The HTTPC download implementation:
- Loads the entire response into memory before writing to disk
- Progress callback is called once at completion with final statistics
- Supports resume downloads using HTTP Range requests
- Automatically creates parent directories
- Includes security checks to prevent path traversal attacks

### Memory Considerations

For very large files (>100MB), consider:
- Using the regular `Get()` method and handling the response stream manually
- Monitoring available memory during downloads
- Using resume functionality to handle interrupted downloads

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

## Helper Functions

### Format Bytes

```go
size := httpc.FormatBytes(1048576)  // "1.00 MB"
```

### Format Speed

```go
// FormatSpeed expects bytes per second as float64
speed := httpc.FormatSpeed(1048576.0)  // "1.00 MB/s"

// Or convert from int64
bytesPerSec := int64(1048576)
speedStr := httpc.FormatBytes(bytesPerSec) + "/s"  // "1.00 MB/s"
```

## Examples

See the [file_download.go](../examples/03_advanced/file_download.go) example for complete working code.


---
