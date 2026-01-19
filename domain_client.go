package httpc

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	stdpath "path"
	"sync"

	"github.com/cybergodev/httpc/internal/validation"
)

type DomainClient struct {
	client  Client
	baseURL string
	domain  string
	mu      sync.RWMutex
	cookies map[string]*http.Cookie
	headers map[string]string
}

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
		client:  client,
		baseURL: baseURL,
		domain:  parsedURL.Hostname(),
		cookies: make(map[string]*http.Cookie),
		headers: make(map[string]string),
	}, nil
}

func (dc *DomainClient) Get(path string, options ...RequestOption) (*Result, error) {
	return dc.request("GET", path, options...)
}

func (dc *DomainClient) Post(path string, options ...RequestOption) (*Result, error) {
	return dc.request("POST", path, options...)
}

func (dc *DomainClient) Put(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PUT", path, options...)
}

func (dc *DomainClient) Patch(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PATCH", path, options...)
}

func (dc *DomainClient) Delete(path string, options ...RequestOption) (*Result, error) {
	return dc.request("DELETE", path, options...)
}

func (dc *DomainClient) Head(path string, options ...RequestOption) (*Result, error) {
	return dc.request("HEAD", path, options...)
}

func (dc *DomainClient) Options(path string, options ...RequestOption) (*Result, error) {
	return dc.request("OPTIONS", path, options...)
}

func (dc *DomainClient) DownloadFile(path string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	fullURL := dc.buildURL(path)

	managedOptions := dc.prepareManagedOptions()
	allOptions := append(managedOptions, options...)

	dc.captureRequestOptions(options)

	return dc.client.DownloadFile(fullURL, filePath, allOptions...)
}

func (dc *DomainClient) DownloadWithOptions(path string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	fullURL := dc.buildURL(path)

	managedOptions := dc.prepareManagedOptions()
	allOptions := append(managedOptions, options...)

	dc.captureRequestOptions(options)

	return dc.client.DownloadWithOptions(fullURL, downloadOpts, allOptions...)
}

func (dc *DomainClient) request(method, path string, options ...RequestOption) (*Result, error) {
	fullURL := dc.buildURL(path)

	managedOptions := dc.prepareManagedOptions()
	allOptions := append(managedOptions, options...)

	dc.captureRequestOptions(options)

	result, err := dc.client.Request(context.Background(), method, fullURL, allOptions...)
	if err != nil {
		return nil, err
	}

	if result != nil {
		dc.updateFromResult(result)
	}

	return result, nil
}

func (dc *DomainClient) buildURL(pathStr string) string {
	if pathStr == "" {
		return dc.baseURL
	}

	parsedURL, err := url.Parse(pathStr)
	if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
		// Validate URL scheme for security
		// Only allow http and https schemes to prevent potential SSRF attacks
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			// Reject URLs with disallowed schemes (file:, data:, javascript:, etc.)
			return dc.baseURL
		}
		return pathStr
	}

	baseURL, err := url.Parse(dc.baseURL)
	if err != nil {
		return dc.baseURL
	}
	baseURL.Path = stdpath.Join(baseURL.Path, pathStr)
	return baseURL.String()
}

func (dc *DomainClient) prepareManagedOptions() []RequestOption {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	cookieCount := len(dc.cookies)
	headerCount := len(dc.headers)

	if cookieCount == 0 && headerCount == 0 {
		return nil
	}

	options := make([]RequestOption, 0, 2)

	if cookieCount > 0 {
		cookies := make([]http.Cookie, 0, cookieCount)
		for _, cookie := range dc.cookies {
			cookies = append(cookies, *cookie)
		}
		options = append(options, WithCookies(cookies))
	}

	if headerCount > 0 {
		headersCopy := make(map[string]string, headerCount)
		maps.Copy(headersCopy, dc.headers)
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

	if result.Response != nil && len(result.Response.Cookies) > 0 {
		for _, cookie := range result.Response.Cookies {
			if cookie != nil {
				dc.cookies[cookie.Name] = cookie
			}
		}
	}
}

func (dc *DomainClient) SetHeader(key, value string) error {
	if err := validation.ValidateHeaderKeyValue(key, value); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.headers[key] = value
	return nil
}

func (dc *DomainClient) SetHeaders(headers map[string]string) error {
	for k, v := range headers {
		if err := validation.ValidateHeaderKeyValue(k, v); err != nil {
			return fmt.Errorf("invalid header %s: %w", k, err)
		}
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	maps.Copy(dc.headers, headers)
	return nil
}

func (dc *DomainClient) DeleteHeader(key string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	delete(dc.headers, key)
}

func (dc *DomainClient) ClearHeaders() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.headers = make(map[string]string)
}

func (dc *DomainClient) GetHeaders() map[string]string {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	headers := make(map[string]string, len(dc.headers))
	maps.Copy(headers, dc.headers)
	return headers
}

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

func (dc *DomainClient) DeleteCookie(name string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	delete(dc.cookies, name)
}

func (dc *DomainClient) ClearCookies() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.cookies = make(map[string]*http.Cookie)
}

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

func (dc *DomainClient) GetCookie(name string) *http.Cookie {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if cookie, ok := dc.cookies[name]; ok {
		cookieCopy := *cookie
		return &cookieCopy
	}
	return nil
}

func (dc *DomainClient) Close() error {
	return dc.client.Close()
}
