# Monitoring and Observability Guide

This guide covers monitoring, metrics, logging, and observability features in HTTPC for production environments.

## Table of Contents

- [Overview](#overview)
- [Built-in Monitoring](#built-in-monitoring)
- [Metrics Collection](#metrics-collection)
- [Logging](#logging)
- [Health Checks](#health-checks)
- [Performance Monitoring](#performance-monitoring)
- [Alerting](#alerting)
- [Troubleshooting](#troubleshooting)

## Overview

HTTPC provides comprehensive monitoring capabilities to help you:

- ✅ **Track Performance** - Request latency, throughput, and success rates
- ✅ **Monitor Health** - Circuit breaker state, connection pool status
- ✅ **Debug Issues** - Detailed logging and error tracking
- ✅ **Capacity Planning** - Resource usage and scaling metrics
- ✅ **SLA Monitoring** - Availability and response time tracking

## Built-in Monitoring

### Response Metrics

Every response includes built-in metrics:

```go
resp, err := client.Get("https://api.example.com/users")
if err != nil {
    log.Printf("Request failed: %v", err)
    return
}

// Built-in metrics available in response
fmt.Printf("Duration: %v\n", resp.Duration)
fmt.Printf("Attempts: %d\n", resp.Attempts)
fmt.Printf("Status: %d\n", resp.StatusCode)
fmt.Printf("Size: %d bytes\n", len(resp.RawBody))
```

### Connection Pool Monitoring

```go
// Monitor connection pool health (conceptual API)
stats := client.GetConnectionStats()
fmt.Printf("Active connections: %d\n", stats.ActiveConnections)
fmt.Printf("Idle connections: %d\n", stats.IdleConnections)
fmt.Printf("Total requests: %d\n", stats.TotalRequests)
```

## Metrics Collection

### Prometheus Integration

```go
package monitoring

import (
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

type HTTPMetrics struct {
    requestsTotal *prometheus.CounterVec
    requestDuration *prometheus.HistogramVec
    requestSize *prometheus.HistogramVec
    responseSize *prometheus.HistogramVec
    circuitBreakerState *prometheus.GaugeVec
    connectionPoolSize *prometheus.GaugeVec
}

func NewHTTPMetrics() *HTTPMetrics {
    return &HTTPMetrics{
        requestsTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "httpc_requests_total",
                Help: "Total number of HTTP requests",
            },
            []string{"method", "host", "status_code"},
        ),
        requestDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "httpc_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 2.5, 5, 10},
            },
            []string{"method", "host"},
        ),
        requestSize: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "httpc_request_size_bytes",
                Help:    "HTTP request size in bytes",
                Buckets: prometheus.ExponentialBuckets(100, 10, 8),
            },
            []string{"method", "host"},
        ),
        responseSize: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "httpc_response_size_bytes",
                Help:    "HTTP response size in bytes",
                Buckets: prometheus.ExponentialBuckets(100, 10, 8),
            },
            []string{"method", "host", "status_code"},
        ),
        circuitBreakerState: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "httpc_circuit_breaker_state",
                Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
            },
            []string{"host"},
        ),
        connectionPoolSize: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "httpc_connection_pool_size",
                Help: "Connection pool size",
            },
            []string{"host", "state"}, // state: active, idle
        ),
    }
}

func (m *HTTPMetrics) RecordRequest(method, host string, duration time.Duration, statusCode int, requestSize, responseSize int64) {
    statusStr := fmt.Sprintf("%d", statusCode)
    
    m.requestsTotal.WithLabelValues(method, host, statusStr).Inc()
    m.requestDuration.WithLabelValues(method, host).Observe(duration.Seconds())
    m.requestSize.WithLabelValues(method, host).Observe(float64(requestSize))
    m.responseSize.WithLabelValues(method, host, statusStr).Observe(float64(responseSize))
}

func (m *HTTPMetrics) UpdateCircuitBreakerState(host string, state int) {
    m.circuitBreakerState.WithLabelValues(host).Set(float64(state))
}

func (m *HTTPMetrics) UpdateConnectionPool(host string, active, idle int) {
    m.connectionPoolSize.WithLabelValues(host, "active").Set(float64(active))
    m.connectionPoolSize.WithLabelValues(host, "idle").Set(float64(idle))
}
```

### Instrumented Client Wrapper

```go
type InstrumentedClient struct {
    client  httpc.Client
    metrics *HTTPMetrics
}

func NewInstrumentedClient(client httpc.Client, metrics *HTTPMetrics) *InstrumentedClient {
    return &InstrumentedClient{
        client:  client,
        metrics: metrics,
    }
}

func (c *InstrumentedClient) Get(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    return c.instrumentRequest("GET", url, func() (*httpc.Response, error) {
        return c.client.Get(url, opts...)
    })
}

func (c *InstrumentedClient) Post(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    return c.instrumentRequest("POST", url, func() (*httpc.Response, error) {
        return c.client.Post(url, opts...)
    })
}

func (c *InstrumentedClient) instrumentRequest(method, url string, fn func() (*httpc.Response, error)) (*httpc.Response, error) {
    start := time.Now()
    
    // Extract host from URL
    parsedURL, _ := neturl.Parse(url)
    host := parsedURL.Host
    
    resp, err := fn()
    
    duration := time.Since(start)
    statusCode := 0
    responseSize := int64(0)
    
    if resp != nil {
        statusCode = resp.StatusCode
        responseSize = int64(len(resp.RawBody))
    }
    
    // Record metrics
    c.metrics.RecordRequest(method, host, duration, statusCode, 0, responseSize)
    
    // Handle circuit breaker errors
    if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
        c.metrics.UpdateCircuitBreakerState(host, 1) // open
    }
    
    return resp, err
}
```

### Custom Metrics

```go
type CustomMetrics struct {
    apiCallsTotal     prometheus.Counter
    authFailures      prometheus.Counter
    rateLimitHits     prometheus.Counter
    cacheHits         prometheus.Counter
    cacheMisses       prometheus.Counter
}

func NewCustomMetrics() *CustomMetrics {
    return &CustomMetrics{
        apiCallsTotal: promauto.NewCounter(prometheus.CounterOpts{
            Name: "api_calls_total",
            Help: "Total API calls made",
        }),
        authFailures: promauto.NewCounter(prometheus.CounterOpts{
            Name: "auth_failures_total",
            Help: "Total authentication failures",
        }),
        rateLimitHits: promauto.NewCounter(prometheus.CounterOpts{
            Name: "rate_limit_hits_total",
            Help: "Total rate limit hits",
        }),
        cacheHits: promauto.NewCounter(prometheus.CounterOpts{
            Name: "cache_hits_total",
            Help: "Total cache hits",
        }),
        cacheMisses: promauto.NewCounter(prometheus.CounterOpts{
            Name: "cache_misses_total",
            Help: "Total cache misses",
        }),
    }
}

func (m *CustomMetrics) RecordAPICall() {
    m.apiCallsTotal.Inc()
}

func (m *CustomMetrics) RecordAuthFailure() {
    m.authFailures.Inc()
}

func (m *CustomMetrics) RecordRateLimitHit() {
    m.rateLimitHits.Inc()
}

func (m *CustomMetrics) RecordCacheHit() {
    m.cacheHits.Inc()
}

func (m *CustomMetrics) RecordCacheMiss() {
    m.cacheMisses.Inc()
}
```

## Logging

### Structured Logging

```go
package logging

import (
    "context"
    "log/slog"
    "time"
    "net/url"
)

type HTTPLogger struct {
    logger *slog.Logger
}

func NewHTTPLogger() *HTTPLogger {
    return &HTTPLogger{
        logger: slog.Default(),
    }
}

func (l *HTTPLogger) LogRequest(ctx context.Context, method, url string, opts ...httpc.RequestOption) {
    parsedURL, _ := neturl.Parse(url)
    
    l.logger.InfoContext(ctx, "HTTP request started",
        slog.String("method", method),
        slog.String("url", url),
        slog.String("host", parsedURL.Host),
        slog.String("path", parsedURL.Path),
    )
}

func (l *HTTPLogger) LogResponse(ctx context.Context, method, url string, resp *httpc.Response, err error, duration time.Duration) {
    parsedURL, _ := neturl.Parse(url)
    
    attrs := []slog.Attr{
        slog.String("method", method),
        slog.String("url", url),
        slog.String("host", parsedURL.Host),
        slog.Duration("duration", duration),
    }
    
    if resp != nil {
        attrs = append(attrs,
            slog.Int("status_code", resp.StatusCode),
            slog.String("status", resp.Status),
            slog.Int("response_size", len(resp.RawBody)),
            slog.Int("attempts", resp.Attempts),
        )
    }
    
    if err != nil {
        attrs = append(attrs, slog.String("error", err.Error()))
        
        // Classify error type
        if strings.Contains(err.Error(), "circuit breaker is open") {
            attrs = append(attrs, slog.String("error_type", "circuit_breaker"))
        } else if strings.Contains(err.Error(), "timeout") {
            attrs = append(attrs, slog.String("error_type", "timeout"))
        } else {
            attrs = append(attrs, slog.String("error_type", "network"))
        }
        
        l.logger.ErrorContext(ctx, "HTTP request failed", attrs...)
    } else {
        l.logger.InfoContext(ctx, "HTTP request completed", attrs...)
    }
}

func (l *HTTPLogger) LogCircuitBreakerEvent(ctx context.Context, host string, state string) {
    l.logger.WarnContext(ctx, "Circuit breaker state changed",
        slog.String("host", host),
        slog.String("state", state),
    )
}
```

### Request/Response Logging Middleware

```go
type LoggingClient struct {
    client httpc.Client
    logger *HTTPLogger
}

func NewLoggingClient(client httpc.Client, logger *HTTPLogger) *LoggingClient {
    return &LoggingClient{
        client: client,
        logger: logger,
    }
}

func (c *LoggingClient) Get(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    return c.loggedRequest("GET", url, func() (*httpc.Response, error) {
        return c.client.Get(url, opts...)
    })
}

func (c *LoggingClient) loggedRequest(method, url string, fn func() (*httpc.Response, error)) (*httpc.Response, error) {
    ctx := context.Background()
    start := time.Now()
    
    c.logger.LogRequest(ctx, method, url)
    
    resp, err := fn()
    
    duration := time.Since(start)
    c.logger.LogResponse(ctx, method, url, resp, err, duration)
    
    return resp, err
}
```

## Health Checks

### Service Health Monitoring

```go
type HealthChecker struct {
    client  httpc.Client
    metrics *HTTPMetrics
    logger  *HTTPLogger
}

func NewHealthChecker(client httpc.Client, metrics *HTTPMetrics, logger *HTTPLogger) *HealthChecker {
    return &HealthChecker{
        client:  client,
        metrics: metrics,
        logger:  logger,
    }
}

func (h *HealthChecker) CheckEndpoint(ctx context.Context, name, url string) *HealthStatus {
    start := time.Now()
    
    resp, err := h.client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithTimeout(5*time.Second),
    )
    
    duration := time.Since(start)
    
    status := &HealthStatus{
        Name:      name,
        URL:       url,
        Timestamp: time.Now(),
        Duration:  duration,
    }
    
    if err != nil {
        status.Status = "unhealthy"
        status.Error = err.Error()
        
        if strings.Contains(err.Error(), "circuit breaker is open") {
            status.Reason = "circuit_breaker_open"
        } else if strings.Contains(err.Error(), "timeout") {
            status.Reason = "timeout"
        } else {
            status.Reason = "network_error"
        }
    } else if !resp.IsSuccess() {
        status.Status = "unhealthy"
        status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
        status.Reason = "http_error"
    } else {
        status.Status = "healthy"
    }
    
    return status
}

type HealthStatus struct {
    Name      string        `json:"name"`
    URL       string        `json:"url"`
    Status    string        `json:"status"` // healthy, unhealthy
    Reason    string        `json:"reason,omitempty"`
    Error     string        `json:"error,omitempty"`
    Duration  time.Duration `json:"duration"`
    Timestamp time.Time     `json:"timestamp"`
}

func (h *HealthChecker) StartPeriodicChecks(ctx context.Context, endpoints map[string]string, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for name, url := range endpoints {
                go func(n, u string) {
                    status := h.CheckEndpoint(ctx, n, u)
                    h.handleHealthStatus(status)
                }(name, url)
            }
        }
    }
}

func (h *HealthChecker) handleHealthStatus(status *HealthStatus) {
    if status.Status == "unhealthy" {
        h.logger.logger.Error("Health check failed",
            slog.String("service", status.Name),
            slog.String("url", status.URL),
            slog.String("reason", status.Reason),
            slog.String("error", status.Error),
            slog.Duration("duration", status.Duration),
        )
        
        // Update metrics
        // h.metrics.RecordHealthCheckFailure(status.Name, status.Reason)
    } else {
        h.logger.logger.Info("Health check passed",
            slog.String("service", status.Name),
            slog.Duration("duration", status.Duration),
        )
    }
}
```

## Performance Monitoring

### Latency Tracking

```go
type LatencyTracker struct {
    percentiles map[string]*Percentile
    mutex       sync.RWMutex
}

type Percentile struct {
    values []float64
    sorted bool
}

func NewLatencyTracker() *LatencyTracker {
    return &LatencyTracker{
        percentiles: make(map[string]*Percentile),
    }
}

func (t *LatencyTracker) RecordLatency(endpoint string, duration time.Duration) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    if _, exists := t.percentiles[endpoint]; !exists {
        t.percentiles[endpoint] = &Percentile{
            values: make([]float64, 0, 1000),
        }
    }
    
    p := t.percentiles[endpoint]
    p.values = append(p.values, duration.Seconds())
    p.sorted = false
    
    // Keep only last 1000 values
    if len(p.values) > 1000 {
        p.values = p.values[len(p.values)-1000:]
    }
}

func (t *LatencyTracker) GetPercentile(endpoint string, percentile float64) time.Duration {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    p, exists := t.percentiles[endpoint]
    if !exists || len(p.values) == 0 {
        return 0
    }
    
    if !p.sorted {
        sort.Float64s(p.values)
        p.sorted = true
    }
    
    index := int(float64(len(p.values)) * percentile / 100.0)
    if index >= len(p.values) {
        index = len(p.values) - 1
    }
    
    return time.Duration(p.values[index] * float64(time.Second))
}

func (t *LatencyTracker) GetStats(endpoint string) map[string]time.Duration {
    return map[string]time.Duration{
        "p50":  t.GetPercentile(endpoint, 50),
        "p90":  t.GetPercentile(endpoint, 90),
        "p95":  t.GetPercentile(endpoint, 95),
        "p99":  t.GetPercentile(endpoint, 99),
        "p999": t.GetPercentile(endpoint, 99.9),
    }
}
```

### Throughput Monitoring

```go
type ThroughputMonitor struct {
    requests map[string]*RequestCounter
    mutex    sync.RWMutex
}

type RequestCounter struct {
    count     int64
    timestamp time.Time
}

func NewThroughputMonitor() *ThroughputMonitor {
    return &ThroughputMonitor{
        requests: make(map[string]*RequestCounter),
    }
}

func (m *ThroughputMonitor) RecordRequest(endpoint string) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    now := time.Now()
    key := fmt.Sprintf("%s:%d", endpoint, now.Unix()/60) // per minute
    
    if counter, exists := m.requests[key]; exists {
        atomic.AddInt64(&counter.count, 1)
    } else {
        m.requests[key] = &RequestCounter{
            count:     1,
            timestamp: now,
        }
    }
    
    // Clean old entries
    m.cleanOldEntries(now)
}

func (m *ThroughputMonitor) cleanOldEntries(now time.Time) {
    cutoff := now.Add(-10 * time.Minute)
    
    for key, counter := range m.requests {
        if counter.timestamp.Before(cutoff) {
            delete(m.requests, key)
        }
    }
}

func (m *ThroughputMonitor) GetRPS(endpoint string) float64 {
    m.mutex.RLock()
    defer m.mutex.RUnlock()
    
    now := time.Now()
    total := int64(0)
    
    for i := 0; i < 60; i++ {
        key := fmt.Sprintf("%s:%d", endpoint, (now.Unix()-int64(i))/60)
        if counter, exists := m.requests[key]; exists {
            total += atomic.LoadInt64(&counter.count)
        }
    }
    
    return float64(total) / 60.0
}
```

## Alerting

### Alert Rules

```go
type AlertRule struct {
    Name        string
    Condition   func(*Metrics) bool
    Severity    string
    Description string
}

type AlertManager struct {
    rules   []AlertRule
    metrics *HTTPMetrics
    alerts  chan Alert
}

type Alert struct {
    Rule        AlertRule
    Timestamp   time.Time
    Value       float64
    Description string
}

func NewAlertManager(metrics *HTTPMetrics) *AlertManager {
    am := &AlertManager{
        metrics: metrics,
        alerts:  make(chan Alert, 100),
    }
    
    // Define alert rules
    am.rules = []AlertRule{
        {
            Name:        "HighErrorRate",
            Condition:   am.checkErrorRate,
            Severity:    "critical",
            Description: "Error rate exceeds 5%",
        },
        {
            Name:        "HighLatency",
            Condition:   am.checkLatency,
            Severity:    "warning",
            Description: "95th percentile latency exceeds 2 seconds",
        },
        {
            Name:        "CircuitBreakerOpen",
            Condition:   am.checkCircuitBreaker,
            Severity:    "critical",
            Description: "Circuit breaker is open",
        },
    }
    
    return am
}

func (am *AlertManager) checkErrorRate(metrics *HTTPMetrics) bool {
    // Implementation would check actual metrics
    // This is a simplified example
    return false // Replace with actual error rate check
}

func (am *AlertManager) checkLatency(metrics *HTTPMetrics) bool {
    // Implementation would check actual latency metrics
    return false // Replace with actual latency check
}

func (am *AlertManager) checkCircuitBreaker(metrics *HTTPMetrics) bool {
    // Implementation would check circuit breaker state
    return false // Replace with actual circuit breaker check
}

func (am *AlertManager) StartMonitoring(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            am.evaluateRules()
        }
    }
}

func (am *AlertManager) evaluateRules() {
    for _, rule := range am.rules {
        if rule.Condition(am.metrics) {
            alert := Alert{
                Rule:        rule,
                Timestamp:   time.Now(),
                Description: rule.Description,
            }
            
            select {
            case am.alerts <- alert:
            default:
                // Alert channel is full, log error
                log.Printf("Alert channel full, dropping alert: %s", rule.Name)
            }
        }
    }
}

func (am *AlertManager) GetAlerts() <-chan Alert {
    return am.alerts
}
```

## Troubleshooting

### Debug Logging

```go
func enableDebugLogging(client httpc.Client) {
    // Enable debug logging for troubleshooting
    config := httpc.DefaultConfig()
    config.Debug = true
    config.LogLevel = "debug"
    
    // This would enable detailed request/response logging
}
```

### Connection Diagnostics

```go
func diagnoseConnections(client httpc.Client) {
    // Get connection pool statistics
    stats := client.GetConnectionStats()
    
    fmt.Printf("Connection Pool Diagnostics:\n")
    fmt.Printf("  Active connections: %d\n", stats.ActiveConnections)
    fmt.Printf("  Idle connections: %d\n", stats.IdleConnections)
    fmt.Printf("  Total connections created: %d\n", stats.TotalConnectionsCreated)
    fmt.Printf("  Connection reuse rate: %.2f%%\n", stats.ReuseRate*100)
    
    if stats.ActiveConnections > stats.MaxConnections*0.8 {
        fmt.Println("WARNING: Connection pool utilization is high")
    }
}
```

### Performance Profiling

```go
import _ "net/http/pprof"

func startProfiling() {
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    
    // Access profiling at http://localhost:6060/debug/pprof/
}
```

## Dashboard Configuration

### Grafana Dashboard JSON

```json
{
  "dashboard": {
    "title": "HTTPC Monitoring",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(httpc_requests_total[5m])",
            "legendFormat": "{{method}} {{host}}"
          }
        ]
      },
      {
        "title": "Request Duration",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(httpc_request_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          }
        ]
      },
      {
        "title": "Error Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(httpc_requests_total{status_code=~\"5..\"}[5m]) / rate(httpc_requests_total[5m])",
            "legendFormat": "Error rate"
          }
        ]
      },
      {
        "title": "Circuit Breaker State",
        "type": "stat",
        "targets": [
          {
            "expr": "httpc_circuit_breaker_state",
            "legendFormat": "{{host}}"
          }
        ]
      }
    ]
  }
}
```

## Related Documentation

- [Configuration](configuration.md) - Client configuration options
- [Error Handling](error-handling.md) - Error handling patterns
- [Circuit Breaker](circuit-breaker.md) - Circuit breaker monitoring
- [Best Practices](best-practices.md) - Production monitoring patterns

---