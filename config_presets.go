package httpc

import (
	"time"
)

func SecureConfig() *Config {
	cfg := DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.MaxIdleConns = 20
	cfg.MaxConnsPerHost = 5
	cfg.MaxResponseBodySize = 5 * 1024 * 1024
	cfg.MaxRetries = 1
	cfg.RetryDelay = 2 * time.Second
	cfg.FollowRedirects = false
	return cfg
}

func PerformanceConfig() *Config {
	cfg := DefaultConfig()
	cfg.Timeout = 60 * time.Second
	cfg.MaxIdleConns = 100
	cfg.MaxConnsPerHost = 20
	cfg.MaxResponseBodySize = 50 * 1024 * 1024
	cfg.StrictContentLength = false
	cfg.RetryDelay = 500 * time.Millisecond
	cfg.BackoffFactor = 1.5
	cfg.EnableCookies = true
	return cfg
}

// TestingConfig returns a configuration optimized for testing environments.
// WARNING: This config disables security features and should NEVER be used in production.
// Use this ONLY for local development and testing with localhost/private networks.
func TestingConfig() *Config {
	cfg := DefaultConfig()
	cfg.InsecureSkipVerify = true
	cfg.AllowPrivateIPs = true
	cfg.MaxIdleConns = 10
	cfg.MaxConnsPerHost = 5
	cfg.MaxRetries = 1
	cfg.RetryDelay = 100 * time.Millisecond
	cfg.UserAgent = "httpc-test/1.0"
	cfg.EnableHTTP2 = false
	cfg.EnableCookies = true
	return cfg
}

// MinimalConfig returns a lightweight configuration with minimal features.
// Use this for simple, one-off requests where you don't need retries or advanced features.
func MinimalConfig() *Config {
	cfg := DefaultConfig()
	cfg.MaxIdleConns = 10
	cfg.MaxConnsPerHost = 2
	cfg.MaxResponseBodySize = 1 * 1024 * 1024
	cfg.MaxRetries = 0
	cfg.RetryDelay = 0
	cfg.BackoffFactor = 1.0
	cfg.FollowRedirects = false
	return cfg
}
