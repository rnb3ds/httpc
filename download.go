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

// DownloadProgressCallback is called during file download to report progress.
// Parameters: downloaded bytes, total bytes, current speed in bytes/second.
type DownloadProgressCallback func(downloaded, total int64, speed float64)

// DownloadOptions configures file download behavior.
type DownloadOptions struct {
	FilePath         string
	ProgressCallback DownloadProgressCallback
	Overwrite        bool
	ResumeDownload   bool
}

// DownloadResult contains information about a completed download.
type DownloadResult struct {
	FilePath      string
	BytesWritten  int64
	Duration      time.Duration
	AverageSpeed  float64
	StatusCode    int
	ContentLength int64
	Resumed       bool
}

// DefaultDownloadOptions creates download options with default settings.
// Overwrite and ResumeDownload are both false by default.
func DefaultDownloadOptions(filePath string) *DownloadOptions {
	return &DownloadOptions{
		FilePath:       filePath,
		Overwrite:      false,
		ResumeDownload: false,
	}
}

// DownloadFile downloads a file from the given URL to the specified file path using the default client.
// Returns DownloadResult with download statistics or an error if the download fails.
func DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}

	return client.DownloadFile(url, filePath, options...)
}

// DownloadWithOptions downloads a file with custom download options using the default client.
// Returns DownloadResult with download statistics or an error if the download fails.
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

func (c *clientImpl) downloadFile(ctx context.Context, url string, opts *DownloadOptions, options ...RequestOption) (result *DownloadResult, err error) {
	if opts == nil {
		return nil, fmt.Errorf("download options cannot be nil")
	}
	if opts.FilePath == "" {
		return nil, ErrEmptyFilePath
	}
	if err := prepareFilePath(opts.FilePath); err != nil {
		return nil, fmt.Errorf("failed to prepare file path: %w", err)
	}

	var resumeOffset int64
	if fileInfo, err := os.Stat(opts.FilePath); err == nil {
		if !opts.Overwrite && !opts.ResumeDownload {
			return nil, fmt.Errorf("%w: %s", ErrFileExists, opts.FilePath)
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

	statusCode := resp.Response.StatusCode
	rawBody := resp.Response.RawBody
	contentLength := resp.Response.ContentLength
	duration := resp.Meta.Duration

	resumed := resumeOffset > 0 && statusCode == http.StatusPartialContent
	if resumeOffset > 0 && statusCode == http.StatusRequestedRangeNotSatisfiable {
		return &DownloadResult{
			FilePath:      opts.FilePath,
			BytesWritten:  0,
			Duration:      duration,
			AverageSpeed:  0,
			StatusCode:    statusCode,
			ContentLength: resumeOffset,
			Resumed:       false,
		}, nil
	}

	if statusCode != http.StatusOK && statusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status code: %d", statusCode)
	}

	var file *os.File
	if resumed {
		file, err = os.OpenFile(opts.FilePath, os.O_WRONLY|os.O_APPEND, filePermissions)
	} else {
		file, err = os.OpenFile(opts.FilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermissions)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	bytesWritten := int64(len(rawBody))
	if _, err = file.Write(rawBody); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	avgSpeed := calculateSpeed(bytesWritten, duration)

	if opts.ProgressCallback != nil {
		totalSize := contentLength
		if resumed {
			totalSize += resumeOffset
		}
		opts.ProgressCallback(resumeOffset+bytesWritten, totalSize, avgSpeed)
	}

	return &DownloadResult{
		FilePath:      opts.FilePath,
		BytesWritten:  bytesWritten,
		Duration:      duration,
		AverageSpeed:  avgSpeed,
		StatusCode:    statusCode,
		ContentLength: contentLength,
		Resumed:       resumed,
	}, nil
}

const (
	maxFilePathLen  = 4096
	dirPermissions  = 0755
	filePermissions = 0644
)

var systemPaths = []string{
	"/etc/", "/sys/", "/proc/", "/dev/", "/boot/", "/root/",
	"/usr/bin/", "/usr/sbin/", "/bin/", "/sbin/",
	"c:/windows/", "c:/system32/", "c:/program files/", "c:/programdata/", "c:/boot/",
	"/library/", "/system/", "/applications/",
}

func calculateSpeed(bytes int64, duration time.Duration) float64 {
	if duration.Seconds() > 0 {
		return float64(bytes) / duration.Seconds()
	}
	return 0
}

// prepareFilePath validates and prepares file paths with comprehensive security checks.
// Prevents path traversal, system path access, and other file system attacks.
func prepareFilePath(filePath string) error {
	filePathLen := len(filePath)
	if filePathLen == 0 {
		return ErrEmptyFilePath
	}
	if filePathLen > maxFilePathLen {
		return fmt.Errorf("file path too long (max %d)", maxFilePathLen)
	}

	// Security check: block UNC paths early
	if filePathLen >= 2 && ((filePath[0] == '\\' && filePath[1] == '\\') || (filePath[0] == '/' && filePath[1] == '/')) {
		return fmt.Errorf("UNC paths not allowed for security")
	}

	// Validate characters in single pass for efficiency
	for i := range filePathLen {
		c := filePath[i]
		if c < 0x20 || c == 0x7F {
			return fmt.Errorf("file path contains invalid characters")
		}
		// Additional security: check for null bytes
		if c == 0 {
			return fmt.Errorf("null byte in file path")
		}
	}

	// Clean and resolve path
	cleanPath := filepath.Clean(filePath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Security check: prevent system path access
	if isSystemPath(absPath) {
		return fmt.Errorf("system path access denied for security")
	}

	// Path traversal protection
	if !filepath.IsAbs(filePath) && strings.Contains(cleanPath, "..") {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		wdAbs, err := filepath.Abs(wd)
		if err != nil {
			return fmt.Errorf("failed to resolve working directory: %w", err)
		}

		// Ensure resolved path is within working directory
		if !strings.HasPrefix(absPath+string(filepath.Separator), wdAbs+string(filepath.Separator)) {
			return fmt.Errorf("path traversal detected: path outside working directory")
		}
	}

	// Create directory structure if needed
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	return nil
}

func isSystemPath(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return true
	}

	cleanPath := strings.ToLower(filepath.Clean(absPath))
	cleanPath = strings.ReplaceAll(cleanPath, "\\", "/")

	for _, sysPath := range systemPaths {
		if strings.HasPrefix(cleanPath, sysPath) {
			return true
		}
	}
	return false
}

// FormatBytes formats bytes in human-readable format (e.g., "1.50 KB", "2.00 MB").
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	units := []rune{'K', 'M', 'G', 'T', 'P', 'E'}
	div := int64(unit)
	exp := 0

	for n := bytes / unit; n >= unit && exp < len(units)-1; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), units[exp])
}

// FormatSpeed formats speed in human-readable format (e.g., "1.50 KB/s", "2.00 MB/s").
func FormatSpeed(bytesPerSecond float64) string {
	const unit = 1024.0
	if bytesPerSecond < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}

	units := []string{"KB/s", "MB/s", "GB/s", "TB/s", "PB/s", "EB/s"}
	div := unit

	for exp := range len(units) {
		if bytesPerSecond < div*unit || exp == len(units)-1 {
			return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[exp])
		}
		div *= unit
	}

	return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[len(units)-1])
}
