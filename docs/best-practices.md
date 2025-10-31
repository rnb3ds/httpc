# Best Practices Guide

This guide covers production-ready patterns, performance optimization, and security best practices for HTTPC.

## Table of Contents

- [Client Management](#client-management)
- [Configuration](#configuration)
- [Error Handling](#error-handling)
- [Performance Optimization](#performance-optimization)
- [Security](#security)
- [Monitoring & Observability](#monitoring--observability)
- [Testing](#testing)
- [Production Deployment](#production-deployment)

## Client Management

### ✅ Client Lifecycle

**DO: Create and reuse client instances**
```go
type APIService struct {
    client httpc.Client
}

func NewAPIService() (*APIService, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    
    return &APIService{client: client}, nil
}

func (s *APIService) Close() error {
    return s.client.Close()
}

// Reuse the same client for multiple requests
func (s *APIService) GetUser(id int) (*User, error) {
    return s.fetchUser(id)
}

func (s *APIService) UpdateUser(user *User) error {
    return s.updateUser(user)
}
```

**DON'T: Create new clients for each request**
```go
// Bad - creates new client every time
func badGetUser(id int) (*User, error) {
    client, _ := httpc.New()
    defer client.Close()
    
    resp, err := client.Get(fmt.Sprintf("/users/%d", id))
    // ... handle response
    return user, nil
}
```

### ✅ Graceful Shutdown

```go
type Server struct {
    apiClient *APIService
}

func (s *Server) Shutdown(ctx context.Context) error {
    // Close HTTP client connections gracefully
    if err := s.apiClient.Close(); err != nil {
        log.Printf("Error closing API client: %v", err)
    }
    
    return nil
}

func main() {
    server := &Server{
        apiClient: NewAPIService(),
    }
    
    // Handle shutdown signals
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        server.Shutdown(ctx)
        os.Exit(0)
    }()
    
    // Start server...
}
```

## Configuration

### ✅ Environment-Based Configuration

```go
func createClient() (httpc.Client, error) {
    var config *httpc.Config
    
    switch os.Getenv("ENVIRONMENT") {
    case "production":
        config = httpc.SecureConfig()
        config.Timeout = 30 * time.Second
        config.MaxRetries = 2
        
    case "staging":
        config = httpc.DefaultConfig()
        config.Timeout = 60 * time.Second
        config.MaxRetries = 3
        
    case "development":
        config = httpc.TestingConfig()
        config.Timeout = 120 * time.Second
        config.MaxRetries = 5
        
    default:
        config = httpc.DefaultConfig()
    }
    
    // Common settings
    config.UserAgent = fmt.Sprintf("MyApp/%s", getVersion())
    config.MaxConcurrentRequests = getMaxConcurrency()
    
    return httpc.New(config)
}

func getMaxConcurrency() int {
    if concurrency := os.Getenv("MAX_CONCURRENCY"); concurrency != "" {
        if n, err := strconv.Atoi(concurrency); err == nil {
            return n
        }
    }
    return 500 // default
}
```

### ✅ Configuration Validation

```go
func validateConfig(config *httpc.Config) error {
    if config.Timeout < time.Second {
        return fmt.Errorf("timeout too short: %v", config.Timeout)
    }
    
    if config.MaxRetries > 10 {
        return fmt.Errorf("too many retries: %d", config.MaxRetries)
    }
    
    if config.MaxConcurrentRequests > 10000 {
        return fmt.Errorf("concurrency limit too high: %d", config.MaxConcurrentRequests)
    }
    
    return nil
}

func createValidatedClient(config *httpc.Config) (httpc.Client, error) {
    if err := validateConfig(config); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }
    
    return httpc.New(config)
}
```

## Error Handling

### ✅ Comprehensive Error Handling

```go
func handleAPIRequest(client httpc.Client, url string) (*APIResponse, error) {
    resp, err := client.Get(url,
        httpc.WithTimeout(30*time.Second),
        httpc.WithMaxRetries(2),
    )
    
    if err != nil {
        // Handle different error types
        return nil, classifyError(err)
    }
    
    // Handle HTTP status codes
    switch {
    case resp.IsSuccess():
        return parseSuccessResponse(resp)
        
    case resp.StatusCode == 429:
        return nil, &RateLimitError{
            RetryAfter: parseRetryAfter(resp.Headers.Get("Retry-After")),
        }
        
    case resp.IsClientError():
        return nil, &ClientError{
            StatusCode: resp.StatusCode,
            Message:    resp.Body,
        }
        
    case resp.IsServerError():
        return nil, &ServerError{
            StatusCode: resp.StatusCode,
            Message:    resp.Body,
        }
        
    default:
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
}

func classifyError(err error) error {
    var httpErr *httpc.HTTPError
    if errors.As(err, &httpErr) {
        return &APIError{
            Type:       "http_error",
            StatusCode: httpErr.StatusCode,
            Message:    httpErr.Status,
        }
    }
    
    if strings.Contains(err.Error(), "circuit breaker is open") {
        return &ServiceUnavailableError{
            Service: "api",
            Reason:  "circuit_breaker_open",
        }
    }
    
    if strings.Contains(err.Error(), "timeout") {
        return &TimeoutError{
            Operation: "api_request",
            Timeout:   30 * time.Second,
        }
    }
    
    return &NetworkError{
        Cause: err,
    }
}
```

### ✅ Custom Error Types

```go
type APIError struct {
    Type       string `json:"type"`
    StatusCode int    `json:"status_code,omitempty"`
    Message    string `json:"message"`
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error (%s): %s", e.Type, e.Message)
}

type RateLimitError struct {
    RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
    return fmt.Sprintf("rate limited, retry after %v", e.RetryAfter)
}

func (e *RateLimitError) ShouldRetry() bool {
    return true
}

func (e *RateLimitError) RetryDelay() time.Duration {
    return e.RetryAfter
}
```

## Performance Optimization

### ✅ Connection Pooling

```go
func createOptimizedClient() (httpc.Client, error) {
    config := httpc.DefaultConfig()
    
    // Optimize connection pooling
    config.MaxIdleConns = 100
    config.MaxIdleConnsPerHost = 20
    config.MaxConnsPerHost = 50
    config.IdleConnTimeout = 90 * time.Second
    
    // Keep-alive settings
    config.KeepAlive = 30 * time.Second
    config.TLSHandshakeTimeout = 10 * time.Second
    
    return httpc.New(config)
}
```

### ✅ Concurrent Requests

```go
func fetchMultipleUsers(client httpc.Client, userIDs []int) ([]*User, error) {
    type result struct {
        user *User
        err  error
    }
    
    results := make(chan result, len(userIDs))
    semaphore := make(chan struct{}, 10) // Limit concurrency
    
    var wg sync.WaitGroup
    
    for _, id := range userIDs {
        wg.Add(1)
        go func(userID int) {
            defer wg.Done()
            
            semaphore <- struct{}{} // Acquire
            defer func() { <-semaphore }() // Release
            
            user, err := fetchUser(client, userID)
            results <- result{user: user, err: err}
        }(id)
    }
    
    wg.Wait()
    close(results)
    
    var users []*User
    var errors []error
    
    for res := range results {
        if res.err != nil {
            errors = append(errors, res.err)
        } else {
            users = append(users, res.user)
        }
    }
    
    if len(errors) > 0 {
        return users, fmt.Errorf("some requests failed: %v", errors)
    }
    
    return users, nil
}
```

### ✅ Response Streaming

```go
func processLargeResponse(client httpc.Client, url string) error {
    resp, err := client.Get(url,
        httpc.WithTimeout(5*time.Minute),
    )
    if err != nil {
        return err
    }
    
    if !resp.IsSuccess() {
        return fmt.Errorf("request failed: %d", resp.StatusCode)
    }
    
    // Process response in chunks to avoid memory issues
    reader := bytes.NewReader(resp.RawBody)
    scanner := bufio.NewScanner(reader)
    
    for scanner.Scan() {
        line := scanner.Text()
        if err := processLine(line); err != nil {
            return fmt.Errorf("processing failed: %w", err)
        }
    }
    
    return scanner.Err()
}
```

## Security

### ✅ Secure Configuration

```go
func createSecureClient() (httpc.Client, error) {
    config := httpc.SecureConfig()
    
    // Additional security settings
    config.TLSConfig = &tls.Config{
        MinVersion: tls.VersionTLS12,
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
    }
    
    // Validate all inputs
    config.ValidateURL = true
    config.ValidateHeaders = true
    
    // Block private IPs for external APIs
    config.AllowPrivateIPs = false
    
    return httpc.New(config)
}
```

### ✅ Credential Management

```go
type SecureAPIClient struct {
    client httpc.Client
    token  string
}

func NewSecureAPIClient() (*SecureAPIClient, error) {
    // Get token from secure source
    token := os.Getenv("API_TOKEN")
    if token == "" {
        return nil, fmt.Errorf("API_TOKEN environment variable required")
    }
    
    client, err := createSecureClient()
    if err != nil {
        return nil, err
    }
    
    return &SecureAPIClient{
        client: client,
        token:  token,
    }, nil
}

func (c *SecureAPIClient) makeAuthenticatedRequest(method, url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    // Always add authentication
    allOpts := append(opts, httpc.WithBearerToken(c.token))
    
    switch method {
    case "GET":
        return c.client.Get(url, allOpts...)
    case "POST":
        return c.client.Post(url, allOpts...)
    default:
        return nil, fmt.Errorf("unsupported method: %s", method)
    }
}
```

### ✅ Input Validation

```go
func validateAndSanitizeInput(input string) (string, error) {
    // Remove potentially dangerous characters
    sanitized := strings.ReplaceAll(input, "\r", "")
    sanitized = strings.ReplaceAll(sanitized, "\n", "")
    
    // Validate length
    if len(sanitized) > 1000 {
        return "", fmt.Errorf("input too long")
    }
    
    // Validate format (example: alphanumeric only)
    if !regexp.MustCompile(`^[a-zA-Z0-9\s]+$`).MatchString(sanitized) {
        return "", fmt.Errorf("invalid characters in input")
    }
    
    return sanitized, nil
}

func safeAPICall(client httpc.Client, userInput string) (*httpc.Response, error) {
    validated, err := validateAndSanitizeInput(userInput)
    if err != nil {
        return nil, fmt.Errorf("input validation failed: %w", err)
    }
    
    return client.Get("https://api.example.com/search",
        httpc.WithQuery("q", validated),
        httpc.WithTimeout(10*time.Second),
    )
}
```

## Monitoring & Observability

### ✅ Structured Logging

```go
type RequestLogger struct {
    logger *slog.Logger
}

func (l *RequestLogger) LogRequest(method, url string, duration time.Duration, statusCode int, err error) {
    attrs := []slog.Attr{
        slog.String("method", method),
        slog.String("url", url),
        slog.Duration("duration", duration),
        slog.Int("status_code", statusCode),
    }
    
    if err != nil {
        attrs = append(attrs, slog.String("error", err.Error()))
        l.logger.LogAttrs(context.Background(), slog.LevelError, "Request failed", attrs...)
    } else {
        l.logger.LogAttrs(context.Background(), slog.LevelInfo, "Request completed", attrs...)
    }
}

func makeRequestWithLogging(client httpc.Client, logger *RequestLogger, url string) (*httpc.Response, error) {
    start := time.Now()
    
    resp, err := client.Get(url,
        httpc.WithTimeout(30*time.Second),
    )
    
    duration := time.Since(start)
    statusCode := 0
    if resp != nil {
        statusCode = resp.StatusCode
    }
    
    logger.LogRequest("GET", url, duration, statusCode, err)
    
    return resp, err
}
```

### ✅ Metrics Collection

```go
type Metrics struct {
    requestsTotal     *prometheus.CounterVec
    requestDuration   *prometheus.HistogramVec
    circuitBreakerOps *prometheus.CounterVec
}

func NewMetrics() *Metrics {
    return &Metrics{
        requestsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "http_requests_total",
                Help: "Total number of HTTP requests",
            },
            []string{"method", "status_code", "endpoint"},
        ),
        requestDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "http_request_duration_seconds",
                Help:    "HTTP request duration in seconds",
                Buckets: prometheus.DefBuckets,
            },
            []string{"method", "endpoint"},
        ),
        circuitBreakerOps: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Name: "circuit_breaker_operations_total",
                Help: "Circuit breaker operations",
            },
            []string{"state", "endpoint"},
        ),
    }
}

func (m *Metrics) RecordRequest(method, endpoint string, duration time.Duration, statusCode int) {
    m.requestsTotal.WithLabelValues(method, fmt.Sprintf("%d", statusCode), endpoint).Inc()
    m.requestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

func (m *Metrics) RecordCircuitBreakerOp(state, endpoint string) {
    m.circuitBreakerOps.WithLabelValues(state, endpoint).Inc()
}
```

### ✅ Health Checks

```go
type HealthChecker struct {
    client httpc.Client
}

func (h *HealthChecker) CheckHealth(ctx context.Context) error {
    resp, err := h.client.Get("https://api.example.com/health",
        httpc.WithContext(ctx),
        httpc.WithTimeout(5*time.Second),
    )
    
    if err != nil {
        if strings.Contains(err.Error(), "circuit breaker is open") {
            return fmt.Errorf("service circuit breaker is open")
        }
        return fmt.Errorf("health check request failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return fmt.Errorf("service unhealthy: status %d", resp.StatusCode)
    }
    
    return nil
}

func (h *HealthChecker) StartHealthChecks(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := h.CheckHealth(ctx); err != nil {
                log.Printf("Health check failed: %v", err)
                // Send alert to monitoring system
            }
        }
    }
}
```

## Testing

### ✅ Unit Testing with Mocks

```go
type MockClient struct {
    responses map[string]*httpc.Response
    errors    map[string]error
}

func (m *MockClient) Get(url string, opts ...httpc.RequestOption) (*httpc.Response, error) {
    if err, exists := m.errors[url]; exists {
        return nil, err
    }
    
    if resp, exists := m.responses[url]; exists {
        return resp, nil
    }
    
    return nil, fmt.Errorf("no mock response for %s", url)
}

func TestAPIClient_GetUser(t *testing.T) {
    mockClient := &MockClient{
        responses: map[string]*httpc.Response{
            "https://api.example.com/users/123": {
                StatusCode: 200,
                Body:       `{"id": 123, "name": "John Doe"}`,
            },
        },
    }
    
    client := &APIClient{client: mockClient}
    
    user, err := client.GetUser(123)
    assert.NoError(t, err)
    assert.Equal(t, 123, user.ID)
    assert.Equal(t, "John Doe", user.Name)
}
```

### ✅ Integration Testing

```go
func TestAPIIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    client, err := httpc.New(httpc.TestingConfig())
    require.NoError(t, err)
    defer client.Close()
    
    // Test against real API
    resp, err := client.Get("https://httpbin.org/json")
    require.NoError(t, err)
    assert.True(t, resp.IsSuccess())
    
    var data map[string]interface{}
    err = resp.JSON(&data)
    require.NoError(t, err)
    assert.NotEmpty(t, data)
}
```

## Production Deployment

### ✅ Configuration Management

```go
type Config struct {
    HTTPClient HTTPClientConfig `yaml:"http_client"`
    API        APIConfig        `yaml:"api"`
}

type HTTPClientConfig struct {
    Timeout         time.Duration `yaml:"timeout"`
    MaxRetries      int           `yaml:"max_retries"`
    MaxConcurrency  int           `yaml:"max_concurrency"`
    SecurityLevel   string        `yaml:"security_level"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var config Config
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, err
    }
    
    return &config, nil
}

func CreateClientFromConfig(cfg *HTTPClientConfig) (httpc.Client, error) {
    var preset *httpc.Config
    
    switch cfg.SecurityLevel {
    case "strict":
        preset = httpc.SecureConfig()
    case "balanced":
        preset = httpc.DefaultConfig()
    case "permissive":
        preset = httpc.TestingConfig()
    default:
        preset = httpc.DefaultConfig()
    }
    
    preset.Timeout = cfg.Timeout
    preset.MaxRetries = cfg.MaxRetries
    preset.MaxConcurrentRequests = cfg.MaxConcurrency
    
    return httpc.New(preset)
}
```

### ✅ Graceful Degradation

```go
type ResilientAPIClient struct {
    primary   httpc.Client
    fallback  httpc.Client
    cache     Cache
    metrics   *Metrics
}

func (c *ResilientAPIClient) GetData(ctx context.Context, key string) (*Data, error) {
    // Try primary service
    data, err := c.tryPrimary(ctx, key)
    if err == nil {
        c.cache.Set(key, data, 1*time.Hour)
        return data, nil
    }
    
    // Log primary failure
    c.metrics.RecordFailure("primary", err)
    
    // Try fallback service
    data, err = c.tryFallback(ctx, key)
    if err == nil {
        c.cache.Set(key, data, 30*time.Minute)
        return data, nil
    }
    
    // Log fallback failure
    c.metrics.RecordFailure("fallback", err)
    
    // Try cache
    if cached := c.cache.Get(key); cached != nil {
        c.metrics.RecordCacheHit(key)
        return cached.(*Data), nil
    }
    
    return nil, fmt.Errorf("all data sources failed: primary=%v, fallback=%v", err, err)
}
```

## Summary Checklist

### Client Management
- [ ] Reuse client instances
- [ ] Implement graceful shutdown
- [ ] Handle client lifecycle properly

### Configuration
- [ ] Use environment-based configuration
- [ ] Validate configuration
- [ ] Choose appropriate security presets

### Error Handling
- [ ] Handle all error types
- [ ] Implement custom error types
- [ ] Log errors appropriately

### Performance
- [ ] Optimize connection pooling
- [ ] Limit concurrent requests
- [ ] Handle large responses efficiently

### Security
- [ ] Use secure TLS configuration
- [ ] Manage credentials securely
- [ ] Validate and sanitize inputs

### Monitoring
- [ ] Implement structured logging
- [ ] Collect metrics
- [ ] Set up health checks

### Testing
- [ ] Write unit tests with mocks
- [ ] Include integration tests
- [ ] Test error scenarios

### Production
- [ ] Use configuration files
- [ ] Implement graceful degradation
- [ ] Monitor circuit breaker state

---