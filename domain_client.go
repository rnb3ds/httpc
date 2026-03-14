package httpc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	stdpath "path"
)

// DomainClient provides a client scoped to a specific domain with session management.
// It maintains cookies and headers across requests and provides convenient methods
// for making HTTP requests relative to a base URL.
type DomainClient struct {
	client    Client
	baseURL   string
	parsedURL *url.URL // Cached parsed URL for efficient URL building
	domain    string
	session   *SessionManager
}

// NewDomain creates a new DomainClient scoped to the specified base URL.
// The client automatically manages cookies and headers across requests.
//
// Example:
//
//	dc, err := httpc.NewDomain("https://api.example.com")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer dc.Close()
//
//	// Set session headers
//	dc.SetHeader("Authorization", "Bearer token")
//
//	// Make requests relative to base URL
//	result, err := dc.Get("/users")
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
		client:    client,
		baseURL:   baseURL,
		parsedURL: parsedURL, // Cache parsed URL for efficient URL building
		domain:    parsedURL.Hostname(),
		session:   NewSessionManager(),
	}, nil
}

// Get makes a GET request to the specified path.
func (dc *DomainClient) Get(path string, options ...RequestOption) (*Result, error) {
	return dc.request("GET", path, options...)
}

// Post makes a POST request to the specified path.
func (dc *DomainClient) Post(path string, options ...RequestOption) (*Result, error) {
	return dc.request("POST", path, options...)
}

// Put makes a PUT request to the specified path.
func (dc *DomainClient) Put(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PUT", path, options...)
}

// Patch makes a PATCH request to the specified path.
func (dc *DomainClient) Patch(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PATCH", path, options...)
}

// Delete makes a DELETE request to the specified path.
func (dc *DomainClient) Delete(path string, options ...RequestOption) (*Result, error) {
	return dc.request("DELETE", path, options...)
}

// GetWithContext makes a GET request with context for cancellation control.
func (dc *DomainClient) GetWithContext(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return dc.Request(ctx, "GET", path, options...)
}

// PostWithContext makes a POST request with context for cancellation control.
func (dc *DomainClient) PostWithContext(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return dc.Request(ctx, "POST", path, options...)
}

// PutWithContext makes a PUT request with context for cancellation control.
func (dc *DomainClient) PutWithContext(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return dc.Request(ctx, "PUT", path, options...)
}

// PatchWithContext makes a PATCH request with context for cancellation control.
func (dc *DomainClient) PatchWithContext(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return dc.Request(ctx, "PATCH", path, options...)
}

// DeleteWithContext makes a DELETE request with context for cancellation control.
func (dc *DomainClient) DeleteWithContext(ctx context.Context, path string, options ...RequestOption) (*Result, error) {
	return dc.Request(ctx, "DELETE", path, options...)
}

// Head makes a HEAD request to the specified path.
func (dc *DomainClient) Head(path string, options ...RequestOption) (*Result, error) {
	return dc.request("HEAD", path, options...)
}

// Options makes an OPTIONS request to the specified path.
func (dc *DomainClient) Options(path string, options ...RequestOption) (*Result, error) {
	return dc.request("OPTIONS", path, options...)
}

// Request makes an HTTP request with the specified method and path.
// The context parameter allows for timeout and cancellation control.
// This method makes DomainClient compatible with the Client interface.
func (dc *DomainClient) Request(ctx context.Context, method, path string, options ...RequestOption) (*Result, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	managedOptions := dc.session.PrepareOptions()
	allOptions := append(managedOptions, options...)

	dc.session.CaptureFromOptions(options)

	result, err := dc.client.Request(ctx, method, fullURL, allOptions...)
	if err != nil {
		return nil, err
	}

	if result != nil {
		dc.session.UpdateFromResult(result)
	}

	return result, nil
}

// DownloadFile downloads a file from the specified path to the given file path.
func (dc *DomainClient) DownloadFile(path string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	managedOptions := dc.session.PrepareOptions()
	allOptions := append(managedOptions, options...)

	dc.session.CaptureFromOptions(options)

	return dc.client.DownloadFile(fullURL, filePath, allOptions...)
}

// DownloadWithOptions downloads a file with custom download options.
func (dc *DomainClient) DownloadWithOptions(path string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	managedOptions := dc.session.PrepareOptions()
	allOptions := append(managedOptions, options...)

	dc.session.CaptureFromOptions(options)

	return dc.client.DownloadWithOptions(fullURL, downloadOpts, allOptions...)
}

func (dc *DomainClient) request(method, path string, options ...RequestOption) (*Result, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	managedOptions := dc.session.PrepareOptions()
	allOptions := append(managedOptions, options...)

	dc.session.CaptureFromOptions(options)

	result, err := dc.client.Request(context.Background(), method, fullURL, allOptions...)
	if err != nil {
		return nil, err
	}

	if result != nil {
		dc.session.UpdateFromResult(result)
	}

	return result, nil
}

func (dc *DomainClient) buildURL(pathStr string) (string, error) {
	if pathStr == "" {
		return dc.baseURL, nil
	}

	// Check if pathStr is already a full URL
	if len(pathStr) > 7 && (pathStr[:7] == "http://" || pathStr[:8] == "https://") {
		parsedURL, err := url.Parse(pathStr)
		if err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			// Validate URL scheme for security
			// Only allow http and https schemes to prevent potential SSRF attacks
			if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
				// Reject URLs with disallowed schemes (file:, data:, javascript:, etc.)
				return "", fmt.Errorf("invalid URL scheme: %q: only http and https are allowed", parsedURL.Scheme)
			}
			return pathStr, nil
		}
	}

	// Use cached parsed URL for efficiency
	if dc.parsedURL == nil {
		// Fallback: should not happen if NewDomain was used
		baseURL, err := url.Parse(dc.baseURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse base URL: %w", err)
		}
		dc.parsedURL = baseURL
	}

	// Clone the cached URL to avoid modifying the original
	result := *dc.parsedURL
	result.Path = stdpath.Join(dc.parsedURL.Path, pathStr)
	return result.String(), nil
}

// SetHeader adds or updates a header in the session.
func (dc *DomainClient) SetHeader(key, value string) error {
	return dc.session.SetHeader(key, value)
}

// SetHeaders adds or updates multiple headers in the session.
func (dc *DomainClient) SetHeaders(headers map[string]string) error {
	return dc.session.SetHeaders(headers)
}

// DeleteHeader removes a header from the session.
func (dc *DomainClient) DeleteHeader(key string) {
	dc.session.DeleteHeader(key)
}

// ClearHeaders removes all headers from the session.
func (dc *DomainClient) ClearHeaders() {
	dc.session.ClearHeaders()
}

// GetHeaders returns a copy of all session headers.
func (dc *DomainClient) GetHeaders() map[string]string {
	return dc.session.GetHeaders()
}

// SetCookie adds or updates a cookie in the session.
func (dc *DomainClient) SetCookie(cookie *http.Cookie) error {
	return dc.session.SetCookie(cookie)
}

// SetCookies adds or updates multiple cookies in the session.
func (dc *DomainClient) SetCookies(cookies []*http.Cookie) error {
	return dc.session.SetCookies(cookies)
}

// DeleteCookie removes a cookie from the session by name.
func (dc *DomainClient) DeleteCookie(name string) {
	dc.session.DeleteCookie(name)
}

// ClearCookies removes all cookies from the session.
func (dc *DomainClient) ClearCookies() {
	dc.session.ClearCookies()
}

// GetCookies returns a copy of all session cookies.
func (dc *DomainClient) GetCookies() []*http.Cookie {
	return dc.session.GetCookies()
}

// GetCookie returns a copy of a cookie by name, or nil if not found.
func (dc *DomainClient) GetCookie(name string) *http.Cookie {
	return dc.session.GetCookie(name)
}

// URL returns the base URL
func (dc *DomainClient) URL() string { return dc.baseURL }

// Domain returns the domain name (host without port)
func (dc *DomainClient) Domain() string { return dc.domain }

// Session returns the underlying SessionManager for advanced session management.
func (dc *DomainClient) Session() *SessionManager {
	return dc.session
}

// Compile-time interface check to ensure DomainClient implements Client.
var _ Client = (*DomainClient)(nil)

// Compile-time interface check to ensure DomainClient implements DomainClienter.
var _ DomainClienter = (*DomainClient)(nil)

// Close closes the underlying HTTP client and releases resources.
func (dc *DomainClient) Close() error {
	return dc.client.Close()
}
