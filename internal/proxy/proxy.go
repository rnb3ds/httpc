package proxy

import (
	"net/http"
	"net/url"
	"sync"
)

// Detector provides automatic system proxy detection
type Detector struct {
	cache   *proxyConfig
	cacheMu sync.RWMutex
}

type proxyConfig struct {
	proxyFunc func(*http.Request) (*url.URL, error)
	enabled   bool
}

// NewDetector creates a new system proxy detector
func NewDetector() *Detector {
	return &Detector{}
}

// GetProxyFunc returns a proxy function that automatically detects system proxy settings.
// It returns nil if no proxy is configured, which means direct connection.
func (d *Detector) GetProxyFunc() func(*http.Request) (*url.URL, error) {
	// Fast path: check cache with read lock
	d.cacheMu.RLock()
	if d.cache != nil {
		proxyFunc := d.cache.proxyFunc
		d.cacheMu.RUnlock()
		return proxyFunc
	}
	d.cacheMu.RUnlock()

	// Slow path: detect and cache with write lock
	d.cacheMu.Lock()
	// Double-check after acquiring write lock (another goroutine may have cached)
	if d.cache != nil {
		proxyFunc := d.cache.proxyFunc
		d.cacheMu.Unlock()
		return proxyFunc
	}

	// Detect and cache proxy configuration
	proxyFunc := d.detect()
	d.cache = &proxyConfig{
		proxyFunc: proxyFunc,
		enabled:   proxyFunc != nil,
	}
	d.cacheMu.Unlock()

	return proxyFunc
}

// detect performs platform-specific proxy detection
func (d *Detector) detect() func(*http.Request) (*url.URL, error) {
	// First try environment variables (works on all platforms)
	if envProxy := d.detectFromEnvironment(); envProxy != nil {
		return envProxy
	}

	// Platform-specific detection
	return d.detectPlatform()
}

// detectFromEnvironment checks environment variables for proxy settings
func (d *Detector) detectFromEnvironment() func(*http.Request) (*url.URL, error) {
	// Use Go's built-in function which reads:
	// HTTP_PROXY, HTTPS_PROXY, NO_PROXY, etc.
	// Test if any proxy environment variable is set
	testURL, _ := url.Parse("https://www.example.com")
	testReq := &http.Request{
		URL: testURL,
	}

	// http.ProxyFromEnvironment is a function, not a pointer - it's never nil
	// Test if it returns a valid proxy for our test request
	if u, err := http.ProxyFromEnvironment(testReq); err == nil && u != nil {
		return http.ProxyFromEnvironment
	}
	return nil
}
