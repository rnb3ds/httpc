package httpc

import (
	"crypto/tls"
	"net/http"
	"slices"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/engine"
	"github.com/cybergodev/httpc/internal/security"
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

// calculateMaxRetries returns the effective MaxRetries, automatically raising
// it when proxy-rotation-on-status is configured so that every proxy in the
// pool is tried at least once before retries are exhausted.
//
// Without this adjustment, a user with 5 proxies and the default MaxRetries=3
// would only try 4 of the 5 proxies before giving up on a 403. When
// ProxyRotateOnStatus is set, the intent is explicitly to rotate through all
// proxies, so the retry budget is raised to len(ProxyPool)-1 (capped at
// maxRetryAttempts to respect the hard ceiling enforced by ValidateConfig).
//
// ProxyRotatePerRequest also raises the budget: its per-request rotation
// consumes attempts from the same budget, and without the raise a large pool
// would silently reduce the per-request retry headroom to the raw MaxRetries.
func calculateMaxRetries(cfg *Config) int {
	maxRetries := cfg.Retry.MaxRetries

	if (len(cfg.Connection.ProxyRotateOnStatus) > 0 || cfg.Connection.ProxyRotatePerRequest) && len(cfg.Connection.ProxyPool) > 1 {
		needed := len(cfg.Connection.ProxyPool) - 1 // retries beyond the initial attempt
		if needed > maxRetries {
			if needed > maxRetryAttempts {
				needed = maxRetryAttempts
			}
			maxRetries = needed
		}
	}

	return maxRetries
}

// convertToEngineConfig converts public Config to engine Config.
// It uses helper functions for cleaner separation of concerns.
func convertToEngineConfig(cfg *Config) (*engine.Config, error) {
	maxRetryDelay := calculateMaxRetryDelay(cfg)

	cookieJar, err := createCookieJar(cfg.Connection.EnableCookies, cfg.Connection.PublicSuffixList)
	if err != nil {
		return nil, err
	}

	engineConfig := &engine.Config{
		// Timeout settings
		Timeout: cfg.Timeouts.Request,

		// Connection settings — the engine embeds a connection.Config seeded
		// from connection defaults and overridden with the public values. This
		// is the single mapping point for transport/proxy/TLS fields; the
		// engine no longer re-copies them (see engine.buildConnectionConfig).
		Connection: buildConnectionSubConfig(cfg, cookieJar),

		ProxyRotatePerRequest: cfg.Connection.ProxyRotatePerRequest,
		EnableCookies:         cfg.Connection.EnableCookies,

		// Security settings
		TLSFingerprint:                      cfg.Security.TLSFingerprint,
		TLSFingerprintDisableSessionTickets: cfg.Security.TLSFingerprintDisableSessionTickets,
		MaxResponseBodySize:                 cfg.Security.MaxResponseBodySize,
		MaxRequestBodySize:                  cfg.Security.MaxRequestBodySize,
		MaxDecompressedBodySize:             cfg.Security.MaxDecompressedBodySize,
		ValidateURL:                         cfg.Security.ValidateURL,
		ValidateHeaders:                     cfg.Security.ValidateHeaders,
		StrictContentLength:                 cfg.Security.StrictContentLength,
		CertificatePinner:                   cfg.Security.CertificatePinner,

		// Retry settings
		MaxRetries:                calculateMaxRetries(cfg),
		RetryDelay:                cfg.Retry.Delay,
		MaxRetryDelay:             maxRetryDelay,
		BackoffFactor:             cfg.Retry.BackoffFactor,
		Jitter:                    cfg.Retry.EnableJitter,
		RetryNonIdempotent:        cfg.Retry.RetryNonIdempotent,
		ExtraRetryableStatusCodes: cfg.Connection.ProxyRotateOnStatus,
		CustomRetryPolicy:         cfg.Retry.CustomPolicy,

		// Request defaults (User-Agent, headers from Config.Defaults; redirect
		// policy from Config.Security).
		UserAgent:       cfg.Defaults.UserAgent,
		Headers:         cfg.Defaults.Headers,
		FollowRedirects: cfg.Security.FollowRedirects,
		MaxRedirects:    cfg.Security.MaxRedirects,
	}

	if len(cfg.Security.RedirectWhitelist) > 0 {
		engineConfig.RedirectWhitelist = security.NewDomainWhitelist(cfg.Security.RedirectWhitelist...)
	}

	if len(cfg.Security.SensitiveHeaders) > 0 {
		engineConfig.SensitiveHeaders = canonicalHeaderNames(cfg.Security.SensitiveHeaders)
	}

	return engineConfig, nil
}

// canonicalHeaderNames canonicalizes header names into the
// http.CanonicalHeaderKey form the engine's strip list compares against, and
// drops duplicates (including entries that canonicalize onto one another).
func canonicalHeaderNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		c := http.CanonicalHeaderKey(n)
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// buildConnectionSubConfig maps the public ConnectionConfig onto the engine's
// embedded connection.Config, starting from connection defaults and overriding
// every public field. It is the single hand-mapping left after the mirror
// collapse; connection-only fields (MaxTotalConns, ExpectContinueTimeout,
// DisableKeepAlives, ForceAttemptHTTP2) keep their connection defaults. The
// certificate pinner is plumbed separately by the engine (buildConnectionConfig
// assigns Config.Connection.CertPinner from Config.CertificatePinner).
//
// jar is the pre-built cookie jar (nil when EnableCookies is false), hoisted
// out of this function so the infallible assignments here don't need an error
// return for a jar-construction failure.
func buildConnectionSubConfig(cfg *Config, jar http.CookieJar) connection.Config {
	conn := *connection.DefaultConfig()

	// Connection pool sizing. MaxIdleConnsPerHost is derived (public knob is
	// MaxConnsPerHost only), matching the previous convertToEngineConfig
	// behavior.
	conn.MaxIdleConns = cfg.Connection.MaxIdleConns
	conn.MaxIdleConnsPerHost = calculateIdleConnsPerHost(cfg.Connection.MaxConnsPerHost)
	conn.MaxConnsPerHost = cfg.Connection.MaxConnsPerHost
	conn.MaxResponseHeaderBytes = cfg.Connection.MaxResponseHeaderBytes

	conn.DialTimeout = cfg.Timeouts.Dial
	conn.KeepAlive = defaultKeepAlive
	conn.TLSHandshakeTimeout = cfg.Timeouts.TLSHandshake
	conn.ResponseHeaderTimeout = cfg.Timeouts.ResponseHeader
	conn.IdleConnTimeout = cfg.Timeouts.IdleConn

	conn.TLSConfig = cfg.Security.TLSConfig
	conn.MinTLSVersion, conn.MaxTLSVersion = resolveTLSVersions(cfg)
	conn.InsecureSkipVerify = cfg.Security.InsecureSkipVerify

	conn.EnableHTTP2 = cfg.Connection.EnableHTTP2
	conn.ProxyURL = cfg.Connection.ProxyURL
	conn.EnableSystemProxy = cfg.Connection.EnableSystemProxy
	conn.ProxyPool = cfg.Connection.ProxyPool
	conn.ProxyPoolStrategy = cfg.Connection.ProxyPoolStrategy
	conn.ProxyFailureThreshold = cfg.Connection.ProxyFailureThreshold
	conn.ProxyCooldown = cfg.Connection.ProxyCooldown
	conn.EnableProxyPoolHTTP2 = cfg.Connection.EnableProxyPoolHTTP2

	conn.AllowPrivateIPs = cfg.Security.AllowPrivateIPs
	conn.ExemptNets = cfg.parsedCIDRs

	conn.CookieJar = jar
	conn.EnableDoH = cfg.Connection.EnableDoH
	conn.DoHCacheTTL = cfg.Connection.DoHCacheTTL

	return conn
}
