package httpc

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sync"
)

// DomainClient provides automatic Cookie and Header management for a specific domain.
// All methods are safe for concurrent use by multiple goroutines.
//
// Features:
// - Automatic Cookie persistence across requests
// - Automatic Header persistence across requests
// - Per-request overrides via WithCookies/WithHeaderMap
// - Thread-safe state management
//
// Example:
//
//	client, _ := httpc.NewDomain("https://www.example.com")
//	defer client.Close()
//
//	// First request with initial cookies and headers
//	resp1, _ := client.Get("/",
//	    httpc.WithCookies(initialCookies),
//	    httpc.WithHeaderMap(map[string]string{"User-Agent": "MyBot"}),
//	)
//
//	// Second request automatically uses cookies from resp1 and persisted headers
//	// Can override with new headers
//	resp2, _ := client.Get("/search?q=test",
//	    httpc.WithHeaderMap(map[string]string{"Accept": "application/json"}),
//	)
type DomainClient struct {
	client     Client
	baseURL    string
	domain     string
	mu         sync.RWMutex
	cookies    map[string]*http.Cookie // name -> cookie
	headers    map[string]string       // key -> value
	autoManage bool
}

// NewDomain creates a new DomainClient for the specified base URL.
// The base URL should include the scheme and domain (e.g., "https://www.example.com").
// Returns an error if the URL is invalid or the client cannot be created.
//
// The client automatically manages:
// - Cookies: Saved from responses, sent in subsequent requests
// - Headers: Persisted across requests unless overridden
//
// Example:
//
//	client, err := httpc.NewDomain("https://api.example.com")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
func NewDomain(baseURL string, config ...*Config) (*DomainClient, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("base URL must include scheme and host")
	}

	// Create config with cookies enabled
	var cfg *Config
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	} else {
		cfg = DefaultConfig()
	}
	cfg.EnableCookies = true

	client, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &DomainClient{
		client:     client,
		baseURL:    baseURL,
		domain:     parsedURL.Hostname(),
		cookies:    make(map[string]*http.Cookie),
		headers:    make(map[string]string),
		autoManage: true,
	}, nil
}

// Get executes a GET request with automatic Cookie and Header management.
func (dc *DomainClient) Get(path string, options ...RequestOption) (*Result, error) {
	return dc.request("GET", path, options...)
}

// Post executes a POST request with automatic Cookie and Header management.
func (dc *DomainClient) Post(path string, options ...RequestOption) (*Result, error) {
	return dc.request("POST", path, options...)
}

// Put executes a PUT request with automatic Cookie and Header management.
func (dc *DomainClient) Put(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PUT", path, options...)
}

// Patch executes a PATCH request with automatic Cookie and Header management.
func (dc *DomainClient) Patch(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PATCH", path, options...)
}

// Delete executes a DELETE request with automatic Cookie and Header management.
func (dc *DomainClient) Delete(path string, options ...RequestOption) (*Result, error) {
	return dc.request("DELETE", path, options...)
}

// Head executes a HEAD request with automatic Cookie and Header management.
func (dc *DomainClient) Head(path string, options ...RequestOption) (*Result, error) {
	return dc.request("HEAD", path, options...)
}

// Options executes an OPTIONS request with automatic Cookie and Header management.
func (dc *DomainClient) Options(path string, options ...RequestOption) (*Result, error) {
	return dc.request("OPTIONS", path, options...)
}

func (dc *DomainClient) request(method, path string, options ...RequestOption) (*Result, error) {
	fullURL := dc.buildURL(path)

	// Prepare managed options (cookies + headers)
	managedOptions := dc.prepareManagedOptions()

	// Combine managed options with user options (user options override)
	allOptions := append(managedOptions, options...)

	// Capture request options before execution
	if dc.autoManage {
		dc.captureRequestOptions(options)
	}

	// Execute request using context-aware Request method
	result, err := dc.client.Request(context.Background(), method, fullURL, allOptions...)
	if err != nil {
		return nil, err
	}

	// Update managed state from response
	if result != nil && dc.autoManage {
		dc.updateFromResult(result)
	}

	return result, nil
}

func (dc *DomainClient) buildURL(path string) string {
	if path == "" {
		return dc.baseURL
	}

	pathLen := len(path)
	// Check if path is a full URL (optimized check)
	if pathLen > 8 && path[:8] == "https://" {
		parsedURL, err := url.Parse(path)
		if err == nil && parsedURL.Hostname() == dc.domain {
			return path
		}
		return path
	}
	if pathLen > 7 && path[:7] == "http://" {
		parsedURL, err := url.Parse(path)
		if err == nil && parsedURL.Hostname() == dc.domain {
			return path
		}
		return path
	}

	// Relative path handling
	if path[0] != '/' {
		return dc.baseURL + "/" + path
	}
	return dc.baseURL + path
}

func (dc *DomainClient) prepareManagedOptions() []RequestOption {
	dc.mu.RLock()
	cookieCount := len(dc.cookies)
	headerCount := len(dc.headers)
	dc.mu.RUnlock()

	if cookieCount == 0 && headerCount == 0 {
		return nil
	}

	options := make([]RequestOption, 0, 2)

	if cookieCount > 0 {
		dc.mu.RLock()
		cookies := make([]http.Cookie, 0, len(dc.cookies))
		for _, cookie := range dc.cookies {
			cookies = append(cookies, *cookie)
		}
		dc.mu.RUnlock()
		options = append(options, WithCookies(cookies))
	}

	if headerCount > 0 {
		dc.mu.RLock()
		headersCopy := make(map[string]string, len(dc.headers))
		maps.Copy(headersCopy, dc.headers)
		dc.mu.RUnlock()
		options = append(options, WithHeaderMap(headersCopy))
	}

	return options
}

func (dc *DomainClient) captureRequestOptions(options []RequestOption) {
	if len(options) == 0 {
		return
	}

	tempReq := &Request{
		Headers: make(map[string]string, 4),
		Cookies: make([]http.Cookie, 0, 4),
	}

	for _, opt := range options {
		if opt != nil {
			_ = opt(tempReq)
		}
	}

	if len(tempReq.Cookies) == 0 && len(tempReq.Headers) == 0 {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	for i := range tempReq.Cookies {
		cookie := &tempReq.Cookies[i]
		dc.cookies[cookie.Name] = cookie
	}

	for key, value := range tempReq.Headers {
		dc.headers[key] = value
	}
}

func (dc *DomainClient) updateFromResult(result *Result) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Update cookies from response
	if result.Response != nil && len(result.Response.Cookies) > 0 {
		for _, cookie := range result.Response.Cookies {
			if cookie != nil {
				dc.cookies[cookie.Name] = cookie
			}
		}
	}

	// Note: Headers are not automatically updated from response headers
	// Only cookies are managed automatically from server responses
}

// SetHeader sets a persistent header that will be sent with all subsequent requests.
// This header can be overridden per-request using WithHeader or WithHeaderMap.
func (dc *DomainClient) SetHeader(key, value string) error {
	if err := validateHeaderKeyValue(key, value); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.headers[key] = value
	return nil
}

// SetHeaders sets multiple persistent headers.
func (dc *DomainClient) SetHeaders(headers map[string]string) error {
	for k, v := range headers {
		if err := validateHeaderKeyValue(k, v); err != nil {
			return fmt.Errorf("invalid header %s: %w", k, err)
		}
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	maps.Copy(dc.headers, headers)
	return nil
}

// DeleteHeader removes a persistent header.
func (dc *DomainClient) DeleteHeader(key string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	delete(dc.headers, key)
}

// ClearHeaders removes all persistent headers.
func (dc *DomainClient) ClearHeaders() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.headers = make(map[string]string)
}

// GetHeaders returns a copy of all persistent headers.
func (dc *DomainClient) GetHeaders() map[string]string {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	headers := make(map[string]string, len(dc.headers))
	maps.Copy(headers, dc.headers)
	return headers
}

// SetCookie sets a persistent cookie that will be sent with all subsequent requests.
// This cookie can be overridden per-request using WithCookie or WithCookies.
func (dc *DomainClient) SetCookie(cookie *http.Cookie) error {
	if cookie == nil {
		return fmt.Errorf("cookie cannot be nil")
	}
	if err := validateCookie(cookie); err != nil {
		return fmt.Errorf("invalid cookie: %w", err)
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.cookies[cookie.Name] = cookie
	return nil
}

// SetCookies sets multiple persistent cookies.
func (dc *DomainClient) SetCookies(cookies []*http.Cookie) error {
	for i, cookie := range cookies {
		if cookie == nil {
			return fmt.Errorf("cookie at index %d is nil", i)
		}
		if err := validateCookie(cookie); err != nil {
			return fmt.Errorf("invalid cookie at index %d: %w", i, err)
		}
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	for _, cookie := range cookies {
		dc.cookies[cookie.Name] = cookie
	}
	return nil
}

// DeleteCookie removes a persistent cookie by name.
func (dc *DomainClient) DeleteCookie(name string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	delete(dc.cookies, name)
}

// ClearCookies removes all persistent cookies.
func (dc *DomainClient) ClearCookies() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.cookies = make(map[string]*http.Cookie)
}

// GetCookies returns a copy of all persistent cookies.
func (dc *DomainClient) GetCookies() []*http.Cookie {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	cookies := make([]*http.Cookie, 0, len(dc.cookies))
	for _, cookie := range dc.cookies {
		cookieCopy := *cookie
		cookies = append(cookies, &cookieCopy)
	}
	return cookies
}

// GetCookie returns a specific persistent cookie by name.
// Returns nil if the cookie doesn't exist.
func (dc *DomainClient) GetCookie(name string) *http.Cookie {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if cookie, ok := dc.cookies[name]; ok {
		cookieCopy := *cookie
		return &cookieCopy
	}
	return nil
}

// Close closes the underlying HTTP client and releases resources.
func (dc *DomainClient) Close() error {
	return dc.client.Close()
}
