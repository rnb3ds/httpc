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
	minIdleConnsPerHost    = 2                // Minimum idle connections per host
	maxIdleConnsPerHostCap = 10               // Maximum cap for idle connections per host
	defaultKeepAlive       = 30 * time.Second // TCP keep-alive interval for connection pooling
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
		idleConns = minIdleConnsPerHost
	}
	if idleConns > maxIdleConnsPerHostCap {
		idleConns = maxIdleConnsPerHostCap
	}
	// Don't exceed max total connections per host
	if idleConns > maxConnsPerHost {
		idleConns = maxConnsPerHost
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

// calculateMaxRetryDelay returns the maximum retry delay from configuration.
// Uses the user-provided MaxRetryDelay if set (> 0), otherwise defaults to 30s.
func calculateMaxRetryDelay(cfg *Config) time.Duration {
	if cfg.Retry.MaxRetryDelay > 0 {
		return cfg.Retry.MaxRetryDelay
	}
	return 30 * time.Second
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
		KeepAlive:             defaultKeepAlive,
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
		BrowserFingerprint:     cfg.Connection.BrowserFingerprint,

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
		Headers:         copyHeadersMap(cfg.Middleware.Headers),
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

// copyHeadersMap creates a shallow copy of a string map to prevent
// shared-reference mutation between the public Config and engine Config.
func copyHeadersMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return src
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
