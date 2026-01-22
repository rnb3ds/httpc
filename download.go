package httpc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// getSystemPaths returns platform-specific system paths that should be protected.
func getSystemPaths() []string {
	switch GetOS() {
	case "windows":
		return []string{
			"c:\\windows\\", "c:\\system32\\",
			"c:\\program files\\", "c:\\programdata\\",
			"c:\\program files (x86)\\",
			// Environment variables that typically point to system directories
			"%systemroot%", "%windir%", "%programfiles%", "%programfiles(x86)%",
		}
	case "darwin":
		return []string{
			"/system/", "/library/", "/applications/",
			"/usr/", "/bin/", "/sbin/", "/etc/", "/var/",
		}
	case "linux":
		fallthrough
	default:
		return []string{
			"/etc/", "/sys/", "/proc/", "/dev/", "/boot/", "/root/",
			"/usr/bin/", "/usr/sbin/", "/bin/", "/sbin/",
			"/lib/", "/lib64/", "/run/", "/sys/fs/",
		}
	}
}

// GetOS returns the current operating system. Useful for testing.
var GetOS = func() string {
	return getOS()
}

func getOS() string {
	// Check if the OS is Windows by looking at PATH separator
	// This is a runtime check that works better than build tags for this case
	// since the binary may be compiled for cross-platform use
	// We'll use the runtime.GOOS as the primary method
	return runtime.GOOS
}

func calculateSpeed(bytes int64, duration time.Duration) float64 {
	if duration.Seconds() > 0 {
		return float64(bytes) / duration.Seconds()
	}
	return 0
}

// prepareFilePath validates and prepares file paths with security checks.
func prepareFilePath(filePath string) error {
	filePathLen := len(filePath)
	if filePathLen == 0 {
		return ErrEmptyFilePath
	}
	if filePathLen > maxFilePathLen {
		return fmt.Errorf("file path too long (max %d)", maxFilePathLen)
	}

	// Check for UNC paths
	if filePathLen >= 2 {
		if (filePath[0] == '\\' && filePath[1] == '\\') || (filePath[0] == '/' && filePath[1] == '/') {
			return fmt.Errorf("UNC paths not allowed for security")
		}
	}

	// Validate characters
	for i := 0; i < filePathLen; i++ {
		c := filePath[i]
		if c < 0x20 || c == 0x7F || c == 0 {
			return fmt.Errorf("file path contains invalid characters at position %d", i)
		}
	}

	cleanPath := filepath.Clean(filePath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check for system path access
	if isSystemPath(absPath) {
		return fmt.Errorf("system path access denied for security")
	}

	// Check for path traversal
	if !filepath.IsAbs(filePath) && strings.Contains(cleanPath, "..") {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		wdAbs, err := filepath.Abs(wd)
		if err != nil {
			return fmt.Errorf("failed to resolve working directory: %w", err)
		}

		if !strings.HasPrefix(absPath+string(filepath.Separator), wdAbs+string(filepath.Separator)) {
			return fmt.Errorf("path traversal detected: path outside working directory")
		}
	}

	// Create directories
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

	cleanPath := filepath.Clean(absPath)

	// On Windows, do case-insensitive comparison
	// On Unix systems, do case-sensitive comparison
	systemPaths := getSystemPaths()

	for _, sysPath := range systemPaths {
		// Handle environment variable patterns on Windows
		if strings.HasPrefix(sysPath, "%") && runtime.GOOS == "windows" {
			// Expand environment variables
			expanded := os.ExpandEnv(sysPath)
			if expanded != sysPath {
				// Check if expanded path matches
				if strings.HasPrefix(strings.ToLower(cleanPath), strings.ToLower(expanded)) {
					return true
				}
			}
		}

		// Convert both paths to use the same separator for comparison
		cleanPathForCompare := cleanPath
		sysPathForCompare := sysPath

		if runtime.GOOS == "windows" {
			// Windows: case-insensitive, use backslashes
			cleanPathForCompare = strings.ToLower(cleanPath)
			cleanPathForCompare = strings.ReplaceAll(cleanPathForCompare, "/", "\\")
			sysPathForCompare = strings.ToLower(sysPathForCompare)
		} else {
			// Unix: case-sensitive, use forward slashes
			cleanPathForCompare = strings.ReplaceAll(cleanPathForCompare, "\\", "/")
			sysPathForCompare = strings.ReplaceAll(sysPathForCompare, "\\", "/")
		}

		if strings.HasPrefix(cleanPathForCompare, sysPathForCompare) {
			return true
		}
	}

	return false
}

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	units := [6]byte{'K', 'M', 'G', 'T', 'P', 'E'}
	div := int64(unit)
	exp := 0

	for n := bytes / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), units[exp])
}

func FormatSpeed(bytesPerSecond float64) string {
	const unit = 1024.0
	if bytesPerSecond < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}

	units := [6]string{"KB/s", "MB/s", "GB/s", "TB/s", "PB/s", "EB/s"}
	div := unit

	for exp := 0; exp < 6; exp++ {
		if bytesPerSecond < div*unit || exp == 5 {
			return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[exp])
		}
		div *= unit
	}

	return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[5])
}
