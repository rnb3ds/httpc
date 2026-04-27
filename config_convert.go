package httpc

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
	"github.com/cybergodev/httpc/internal/security"
	"github.com/cybergodev/httpc/internal/validation"
)

const (
	minIdleConnsPerHost            = 2  // Minimum idle connections per host
	maxIdleConnsPerHostCap         = 10 // Maximum cap for idle connections per host
	maxRetryDelayBackoffMultiplier = 3  // Multiplier for Delay*BackoffFactor to cap max retry delay
)

// calculateIdleConnsPerHost calculates the optimal number of idle connections per host
// based on MaxConnsPerHost configuration.
func calculateIdleConnsPerHost(maxConnsPerHost int) int {
	if maxConnsPerHost == 0 {
		// Unlimited max connections - use reasonable default for idle
		return maxIdleConnsPerHostCap
	}
	idleConns := maxConnsPerHost / 2
	if idleConns < minIdleConnsPerHost {
		return minIdleConnsPerHost
	}
	if idleConns > maxIdleConnsPerHostCap {
		return maxIdleConnsPerHostCap
	}
	return idleConns
}

// resolveTLSVersions returns the minimum and maximum TLS versions from config.
// Falls back to TLS 1.2 and TLS 1.3 if not specified.
func resolveTLSVersions(cfg *Config) (min, max uint16) {
	min = cfg.Security.MinTLSVersion
	if min == 0 {
		min = tls.VersionTLS12
	}
	max = cfg.Security.MaxTLSVersion
	if max == 0 {
		max = tls.VersionTLS13
	}
	return min, max
}

// calculateMaxRetryDelay calculates the maximum retry delay based on configuration.
// Formula: min(RetryDelay * BackoffFactor * 3, 30s)
func calculateMaxRetryDelay(cfg *Config) time.Duration {
	const (
		defaultMaxRetryDelay  = 5 * time.Second
		absoluteMaxRetryDelay = 30 * time.Second
	)

	if cfg.Retry.Delay <= 0 || cfg.Retry.BackoffFactor <= 0 {
		return defaultMaxRetryDelay
	}

	calculated := time.Duration(float64(cfg.Retry.Delay) * cfg.Retry.BackoffFactor * maxRetryDelayBackoffMultiplier)
	if calculated > absoluteMaxRetryDelay {
		return absoluteMaxRetryDelay
	}
	if calculated < defaultMaxRetryDelay {
		return defaultMaxRetryDelay
	}
	return calculated
}

// convertToEngineConfig converts public Config to engine Config.
// It uses helper functions for cleaner separation of concerns.
func convertToEngineConfig(cfg *Config) (*engine.Config, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	idleConnsPerHost := calculateIdleConnsPerHost(cfg.Connection.MaxConnsPerHost)
	minTLSVersion, maxTLSVersion := resolveTLSVersions(cfg)
	maxRetryDelay := calculateMaxRetryDelay(cfg)

	cookieJar, err := createCookieJar(cfg.Connection.EnableCookies)
	if err != nil {
		return nil, err
	}

	engineConfig := &engine.Config{
		// Timeout settings
		Timeout:               cfg.Timeouts.Request,
		DialTimeout:           cfg.Timeouts.Dial,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   cfg.Timeouts.TLSHandshake,
		ResponseHeaderTimeout: cfg.Timeouts.ResponseHeader,
		IdleConnTimeout:       cfg.Timeouts.IdleConn,

		// Connection settings
		MaxIdleConns:           cfg.Connection.MaxIdleConns,
		MaxIdleConnsPerHost:    idleConnsPerHost,
		MaxConnsPerHost:        cfg.Connection.MaxConnsPerHost,
		MaxResponseHeaderBytes: cfg.Connection.MaxResponseHeaderBytes,
		ProxyURL:               cfg.Connection.ProxyURL,
		EnableSystemProxy:      cfg.Connection.EnableSystemProxy,
		EnableHTTP2:            cfg.Connection.EnableHTTP2,
		CookieJar:              cookieJar,
		EnableCookies:          cfg.Connection.EnableCookies,
		EnableDoH:              cfg.Connection.EnableDoH,
		DoHCacheTTL:            cfg.Connection.DoHCacheTTL,

		// Security settings
		TLSConfig:               cfg.Security.TLSConfig,
		MinTLSVersion:           minTLSVersion,
		MaxTLSVersion:           maxTLSVersion,
		InsecureSkipVerify:      cfg.Security.InsecureSkipVerify,
		MaxResponseBodySize:     cfg.Security.MaxResponseBodySize,
		MaxRequestBodySize:      cfg.Security.MaxRequestBodySize,
		MaxDecompressedBodySize: cfg.Security.MaxDecompressedBodySize,
		ValidateURL:             cfg.Security.ValidateURL,
		ValidateHeaders:         cfg.Security.ValidateHeaders,
		AllowPrivateIPs:         cfg.Security.AllowPrivateIPs,
		StrictContentLength:     cfg.Security.StrictContentLength,

		// Retry settings
		MaxRetries:        cfg.Retry.MaxRetries,
		RetryDelay:        cfg.Retry.Delay,
		MaxRetryDelay:     maxRetryDelay,
		BackoffFactor:     cfg.Retry.BackoffFactor,
		Jitter:            cfg.Retry.EnableJitter,
		CustomRetryPolicy: cfg.Retry.CustomPolicy,

		// Middleware settings
		UserAgent:       cfg.Middleware.UserAgent,
		Headers:         cfg.Middleware.Headers,
		FollowRedirects: cfg.Middleware.FollowRedirects,
		MaxRedirects:    cfg.Middleware.MaxRedirects,
	}

	if len(cfg.Security.RedirectWhitelist) > 0 {
		engineConfig.RedirectWhitelist = security.NewDomainWhitelist(cfg.Security.RedirectWhitelist...)
	}

	// Parse SSRF exempt CIDRs
	exemptNets, err := parseExemptCIDRs(cfg.Security.SSRFExemptCIDRs)
	if err != nil {
		return nil, err
	}
	engineConfig.ExemptNets = exemptNets

	return engineConfig, nil
}

// parseExemptCIDRs parses and validates SSRF exempt CIDR strings.
func parseExemptCIDRs(cidrs []string) ([]*net.IPNet, error) {
	if len(cidrs) == 0 {
		return nil, nil
	}
	exemptNets, err := validation.ParseExemptCIDRs(cidrs)
	if err != nil {
		return nil, fmt.Errorf("invalid SSRF exempt CIDRs: %w", err)
	}
	return exemptNets, nil
}
