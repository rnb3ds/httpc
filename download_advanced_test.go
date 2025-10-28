package httpc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// ADVANCED DOWNLOAD TESTS
// ============================================================================

func TestDownload_LargeFile(t *testing.T) {
	// Create a large file content (1MB)
	largeContent := []byte(strings.Repeat("x", 1024*1024))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(largeContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(largeContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "large-file.bin")

	result, err := client.DownloadFile(server.URL, filePath)
	if err != nil {
		t.Fatalf("Large file download failed: %v", err)
	}

	if result.BytesWritten != int64(len(largeContent)) {
		t.Errorf("Expected %d bytes, got %d", len(largeContent), result.BytesWritten)
	}

	if result.AverageSpeed <= 0 {
		t.Error("Average speed should be positive")
	}
}

func TestDownload_WithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.Timeout = 100 * time.Millisecond // Short timeout
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "timeout-test.txt")

	_, err = client.DownloadFile(server.URL, filePath)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestDownload_ResumeNotSupported(t *testing.T) {
	testContent := []byte("full content for resume test")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			// Server doesn't support range requests
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(testContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "resume-not-supported.txt")

	// Create partial file
	partialContent := testContent[:10]
	err = os.WriteFile(filePath, partialContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create partial file: %v", err)
	}

	opts := &DownloadOptions{
		FilePath:       filePath,
		ResumeDownload: true,
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Should complete successfully even though resume wasn't supported
	if result.Resumed {
		t.Error("Download should not be marked as resumed when server doesn't support it")
	}

	if result.BytesWritten != 0 {
		t.Errorf("Expected 0 bytes written when range not satisfiable, got %d", result.BytesWritten)
	}
}

func TestDownload_PartialContent(t *testing.T) {
	fullContent := []byte("This is the full content of the file for partial download test")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			// Parse range header and return partial content
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 10-%d/%d", len(fullContent)-1, len(fullContent)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)-10))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(fullContent[10:])
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)))
			w.WriteHeader(http.StatusOK)
			w.Write(fullContent)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "partial-content.txt")

	// Create partial file (first 10 bytes)
	partialContent := fullContent[:10]
	err = os.WriteFile(filePath, partialContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create partial file: %v", err)
	}

	opts := &DownloadOptions{
		FilePath:       filePath,
		ResumeDownload: true,
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Partial download failed: %v", err)
	}

	if !result.Resumed {
		t.Error("Download should be marked as resumed")
	}

	expectedBytes := int64(len(fullContent) - 10)
	if result.BytesWritten != expectedBytes {
		t.Errorf("Expected %d bytes written, got %d", expectedBytes, result.BytesWritten)
	}
}

func TestDownload_InvalidPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name     string
		filePath string
	}{
		{"Empty path", ""},
		{"Path traversal", "../../../etc/passwd"},
		{"Null byte", "test\x00file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.DownloadFile(server.URL, tt.filePath)
			if err == nil {
				t.Errorf("Expected error for invalid path: %s", tt.filePath)
			}
		})
	}
}

func TestDownload_FileAlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("new content"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "existing-file.txt")

	// Create existing file
	err = os.WriteFile(filePath, []byte("existing content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	// Try to download without overwrite flag
	_, err = client.DownloadFile(server.URL, filePath)
	if err == nil {
		t.Error("Expected error when file already exists")
	}

	// Try with overwrite option
	opts := &DownloadOptions{
		FilePath:  filePath,
		Overwrite: true,
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download with overwrite failed: %v", err)
	}

	if result.BytesWritten != 11 { // "new content" = 11 bytes
		t.Errorf("Expected 11 bytes written, got %d", result.BytesWritten)
	}
}

func TestDownload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "error-test.txt")

	_, err = client.DownloadFile(server.URL, filePath)
	if err == nil {
		t.Error("Expected error for HTTP 404")
	}
}

func TestDownload_ProgressCallbackError(t *testing.T) {
	testContent := []byte(strings.Repeat("x", 1024)) // 1KB

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(testContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "progress-error-test.txt")

	var callCount int
	opts := &DownloadOptions{
		FilePath: filePath,
		ProgressCallback: func(downloaded, total int64, speed float64) {
			callCount++
			// Callback that might panic or have issues
			if downloaded < 0 || total < 0 || speed < 0 {
				panic("Invalid progress values")
			}
		},
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download with progress callback failed: %v", err)
	}

	if callCount == 0 {
		t.Error("Progress callback was not called")
	}

	if result.BytesWritten != int64(len(testContent)) {
		t.Errorf("Expected %d bytes written, got %d", len(testContent), result.BytesWritten)
	}
}

func TestDownload_NilOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.DownloadWithOptions(server.URL, nil)
	if err == nil {
		t.Error("Expected error for nil download options")
	}
}

func TestDownload_URLValidation(t *testing.T) {
	config := DefaultConfig()
	config.AllowPrivateIPs = false // Strict URL validation
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "url-test.txt")

	tests := []string{
		"",
		"invalid-url",
		"ftp://example.com/file.txt",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"http://localhost/file.txt", // Should be blocked when AllowPrivateIPs is false
	}

	for _, url := range tests {
		t.Run("URL_"+url, func(t *testing.T) {
			_, err := client.DownloadFile(url, filePath)
			if err == nil {
				t.Errorf("Expected error for invalid URL: %s", url)
			}
		})
	}
}

func TestDownload_ContentLengthMismatch(t *testing.T) {
	actualContent := []byte("short content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Lie about content length
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write(actualContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "content-length-test.txt")

	result, err := client.DownloadFile(server.URL, filePath)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Should download actual content, not what Content-Length claimed
	if result.BytesWritten != int64(len(actualContent)) {
		t.Errorf("Expected %d bytes written, got %d", len(actualContent), result.BytesWritten)
	}
}

func TestDownload_NoContentLength(t *testing.T) {
	testContent := []byte("content without length header")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't set Content-Length header
		w.WriteHeader(http.StatusOK)
		w.Write(testContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "no-content-length.txt")

	result, err := client.DownloadFile(server.URL, filePath)
	if err != nil {
		t.Fatalf("Download without Content-Length failed: %v", err)
	}

	if result.BytesWritten != int64(len(testContent)) {
		t.Errorf("Expected %d bytes written, got %d", len(testContent), result.BytesWritten)
	}

	// Content length should be 0 or -1 when not provided
	if result.ContentLength > 0 {
		t.Errorf("Expected ContentLength <= 0 when not provided, got %d", result.ContentLength)
	}
}
