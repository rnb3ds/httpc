package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// Client represents the HTTP client interface
type Client interface {
	// HTTP methods
	Get(url string, options ...RequestOption) (*Response, error)
	Post(url string, options ...RequestOption) (*Response, error)
	Put(url string, options ...RequestOption) (*Response, error)
	Patch(url string, options ...RequestOption) (*Response, error)
	Delete(url string, options ...RequestOption) (*Response, error)
	Head(url string, options ...RequestOption) (*Response, error)
	Options(url string, options ...RequestOption) (*Response, error)

	// Generic request method
	Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error)

	// File download
	DownloadFile(url string, filePath string, options ...RequestOption) (*DownloadResult, error)
	DownloadWithOptions(url string, downloadOpts *DownloadOptions, options ...RequestOption) (*DownloadResult, error)

	// Client management
	Close() error
}

// clientImpl implements the Client interface using the engine
type clientImpl struct {
	engine *engine.Client
}

// New creates a new HTTP client with the provided configuration.
// If no configuration is provided, secure defaults are used.
func New(config ...*Config) (Client, error) {
	var cfg *Config
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
		// 验证配置安全性
		if err := ValidateConfig(cfg); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
	} else {
		cfg = DefaultConfig()
	}

	engineConfig := convertToEngineConfig(cfg)

	engineClient, err := engine.NewClient(engineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &clientImpl{engine: engineClient}, nil
}

func (c *clientImpl) Get(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Get(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Post(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Post(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Put(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Put(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Patch(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Patch(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Delete(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Delete(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Head(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Head(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Options(url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Options(url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Request(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	engineOptions := convertRequestOptions(options)
	resp, err := c.engine.Request(ctx, method, url, engineOptions...)
	return convertEngineResponse(resp), err
}

func (c *clientImpl) Close() error {
	return c.engine.Close()
}

// Default client instance for package-level functions
var (
	defaultClient Client
	defaultOnce   sync.Once
	defaultErr    error
	defaultMu     sync.RWMutex
)

// getDefaultClient returns the default client, creating it if necessary
func getDefaultClient() (Client, error) {
	defaultMu.RLock()
	if defaultClient != nil {
		client := defaultClient
		defaultMu.RUnlock()
		return client, nil
	}
	defaultMu.RUnlock()

	defaultOnce.Do(func() {
		defaultClient, defaultErr = New()
	})
	return defaultClient, defaultErr
}

// Get executes a GET request using the default client
func Get(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Get(url, options...)
}

// Post executes a POST request using the default client
func Post(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Post(url, options...)
}

// Put executes a PUT request using the default client
func Put(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Put(url, options...)
}

// Patch executes a PATCH request using the default client
func Patch(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Patch(url, options...)
}

// Delete executes a DELETE request using the default client
func Delete(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Delete(url, options...)
}

// Head executes a HEAD request using the default client
func Head(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Head(url, options...)
}

// Options executes an OPTIONS request using the default client
func Options(url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Options(url, options...)
}

// Do execute a request using the default client
func Do(ctx context.Context, method, url string, options ...RequestOption) (*Response, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.Request(ctx, method, url, options...)
}

// SetDefaultClient sets the default client used by package-level functions
func SetDefaultClient(client Client) error {
	if client == nil {
		return fmt.Errorf("client cannot be nil")
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()

	if defaultClient != nil {
		_ = defaultClient.Close()
	}

	defaultClient = client
	return nil
}

// CloseDefaultClient closes the default client and releases its resources
func CloseDefaultClient() error {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	var err error
	if defaultClient != nil {
		err = defaultClient.Close()
	}

	defaultClient = nil
	defaultErr = nil

	return err
}

func convertToEngineConfig(cfg *Config) *engine.Config {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 安全性验证和限制
	maxIdleConnsPerHost := cfg.MaxIdleConns / 10
	if maxIdleConnsPerHost < 1 {
		maxIdleConnsPerHost = 1
	}
	if maxIdleConnsPerHost > 50 {
		maxIdleConnsPerHost = 50 // 限制最大值
	}

	// 限制最大并发请求数以防止资源耗尽
	maxConcurrent := 500
	if cfg.MaxConnsPerHost > 0 && cfg.MaxConnsPerHost < maxConcurrent {
		maxConcurrent = cfg.MaxConnsPerHost * 10
	}

	return &engine.Config{
		Timeout:               cfg.Timeout,
		DialTimeout:           15 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		ProxyURL:              cfg.ProxyURL,
		TLSConfig:             cfg.TLSConfig,
		MinTLSVersion:         tls.VersionTLS12, // 强制最低TLS 1.2
		MaxTLSVersion:         tls.VersionTLS13,
		InsecureSkipVerify:    cfg.InsecureSkipVerify,
		MaxResponseBodySize:   cfg.MaxResponseBodySize,
		MaxConcurrentRequests: maxConcurrent,
		ValidateURL:           true, // 强制启用URL验证
		ValidateHeaders:       true, // 强制启用头部验证
		AllowPrivateIPs:       cfg.AllowPrivateIPs,
		MaxRetries:            cfg.MaxRetries,
		RetryDelay:            cfg.RetryDelay,
		MaxRetryDelay:         1 * time.Second, // 固定最大延迟防止DoS
		BackoffFactor:         cfg.BackoffFactor,
		Jitter:                true, // 启用抖动防止雷群效应
		UserAgent:             cfg.UserAgent,
		Headers:               cfg.Headers,
		FollowRedirects:       cfg.FollowRedirects,
		EnableHTTP2:           cfg.EnableHTTP2,
		CookieJar:             createCookieJar(cfg.EnableCookies),
		EnableCookies:         cfg.EnableCookies,
	}
}

func convertRequestOptions(options []RequestOption) []engine.RequestOption {
	if len(options) == 0 {
		return nil
	}

	engineOptions := make([]engine.RequestOption, 0, len(options))
	for _, opt := range options {
		if opt == nil {
			continue
		}
		option := opt
		engineOptions = append(engineOptions, func(req *engine.Request) {
			publicReq := &Request{
				Method:      req.Method,
				URL:         req.URL,
				Headers:     req.Headers,
				QueryParams: req.QueryParams,
				Body:        req.Body,
				Context:     req.Context,
				Timeout:     req.Timeout,
				MaxRetries:  req.MaxRetries,
				Cookies:     req.Cookies, // Preserve existing cookies
			}

			option(publicReq)

			req.Method = publicReq.Method
			req.URL = publicReq.URL
			req.Headers = publicReq.Headers
			req.QueryParams = publicReq.QueryParams
			req.Body = publicReq.Body
			req.Context = publicReq.Context
			req.Timeout = publicReq.Timeout
			req.MaxRetries = publicReq.MaxRetries
			req.Cookies = publicReq.Cookies
		})
	}

	return engineOptions
}

func convertEngineResponse(engineResp *engine.Response) *Response {
	if engineResp == nil {
		return nil
	}

	return &Response{
		StatusCode:    engineResp.StatusCode,
		Status:        engineResp.Status,
		Headers:       engineResp.Headers,
		Body:          engineResp.Body,
		RawBody:       engineResp.RawBody,
		ContentLength: engineResp.ContentLength,
		Duration:      engineResp.Duration,
		Attempts:      engineResp.Attempts,
		Cookies:       engineResp.Cookies,
	}
}

// createCookieJar creates a cookie jar if cookies are enabled
func createCookieJar(enableCookies bool) interface{} {
	if !enableCookies {
		return nil
	}

	jar, err := NewCookieJar()
	if err != nil {
		// If we can't create a cookie jar, return nil
		return nil
	}

	return jar
}
