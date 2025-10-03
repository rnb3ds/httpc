package httpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DownloadProgressCallback is called periodically during download to report progress
type DownloadProgressCallback func(downloaded, total int64, speed float64)

// DownloadOptions configures file download behavior
type DownloadOptions struct {
	// FilePath is the destination file path (required)
	FilePath string

	// ProgressCallback is called periodically to report download progress
	ProgressCallback DownloadProgressCallback

	// ProgressInterval is how often to call the progress callback (default: 500ms)
	ProgressInterval time.Duration

	// BufferSize is the size of the buffer used for copying data (default: 32KB)
	BufferSize int

	// CreateDirs creates parent directories if they don't exist (default: true)
	CreateDirs bool

	// Overwrite allows overwriting existing files (default: false)
	Overwrite bool

	// ResumeDownload attempts to resume partial downloads using Range requests (default: false)
	ResumeDownload bool

	// FileMode is the permission mode for the created file (default: 0644)
	FileMode os.FileMode
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
		FilePath:         filePath,
		ProgressInterval: 500 * time.Millisecond,
		BufferSize:       32 * 1024,
		CreateDirs:       true,
		Overwrite:        false,
		ResumeDownload:   false,
		FileMode:         0644,
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

func DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}

	return client.(*clientImpl).downloadFile(context.Background(), url, downloadOpts, options...)
}

func (c *clientImpl) DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	downloadOpts := DefaultDownloadOptions(filePath)
	return c.downloadFile(context.Background(), url, downloadOpts, options...)
}

// DownloadFileWithOptions downloads a file with custom download options
func (c *clientImpl) DownloadFileWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	return c.downloadFile(context.Background(), url, downloadOpts, options...)
}

// downloadFile implements the core download logic
func (c *clientImpl) downloadFile(ctx context.Context, url string, opts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("download options cannot be nil")
	}

	if opts.FilePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	if err := prepareFilePath(opts); err != nil {
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
		file, err = os.OpenFile(opts.FilePath, os.O_WRONLY|os.O_APPEND, opts.FileMode)
	} else {
		file, err = os.OpenFile(opts.FilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, opts.FileMode)
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

func prepareFilePath(opts *DownloadOptions) error {
	opts.FilePath = filepath.Clean(opts.FilePath)

	if opts.CreateDirs {
		dir := filepath.Dir(opts.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directories: %w", err)
		}
	}

	return nil
}

func copyWithProgress(dst io.Writer, src io.Reader, offset, total int64, opts *DownloadOptions) (int64, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 32 * 1024
	}

	buffer := make([]byte, opts.BufferSize)
	var written int64
	var lastProgress time.Time

	progressInterval := opts.ProgressInterval
	if progressInterval <= 0 {
		progressInterval = 500 * time.Millisecond
	}

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

// SaveToFile saves the response body to a file.
func (r *Response) SaveToFile(filePath string) error {
	if r.RawBody == nil {
		return fmt.Errorf("response body is empty")
	}

	filePath = filepath.Clean(filePath)

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(filePath, r.RawBody, 0644); err != nil {
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
