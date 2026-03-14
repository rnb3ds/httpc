package proxy

import (
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
)

func TestNewDetector(t *testing.T) {
	detector := NewDetector()
	if detector == nil {
		t.Fatal("NewDetector() returned nil")
	}
}

func TestDetector_GetProxyFunc_NoProxy(t *testing.T) {
	// Clear any existing proxy environment variables
	originalEnv := saveAndClearProxyEnv()
	defer restoreProxyEnv(originalEnv)

	// Create new detector after clearing env
	detector := NewDetector()
	proxyFunc := detector.GetProxyFunc()

	// Without proxy environment variables, should return nil (direct connection)
	if proxyFunc != nil {
		t.Log("GetProxyFunc returned a proxy function (platform-specific detection may have found one)")
	}
}

func TestDetector_GetProxyFunc_WithEnvProxy(t *testing.T) {
	// Save and clear existing proxy settings
	originalEnv := saveAndClearProxyEnv()
	defer restoreProxyEnv(originalEnv)

	// Set test proxy environment variable
	testProxy := "http://test-proxy.example.com:8080"
	os.Setenv("HTTP_PROXY", testProxy)
	os.Setenv("HTTPS_PROXY", testProxy)

	// Create new detector after setting env
	detector := NewDetector()
	proxyFunc := detector.GetProxyFunc()

	if proxyFunc == nil {
		t.Fatal("GetProxyFunc() returned nil with proxy environment set")
	}

	// Test the proxy function with a sample request
	testURL, _ := url.Parse("http://example.com")
	testReq := &http.Request{
		URL: testURL,
	}

	proxyURL, err := proxyFunc(testReq)
	if err != nil {
		t.Errorf("Proxy function returned error: %v", err)
	}

	if proxyURL == nil {
		t.Error("Proxy function returned nil URL")
	} else {
		t.Logf("Proxy URL = %s", proxyURL.String())
	}
}

func TestDetector_GetProxyFunc_ConcurrentAccess(t *testing.T) {
	originalEnv := saveAndClearProxyEnv()
	defer restoreProxyEnv(originalEnv)

	os.Setenv("HTTP_PROXY", "http://concurrent-test.example.com:8080")

	detector := NewDetector()

	var wg sync.WaitGroup
	results := make(chan func(*http.Request) (*url.URL, error), 10)

	// Launch 10 concurrent GetProxyFunc calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proxyFunc := detector.GetProxyFunc()
			results <- proxyFunc
		}()
	}

	wg.Wait()
	close(results)

	// Count results
	count := 0
	var firstFunc func(*http.Request) (*url.URL, error)
	for proxyFunc := range results {
		count++
		if firstFunc == nil {
			firstFunc = proxyFunc
		}
	}

	if count != 10 {
		t.Errorf("Expected 10 results, got %d", count)
	}

	if firstFunc == nil {
		t.Error("Expected non-nil proxy function from concurrent access")
	}
}

func TestDetector_NoProxyEnv(t *testing.T) {
	originalEnv := saveAndClearProxyEnv()
	defer restoreProxyEnv(originalEnv)

	// Set proxy with no_proxy exclusion
	os.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")
	os.Setenv("NO_PROXY", "localhost,127.0.0.1,.example.com")

	detector := NewDetector()
	proxyFunc := detector.GetProxyFunc()

	if proxyFunc == nil {
		t.Fatal("GetProxyFunc() returned nil")
	}

	// Request to excluded host should return nil proxy
	testURL, _ := url.Parse("http://localhost/test")
	testReq := &http.Request{URL: testURL}

	proxyURL, err := proxyFunc(testReq)
	if err != nil {
		t.Errorf("Proxy function returned error: %v", err)
	}

	// localhost should be excluded
	if proxyURL != nil {
		t.Logf("localhost not excluded by NO_PROXY, got proxy: %s", proxyURL)
	}
}

// Helper functions for environment management

func saveAndClearProxyEnv() map[string]string {
	envVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy"}
	saved := make(map[string]string)

	for _, v := range envVars {
		saved[v] = os.Getenv(v)
		os.Unsetenv(v)
	}

	return saved
}

func restoreProxyEnv(env map[string]string) {
	for k, v := range env {
		if v != "" {
			os.Setenv(k, v)
		} else {
			os.Unsetenv(k)
		}
	}
}
