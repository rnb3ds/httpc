package connection

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/dns"
	"github.com/cybergodev/httpc/internal/proxy"
	"github.com/cybergodev/httpc/internal/validation"
)

// PoolManager provides intelligent connection pool management with monitoring
type PoolManager struct {
	config *Config

	transport   *http.Transport
	dohResolver *dns.DoHResolver

	activeConns   int64
	totalConns    int64
	rejectedConns int64

	hostConns sync.Map

	metrics *metrics

	closed int32
	mu     sync.RWMutex
}

// certPinner defines the interface for certificate pinning
type certPinner interface {
	VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
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

	CookieJar http.CookieJar

	// DNS configuration
	EnableDoH   bool          // Enable DNS-over-HTTPS
	DoHCacheTTL time.Duration // DoH cache TTL

	// Certificate pinning
	certPinner certPinner
}

// hostStats tracks per-host connection statistics
type hostStats struct {
	Host           string
	ActiveConns    int64
	TotalConns     int64
	FailedConns    int64
	LastUsed       int64      // Unix timestamp
	AverageLatency int64      // Nanoseconds
	mu             sync.Mutex // Protects AverageLatency updates
}

// metrics provides connection pool performance metrics
type metrics struct {
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

		AllowPrivateIPs: false,
	}
}

// NewPoolManager creates a new connection pool manager
func NewPoolManager(config *Config) (*PoolManager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	pm := &PoolManager{
		config:  config,
		metrics: &metrics{},
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
					if err := validation.ValidateIP(ip.IP); err != nil {
						atomic.AddInt64(&pm.rejectedConns, 1)
						return nil, fmt.Errorf("SSRF protection: %w", err)
					}
				}
			}

			// Try to connect to each resolved IP until one succeeds
			var lastErr error
			for _, ipAddr := range ips {
				ipAddress := net.JoinHostPort(ipAddr.IP.String(), port)
				attemptStart := time.Now()
				conn, err := dialer.DialContext(ctx, network, ipAddress)
				connTime := time.Since(attemptStart).Nanoseconds()
				stats := pm.updateConnectionMetrics(address, connTime, err == nil)

				if err == nil {
					atomic.AddInt64(&pm.totalConns, 1)
					atomic.AddInt64(&pm.activeConns, 1)
					return &trackedConn{
						Conn:  conn,
						pm:    pm,
						host:  address,
						stats: stats,
					}, nil
				}
				lastErr = err
			}

			atomic.AddInt64(&pm.rejectedConns, 1)
			return nil, fmt.Errorf("connection failed after trying %d IPs: %w", len(ips), lastErr)
		}

		// Standard path without DoH
		// SECURITY: Resolve DNS, validate all IPs, then dial the validated IP directly
		// to prevent DNS rebinding TOCTOU attacks where an attacker-controlled DNS
		// server returns a different IP between validation and actual connection.
		if !pm.config.AllowPrivateIPs {
			validatedAddr, err := pm.resolveAndValidateAddress(address)
			if err != nil {
				atomic.AddInt64(&pm.rejectedConns, 1)
				return nil, fmt.Errorf("SSRF protection: %w", err)
			}
			address = validatedAddr
		}

		conn, err := dialer.DialContext(ctx, network, address)
		connTime := time.Since(startTime).Nanoseconds()
		stats := pm.updateConnectionMetrics(address, connTime, err == nil)

		if err != nil {
			atomic.AddInt64(&pm.rejectedConns, 1)
			return nil, fmt.Errorf("connection failed: %w", err)
		}

		atomic.AddInt64(&pm.totalConns, 1)
		atomic.AddInt64(&pm.activeConns, 1)

		return &trackedConn{
			Conn:  conn,
			pm:   pm,
			host: address,
			stats: stats,
		}, nil
	}
}

// resolveAndValidateAddress resolves the given address and validates all resulting IPs
// against SSRF protection rules. It returns a validated "ip:port" string that should be
// dialed directly to prevent DNS rebinding TOCTOU attacks.
//
// SECURITY: By resolving DNS once and dialing the validated IP directly (instead of
// the original hostname), we eliminate the window where an attacker-controlled DNS
// server could return a different (private) IP on the second resolution.
func (pm *PoolManager) resolveAndValidateAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		port = "443"
	}

	// If the address is already an IP, validate it directly
	if ip := net.ParseIP(host); ip != nil {
		if err := validation.ValidateIP(ip); err != nil {
			return "", err
		}
		return address, nil
	}

	// For domain names, resolve and validate all IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("DNS resolution failed for SSRF validation of %s: %w", host, err)
	}

	// Check all resolved IPs — if any point to a private/reserved address, block it
	for _, ip := range ips {
		if err := validation.ValidateIP(ip); err != nil {
			return "", fmt.Errorf("domain %s resolves to blocked address: %w", host, err)
		}
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("DNS resolution returned no addresses for %s", host)
	}

	// Return the first validated IP for direct dialing to prevent DNS rebinding
	return net.JoinHostPort(ips[0].String(), port), nil
}

func (pm *PoolManager) createTLSConfig() *tls.Config {
	// If a custom TLS config is provided, use it (but add cert pinning if configured)
	if pm.config.TLSConfig != nil {
		tlsConfig := pm.config.TLSConfig.Clone()
		// Add certificate pinning verification if configured
		if pm.config.certPinner != nil {
			tlsConfig.VerifyPeerCertificate = pm.createVerifyPeerCertificate(tlsConfig)
		}
		return tlsConfig
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

	// Add certificate pinning verification if configured
	if pm.config.certPinner != nil {
		tlsConfig.VerifyPeerCertificate = pm.createVerifyPeerCertificate(tlsConfig)
	}

	return tlsConfig
}

// createVerifyPeerCertificate creates a certificate verification function
// that combines standard verification with certificate pinning
func (pm *PoolManager) createVerifyPeerCertificate(tlsConfig *tls.Config) func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// First, run the pinner verification
		if err := pm.config.certPinner.VerifyPeerCertificate(rawCerts, verifiedChains); err != nil {
			return fmt.Errorf("certificate pinning failed: %w", err)
		}

		// If InsecureSkipVerify is true, we skip standard verification
		if tlsConfig.InsecureSkipVerify {
			return nil
		}

		// Otherwise, standard TLS verification is performed by Go's TLS implementation
		// This function only adds the pinning check on top of standard verification
		return nil
	}
}

type trackedConn struct {
	net.Conn
	pm        *PoolManager
	host      string
	stats     *hostStats // captured at creation for direct Close() updates
	closeOnce sync.Once
	closed    int32 // Atomic flag for fast double-close detection
}

func (tc *trackedConn) Close() error {
	// Fast path: check if already closed (atomic check before sync.Once overhead)
	if atomic.LoadInt32(&tc.closed) == 1 {
		return nil
	}

	var closeErr error
	tc.closeOnce.Do(func() {
		atomic.StoreInt32(&tc.closed, 1)
		atomic.AddInt64(&tc.pm.activeConns, -1)
		if tc.stats != nil {
			atomic.AddInt64(&tc.stats.ActiveConns, -1)
		}
		closeErr = tc.Conn.Close()
	})
	return closeErr
}

// updateConnectionMetrics efficiently updates per-host connection statistics.
// Returns the hostStats pointer so callers can capture it for trackedConn.
func (pm *PoolManager) updateConnectionMetrics(host string, connTime int64, success bool) *hostStats {
	// Use a pre-allocated stats pointer to avoid allocation in the hot path
	value, _ := pm.hostConns.LoadOrStore(host, &hostStats{
		Host:     host,
		LastUsed: time.Now().Unix(),
	})

	// Safe type assertion with defensive check
	stats, ok := value.(*hostStats)
	if !ok || stats == nil {
		return nil // Defensive: skip update if type assertion fails
	}

	if success {
		atomic.AddInt64(&stats.TotalConns, 1)
		atomic.AddInt64(&stats.ActiveConns, 1)

		// Use mutex for latency update to ensure consistency under high contention
		// This is acceptable since latency tracking is not on the critical path
		stats.mu.Lock()
		current := stats.AverageLatency
		if current == 0 {
			stats.AverageLatency = connTime
		} else {
			stats.AverageLatency = (current*9 + connTime) / 10
		}
		stats.mu.Unlock()
	} else {
		atomic.AddInt64(&stats.FailedConns, 1)
	}

	atomic.StoreInt64(&stats.LastUsed, time.Now().Unix())
	return stats
}

func (pm *PoolManager) GetTransport() *http.Transport {
	return pm.transport
}

func (pm *PoolManager) GetMetrics() metrics {
	total := atomic.LoadInt64(&pm.totalConns)
	rejected := atomic.LoadInt64(&pm.rejectedConns)
	hitRate := 0.0
	if total+rejected > 0 {
		hitRate = float64(total) / float64(total+rejected)
	}

	return metrics{
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

	// Close DoH resolver first to release its HTTP client resources
	if pm.dohResolver != nil {
		if err := pm.dohResolver.Close(); err != nil {
			// Log but don't fail - continue closing other resources
			// In production, this could be logged to a proper logger
		}
	}

	if pm.transport != nil {
		pm.transport.CloseIdleConnections()
	}
	return nil
}
