# Best Practices

This guide covers recommended patterns and best practices for using HTTPC effectively.

## Table of Contents

- [Client Management](#client-management)
- [Error Handling](#error-handling)
- [Performance Optimization](#performance-optimization)
- [Security](#security)
- [Timeouts and Retries](#timeouts-and-retries)
- [Testing](#testing)
- [Deployment Patterns](#deployment-patterns)

## Client Management

### ✅ Reuse Client Instances

**DO:**
```go
// Create once, use many times
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Reuse for multiple requests
resp1, _ := client.Get(url1)
resp2, _ := client.Get(url2)
resp3, _ := client.Post(url3, httpc.WithJSON(data))
```

**DON'T:**
```go
// Bad - creates new client for each request
func fetchData(url string) (*Response, error) {
    client, _ := httpc.New()
    defer client.Close()
    return client.Get(url)
}
```

**Why?** Creating a new client for each request:
- Wastes resources (connection pools, goroutines)
- Prevents connection reuse
- Increases latency
- Increases memory usage

### ✅ Always Close Clients

**DO:**
```go
client, err := httpc.New()
if err != nil {
    return err
}
defer client.Close()  // Always close!
```

**Why?** Closing releases:
- Connection pools
- Background goroutines
- Memory buffers
- File descriptors

### ✅ Use Package-Level Functions Sparingly

**DO (Recommended):**
```go
type APIClient struct {
    client httpc.Client
}

func NewAPIClient() (*APIClient, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    return &APIClient{client: client}, nil
}
```

**DON'T (Not Recommended):**
```go
// Not recommended - uses shared default client
func fetchData(url string) ([]byte, error) {
    resp, err := httpc.Get(url)  // Package-level function
    if err != nil {
        return nil, err
    }
    return resp.RawBody, nil
}
```

**When to use package-level functions:**
- Quick scripts
- One-off requests
- Testing
- Prototyping

## Error Handling

### ✅ Always Check Errors

**DO:**
```go
resp, err := client.Get(url)
if err != nil {
    return fmt.Errorf("request failed: %w", err)
}

if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

**DON'T:**
```go
// Bad - ignoring errors
resp, _ := client.Get(url)
var data MyStruct
resp.JSON(&data)  // Could fail!
```

### ✅ Wrap Errors with Context

**DO:**
```go
if err != nil {
    return fmt.Errorf("failed to fetch user %d: %w", userID, err)
}
```

**DON'T:**
```go
// Bad - loses context
if err != nil {
    return err
}
```

### ✅ Handle Circuit Breaker Errors

**DO:**
```go
resp, err := client.Get(url)
if err != nil {
    if strings.Contains(err.Error(), "circuit breaker is open") {
        log.Printf("Service unavailable, using cache")
        return getCachedData(), nil
    }
    return nil, err
}
```

## Performance Optimization

### ✅ Configure Connection Pooling

**DO:**
```go
config := httpc.DefaultConfig()
config.MaxIdleConns = 100
config.MaxIdleConnsPerHost = 20
config.MaxConnsPerHost = 50

client, err := httpc.New(config)
```

**Guidelines:**
- **Low traffic** (< 100 req/s): Default settings
- **Medium traffic** (100-1000 req/s): Increase to 50-100 per host
- **High traffic** (> 1000 req/s): Increase to 100-200 per host

### ✅ Use HTTP/2

**DO:**
```go
// HTTP/2 is enabled by default
client, err := httpc.New()
```

**Benefits:**
- Multiplexing (multiple requests over one connection)
- Header compression
- Server push support
- Better performance

### ✅ Set Appropriate Timeouts

**DO:**
```go
// Different timeouts for different scenarios
config := httpc.DefaultConfig()

// Fast internal APIs
config.Timeout = 5 * time.Second

// External APIs
config.Timeout = 30 * time.Second

// Long-running operations
config.Timeout = 2 * time.Minute
```

**Guidelines:**
- **Internal APIs**: 5-10 seconds
- **External APIs**: 30-60 seconds
- **File downloads**: 5-30 minutes
- **Streaming**: No timeout or very long

### ✅ Use Context for Cancellation

**DO:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.Get(url,
    httpc.WithContext(ctx),
)
```

**Benefits:**
- Request cancellation
- Deadline enforcement
- Graceful shutdown
- Resource cleanup

## Security

### ✅ Use TLS 1.2+ in Production

**DO:**
```go
// Use balanced or strict preset
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelBalanced))
```

**DON'T:**
```go
// Bad - permissive mode in production
client, err := httpc.New(httpc.ConfigPreset(httpc.SecurityLevelPermissive))
```

### ✅ Validate Inputs

**DO:**
```go
func fetchUser(client httpc.Client, userID int) (*User, error) {
    if userID <= 0 {
        return nil, fmt.Errorf("invalid user ID: %d", userID)
    }
    
    url := fmt.Sprintf("https://api.example.com/users/%d", userID)
    // ... rest of code
}
```

### ✅ Never Skip TLS Verification in Production

**DO:**
```go
// Production - secure TLS
client, err := httpc.New()
```

**DON'T:**
```go
// Bad - insecure!
config := httpc.DefaultConfig()
config.InsecureSkipVerify = true  // Never in production!
client, err := httpc.New(config)
```

### ✅ Use Environment-Specific Configuration

**DO:**
```go
func newClient() (httpc.Client, error) {
    var config *httpc.Config
    
    switch os.Getenv("ENV") {
    case "production":
        config = httpc.ConfigPreset(httpc.SecurityLevelStrict)
    case "staging":
        config = httpc.ConfigPreset(httpc.SecurityLevelBalanced)
    case "development":
        config = httpc.ConfigPreset(httpc.SecurityLevelPermissive)
    default:
        config = httpc.DefaultConfig()
    }
    
    return httpc.New(config)
}
```

## Timeouts and Retries

### ✅ Set Appropriate Retry Counts

**DO:**
```go
// For idempotent operations (GET, PUT, DELETE)
resp, err := client.Get(url,
    httpc.WithMaxRetries(3),
)

// For non-idempotent operations (POST)
resp, err := client.Post(url,
    httpc.WithMaxRetries(0),  // No retries
    httpc.WithJSON(data),
)
```

**Guidelines:**
- **GET requests**: 2-3 retries
- **POST requests**: 0-1 retries (be careful!)
- **PUT/DELETE requests**: 1-2 retries
- **Idempotent operations**: More retries OK
- **Non-idempotent operations**: Fewer retries

### ✅ Use Exponential Backoff

**DO:**
```go
config := httpc.DefaultConfig()
config.MaxRetries = 3
config.RetryDelay = 1 * time.Second
config.MaxRetryDelay = 30 * time.Second
config.BackoffFactor = 2.0
config.Jitter = true  // Add randomness

client, err := httpc.New(config)
```

**Why?**
- Prevents thundering herd
- Gives service time to recover
- Reduces load on failing services

## Testing

### ✅ Use Test Clients

**DO:**
```go
func TestFetchUser(t *testing.T) {
    // Create test client with permissive settings
    config := httpc.ConfigPreset(httpc.SecurityLevelPermissive)
    config.Timeout = 5 * time.Second
    
    client, err := httpc.New(config)
    if err != nil {
        t.Fatal(err)
    }
    defer client.Close()
    
    // Test your code
    user, err := fetchUser(client, 123)
    if err != nil {
        t.Errorf("fetchUser failed: %v", err)
    }
}
```

### ✅ Mock External Services

**DO:**
```go
// Use httptest for mocking
func TestAPIClient(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"id": 1, "name": "Test"}`))
    }))
    defer server.Close()
    
    client, _ := httpc.New()
    defer client.Close()
    
    resp, err := client.Get(server.URL)
    // ... assertions
}
```

## Deployment Patterns

### Pattern 1: API Client Wrapper

```go
type APIClient struct {
    client  httpc.Client
    baseURL string
    token   string
}

func NewAPIClient(baseURL, token string) (*APIClient, error) {
    config := httpc.ConfigPreset(httpc.SecurityLevelBalanced)
    config.Timeout = 30 * time.Second
    
    client, err := httpc.New(config)
    if err != nil {
        return nil, err
    }
    
    return &APIClient{
        client:  client,
        baseURL: baseURL,
        token:   token,
    }, nil
}

func (c *APIClient) Close() error {
    return c.client.Close()
}

func (c *APIClient) GetUser(ctx context.Context, userID int) (*User, error) {
    url := fmt.Sprintf("%s/users/%d", c.baseURL, userID)
    
    resp, err := c.client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithBearerToken(c.token),
        httpc.WithTimeout(10*time.Second),
    )
    
    if err != nil {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("API error: %d", resp.StatusCode)
    }
    
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, fmt.Errorf("failed to parse user: %w", err)
    }
    
    return &user, nil
}
```

### Pattern 2: Retry with Fallback

```go
func fetchWithFallback(client httpc.Client, primaryURL, fallbackURL string) ([]byte, error) {
    // Try primary with retries
    resp, err := client.Get(primaryURL,
        httpc.WithMaxRetries(2),
        httpc.WithTimeout(10*time.Second),
    )
    
    if err == nil && resp.IsSuccess() {
        return resp.RawBody, nil
    }
    
    log.Printf("Primary failed: %v, trying fallback", err)
    
    // Try fallback
    resp, err = client.Get(fallbackURL,
        httpc.WithTimeout(10*time.Second),
    )
    
    if err != nil {
        return nil, fmt.Errorf("both endpoints failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("fallback status: %d", resp.StatusCode)
    }
    
    return resp.RawBody, nil
}
```

### Pattern 3: Concurrent Requests

```go
func fetchMultiple(client httpc.Client, urls []string) ([][]byte, error) {
    type result struct {
        data []byte
        err  error
    }
    
    results := make(chan result, len(urls))
    
    for _, url := range urls {
        go func(u string) {
            resp, err := client.Get(u)
            if err != nil {
                results <- result{err: err}
                return
            }
            
            if !resp.IsSuccess() {
                results <- result{err: fmt.Errorf("status %d", resp.StatusCode)}
                return
            }
            
            results <- result{data: resp.RawBody}
        }(url)
    }
    
    var data [][]byte
    for i := 0; i < len(urls); i++ {
        r := <-results
        if r.err != nil {
            return nil, r.err
        }
        data = append(data, r.data)
    }
    
    return data, nil
}
```

### Pattern 4: Rate Limiting

```go
type RateLimitedClient struct {
    client  httpc.Client
    limiter *rate.Limiter
}

func NewRateLimitedClient(requestsPerSecond int) (*RateLimitedClient, error) {
    client, err := httpc.New()
    if err != nil {
        return nil, err
    }
    
    return &RateLimitedClient{
        client:  client,
        limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond),
    }, nil
}

func (c *RateLimitedClient) Get(ctx context.Context, url string) (*httpc.Response, error) {
    if err := c.limiter.Wait(ctx); err != nil {
        return nil, err
    }
    
    return c.client.Get(url, httpc.WithContext(ctx))
}
```

## Summary Checklist

### Client Setup
- [ ] Reuse client instances
- [ ] Always close clients with `defer`
- [ ] Use appropriate security preset
- [ ] Configure connection pooling
- [ ] Set reasonable timeouts

### Error Handling
- [ ] Check all errors
- [ ] Check HTTP status codes
- [ ] Wrap errors with context
- [ ] Handle circuit breaker errors
- [ ] Implement fallback strategies

### Performance
- [ ] Enable HTTP/2 (default)
- [ ] Configure connection pools
- [ ] Use context for cancellation
- [ ] Set appropriate timeouts
- [ ] Implement retry logic

### Security
- [ ] Use TLS 1.2+ in production
- [ ] Never skip TLS verification
- [ ] Validate all inputs
- [ ] Use environment-specific configs
- [ ] Protect sensitive data

### Testing
- [ ] Test with mock servers
- [ ] Use permissive config for tests
- [ ] Test error scenarios
- [ ] Test timeout behavior
- [ ] Test retry logic

---
