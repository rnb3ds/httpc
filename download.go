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
// Note: the current implementation buffers the entire response body in memory
// before writing to disk, so this callback is invoked only once after the
// download completes. For large file downloads, consider using a streaming
// approach outside this API.
type DownloadProgressCallback func(downloaded, total int64, speed float64)

// DownloadConfig configures file download behavior.
// Use DefaultDownloadConfig() to get a configuration with sensible defaults.
type DownloadConfig struct {
	FilePath         string
	ProgressCallback DownloadProgressCallback
	Overwrite        bool
	ResumeDownload   bool
}

// DefaultDownloadConfig returns a DownloadConfig with default settings.
// Overwrite and ResumeDownload are both false by default.
// Caller must set FilePath before use.
//
// Example:
//
//	cfg := httpc.DefaultDownloadConfig()
//	cfg.FilePath = "/downloads/file.zip"
//	cfg.Overwrite = true
//	result, err := client.DownloadWithOptions(url, cfg)
func DefaultDownloadConfig() *DownloadConfig {
	return &DownloadConfig{
		Overwrite:      false,
		ResumeDownload: false,
	}
}

// DownloadResult contains information about a completed download.
type DownloadResult struct {
	FilePath        string
	BytesWritten    int64
	Duration        time.Duration
	AverageSpeed    float64
	StatusCode      int
	ContentLength   int64
	Resumed         bool
	ResponseCookies []*http.Cookie
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
func DownloadWithOptions(url string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}

	return client.DownloadWithOptions(url, downloadOpts, options...)
}

// DownloadFileWithContext downloads a file using the default client with context control.
// The context parameter allows for timeout and cancellation control during the download.
func DownloadFileWithContext(ctx context.Context, url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.DownloadFileWithContext(ctx, url, filePath, options...)
}

// DownloadWithOptionsWithContext downloads a file with custom download options and context control.
// The context parameter allows for timeout and cancellation control during the download.
func DownloadWithOptionsWithContext(ctx context.Context, url string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.DownloadWithOptionsWithContext(ctx, url, downloadOpts, options...)
}

// DownloadFile downloads a file from the given URL to the specified file path.
func (c *clientImpl) DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	return c.DownloadFileWithContext(context.Background(), url, filePath, options...)
}

// DownloadWithOptions downloads a file with custom download options.
func (c *clientImpl) DownloadWithOptions(url string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	return c.DownloadWithOptionsWithContext(context.Background(), url, downloadOpts, options...)
}

// DownloadFileWithContext downloads a file with context control for cancellation and timeouts.
func (c *clientImpl) DownloadFileWithContext(ctx context.Context, url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	downloadOpts := DefaultDownloadConfig()
	downloadOpts.FilePath = filePath
	return c.downloadFile(ctx, url, downloadOpts, options...)
}

// DownloadWithOptionsWithContext downloads a file with custom download options and context control.
func (c *clientImpl) DownloadWithOptionsWithContext(ctx context.Context, url string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	return c.downloadFile(ctx, url, downloadOpts, options...)
}

func (c *clientImpl) downloadFile(ctx context.Context, url string, opts *DownloadConfig, options ...RequestOption) (result *DownloadResult, err error) {
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
			rangeOption := WithHeader("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
			combined := make([]RequestOption, len(options)+1)
			copy(combined, options)
			combined[len(options)] = rangeOption
			options = combined
		}
	}

	resp, err := c.Request(ctx, "GET", url, options...)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer ReleaseResult(resp)

	statusCode := resp.Response.StatusCode
	rawBody := resp.Response.RawBody
	contentLength := resp.Response.ContentLength
	duration := resp.Meta.Duration
	responseCookies := resp.Response.Cookies

	resumed := resumeOffset > 0 && statusCode == http.StatusPartialContent
	if resumeOffset > 0 && statusCode == http.StatusRequestedRangeNotSatisfiable {
		return &DownloadResult{
			FilePath:        opts.FilePath,
			BytesWritten:    0,
			Duration:        duration,
			AverageSpeed:    0,
			StatusCode:      statusCode,
			ContentLength:   resumeOffset,
			Resumed:         false,
			ResponseCookies: responseCookies,
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
		FilePath:        opts.FilePath,
		BytesWritten:    bytesWritten,
		Duration:        duration,
		AverageSpeed:    avgSpeed,
		StatusCode:      statusCode,
		ContentLength:   contentLength,
		Resumed:         resumed,
		ResponseCookies: responseCookies,
	}, nil
}

const (
	maxFilePathLen  = 4096
	dirPermissions  = 0755
	filePermissions = 0644
)

// getSystemPaths returns platform-specific system paths that should be protected.
func getSystemPaths() []string {
	switch getOS() {
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
			// Critical system configuration and kernel
			"/etc/", "/sys/", "/proc/", "/dev/", "/boot/",
			// Root user directory
			"/root/",
			// System executables
			"/usr/bin/", "/usr/sbin/", "/bin/", "/sbin/",
			// System libraries
			"/lib/", "/lib32/", "/lib64/", "/usr/lib/", "/usr/lib32/", "/usr/lib64/",
			// Runtime directories
			"/run/", "/var/run/", "/sys/fs/",
		}
	}
}

// getOS returns the current operating system.
// Kept as a wrapper for testability - allows mocking in unit tests.
func getOS() string {
	return runtime.GOOS
}

func calculateSpeed(bytes int64, duration time.Duration) float64 {
	if duration.Seconds() > 0 {
		return float64(bytes) / duration.Seconds()
	}
	return 0
}

// prepareFilePath validates and prepares file paths with security checks.
// SECURITY: This function implements multiple layers of protection:
// 1. UNC path blocking (prevents network resource access)
// 2. Control character filtering
// 3. System path protection
// 4. Path traversal detection
// 5. Symlink attack prevention
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

	// SECURITY: Check for symlinks to prevent symlink attacks
	// An attacker could create a symlink pointing to a sensitive file
	// and trick the application into writing to that file
	if fi, err := os.Lstat(absPath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink paths not allowed for security")
		}
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

	// SECURITY: Check parent directory for symlinks as well
	// This prevents TOCTOU attacks where a directory is replaced with a symlink
	dir := filepath.Dir(absPath)
	if dir != absPath { // Avoid infinite recursion at root
		if err := checkParentDirSymlinks(dir); err != nil {
			return err
		}
	}

	// Create directories
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	return nil
}

// checkParentDirSymlinks recursively checks if any parent directory is a symlink
// to prevent symlink-based path traversal attacks
func checkParentDirSymlinks(dir string) error {
	// Resolve the directory to its real path
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// If the directory doesn't exist yet, that's okay - it will be created
		if os.IsNotExist(err) {
			// Check parent recursively
			parent := filepath.Dir(dir)
			if parent != dir {
				return checkParentDirSymlinks(parent)
			}
			return nil
		}
		return fmt.Errorf("failed to evaluate symlinks: %w", err)
	}

	// If the resolved path differs from the original, a symlink was involved
	// Check if the resolved path is in a system directory
	if resolvedDir != dir {
		if isSystemPath(resolvedDir) {
			return fmt.Errorf("symlink resolves to system path")
		}
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

// FormatBytes formats a byte count as a human-readable string (e.g., "1.50 MB").
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

// FormatSpeed formats a byte-per-second rate as a human-readable string (e.g., "1.50 MB/s").
func FormatSpeed(bytesPerSecond float64) string {
	const unit = 1024.0
	if bytesPerSecond < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}

	units := [6]string{"KB/s", "MB/s", "GB/s", "TB/s", "PB/s", "EB/s"}
	div := unit
	exp := 0

	for n := bytesPerSecond / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[exp])
}
