package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// COVERAGE IMPROVEMENT TESTS - Target uncovered code paths
// ============================================================================

// ----------------------------------------------------------------------------
// Config Presets Tests (0% coverage)
// ----------------------------------------------------------------------------

func TestConfigPresets(t *testing.T) {
	t.Run("NewSecure", func(t *testing.T) {
		client, err := NewSecure()
		if err != nil {
			t.Fatalf("NewSecure failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("NewPerformance", func(t *testing.T) {
		client, err := NewPerformance()
		if err != nil {
			t.Fatalf("NewPerformance failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("NewMinimal", func(t *testing.T) {
		client, err := NewMinimal()
		if err != nil {
			t.Fatalf("NewMinimal failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("TestingConfig", func(t *testing.T) {
		cfg := TestingConfig()
		if cfg.Timeout != 30*time.Second {
			t.Errorf("Expected timeout 30s, got %v", cfg.Timeout)
		}
		if !cfg.AllowPrivateIPs {
			t.Error("Expected AllowPrivateIPs to be true")
		}
		if !cfg.InsecureSkipVerify {
			t.Error("Expected InsecureSkipVerify to be true for testing")
		}
	})
}

// ----------------------------------------------------------------------------
// Package-Level HTTP Methods (0% coverage)
// ----------------------------------------------------------------------------

func TestPackageLevel_UncoveredMethods(t *testing.T) {
	setupPackageLevelTests()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	t.Run("Patch", func(t *testing.T) {
		resp, err := Patch(server.URL, WithJSON(map[string]string{"update": "data"}))
		if err != nil {
			t.Fatalf("Patch failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Options", func(t *testing.T) {
		resp, err := Options(server.URL)
		if err != nil {
			t.Fatalf("Options failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestCloseDefaultClient(t *testing.T) {
	// Create and set a custom default client
	cfg := DefaultConfig()
	cfg.AllowPrivateIPs = true
	client, _ := New(cfg)
	SetDefaultClient(client)

	// Close it
	if err := CloseDefaultClient(); err != nil {
		t.Errorf("CloseDefaultClient failed: %v", err)
	}

	// Reset to a new client for other tests
	newClient, _ := newTestClient()
	SetDefaultClient(newClient)
}

// ----------------------------------------------------------------------------
// Request Options (0% coverage)
// ----------------------------------------------------------------------------

func TestOptions_Uncovered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	t.Run("WithJSONAccept", func(t *testing.T) {
		_, err := client.Get(server.URL, WithJSONAccept())
		if err != nil {
			t.Fatalf("WithJSONAccept failed: %v", err)
		}
	})

	t.Run("WithXMLAccept", func(t *testing.T) {
		_, err := client.Get(server.URL, WithXMLAccept())
		if err != nil {
			t.Fatalf("WithXMLAccept failed: %v", err)
		}
	})

	t.Run("WithMaxRetries", func(t *testing.T) {
		_, err := client.Get(server.URL, WithMaxRetries(3))
		if err != nil {
			t.Fatalf("WithMaxRetries failed: %v", err)
		}
	})
}

func TestOptions_Cookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("test-cookie")
		if err == nil && cookie.Value == "test-value" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	t.Run("WithCookie", func(t *testing.T) {
		cookie := &http.Cookie{
			Name:  "test-cookie",
			Value: "test-value",
		}
		_, err := client.Get(server.URL, WithCookie(cookie))
		if err != nil {
			t.Fatalf("WithCookie failed: %v", err)
		}
	})

	t.Run("WithCookies", func(t *testing.T) {
		cookies := []*http.Cookie{
			{Name: "test-cookie", Value: "test-value"},
		}
		_, err := client.Get(server.URL, WithCookies(cookies))
		if err != nil {
			t.Fatalf("WithCookies failed: %v", err)
		}
	})

	t.Run("WithCookieValue", func(t *testing.T) {
		_, err := client.Get(server.URL, WithCookieValue("test-cookie", "test-value"))
		if err != nil {
			t.Fatalf("WithCookieValue failed: %v", err)
		}
	})
}

func TestOptions_File(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("Failed to parse multipart form: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Create a temporary file
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	t.Run("WithFile", func(t *testing.T) {
		content, _ := os.ReadFile(filePath)
		_, err := client.Post(server.URL, WithFile("file", "test.txt", content))
		if err != nil {
			t.Fatalf("WithFile failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Response Methods (0% coverage)
// ----------------------------------------------------------------------------

func TestResponse_Cookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:  "session",
			Value: "abc123",
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	t.Run("GetCookie", func(t *testing.T) {
		cookie := resp.GetCookie("session")
		if cookie == nil {
			t.Fatal("Expected cookie to be found")
		}
		if cookie.Value != "abc123" {
			t.Errorf("Expected cookie value abc123, got %s", cookie.Value)
		}
	})

	t.Run("HasCookie", func(t *testing.T) {
		if !resp.HasCookie("session") {
			t.Error("Expected HasCookie to return true")
		}
		if resp.HasCookie("nonexistent") {
			t.Error("Expected HasCookie to return false for nonexistent cookie")
		}
	})
}

// ----------------------------------------------------------------------------
// Download Package-Level Function (0% coverage)
// ----------------------------------------------------------------------------

func TestDownload_PackageLevelWithOptions(t *testing.T) {
	setupPackageLevelTests()

	content := []byte("download with options")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "options-download.txt")

	opts := &DownloadOptions{
		FilePath:  filePath,
		Overwrite: true,
	}

	result, err := DownloadWithOptions(server.URL, opts)
	if err != nil {
		t.Fatalf("DownloadWithOptions failed: %v", err)
	}

	if result.BytesWritten != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), result.BytesWritten)
	}
}

// ----------------------------------------------------------------------------
// Response JSON Error Handling
// ----------------------------------------------------------------------------

func TestResponse_JSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	var result map[string]interface{}
	err = resp.JSON(&result)
	if err == nil {
		t.Error("Expected JSON parsing error")
	}
}

// ----------------------------------------------------------------------------
// Client Helper Functions
// ----------------------------------------------------------------------------

func TestClient_HelperFunctions(t *testing.T) {
	t.Run("calculateOptimalIdleConnsPerHost", func(t *testing.T) {
		// Test with various MaxIdleConns values
		tests := []struct {
			maxIdle     int
			maxPerHost  int
			minExpected int
		}{
			{0, 0, 2},
			{10, 5, 2},
			{100, 50, 2},
			{1000, 100, 2},
		}

		for _, tt := range tests {
			result := calculateOptimalIdleConnsPerHost(tt.maxIdle, tt.maxPerHost)
			if result < tt.minExpected {
				t.Errorf("Result should be at least %d, got %d", tt.minExpected, result)
			}
		}
	})

	t.Run("calculateMaxRetryDelay", func(t *testing.T) {
		tests := []struct {
			baseDelay     time.Duration
			backoffFactor float64
		}{
			{1 * time.Second, 2.0},
			{5 * time.Second, 1.5},
			{0, 2.0},
		}

		for _, tt := range tests {
			result := calculateMaxRetryDelay(tt.baseDelay, tt.backoffFactor)
			if result <= 0 {
				t.Errorf("Result should be positive, got %v", result)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Context Cancellation Edge Cases
// ----------------------------------------------------------------------------

func TestClient_ContextEdgeCases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	t.Run("PreCanceledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel before request

		_, err := client.Request(ctx, "GET", server.URL)
		if err == nil {
			t.Error("Expected error with pre-canceled context")
		}
	})

	t.Run("ExpiredDeadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Hour))
		defer cancel()

		_, err := client.Request(ctx, "GET", server.URL)
		if err == nil {
			t.Error("Expected error with expired deadline")
		}
	})
}

// ----------------------------------------------------------------------------
// Error Path Coverage
// ----------------------------------------------------------------------------

func TestClient_ErrorPaths(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	t.Run("InvalidURL", func(t *testing.T) {
		_, err := client.Get("://invalid-url")
		if err == nil {
			t.Error("Expected error for invalid URL")
		}
	})

	t.Run("EmptyURL", func(t *testing.T) {
		_, err := client.Get("")
		if err == nil {
			t.Error("Expected error for empty URL")
		}
	})

	t.Run("InvalidMethod", func(t *testing.T) {
		_, err := client.Request(context.Background(), "INVALID\nMETHOD", "http://example.com")
		if err == nil {
			t.Error("Expected error for invalid method")
		}
	})
}

// ----------------------------------------------------------------------------
// Config Validation Edge Cases
// ----------------------------------------------------------------------------

func TestConfig_ValidationEdgeCases(t *testing.T) {
	t.Run("NegativeTimeout", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Timeout = -1 * time.Second
		err := ValidateConfig(cfg)
		if err == nil {
			t.Error("Expected error for negative timeout")
		}
	})

	t.Run("NegativeMaxRetries", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MaxRetries = -1
		err := ValidateConfig(cfg)
		if err == nil {
			t.Error("Expected error for negative max retries")
		}
	})

	t.Run("ExcessiveMaxRetries", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MaxRetries = 100
		err := ValidateConfig(cfg)
		if err == nil {
			t.Error("Expected error for excessive max retries")
		}
	})

	t.Run("InvalidHeaderKey", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Headers = map[string]string{
			"Invalid\nKey": "value",
		}
		err := ValidateConfig(cfg)
		if err == nil {
			t.Error("Expected error for invalid header key")
		}
	})

	t.Run("InvalidHeaderValue", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Headers = map[string]string{
			"X-Custom": "value\r\nInjection",
		}
		err := ValidateConfig(cfg)
		if err == nil {
			t.Error("Expected error for invalid header value")
		}
	})
}
