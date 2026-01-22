package connection

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/dns"
	"github.com/cybergodev/httpc/internal/netutil"
	"github.com/cybergodev/httpc/internal/proxy"
)

// PoolManager provides intelligent connection pool management with monitoring
type PoolManager struct {
	config *Config

	transport *http.Transport
	dohResolver *dns.DoHResolver

	activeConns   int64
	totalConns    int64
	rejectedConns int64

	hostConns sync.Map

	metrics *Metrics

	closed int32
	mu     sync.RWMutex
}

// Config defines connection pool configuration
type Config struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	MaxTotalConns       int

	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	ExpectContinueTimeout time.Duration

	TLSConfig          *tls.Config
	MinTLSVersion      uint16
	MaxTLSVersion      uint16
	InsecureSkipVerify bool

	EnableHTTP2 bool
	ProxyURL    string

	// System proxy configuration
	EnableSystemProxy bool // Automatically detect and use system proxy settings

	AllowPrivateIPs bool

	DisableCompression bool
	DisableKeepAlives  bool
	ForceAttemptHTTP2  bool

	CookieJar interface{}

	// DNS configuration
	EnableDoH        bool          // Enable DNS-over-HTTPS
	DoHCacheTTL      time.Duration // DoH cache TTL
}

// HostStats tracks per-host connection statistics
type HostStats struct {
	Host           string
	ActiveConns    int64
	IdleConns      int64
	TotalConns     int64
	FailedConns    int64
	LastUsed       int64 // Unix timestamp
	AverageLatency int64 // Nanoseconds
}

// Metrics provides connection pool performance metrics
type Metrics struct {
	ActiveConnections   int64
	TotalConnections    int64
	RejectedConnections int64
	ConnectionHitRate   float64
	LastUpdate          int64
}

// DefaultConfig returns optimized default configuration
func DefaultConfig() *Config {
	return &Config{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     50,
		MaxTotalConns:       1000,

		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		MinTLSVersion:      tls.VersionTLS12,
		MaxTLSVersion:      tls.VersionTLS13,
		InsecureSkipVerify: false,

		EnableHTTP2: true,

		DisableCompression: false,
		DisableKeepAlives:  false,
		ForceAttemptHTTP2:  true,

		AllowPrivateIPs: true,
	}
}

// NewPoolManager creates a new connection pool manager
func NewPoolManager(config *Config) (*PoolManager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	pm := &PoolManager{
		config:  config,
		metrics: &Metrics{},
	}

	// Initialize DoH resolver if enabled
	if config.EnableDoH {
		pm.dohResolver = dns.NewDoHResolver(nil, config.DoHCacheTTL)
	}

	transport := &http.Transport{
		DialContext:           pm.createDialer(),
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		TLSClientConfig:       pm.createTLSConfig(),
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
		IdleConnTimeout:       config.IdleConnTimeout,
		ExpectContinueTimeout: config.ExpectContinueTimeout,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       config.MaxConnsPerHost,
		ForceAttemptHTTP2:     config.ForceAttemptHTTP2,
		DisableCompression:    true, // Always disable automatic decompression - we handle it manually
		DisableKeepAlives:     config.DisableKeepAlives,
	}
	// Configure proxy settings with priority:
	// 1. Manual proxy URL (highest priority)
	// 2. System proxy detection (if enabled)
	// 3. Direct connection (no proxy)
	if config.ProxyURL != "" {
		// User explicitly specified a proxy URL - use it
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	} else if config.EnableSystemProxy {
		// No manual proxy, but system proxy detection is enabled
		// Automatically detect system proxy settings (reads from Windows registry,
		// macOS system settings, environment variables, etc.)
		detector := proxy.NewDetector()
		if proxyFunc := detector.GetProxyFunc(); proxyFunc != nil {
			transport.Proxy = proxyFunc
		}
		// If proxyFunc is nil, transport.Proxy remains nil (direct connection)
	}
	// If neither condition is met, transport.Proxy remains nil (direct connection)

	pm.transport = transport
	return pm, nil
}

// createDialer creates an optimized dialer with SSRF protection and connection tracking.
func (pm *PoolManager) createDialer() func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   pm.config.DialTimeout,
		KeepAlive: pm.config.KeepAlive,
		// Note: Control is not used here due to cross-platform compatibility issues.
		// SSRF protection is implemented directly in the dialer function instead.
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		startTime := time.Now()

		// If DoH is enabled, resolve the address using DoH and dial the IP directly
		if pm.dohResolver != nil {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				host = address
				port = "443"
			}

			// Use DoH resolver for DNS lookup
			ips, err := pm.dohResolver.LookupIPAddr(ctx, host)
			if err != nil {
				atomic.AddInt64(&pm.rejectedConns, 1)
				return nil, fmt.Errorf("DoH DNS resolution failed: %w", err)
			}

			// SSRF protection check
			if !pm.config.AllowPrivateIPs {
				for _, ip := range ips {
					if err := netutil.ValidateIP(ip.IP); err != nil {
						atomic.AddInt64(&pm.rejectedConns, 1)
						return nil, fmt.Errorf("SSRF protection: %w", err)
					}
				}
			}

			// Try to connect to each resolved IP until one succeeds
			var lastErr error
			for _, ipAddr := range ips {
				ipAddress := net.JoinHostPort(ipAddr.IP.String(), port)
				conn, err := dialer.DialContext(ctx, network, ipAddress)
				connTime := time.Since(startTime).Nanoseconds()
				pm.updateConnectionMetrics(address, connTime, err == nil)

				if err == nil {
					atomic.AddInt64(&pm.totalConns, 1)
					atomic.AddInt64(&pm.activeConns, 1)
					return &trackedConn{
						Conn: conn,
						pm:   pm,
						host: address,
					}, nil
				}
				lastErr = err
			}

			atomic.AddInt64(&pm.rejectedConns, 1)
			return nil, fmt.Errorf("connection failed after trying %d IPs: %w", len(ips), lastErr)
		}

		// Standard path without DoH
		// Perform SSRF protection check before dialing if enabled
		if !pm.config.AllowPrivateIPs {
			if err := pm.validateAddressBeforeDial(address); err != nil {
				atomic.AddInt64(&pm.rejectedConns, 1)
				return nil, fmt.Errorf("SSRF protection: %w", err)
			}
		}

		conn, err := dialer.DialContext(ctx, network, address)
		connTime := time.Since(startTime).Nanoseconds()
		pm.updateConnectionMetrics(address, connTime, err == nil)

		if err != nil {
			atomic.AddInt64(&pm.rejectedConns, 1)
			return nil, fmt.Errorf("connection failed: %w", err)
		}

		atomic.AddInt64(&pm.totalConns, 1)
		atomic.AddInt64(&pm.activeConns, 1)

		return &trackedConn{
			Conn: conn,
			pm:   pm,
			host: address,
		}, nil
	}
}

// validateAddressBeforeDial performs address validation before dialing to prevent SSRF attacks.
// This method validates both IP addresses and domain names to ensure they don't point
// to private, reserved, or local addresses when SSRF protection is enabled.
func (pm *PoolManager) validateAddressBeforeDial(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	// If the address is already an IP, validate it directly
	if ip := net.ParseIP(host); ip != nil {
		return netutil.ValidateIP(ip)
	}

	// For domain names, we need to resolve them first to check all potential IPs
	// This provides defense in depth against DNS rebinding attacks
	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS resolution fails, we'll let the connection attempt proceed
		// The dialer will handle the actual connection and report the error
		return nil
	}

	// Check all resolved IPs - if any point to a private/reserved address, block it
	for _, ip := range ips {
		if err := netutil.ValidateIP(ip); err != nil {
			return fmt.Errorf("domain %s resolves to blocked address: %w", host, err)
		}
	}

	return nil
}

func (pm *PoolManager) createTLSConfig() *tls.Config {
	if pm.config.TLSConfig != nil {
		return pm.config.TLSConfig.Clone()
	}

	tlsConfig := &tls.Config{
		MinVersion:         pm.config.MinTLSVersion,
		MaxVersion:         pm.config.MaxTLSVersion,
		InsecureSkipVerify: pm.config.InsecureSkipVerify,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		SessionTicketsDisabled: false,
		ClientSessionCache:     tls.NewLRUClientSessionCache(256),
		Renegotiation:          tls.RenegotiateNever,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
	}

	return tlsConfig
}

type trackedConn struct {
	net.Conn
	pm   *PoolManager
	host string
}

func (tc *trackedConn) Close() error {
	atomic.AddInt64(&tc.pm.activeConns, -1)
	if value, ok := tc.pm.hostConns.Load(tc.host); ok {
		stats := value.(*HostStats)
		atomic.AddInt64(&stats.ActiveConns, -1)
	}
	return tc.Conn.Close()
}

// updateConnectionMetrics efficiently updates per-host connection statistics.
func (pm *PoolManager) updateConnectionMetrics(host string, connTime int64, success bool) {
	value, _ := pm.hostConns.LoadOrStore(host, &HostStats{
		Host:     host,
		LastUsed: time.Now().Unix(),
	})

	stats := value.(*HostStats)

	if success {
		atomic.AddInt64(&stats.TotalConns, 1)
		atomic.AddInt64(&stats.ActiveConns, 1)

		// Update average latency with limited retries to prevent CPU thrashing
		const maxAtomicRetries = 10
		for i := 0; i < maxAtomicRetries; i++ {
			current := atomic.LoadInt64(&stats.AverageLatency)
			newAvg := (current*9 + connTime) / 10
			if atomic.CompareAndSwapInt64(&stats.AverageLatency, current, newAvg) {
				break
			}
		}
	} else {
		atomic.AddInt64(&stats.FailedConns, 1)
	}

	atomic.StoreInt64(&stats.LastUsed, time.Now().Unix())
}

func (pm *PoolManager) GetTransport() *http.Transport {
	return pm.transport
}

func (pm *PoolManager) GetMetrics() Metrics {
	total := atomic.LoadInt64(&pm.totalConns)
	rejected := atomic.LoadInt64(&pm.rejectedConns)
	hitRate := 0.0
	if total+rejected > 0 {
		hitRate = float64(total) / float64(total+rejected)
	}

	return Metrics{
		ActiveConnections:   atomic.LoadInt64(&pm.activeConns),
		TotalConnections:    total,
		RejectedConnections: rejected,
		ConnectionHitRate:   hitRate,
		LastUpdate:          time.Now().Unix(),
	}
}

func (pm *PoolManager) Close() error {
	if !atomic.CompareAndSwapInt32(&pm.closed, 0, 1) {
		return nil
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.transport != nil {
		pm.transport.CloseIdleConnections()
	}
	return nil
}

func (pm *PoolManager) IsHealthy() bool {
	metrics := pm.GetMetrics()
	if metrics.ConnectionHitRate < 0.9 && metrics.TotalConnections > 10 {
		return false
	}
	if metrics.ActiveConnections >= int64(pm.config.MaxTotalConns)*9/10 {
		return false
	}
	return true
}
