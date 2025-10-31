package httpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// setupPackageLevelTests initializes the default client with AllowPrivateIPs for testing
func setupPackageLevelTests() {
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	_ = SetDefaultClient(client) // Ignore error in test setup
}

// ============================================================================
// PACKAGE-LEVEL FUNCTION TESTS
// ============================================================================

func TestPackageLevel_Post(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var data map[string]interface{}
		json.Unmarshal(body, &data)

		if data["test"] != "value" {
			t.Errorf("Expected test=value, got %v", data["test"])
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"created"}`))
	}))
	defer server.Close()

	resp, err := Post(server.URL, WithJSON(map[string]string{"test": "value"}))
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}
}

func TestPackageLevel_Put(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("Expected PUT method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"updated"}`))
	}))
	defer server.Close()

	resp, err := Put(server.URL, WithJSON(map[string]string{"update": "data"}))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestPackageLevel_Patch(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("Expected PATCH method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"patched"}`))
	}))
	defer server.Close()

	resp, err := Patch(server.URL, WithJSON(map[string]string{"patch": "data"}))
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestPackageLevel_Delete(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	resp, err := Delete(server.URL)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
}

func TestPackageLevel_Head(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("Expected HEAD method, got %s", r.Method)
		}
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := Head(server.URL)
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Headers.Get("X-Custom-Header") != "test-value" {
		t.Errorf("Expected X-Custom-Header to be test-value")
	}
}

func TestPackageLevel_Options(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "OPTIONS" {
			t.Errorf("Expected OPTIONS method, got %s", r.Method)
		}
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := Options(server.URL)
	if err != nil {
		t.Fatalf("Options failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Headers.Get("Allow") == "" {
		t.Error("Expected Allow header to be set")
	}
}

func TestPackageLevel_SetDefaultClient(t *testing.T) {
	// Create a custom client with specific config
	config := DefaultConfig()
	config.Timeout = 5 * time.Second
	config.MaxRetries = 1
	config.AllowPrivateIPs = true

	customClient, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create custom client: %v", err)
	}

	// Set as default
	if err := SetDefaultClient(customClient); err != nil {
		t.Fatalf("Failed to set default client: %v", err)
	}

	// Test that package-level functions use the custom client
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	resp, err := Get(server.URL)
	if err != nil {
		t.Fatalf("Get with custom default client failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Reset to default client for other tests BEFORE closing the custom client
	defaultClient, _ := newTestClient()
	if err := SetDefaultClient(defaultClient); err != nil {
		t.Fatalf("Failed to reset default client: %v", err)
	}

	// Now it's safe to close the custom client
	customClient.Close()
}

func TestPackageLevel_ConcurrentUsage(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Test concurrent usage of package-level functions
	const numRequests = 50
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			var err error
			switch idx % 6 {
			case 0:
				_, err = Get(server.URL)
			case 1:
				_, err = Post(server.URL, WithJSON(map[string]int{"id": idx}))
			case 2:
				_, err = Put(server.URL, WithJSON(map[string]int{"id": idx}))
			case 3:
				_, err = Patch(server.URL, WithJSON(map[string]int{"id": idx}))
			case 4:
				_, err = Delete(server.URL)
			case 5:
				_, err = Head(server.URL)
			}
			errors <- err
		}(i)
	}

	// Collect results
	failCount := 0
	for i := 0; i < numRequests; i++ {
		if err := <-errors; err != nil {
			t.Logf("Request failed: %v", err)
			failCount++
		}
	}

	if failCount > 0 {
		t.Errorf("Failed %d out of %d concurrent requests", failCount, numRequests)
	}
}

func TestPackageLevel_WithTimeout(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client, _ := getDefaultClient()
	_, err := client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestPackageLevel_WithContext(t *testing.T) {
	setupPackageLevelTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client, _ := getDefaultClient()
	_, err := client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected context canceled error, got nil")
	}
}

func TestPackageLevel_ErrorHandling(t *testing.T) {
	setupPackageLevelTests()
	// Test with invalid URL
	_, err := Get("not-a-valid-url")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	// Test with non-existent server
	_, err = Post("http://localhost:99999", WithJSON(map[string]string{"test": "data"}))
	if err == nil {
		t.Error("Expected error for non-existent server")
	}
}

// ============================================================================
// UTILITY FUNCTION TESTS
// ============================================================================

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
	}

	for _, test := range tests {
		result := FormatBytes(test.input)
		if result != test.expected {
			t.Errorf("FormatBytes(%d) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0, "0 B/s"},
		{512, "512 B/s"},
		{1024, "1.00 KB/s"},
		{1536, "1.50 KB/s"},
		{1048576, "1.00 MB/s"},
		{1073741824, "1.00 GB/s"},
		{1099511627776, "1.00 TB/s"},
	}

	for _, test := range tests {
		result := FormatSpeed(test.input)
		if result != test.expected {
			t.Errorf("FormatSpeed(%f) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestDefaultDownloadOptions(t *testing.T) {
	filePath := "test/file.txt"
	opts := DefaultDownloadOptions(filePath)

	if opts.FilePath != filePath {
		t.Errorf("Expected FilePath %s, got %s", filePath, opts.FilePath)
	}

	if opts.Overwrite != false {
		t.Errorf("Expected Overwrite to be false, got %t", opts.Overwrite)
	}

	if opts.ResumeDownload != false {
		t.Errorf("Expected ResumeDownload to be false, got %t", opts.ResumeDownload)
	}
}
