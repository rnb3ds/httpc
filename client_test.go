package httpc

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// CLIENT TESTS - Instance and package-level client functionality
// ============================================================================

// ----------------------------------------------------------------------------
// Client Instance Tests
// ----------------------------------------------------------------------------

func TestClient_Creation(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		client, err := newTestClient()
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("WithConfig", func(t *testing.T) {
		config := DefaultConfig()
		config.Timeout = 10 * time.Second
		config.MaxRetries = 2
		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()
	})

	t.Run("WithTLSConfig", func(t *testing.T) {
		config := DefaultConfig()
		config.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		}
		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client with TLS config: %v", err)
		}
		defer client.Close()
	})
}

func TestClient_HTTPMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
		fn     func(Client, string, ...RequestOption) (*Result, error)
	}{
		{"GET", "GET", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Get(url, opts...) }},
		{"POST", "POST", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Post(url, opts...) }},
		{"PUT", "PUT", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Put(url, opts...) }},
		{"PATCH", "PATCH", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Patch(url, opts...) }},
		{"DELETE", "DELETE", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Delete(url, opts...) }},
		{"HEAD", "HEAD", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Head(url, opts...) }},
		{"OPTIONS", "OPTIONS", func(c Client, url string, opts ...RequestOption) (*Result, error) { return c.Options(url, opts...) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method {
					t.Errorf("Expected method %s, got %s", tt.method, r.Method)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"message":"success"}`))
			}))
			defer server.Close()

			client, err := newTestClient()
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			resp, err := tt.fn(client, server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			if resp.StatusCode() != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode())
			}
		})
	}
}

func TestClient_Lifecycle(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		client, _ := newTestClient()
		if err := client.Close(); err != nil {
			t.Errorf("Client close should not error: %v", err)
		}
	})

	t.Run("DoubleClose", func(t *testing.T) {
		client, _ := newTestClient()
		client.Close()
		err := client.Close()
		if err == nil {
			t.Log("Double close is idempotent")
		}
	})

	t.Run("UseAfterClose", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		client.Close()
		_, err := client.Get(server.URL)
		if err == nil {
			t.Error("Expected error when using closed client")
		}
	})
}

func TestClient_Timeout(t *testing.T) {
	t.Run("RequestTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithTimeout(100*time.Millisecond))
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := client.Request(ctx, "GET", server.URL)
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := client.Request(ctx, "GET", server.URL)
		if err == nil {
			t.Error("Expected context canceled error")
		}
	})
}

func TestClient_Concurrency(t *testing.T) {
	t.Run("ConcurrentRequests", func(t *testing.T) {
		requestCount := int32(0)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		const numRequests = 100
		var wg sync.WaitGroup
		errors := make(chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := client.Get(server.URL)
				if err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		errorCount := 0
		for err := range errors {
			t.Errorf("Request failed: %v", err)
			errorCount++
		}

		if errorCount > 0 {
			t.Fatalf("Failed %d out of %d requests", errorCount, numRequests)
		}

		if atomic.LoadInt32(&requestCount) != numRequests {
			t.Errorf("Expected %d requests, got %d", numRequests, atomic.LoadInt32(&requestCount))
		}
	})

	t.Run("ConfigModificationSafety", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.AllowPrivateIPs = true
		cfg.Headers = map[string]string{"X-Initial": "value"}

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		var wg sync.WaitGroup
		errChan := make(chan error, 100)

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := client.Get(server.URL)
				if err != nil {
					errChan <- err
				}
			}()
		}

		// Modify original config (should not affect client)
		for i := 0; i < 50; i++ {
			cfg.Headers["X-Modified"] = "new-value"
			cfg.Timeout = time.Duration(i) * time.Second
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			t.Errorf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Package-Level Function Tests
// ----------------------------------------------------------------------------

func TestPackageLevel_Functions(t *testing.T) {
	// Setup default client for package-level tests
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	_ = SetDefaultClient(client)
	defer CloseDefaultClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	tests := []struct {
		name string
		fn   func(string, ...RequestOption) (*Result, error)
	}{
		{"Get", Get},
		{"Post", Post},
		{"Put", Put},
		{"Patch", Patch},
		{"Delete", Delete},
		{"Head", Head},
		{"Options", Options},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.fn(server.URL)
			if err != nil {
				t.Fatalf("Package-level %s failed: %v", tt.name, err)
			}
			if resp.StatusCode() != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode())
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Type Tests
// ----------------------------------------------------------------------------

func TestTypes(t *testing.T) {
	t.Run("HTTPError", func(t *testing.T) {
		err := &HTTPError{
			StatusCode: 404,
			Status:     "Not Found",
			Method:     "GET",
			URL:        "https://example.com",
		}
		expected := "HTTP 404: GET https://example.com"
		if err.Error() != expected {
			t.Errorf("HTTPError.Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("FormData", func(t *testing.T) {
		formData := &FormData{
			Fields: map[string]string{
				"field1": "value1",
				"field2": "value2",
			},
			Files: map[string]*FileData{
				"file1": {
					Filename: "test.txt",
					Content:  []byte("test content"),
				},
			},
		}

		if len(formData.Fields) != 2 {
			t.Error("FormData should have 2 fields")
		}
		if len(formData.Files) != 1 {
			t.Error("FormData should have 1 file")
		}
		file := formData.Files["file1"]
		if file.Filename != "test.txt" {
			t.Error("File filename should be test.txt")
		}
		if string(file.Content) != "test content" {
			t.Error("File content should match")
		}
	})
}

// ----------------------------------------------------------------------------
// Error Handling Tests
// ----------------------------------------------------------------------------

func TestClient_ErrorHandling(t *testing.T) {
	t.Run("InvalidURL", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get("://invalid-url")
		if err == nil {
			t.Error("Expected error for invalid URL")
		}
	})

	t.Run("NetworkError", func(t *testing.T) {
		config := DefaultConfig()
		config.Timeout = 1 * time.Second
		config.AllowPrivateIPs = true
		client, _ := New(config)
		defer client.Close()

		// Use a non-routable IP address
		_, err := client.Get("http://192.0.2.1:12345")
		if err == nil {
			t.Error("Expected network error")
		}
	})

	t.Run("NilConfig", func(t *testing.T) {
		// Nil config should use defaults
		client, err := New(nil)
		if err != nil {
			t.Errorf("Unexpected error for nil config: %v", err)
		}
		if client != nil {
			defer client.Close()
		}
	})

	t.Run("EmptyURL", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get("")
		if err == nil {
			t.Error("Expected error for empty URL")
		}
	})
}

// ----------------------------------------------------------------------------
// Utility Function Tests
// ----------------------------------------------------------------------------

func TestUtilityFunctions(t *testing.T) {
	t.Run("FormatBytes", func(t *testing.T) {
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
		}

		for _, test := range tests {
			result := FormatBytes(test.input)
			if result != test.expected {
				t.Errorf("FormatBytes(%d) = %s, expected %s", test.input, result, test.expected)
			}
		}
	})

	t.Run("FormatSpeed", func(t *testing.T) {
		tests := []struct {
			input    float64
			expected string
		}{
			{0, "0 B/s"},
			{512, "512 B/s"},
			{1024, "1.00 KB/s"},
			{1048576, "1.00 MB/s"},
		}

		for _, test := range tests {
			result := FormatSpeed(test.input)
			if result != test.expected {
				t.Errorf("FormatSpeed(%f) = %s, expected %s", test.input, result, test.expected)
			}
		}
	})

	t.Run("DefaultDownloadOptions", func(t *testing.T) {
		filePath := "test/file.txt"
		opts := DefaultDownloadOptions(filePath)

		if opts.FilePath != filePath {
			t.Errorf("Expected FilePath %s, got %s", filePath, opts.FilePath)
		}
		if opts.Overwrite != false {
			t.Error("Expected Overwrite to be false")
		}
		if opts.ResumeDownload != false {
			t.Error("Expected ResumeDownload to be false")
		}
	})
}
