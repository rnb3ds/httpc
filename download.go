package httpc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DownloadProgressCallback func(downloaded, total int64, speed float64)

type DownloadOptions struct {
	FilePath         string
	ProgressCallback DownloadProgressCallback
	Overwrite        bool
	ResumeDownload   bool
}

type DownloadResult struct {
	FilePath      string
	BytesWritten  int64
	Duration      time.Duration
	AverageSpeed  float64
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

	return client.DownloadFile(url, filePath, options...)
}

func DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}

	return client.DownloadWithOptions(url, downloadOpts, options...)
}

func (c *clientImpl) DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	downloadOpts := DefaultDownloadOptions(filePath)
	return c.downloadFile(context.Background(), url, downloadOpts, options...)
}

func (c *clientImpl) DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	return c.downloadFile(context.Background(), url, downloadOpts, options...)
}

func (c *clientImpl) downloadFile(ctx context.Context, url string, opts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("download options cannot be nil")
	}

	if opts.FilePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	if err := prepareFilePath(opts.FilePath); err != nil {
		return nil, fmt.Errorf("failed to prepare file path: %w", err)
	}

	var resumeOffset int64
	if fileInfo, err := os.Stat(opts.FilePath); err == nil {
		if !opts.Overwrite && !opts.ResumeDownload {
			return nil, fmt.Errorf("file already exists: %s", opts.FilePath)
		}

		if opts.ResumeDownload {
			resumeOffset = fileInfo.Size()
			options = append(options, WithHeader("Range", fmt.Sprintf("bytes=%d-", resumeOffset)))
		}
	}

	resp, err := c.Request(ctx, "GET", url, options...)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}

	resumed := resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent

	if resumeOffset > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return &DownloadResult{
			FilePath:      opts.FilePath,
			BytesWritten:  0,
			Duration:      resp.Duration,
			AverageSpeed:  0,
			StatusCode:    resp.StatusCode,
			ContentLength: resumeOffset,
			Resumed:       false,
		}, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
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

	bytesWritten := int64(len(resp.RawBody))
	if _, err := file.Write(resp.RawBody); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	if opts.ProgressCallback != nil {
		totalSize := resp.ContentLength
		if resumed {
			totalSize += resumeOffset
		}
		speed := float64(bytesWritten) / resp.Duration.Seconds()
		opts.ProgressCallback(resumeOffset+bytesWritten, totalSize, speed)
	}

	averageSpeed := float64(bytesWritten) / resp.Duration.Seconds()

	return &DownloadResult{
		FilePath:      opts.FilePath,
		BytesWritten:  bytesWritten,
		Duration:      resp.Duration,
		AverageSpeed:  averageSpeed,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Resumed:       resumed,
	}, nil
}

func prepareFilePath(filePath string) error {
	if strings.TrimSpace(filePath) == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	if strings.ContainsAny(filePath, "\x00\r\n") {
		return fmt.Errorf("file path contains invalid characters")
	}

	if len(filePath) > 4096 {
		return fmt.Errorf("file path too long (max 4096 characters)")
	}

	cleanPath := filepath.Clean(filePath)

	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path traversal detected")
	}

	if filepath.IsAbs(cleanPath) && isSystemPath(cleanPath) {
		return fmt.Errorf("access to system path denied")
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	return nil
}

func isSystemPath(path string) bool {
	systemPaths := []string{
		"/etc/", "/sys/", "/proc/", "/dev/", "/boot/", "/root/",
		"/usr/bin/", "/usr/sbin/", "/bin/", "/sbin/",
		"c:\\windows\\", "c:\\system32\\", "c:\\program files\\",
		"c:\\programdata\\", "c:\\boot\\",
		"/library/", "/system/", "/applications/",
	}

	cleanPath := strings.ToLower(filepath.Clean(path))
	for _, sysPath := range systemPaths {
		if strings.HasPrefix(cleanPath, sysPath) {
			return true
		}
	}
	return false
}

func (r *Response) SaveToFile(filePath string) error {
	if r.RawBody == nil {
		return fmt.Errorf("response body is empty")
	}

	if err := prepareFilePath(filePath); err != nil {
		return fmt.Errorf("file path validation failed: %w", err)
	}

	cleanPath := filepath.Clean(filePath)
	if err := os.WriteFile(cleanPath, r.RawBody, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
