package httpc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	stdpath "path"
	"strings"
)

// DomainClient provides a client scoped to a specific domain with session management.
// It maintains cookies and headers across requests and provides convenient methods
// for making HTTP requests relative to a base URL.
//
// For better flexibility, use the DomainClienter interface instead of the concrete type:
//
//	var dc httpc.DomainClienter
//	dc, err := httpc.NewDomain("https://api.example.com")
type DomainClient struct {
	client    Client
	baseURL   string
	parsedURL *url.URL // Cached parsed URL for efficient URL building
	domain    string
	session   *SessionManager
}

// NewDomain creates a new DomainClient scoped to the specified base URL.
// The client automatically manages cookies and headers across requests.
// If no configuration is provided or nil is passed, DefaultConfig() is used.
// Note: Cookies are automatically enabled for DomainClient.
//
// Returns a DomainClienter interface for flexibility and testability.
// Type-assert to *DomainClient if access to the concrete type is needed.
//
// Examples:
//
//	// Use default configuration
//	dc, err := httpc.NewDomain("https://api.example.com")
//
//	// Use custom configuration
//	cfg := httpc.DefaultConfig()
//	cfg.Timeouts.Request = 60 * time.Second
//	dc, err := httpc.NewDomain("https://api.example.com", cfg)
//
//	// Set session headers
//	dc.SetHeader("Authorization", "Bearer token")
//
//	// Make requests relative to base URL
//	result, err := dc.Get("/users")
func NewDomain(baseURL string, config ...*Config) (DomainClienter, error) {
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
		if err := ValidateConfig(config[0]); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
		cfg = deepCopyConfig(config[0])
	} else {
		cfg = DefaultConfig()
	}
	cfg.Connection.EnableCookies = true

	client, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	session, err := NewSessionManager()
	if err != nil {
		_ = client.Close() // best-effort cleanup
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &DomainClient{
		client:    client,
		baseURL:   baseURL,
		parsedURL: parsedURL, // Cache parsed URL for efficient URL building
		domain:    parsedURL.Hostname(),
		session:   session,
	}, nil
}

// Get makes a GET request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Get(path string, options ...RequestOption) (*Result, error) {
	return dc.request("GET", path, options...)
}

// Post makes a POST request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Post(path string, options ...RequestOption) (*Result, error) {
	return dc.request("POST", path, options...)
}

// Put makes a PUT request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Put(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PUT", path, options...)
}

// Patch makes a PATCH request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Patch(path string, options ...RequestOption) (*Result, error) {
	return dc.request("PATCH", path, options...)
}

// Delete makes a DELETE request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Delete(path string, options ...RequestOption) (*Result, error) {
	return dc.request("DELETE", path, options...)
}

// Head makes a HEAD request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Head(path string, options ...RequestOption) (*Result, error) {
	return dc.request("HEAD", path, options...)
}

// Options makes an OPTIONS request to the specified path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
func (dc *DomainClient) Options(path string, options ...RequestOption) (*Result, error) {
	return dc.request("OPTIONS", path, options...)
}

// Request makes an HTTP request with the specified method and path relative to the base URL.
// If path is a full URL (with scheme), it is used directly.
// The context parameter allows for timeout and cancellation control.
// This method makes DomainClient compatible with the Client interface.
func (dc *DomainClient) Request(ctx context.Context, method, path string, options ...RequestOption) (*Result, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	allOptions := dc.prepareSessionOptions(options)

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
// Response cookies are captured into the session, consistent with Request behavior.
func (dc *DomainClient) DownloadFile(path string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	return dc.DownloadFileWithContext(context.Background(), path, filePath, options...)
}

// DownloadWithOptions downloads a file with custom download options.
// Response cookies are captured into the session, consistent with Request behavior.
func (dc *DomainClient) DownloadWithOptions(path string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	return dc.DownloadWithOptionsWithContext(context.Background(), path, downloadOpts, options...)
}

// DownloadFileWithContext downloads a file with context control for cancellation and timeouts.
// Response cookies are captured into the session, consistent with Request behavior.
func (dc *DomainClient) DownloadFileWithContext(ctx context.Context, path string, filePath string, options ...RequestOption) (*DownloadResult, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	allOptions := dc.prepareSessionOptions(options)

	result, err := dc.client.DownloadFileWithContext(ctx, fullURL, filePath, allOptions...)
	if err != nil {
		return nil, err
	}

	dc.captureDownloadCookies(result)
	return result, nil
}

// DownloadWithOptionsWithContext downloads a file with custom download options and context control.
// Response cookies are captured into the session, consistent with Request behavior.
func (dc *DomainClient) DownloadWithOptionsWithContext(ctx context.Context, path string, downloadOpts *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	fullURL, err := dc.buildURL(path)
	if err != nil {
		return nil, err
	}

	allOptions := dc.prepareSessionOptions(options)

	result, err := dc.client.DownloadWithOptionsWithContext(ctx, fullURL, downloadOpts, allOptions...)
	if err != nil {
		return nil, err
	}

	dc.captureDownloadCookies(result)
	return result, nil
}

// prepareSessionOptions merges session state (headers, cookies) with user-provided options.
func (dc *DomainClient) prepareSessionOptions(options []RequestOption) []RequestOption {
	managedOptions := dc.session.prepareOptions()
	allOptions := append(managedOptions, options...)
	dc.session.captureFromOptions(options)
	return allOptions
}

// captureDownloadCookies captures response cookies from a download result into the session.
func (dc *DomainClient) captureDownloadCookies(result *DownloadResult) {
	if result != nil {
		dc.session.UpdateFromCookies(result.ResponseCookies)
	}
}

// request is an internal helper that delegates to Request with a background context.
// This eliminates code duplication between Request() and the convenience methods.
func (dc *DomainClient) request(method, path string, options ...RequestOption) (*Result, error) {
	return dc.Request(context.Background(), method, path, options...)
}

func (dc *DomainClient) buildURL(pathStr string) (string, error) {
	if pathStr == "" {
		return dc.baseURL, nil
	}

	// Check if pathStr is already a full URL
	if strings.HasPrefix(pathStr, "http://") || strings.HasPrefix(pathStr, "https://") {
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

	// Use cached parsed URL (initialized in NewDomain, read-only here)
	if dc.parsedURL == nil {
		return "", fmt.Errorf("base URL was not properly initialized")
	}

	// Clone the cached URL to avoid modifying the original
	result := *dc.parsedURL

	// Parse pathStr to separate path from query/fragment
	parsed, err := url.Parse(pathStr)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", pathStr, err)
	}
	result.Path = stdpath.Join(dc.parsedURL.Path, parsed.Path)
	if parsed.RawQuery != "" {
		result.RawQuery = parsed.RawQuery
	}
	if parsed.Fragment != "" {
		result.Fragment = parsed.Fragment
	}
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
