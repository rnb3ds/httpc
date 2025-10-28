package httpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadProgressCallback is called periodically during download to report progress
type DownloadProgressCallback func(downloaded, total int64, speed float64)

// DownloadOptions configures file download behavior
type DownloadOptions struct {
	FilePath         string
	ProgressCallback DownloadProgressCallback
	Overwrite        bool
	ResumeDownload   bool
}

// DownloadResult contains information about a completed download
type DownloadResult struct {
	FilePath      string
	BytesWritten  int64
	Duration      time.Duration
	AverageSpeed  float64 // bytes per second
	StatusCode    int
	ContentLength int64
	Resumed       bool
}

func DefaultDownloadOptions(filePath string) *DownloadOptions {
	return &DownloadOptions{
		FilePath:       filePath,
		Overwrite:      false,
		ResumeDownload: false,
	}
}

func DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}

	downloadOpts := DefaultDownloadOptions(filePath)
	return client.(*clientImpl).downloadFile(context.Background(), url, downloadOpts, options...)
}

func (c *clientImpl) DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	downloadOpts := DefaultDownloadOptions(filePath)
	return c.downloadFile(context.Background(), url, downloadOpts, options...)
}

func (c *clientImpl) DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	return c.downloadFile(context.Background(), url, downloadOpts, options...)
}

// downloadFile implements the core download logic with security checks
func (c *clientImpl) downloadFile(ctx context.Context, url string, opts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("download options cannot be nil")
	}

	if opts.FilePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	// 验证URL安全性
	if err := validateDownloadURL(url); err != nil {
		return nil, fmt.Errorf("URL validation failed: %w", err)
	}

	if err := prepareFilePath(opts.FilePath); err != nil {
		return nil, fmt.Errorf("failed to prepare file path: %w", err)
	}
	var resumeOffset int64
	if fileInfo, err := os.Stat(opts.FilePath); err == nil {
		if !opts.Overwrite && !opts.ResumeDownload {
			return nil, fmt.Errorf("file already exists: %s (use Overwrite or ResumeDownload option)", opts.FilePath)
		}

		if opts.ResumeDownload {
			resumeOffset = fileInfo.Size()
			options = append(options, WithHeader("Range", fmt.Sprintf("bytes=%d-", resumeOffset)))
		}
	}

	startTime := time.Now()
	req := &Request{
		Method:      "GET",
		URL:         url,
		Context:     ctx,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]any),
	}

	for _, opt := range options {
		if opt != nil {
			opt(req)
		}
	}
	httpReq, err := http.NewRequestWithContext(req.Context, req.Method, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	if len(req.QueryParams) > 0 {
		q := httpReq.URL.Query()
		for key, value := range req.QueryParams {
			q.Add(key, fmt.Sprintf("%v", value))
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	for _, cookie := range req.Cookies {
		httpReq.AddCookie(cookie)
	}
	httpClient := &http.Client{
		Timeout: req.Timeout,
	}

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer httpResp.Body.Close()

	resumed := false
	if resumeOffset > 0 && httpResp.StatusCode == http.StatusPartialContent {
		resumed = true
	} else if resumeOffset > 0 && httpResp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		duration := time.Since(startTime)
		return &DownloadResult{
			FilePath:      opts.FilePath,
			BytesWritten:  resumeOffset,
			Duration:      duration,
			AverageSpeed:  0,
			StatusCode:    httpResp.StatusCode,
			ContentLength: resumeOffset,
			Resumed:       false,
		}, nil
	} else if resumeOffset > 0 {
		resumeOffset = 0
	}

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}
	contentLength := httpResp.ContentLength
	if resumed {
		contentLength += resumeOffset
	}

	var file *os.File
	if resumed {
		file, err = os.OpenFile(opts.FilePath, os.O_WRONLY|os.O_APPEND, 0644)
	} else {
		file, err = os.OpenFile(opts.FilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	bytesWritten, err := copyWithProgress(file, httpResp.Body, resumeOffset, contentLength, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	duration := time.Since(startTime)
	averageSpeed := float64(bytesWritten) / duration.Seconds()

	return &DownloadResult{
		FilePath:      opts.FilePath,
		BytesWritten:  bytesWritten,
		Duration:      duration,
		AverageSpeed:  averageSpeed,
		StatusCode:    httpResp.StatusCode,
		ContentLength: contentLength,
		Resumed:       resumed,
	}, nil
}

func prepareFilePath(filePath string) error {
	// 防止路径遍历攻击
	cleanPath := filepath.Clean(filePath)

	// 检查是否包含路径遍历尝试
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path traversal detected: %s", filePath)
	}

	// 确保路径是绝对路径或相对于当前目录的安全路径
	if filepath.IsAbs(cleanPath) {
		// 对于绝对路径，确保不在系统敏感目录
		if isSystemPath(cleanPath) {
			return fmt.Errorf("access to system path denied: %s", cleanPath)
		}
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	return nil
}

// isSystemPath 检查是否为系统敏感路径
func isSystemPath(path string) bool {
	systemPaths := []string{
		"/etc", "/sys", "/proc", "/dev", "/boot",
		"C:\\Windows", "C:\\System32", "C:\\Program Files",
	}

	cleanPath := strings.ToLower(filepath.Clean(path))
	for _, sysPath := range systemPaths {
		if strings.HasPrefix(cleanPath, strings.ToLower(sysPath)) {
			return true
		}
	}
	return false
}

func copyWithProgress(dst io.Writer, src io.Reader, offset, total int64, opts *DownloadOptions) (int64, error) {
	const bufferSize = 32 * 1024
	const progressInterval = 500 * time.Millisecond

	buffer := make([]byte, bufferSize)
	var written int64
	var lastProgress time.Time
	startTime := time.Now()
	var lastWritten int64

	for {
		nr, readErr := src.Read(buffer)
		if nr > 0 {
			nw, writeErr := dst.Write(buffer[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if writeErr != nil {
				return written, writeErr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}

			if opts.ProgressCallback != nil && time.Since(lastProgress) >= progressInterval {
				currentSpeed := float64(written-lastWritten) / time.Since(lastProgress).Seconds()
				opts.ProgressCallback(offset+written, total, currentSpeed)
				lastProgress = time.Now()
				lastWritten = written
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if opts.ProgressCallback != nil {
					elapsed := time.Since(startTime).Seconds()
					avgSpeed := float64(written) / elapsed
					opts.ProgressCallback(offset+written, total, avgSpeed)
				}
				break
			}
			return written, readErr
		}
	}

	return written, nil
}

// SaveToFile saves the response body to a file with security checks.
func (r *Response) SaveToFile(filePath string) error {
	if r.RawBody == nil {
		return fmt.Errorf("response body is empty")
	}

	// 使用相同的安全检查
	if err := prepareFilePath(filePath); err != nil {
		return fmt.Errorf("file path validation failed: %w", err)
	}

	cleanPath := filepath.Clean(filePath)
	if err := os.WriteFile(cleanPath, r.RawBody, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func FormatSpeed(bytesPerSecond float64) string {
	return FormatBytes(int64(bytesPerSecond)) + "/s"
}

// validateDownloadURL 验证下载URL的安全性
func validateDownloadURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// 解析URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// 只允许HTTP和HTTPS协议
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported protocol: %s (only http/https allowed)", parsedURL.Scheme)
	}

	// 检查主机名
	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// 防止本地文件访问
	if strings.HasPrefix(strings.ToLower(parsedURL.Host), "localhost") ||
		strings.HasPrefix(parsedURL.Host, "127.") ||
		parsedURL.Host == "::1" {
		return fmt.Errorf("localhost downloads are not allowed for security reasons")
	}

	return nil
}
