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
)

// PoolManager provides intelligent connection pool management with monitoring
type PoolManager struct {
	// Configuration
	config *Config

	// Transport with optimized settings
	transport *http.Transport

	// Connection tracking
	activeConns   int64
	idleConns     int64
	totalConns    int64
	rejectedConns int64

	// Per-host connection tracking
	hostConns sync.Map // map[string]*HostStats

	// Metrics and monitoring
	metrics *Metrics

	// Lifecycle management
	closed int32
	done   chan struct{} // Channel to signal shutdown
	mu     sync.RWMutex
}

// Config defines connection pool configuration
type Config struct {
	// Connection limits
	MaxIdleConns        int // Total idle connections across all hosts
	MaxIdleConnsPerHost int // Idle connections per host
	MaxConnsPerHost     int // Total connections per host
	MaxTotalConns       int // Global connection limit

	// Timeouts
	DialTimeout           time.Duration // Connection establishment timeout
	KeepAlive             time.Duration // TCP keep-alive interval
	TLSHandshakeTimeout   time.Duration // TLS handshake timeout
	ResponseHeaderTimeout time.Duration // Response header read timeout
	IdleConnTimeout       time.Duration // Idle connection timeout
	ExpectContinueTimeout time.Duration // Expect: 100-continue timeout

	// TLS configuration
	TLSConfig          *tls.Config
	MinTLSVersion      uint16
	MaxTLSVersion      uint16
	InsecureSkipVerify bool

	// HTTP/2 settings
	EnableHTTP2     bool
	HTTP2MaxStreams int

	// Proxy settings
	ProxyURL string

	// Advanced settings
	DisableCompression bool
	DisableKeepAlives  bool
	ForceAttemptHTTP2  bool

	// Cookie settings
	CookieJar interface{} // http.CookieJar

	// Monitoring
	EnableMetrics   bool
	MetricsInterval time.Duration
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
	// Connection counts
	ActiveConnections   int64
	IdleConnections     int64
	TotalConnections    int64
	RejectedConnections int64

	// Per-host metrics
	HostCount           int64
	AverageConnsPerHost float64
	MaxConnsPerHost     int64

	// Performance metrics
	AverageConnTime   int64 // Nanoseconds
	MaxConnTime       int64 // Nanoseconds
	ConnectionHitRate float64

	// Health metrics
	HealthyHosts   int64
	UnhealthyHosts int64

	// Timestamps
	LastUpdate int64
}

// DefaultConfig returns optimized default configuration
func DefaultConfig() *Config {
	return &Config{
		// Optimized connection limits for high concurrency
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     50,   // Increased for better parallelism
		MaxTotalConns:       1000, // Global limit to prevent resource exhaustion

		// Optimized timeouts
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// Secure TLS defaults
		MinTLSVersion:      tls.VersionTLS12,
		MaxTLSVersion:      tls.VersionTLS13,
		InsecureSkipVerify: false,

		// HTTP/2 optimization
		EnableHTTP2:     true,
		HTTP2MaxStreams: 100,

		// Performance optimizations
		DisableCompression: false,
		DisableKeepAlives:  false,
		ForceAttemptHTTP2:  true,

		// Monitoring
		EnableMetrics:   true,
		MetricsInterval: 30 * time.Second,
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
		DisableCompression:    config.DisableCompression,
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

	if config.EnableMetrics {
		go pm.metricsLoop()
	}

	return pm, nil
}

func (pm *PoolManager) createDialer() func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   pm.config.DialTimeout,
		KeepAlive: pm.config.KeepAlive,
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		startTime := time.Now()

		conn, err := dialer.DialContext(ctx, network, address)

		connTime := time.Since(startTime).Nanoseconds()
		pm.updateConnectionMetrics(address, connTime, err == nil)

		if err != nil {
			atomic.AddInt64(&pm.rejectedConns, 1)
			return nil, err
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
	// Update connection metrics
	atomic.AddInt64(&tc.pm.activeConns, -1)

	// Update host-specific metrics
	if value, ok := tc.pm.hostConns.Load(tc.host); ok {
		stats := value.(*HostStats)
		atomic.AddInt64(&stats.ActiveConns, -1)
	}

	return tc.Conn.Close()
}

func (pm *PoolManager) updateConnectionMetrics(host string, connTime int64, success bool) {
	value, _ := pm.hostConns.LoadOrStore(host, &HostStats{
		Host:     host,
		LastUsed: time.Now().Unix(),
	})

	stats := value.(*HostStats)

	if success {
		atomic.AddInt64(&stats.TotalConns, 1)
		atomic.AddInt64(&stats.ActiveConns, 1)

		for {
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
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.transport
}

func (pm *PoolManager) metricsLoop() {
	ticker := time.NewTicker(pm.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if atomic.LoadInt32(&pm.closed) == 1 {
				return
			}
			pm.updateMetrics()
		case <-pm.done:
			return
		}
	}
}

func (pm *PoolManager) updateMetrics() {
	atomic.StoreInt64(&pm.metrics.ActiveConnections, atomic.LoadInt64(&pm.activeConns))
	atomic.StoreInt64(&pm.metrics.IdleConnections, atomic.LoadInt64(&pm.idleConns))
	atomic.StoreInt64(&pm.metrics.TotalConnections, atomic.LoadInt64(&pm.totalConns))
	atomic.StoreInt64(&pm.metrics.RejectedConnections, atomic.LoadInt64(&pm.rejectedConns))

	var hostCount int64
	var totalConnsPerHost int64
	var maxConnsPerHost int64
	var healthyHosts int64

	pm.hostConns.Range(func(key, value interface{}) bool {
		stats := value.(*HostStats)
		hostCount++

		conns := atomic.LoadInt64(&stats.ActiveConns)
		totalConnsPerHost += conns

		if conns > maxConnsPerHost {
			maxConnsPerHost = conns
		}

		lastUsed := atomic.LoadInt64(&stats.LastUsed)
		totalConns := atomic.LoadInt64(&stats.TotalConns)
		failedConns := atomic.LoadInt64(&stats.FailedConns)

		if time.Now().Unix()-lastUsed < 300 {
			if totalConns == 0 || float64(failedConns)/float64(totalConns) < 0.1 {
				healthyHosts++
			}
		}

		return true
	})

	atomic.StoreInt64(&pm.metrics.HostCount, hostCount)
	atomic.StoreInt64(&pm.metrics.MaxConnsPerHost, maxConnsPerHost)
	atomic.StoreInt64(&pm.metrics.HealthyHosts, healthyHosts)
	atomic.StoreInt64(&pm.metrics.UnhealthyHosts, hostCount-healthyHosts)

	if hostCount > 0 {
		pm.metrics.AverageConnsPerHost = float64(totalConnsPerHost) / float64(hostCount)
	}

	total := atomic.LoadInt64(&pm.totalConns)
	rejected := atomic.LoadInt64(&pm.rejectedConns)
	if total+rejected > 0 {
		pm.metrics.ConnectionHitRate = float64(total) / float64(total+rejected)
	}

	atomic.StoreInt64(&pm.metrics.LastUpdate, time.Now().Unix())
}

func (pm *PoolManager) GetMetrics() Metrics {
	return Metrics{
		ActiveConnections:   atomic.LoadInt64(&pm.metrics.ActiveConnections),
		IdleConnections:     atomic.LoadInt64(&pm.metrics.IdleConnections),
		TotalConnections:    atomic.LoadInt64(&pm.metrics.TotalConnections),
		RejectedConnections: atomic.LoadInt64(&pm.metrics.RejectedConnections),
		HostCount:           atomic.LoadInt64(&pm.metrics.HostCount),
		AverageConnsPerHost: pm.metrics.AverageConnsPerHost,
		MaxConnsPerHost:     atomic.LoadInt64(&pm.metrics.MaxConnsPerHost),
		AverageConnTime:     atomic.LoadInt64(&pm.metrics.AverageConnTime),
		MaxConnTime:         atomic.LoadInt64(&pm.metrics.MaxConnTime),
		ConnectionHitRate:   pm.metrics.ConnectionHitRate,
		HealthyHosts:        atomic.LoadInt64(&pm.metrics.HealthyHosts),
		UnhealthyHosts:      atomic.LoadInt64(&pm.metrics.UnhealthyHosts),
		LastUpdate:          atomic.LoadInt64(&pm.metrics.LastUpdate),
	}
}

func (pm *PoolManager) Close() error {
	if !atomic.CompareAndSwapInt32(&pm.closed, 0, 1) {
		return nil
	}

	// Signal metrics goroutine to stop
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

	if metrics.ConnectionHitRate < 0.9 {
		return false
	}

	if metrics.HostCount > 0 && metrics.HealthyHosts == 0 {
		return false
	}

	if metrics.ActiveConnections >= int64(pm.config.MaxTotalConns)*9/10 {
		return false
	}

	return true
}
