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
	"syscall"
	"time"
)

// PoolManager provides intelligent connection pool management with monitoring
type PoolManager struct {
	config *Config

	transport *http.Transport

	activeConns   int64
	idleConns     int64
	totalConns    int64
	rejectedConns int64

	hostConns sync.Map

	metrics *Metrics

	closed int32
	done   chan struct{}
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

	AllowPrivateIPs bool

	DisableCompression bool
	DisableKeepAlives  bool
	ForceAttemptHTTP2  bool

	CookieJar interface{}
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
	IdleConnections     int64
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
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	pm.transport = transport
	pm.done = make(chan struct{})

	return pm, nil
}

// createDialer creates an optimized dialer with SSRF protection and connection tracking.
func (pm *PoolManager) createDialer() func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   pm.config.DialTimeout,
		KeepAlive: pm.config.KeepAlive,
		Control:   pm.createControlFunc(),
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		startTime := time.Now()

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

// createControlFunc creates a control function for SSRF validation before connection.
func (pm *PoolManager) createControlFunc() func(network, address string, c syscall.RawConn) error {
	if pm.config.AllowPrivateIPs {
		return nil
	}

	return func(network, address string, c syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}

		if ip := net.ParseIP(host); ip != nil {
			if err := pm.validateResolvedIP(ip); err != nil {
				return fmt.Errorf("SSRF protection blocked connection: %w", err)
			}
		}
		return nil
	}
}

// validateResolvedIP performs IP validation to prevent SSRF attacks.
func (pm *PoolManager) validateResolvedIP(ip net.IP) error {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("blocked IP: %s", ip.String())
	}

	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] >= 240 || ip4[0] == 0 || (ip4[0] == 100 && (ip4[1]&0xC0) == 64) ||
			(ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)) {
			return fmt.Errorf("reserved IP blocked: %s", ip.String())
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
		IdleConnections:     atomic.LoadInt64(&pm.idleConns),
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

	close(pm.done)

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
