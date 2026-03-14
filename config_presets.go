package httpc

import (
	"time"
)

func SecureConfig() *Config {
	cfg := DefaultConfig()
	cfg.Timeouts.Request = 15 * time.Second
	cfg.Connections.MaxIdleConns = 20
	cfg.Connections.MaxConnsPerHost = 5
	cfg.Security.MaxResponseBodySize = 5 * 1024 * 1024
	cfg.Security.AllowPrivateIPs = false // Strict SSRF protection for security-critical applications
	cfg.Retry.MaxRetries = 1
	cfg.Retry.Delay = 2 * time.Second
	cfg.Middleware.FollowRedirects = false
	cfg.Timeouts.Dial = 5 * time.Second
	cfg.Timeouts.TLSHandshake = 5 * time.Second
	cfg.Timeouts.ResponseHeader = 10 * time.Second
	cfg.Timeouts.IdleConn = 30 * time.Second
	cfg.Security.ValidateURL = true
	cfg.Security.ValidateHeaders = true
	cfg.Retry.EnableJitter = true
	return cfg
}

func PerformanceConfig() *Config {
	cfg := DefaultConfig()
	cfg.Timeouts.Request = 60 * time.Second
	cfg.Connections.MaxIdleConns = 100
	cfg.Connections.MaxConnsPerHost = 20
	cfg.Security.MaxResponseBodySize = 50 * 1024 * 1024
	cfg.Security.StrictContentLength = false
	cfg.Retry.Delay = 500 * time.Millisecond
	cfg.Retry.BackoffFactor = 1.5
	cfg.Connections.EnableCookies = true
	cfg.Timeouts.Dial = 15 * time.Second
	cfg.Timeouts.TLSHandshake = 15 * time.Second
	cfg.Timeouts.ResponseHeader = 60 * time.Second
	cfg.Timeouts.IdleConn = 120 * time.Second
	cfg.Security.ValidateURL = true
	cfg.Security.ValidateHeaders = false
	cfg.Retry.EnableJitter = true
	return cfg
}

// TestingConfig returns a configuration optimized for testing environments.
// WARNING: This config disables security features and should NEVER be used in production.
// Use this ONLY for local development and testing with localhost/private networks.
func TestingConfig() *Config {
	cfg := DefaultConfig()
	cfg.Security.InsecureSkipVerify = true
	cfg.Security.AllowPrivateIPs = true
	cfg.Connections.MaxIdleConns = 10
	cfg.Connections.MaxConnsPerHost = 5
	cfg.Retry.MaxRetries = 1
	cfg.Retry.Delay = 100 * time.Millisecond
	cfg.Middleware.UserAgent = "httpc-test/1.0"
	cfg.Connections.EnableHTTP2 = false
	cfg.Connections.EnableCookies = true
	cfg.Timeouts.Dial = 5 * time.Second
	cfg.Timeouts.TLSHandshake = 5 * time.Second
	cfg.Timeouts.ResponseHeader = 10 * time.Second
	cfg.Timeouts.IdleConn = 30 * time.Second
	cfg.Security.ValidateURL = false
	cfg.Security.ValidateHeaders = false
	cfg.Retry.EnableJitter = false
	return cfg
}

// MinimalConfig returns a lightweight configuration with minimal features.
// Use this for simple, one-off requests where you don't need retries or advanced features.
func MinimalConfig() *Config {
	cfg := DefaultConfig()
	cfg.Connections.MaxIdleConns = 10
	cfg.Connections.MaxConnsPerHost = 2
	cfg.Security.MaxResponseBodySize = 1 * 1024 * 1024
	cfg.Retry.MaxRetries = 0
	cfg.Retry.Delay = 0
	cfg.Retry.BackoffFactor = 1.0
	cfg.Middleware.FollowRedirects = false
	cfg.Timeouts.Dial = 5 * time.Second
	cfg.Timeouts.TLSHandshake = 5 * time.Second
	cfg.Timeouts.ResponseHeader = 10 * time.Second
	cfg.Timeouts.IdleConn = 30 * time.Second
	cfg.Security.ValidateURL = true
	cfg.Security.ValidateHeaders = true
	cfg.Retry.EnableJitter = false
	return cfg
}
