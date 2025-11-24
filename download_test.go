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
// DOWNLOAD TESTS - File downloads, resume, progress tracking
// ============================================================================

func TestDownload_Basic(t *testing.T) {
	content := []byte("test file content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")

	result, err := client.DownloadFile(server.URL, filePath)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.FilePath != filePath {
		t.Errorf("Expected file path %s, got %s", filePath, result.FilePath)
	}
	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
	}
	if result.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", result.StatusCode)
	}

	// Verify file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}
	if fileInfo.Size() != result.BytesWritten {
		t.Errorf("File size mismatch: expected %d, got %d", result.BytesWritten, fileInfo.Size())
	}
}

func TestDownload_LargeFile(t *testing.T) {
	largeContent := []byte(strings.Repeat("x", 1024*1024)) // 1MB

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(largeContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(largeContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
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

func TestDownload_WithProgress(t *testing.T) {
	content := []byte(strings.Repeat("x", 10240)) // 10KB

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "progress-test.bin")

	progressCalled := false
	opts := &DownloadOptions{
		FilePath: filePath,
		ProgressCallback: func(downloaded, total int64, speed float64) {
			progressCalled = true
			if total > 0 {
				t.Logf("Progress: %d/%d bytes (%.2f%%)", downloaded, total, float64(downloaded)/float64(total)*100)
			}
		},
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
	}
	if !progressCalled {
		t.Error("Progress callback was not called")
	}
}

func TestDownload_WithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.Timeout = 100 * time.Millisecond
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "timeout-test.txt")

	_, err := client.DownloadFile(server.URL, filePath)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestDownload_ResumeNotSupported(t *testing.T) {
	content := []byte("test content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server doesn't support range requests
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "resume-test.txt")

	// Create partial file
	os.WriteFile(filePath, []byte("partial"), 0644)

	opts := &DownloadOptions{
		FilePath:       filePath,
		ResumeDownload: true,
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Should download full file since resume not supported
	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
	}
}

func TestDownload_PartialContent(t *testing.T) {
	fullContent := []byte("full file content here")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			// Support range requests
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 7-%d/%d", len(fullContent)-1, len(fullContent)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)-7))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(fullContent[7:])
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)))
			w.WriteHeader(http.StatusOK)
			w.Write(fullContent)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "partial-test.txt")

	// Create partial file
	os.WriteFile(filePath, []byte("partial"), 0644)

	opts := &DownloadOptions{
		FilePath:       filePath,
		ResumeDownload: true,
	}

	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.StatusCode != http.StatusPartialContent {
		t.Errorf("Expected status 206, got %d", result.StatusCode)
	}
}

func TestDownload_InvalidPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("content"))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	// Invalid path with directory traversal attempt
	_, err := client.DownloadFile(server.URL, "../../../etc/passwd")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestDownload_FileAlreadyExists(t *testing.T) {
	content := []byte("new content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "existing.txt")

	// Create existing file
	os.WriteFile(filePath, []byte("old content"), 0644)

	// Try download without overwrite
	opts := &DownloadOptions{
		FilePath:  filePath,
		Overwrite: false,
	}
	_, err := client.DownloadWithOptions(server.URL, opts)
	if err == nil {
		t.Error("Expected error when file exists and overwrite is false")
	}

	// Try with overwrite
	opts.Overwrite = true
	result, err := client.DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("Download with overwrite failed: %v", err)
	}
	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
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
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "error-test.txt")

	_, err := client.DownloadFile(server.URL, filePath)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestDownload_CreateDirectories(t *testing.T) {
	content := []byte("test content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "subdir1", "subdir2", "file.txt")

	result, err := client.DownloadFile(server.URL, filePath)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
	}

	// Verify directories were created
	if _, err := os.Stat(filepath.Dir(filePath)); os.IsNotExist(err) {
		t.Error("Directories were not created")
	}
}

func TestDownload_PackageLevel(t *testing.T) {
	setupPackageLevelTests()

	content := []byte("package level download")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "package-download.txt")

	result, err := DownloadFile(server.URL, filePath)
	if err != nil {
		t.Fatalf("Package level download failed: %v", err)
	}

	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
	}
}

func TestResponse_SaveToFile(t *testing.T) {
	content := []byte("response content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "response-save.txt")

	if err := resp.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Verify file content
	savedContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(savedContent) != string(content) {
		t.Errorf("Content mismatch: expected %s, got %s", string(content), string(savedContent))
	}
}
