package httpc

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadFile(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create temp directory for test downloads
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test-download.txt")

	// DownloadFile a small file
	result, err := client.DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		filePath,
	)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify result
	if result.FilePath != filePath {
		t.Errorf("Expected file path %s, got %s", filePath, result.FilePath)
	}

	if result.BytesWritten <= 0 {
		t.Errorf("Expected bytes written > 0, got %d", result.BytesWritten)
	}

	if result.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", result.StatusCode)
	}

	// Verify file exists and has content
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	if fileInfo.Size() != result.BytesWritten {
		t.Errorf("File size mismatch: expected %d, got %d", result.BytesWritten, fileInfo.Size())
	}
}

func TestDownloadFileWithProgress(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test-progress.bin")

	progressCalled := false
	opts := DefaultDownloadOptions(filePath)
	opts.ProgressInterval = 100 * time.Millisecond
	opts.ProgressCallback = func(downloaded, total int64, speed float64) {
		progressCalled = true
		if total > 0 {
			t.Logf("Progress: %d/%d bytes (%.2f%%)", downloaded, total, float64(downloaded)/float64(total)*100)
		} else {
			t.Logf("Progress: %d bytes downloaded", downloaded)
		}
	}

	// Use a more reliable test URL - GitHub's raw content
	// This file is large enough to trigger progress callbacks
	result, err := client.DownloadFileWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/src/go/parser/parser.go",
		opts,
		WithTimeout(60*time.Second),
	)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	if !progressCalled {
		t.Error("Progress callback was not called")
	}

	if result.BytesWritten <= 0 {
		t.Errorf("Expected bytes written > 0, got %d", result.BytesWritten)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("File not created: %v", err)
	}

	t.Logf("Downloaded %d bytes in %v (avg speed: %.2f KB/s)",
		result.BytesWritten,
		result.Duration,
		result.AverageSpeed/1024)
}

func TestDownloadFileOverwrite(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test-overwrite.txt")

	// Create an existing file
	if err := os.WriteFile(filePath, []byte("existing content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Try to download without overwrite (should fail)
	_, err = client.DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		filePath,
	)
	if err == nil {
		t.Error("Expected error when file exists without overwrite option")
	}

	// DownloadFile with overwrite
	opts := DefaultDownloadOptions(filePath)
	opts.Overwrite = true

	result, err := client.DownloadFileWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		opts,
	)
	if err != nil {
		t.Fatalf("DownloadFile with overwrite failed: %v", err)
	}

	// Verify file was overwritten
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) == "existing content" {
		t.Error("File was not overwritten")
	}

	if int64(len(content)) != result.BytesWritten {
		t.Errorf("Content size mismatch: expected %d, got %d", result.BytesWritten, len(content))
	}
}

func TestDownloadFileCreateDirs(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "subdir1", "subdir2", "test-file.txt")

	// DownloadFile with CreateDirs enabled (default)
	result, err := client.DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		filePath,
	)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// Verify directories were created
	if _, err := os.Stat(filepath.Dir(filePath)); err != nil {
		t.Errorf("Parent directories not created: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("File not created: %v", err)
	}

	if result.BytesWritten <= 0 {
		t.Errorf("Expected bytes written > 0, got %d", result.BytesWritten)
	}
}

func TestResponseSaveToFile(t *testing.T) {
	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Make a regular GET request
	resp, err := client.Get("https://raw.githubusercontent.com/golang/go/master/LICENSE")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "license.txt")

	// Save response to file
	if err := resp.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Verify file exists and has correct content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if len(content) != len(resp.RawBody) {
		t.Errorf("File size mismatch: expected %d, got %d", len(resp.RawBody), len(content))
	}

	if string(content) != string(resp.RawBody) {
		t.Error("File content does not match response body")
	}
}

func TestDownloadWithAuthentication(t *testing.T) {
	// Create a local test server that requires authentication
	expectedToken := "test-secret-token"
	testContent := []byte(`{"authenticated": true, "token": "valid"}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for Bearer token
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + expectedToken

		if authHeader != expectedAuth {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "unauthorized"}`))
			return
		}

		// Valid authentication
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(testContent)
	}))
	defer server.Close()

	client, err := New()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "auth-test.json")

	opts := DefaultDownloadOptions(filePath)
	opts.Overwrite = true

	// Test 1: Download with correct authentication
	result, err := client.DownloadFileWithOptions(
		server.URL,
		opts,
		WithBearerToken(expectedToken),
	)
	if err != nil {
		t.Fatalf("Authenticated download failed: %v", err)
	}

	if result.BytesWritten != int64(len(testContent)) {
		t.Errorf("Expected bytes written %d, got %d", len(testContent), result.BytesWritten)
	}

	// Verify file exists and content is correct
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if !bytes.Equal(content, testContent) {
		t.Errorf("Downloaded content mismatch.\nExpected: %s\nGot: %s", testContent, content)
	}

	// Test 2: Download without authentication should fail
	filePath2 := filepath.Join(tempDir, "auth-test-fail.json")
	opts2 := DefaultDownloadOptions(filePath2)
	opts2.Overwrite = true

	_, err = client.DownloadFileWithOptions(
		server.URL,
		opts2,
		// No authentication header
	)
	if err == nil {
		t.Error("Expected download to fail without authentication, but it succeeded")
	}
	if err != nil && !strings.Contains(err.Error(), "401") {
		t.Logf("Got expected error: %v", err)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d bytes", tt.bytes), func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("FormatBytes(%d) = %s, want %s", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		speed    float64
		expected string
	}{
		{1024, "1.00 KB/s"},
		{1048576, "1.00 MB/s"},
		{1073741824, "1.00 GB/s"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.0f bytes/s", tt.speed), func(t *testing.T) {
			result := FormatSpeed(tt.speed)
			if result != tt.expected {
				t.Errorf("FormatSpeed(%.0f) = %s, want %s", tt.speed, result, tt.expected)
			}
		})
	}
}

func TestPackageLevelDownload(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "package-level-download.txt")

	// Test package-level DownloadFile function
	result, err := DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		filePath,
	)
	if err != nil {
		t.Fatalf("Package-level DownloadFile failed: %v", err)
	}

	if result.BytesWritten <= 0 {
		t.Errorf("Expected bytes written > 0, got %d", result.BytesWritten)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("File not created: %v", err)
	}
}
