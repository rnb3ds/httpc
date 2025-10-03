package httpc

import (
	"crypto/tls"
	"fmt"
	"time"
)

// SecurityLevel defines the security level for client configuration
type SecurityLevel int

const (
	// SecurityLevelPermissive allows more relaxed security settings
	SecurityLevelPermissive SecurityLevel = iota
	// SecurityLevelBalanced provides a balance between security and compatibility
	SecurityLevelBalanced
	// SecurityLevelStrict enforces strict security settings
	SecurityLevelStrict
)

// ConfigPreset creates a configuration with predefined security settings
func ConfigPreset(level SecurityLevel) *Config {
	switch level {
	case SecurityLevelPermissive:
		return permissiveConfig()
	case SecurityLevelBalanced:
		return balancedConfig()
	case SecurityLevelStrict:
		return strictConfig()
	default:
		return DefaultConfig()
	}
}

// permissiveConfig returns a configuration with relaxed security settings
func permissiveConfig() *Config {
	return &Config{
		Timeout:               120 * time.Second,
		DialTimeout:           30 * time.Second,
		KeepAlive:             60 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       120 * time.Second,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       50,

		MinTLSVersion:         tls.VersionTLS10,
		MaxTLSVersion:         tls.VersionTLS13,
		InsecureSkipVerify:    false,
		MaxResponseBodySize:   100 * 1024 * 1024,
		MaxConcurrentRequests: 1000,
		ValidateURL:           true,
		ValidateHeaders:       false,
		AllowPrivateIPs:       true,

		MaxRetries:    5,
		RetryDelay:    1 * time.Second,
		MaxRetryDelay: 120 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,

		UserAgent:       "httpc/1.0 (Go HTTP Client)",
		Headers:         make(map[string]string),
		FollowRedirects: true,
		EnableHTTP2:     true,

		EnableCookies: true,
	}
}

// balancedConfig returns a configuration with balanced security settings
func balancedConfig() *Config {
	return DefaultConfig()
}

// strictConfig returns a configuration with strict security settings
func strictConfig() *Config {
	return &Config{
		Timeout:               30 * time.Second,
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   5,
		MaxConnsPerHost:       10,

		MinTLSVersion:         tls.VersionTLS13,
		MaxTLSVersion:         tls.VersionTLS13,
		InsecureSkipVerify:    false,
		MaxResponseBodySize:   10 * 1024 * 1024,
		MaxConcurrentRequests: 100,
		ValidateURL:           true,
		ValidateHeaders:       true,
		AllowPrivateIPs:       false,

		MaxRetries:    1,
		RetryDelay:    3 * time.Second,
		MaxRetryDelay: 30 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,

		UserAgent:       "httpc/1.0 (Go HTTP Client)",
		Headers:         make(map[string]string),
		FollowRedirects: false,
		EnableHTTP2:     true,

		EnableCookies: true,
	}
}

func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate timeouts
	if cfg.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	if cfg.DialTimeout < 0 {
		return fmt.Errorf("dial timeout cannot be negative")
	}
	if cfg.TLSHandshakeTimeout < 0 {
		return fmt.Errorf("TLS handshake timeout cannot be negative")
	}
	if cfg.ResponseHeaderTimeout < 0 {
		return fmt.Errorf("response header timeout cannot be negative")
	}
	if cfg.IdleConnTimeout < 0 {
		return fmt.Errorf("idle connection timeout cannot be negative")
	}

	// Validate connection limits
	if cfg.MaxIdleConns < 0 {
		return fmt.Errorf("max idle connections cannot be negative")
	}
	if cfg.MaxIdleConnsPerHost < 0 {
		return fmt.Errorf("max idle connections per host cannot be negative")
	}
	if cfg.MaxConnsPerHost < 0 {
		return fmt.Errorf("max connections per host cannot be negative")
	}
	if cfg.MaxIdleConnsPerHost > cfg.MaxIdleConns {
		return fmt.Errorf("max idle connections per host cannot exceed max idle connections")
	}

	// Validate TLS settings
	if cfg.MinTLSVersion > cfg.MaxTLSVersion {
		return fmt.Errorf("min TLS version cannot be greater than max TLS version")
	}
	validTLSVersions := map[uint16]bool{
		tls.VersionTLS10: true,
		tls.VersionTLS11: true,
		tls.VersionTLS12: true,
		tls.VersionTLS13: true,
	}
	if !validTLSVersions[cfg.MinTLSVersion] {
		return fmt.Errorf("invalid min TLS version: %d", cfg.MinTLSVersion)
	}
	if !validTLSVersions[cfg.MaxTLSVersion] {
		return fmt.Errorf("invalid max TLS version: %d", cfg.MaxTLSVersion)
	}

	// Validate security settings
	if cfg.MaxResponseBodySize < 0 {
		return fmt.Errorf("max response body size cannot be negative")
	}
	if cfg.MaxResponseBodySize > 1024*1024*1024 { // 1GB
		return fmt.Errorf("max response body size too large (max 1GB)")
	}
	if cfg.MaxConcurrentRequests < 0 {
		return fmt.Errorf("max concurrent requests cannot be negative")
	}
	if cfg.MaxConcurrentRequests > 10000 {
		return fmt.Errorf("max concurrent requests too large (max 10000)")
	}

	// Validate retry settings
	if cfg.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	if cfg.MaxRetries > 10 {
		return fmt.Errorf("max retries too large (max 10)")
	}
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("retry delay cannot be negative")
	}
	if cfg.MaxRetryDelay < 0 {
		return fmt.Errorf("max retry delay cannot be negative")
	}
	if cfg.MaxRetryDelay > 0 && cfg.RetryDelay > cfg.MaxRetryDelay {
		return fmt.Errorf("retry delay cannot be greater than max retry delay")
	}
	if cfg.BackoffFactor < 1.0 {
		return fmt.Errorf("backoff factor must be at least 1.0")
	}
	if cfg.BackoffFactor > 10.0 {
		return fmt.Errorf("backoff factor too large (max 10.0)")
	}

	// Validate user agent
	if len(cfg.UserAgent) > 256 {
		return fmt.Errorf("user agent too long (max 256 characters)")
	}

	return nil
}
