# Circuit Breaker Guide

This guide covers the circuit breaker pattern implementation in HTTPC for automatic fault protection and service resilience.

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
- [Configuration](#configuration)
- [Usage Examples](#usage-examples)
- [Monitoring](#monitoring)
- [Best Practices](#best-practices)

## Overview

The circuit breaker pattern prevents cascading failures by automatically detecting when a service is failing and temporarily blocking requests to that service. HTTPC includes a built-in circuit breaker that:

- ✅ **Automatic Detection** - Monitors request failures and response times
- ✅ **Configurable Thresholds** - Customizable failure rates and timeouts
- ✅ **Graceful Degradation** - Fails fast when service is down
- ✅ **Automatic Recovery** - Tests service health and reopens when recovered
- ✅ **Per-Host Isolation** - Independent circuit breakers for each host

## How It Works

### Circuit States

The circuit breaker has three states

#### CLOSED State (Normal Operation)
- All requests are allowed through
- Monitors failure rate and response times
- Transitions to OPEN when failure threshold is exceeded

#### OPEN State (Service Down)
- All requests are immediately rejected
- Returns circuit breaker error without making actual request
- Periodically allows test requests to check if service recovered

#### HALF-OPEN State (Testing Recovery)
- Limited number of requests allowed through
- If requests succeed, transitions back to CLOSED
- If requests fail, transitions back to OPEN

### Failure Detection

The circuit breaker considers these as failures:
- HTTP 5xx status codes (server errors)
- Network timeouts
- Connection failures
- DNS resolution failures

**Not considered failures:**
- HTTP 4xx status codes (client errors)
- Successful responses (2xx, 3xx)

## Configuration

### Default Configuration

```go
// Circuit breaker is enabled by default with sensible settings
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Circuit breaker works automatically
resp, err := client.Get("https://api.example.com/users")
if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
    // Service is temporarily unavailable
    return handleServiceDown()
}
```

### Custom Configuration

```go
config := httpc.DefaultConfig()

// Circuit breaker settings
config.CircuitBreaker = &httpc.CircuitBreakerConfig{
    // Failure threshold (percentage)
    FailureThreshold: 50.0, // Open circuit at 50% failure rate
    
    // Minimum requests before evaluating failure rate
    MinRequests: 10,
    
    // Time window for failure rate calculation
    TimeWindow: 60 * time.Second,
    
    // How long to keep circuit open before testing
    OpenTimeout: 30 * time.Second,
    
    // Maximum number of test requests in half-open state
    MaxTestRequests: 3,
    
    // Response time threshold (optional)
    SlowCallThreshold: 5 * time.Second,
    SlowCallRate: 80.0, // Open if 80% of calls are slow
}

client, err := httpc.New(config)
```

### Per-Host Configuration

```go
// Different settings for different services
config := httpc.DefaultConfig()

// Strict settings for critical service
config.CircuitBreaker.FailureThreshold = 30.0
config.CircuitBreaker.OpenTimeout = 60 * time.Second

client, err := httpc.New(config)

// The circuit breaker automatically isolates failures per host
resp1, _ := client.Get("https://critical-api.example.com/data")    // Strict settings
resp2, _ := client.Get("https://other-api.example.com/data")       // Same settings, different circuit
```

## Usage Examples

### Basic Usage with Fallback

```go
func fetchUserData(client httpc.Client, userID int) (*UserData, error) {
    url := fmt.Sprintf("https://api.example.com/users/%d", userID)
    
    resp, err := client.Get(url,
        httpc.WithTimeout(10*time.Second),
        httpc.WithMaxRetries(2),
    )
    
    if err != nil {
        // Check if circuit breaker is open
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("Service unavailable, using cached data for user %d", userID)
            return getCachedUserData(userID)
        }
        
        return nil, fmt.Errorf("failed to fetch user data: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("API error: %d", resp.StatusCode)
    }
    
    var userData UserData
    if err := resp.JSON(&userData); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    return &userData, nil
}

func getCachedUserData(userID int) (*UserData, error) {
    // Return cached or default data when service is down
    return &UserData{
        ID:   userID,
        Name: "Unknown User",
    }, nil
}
```

### Graceful Degradation Pattern

```go
type APIClient struct {
    client httpc.Client
    cache  Cache
}

func (c *APIClient) GetRecommendations(userID int) ([]Recommendation, error) {
    // Try to get fresh recommendations
    recommendations, err := c.fetchRecommendations(userID)
    if err != nil {
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("Recommendation service down, using fallback for user %d", userID)
            
            // Use cached recommendations
            if cached := c.cache.Get(fmt.Sprintf("recommendations:%d", userID)); cached != nil {
                return cached.([]Recommendation), nil
            }
            
            // Use default recommendations
            return c.getDefaultRecommendations(), nil
        }
        
        return nil, err
    }
    
    // Cache successful response
    c.cache.Set(fmt.Sprintf("recommendations:%d", userID), recommendations, 1*time.Hour)
    
    return recommendations, nil
}

func (c *APIClient) fetchRecommendations(userID int) ([]Recommendation, error) {
    resp, err := c.client.Get(fmt.Sprintf("https://recommendations.example.com/users/%d", userID),
        httpc.WithTimeout(5*time.Second),
    )
    
    if err != nil {
        return nil, err
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("recommendations API error: %d", resp.StatusCode)
    }
    
    var recommendations []Recommendation
    if err := resp.JSON(&recommendations); err != nil {
        return nil, err
    }
    
    return recommendations, nil
}

func (c *APIClient) getDefaultRecommendations() []Recommendation {
    return []Recommendation{
        {ID: 1, Title: "Popular Item 1"},
        {ID: 2, Title: "Popular Item 2"},
        {ID: 3, Title: "Popular Item 3"},
    }
}
```

### Health Check Pattern

```go
func (c *APIClient) HealthCheck() error {
    resp, err := c.client.Get("https://api.example.com/health",
        httpc.WithTimeout(5*time.Second),
    )
    
    if err != nil {
        if strings.Contains(err.Error(), "circuit breaker is open") {
            return fmt.Errorf("service circuit breaker is open")
        }
        return fmt.Errorf("health check failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return fmt.Errorf("service unhealthy: %d", resp.StatusCode)
    }
    
    return nil
}

// Use in monitoring
func monitorServices() {
    client, _ := httpc.New()
    defer client.Close()
    
    apiClient := &APIClient{client: client}
    
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        if err := apiClient.HealthCheck(); err != nil {
            log.Printf("Service health check failed: %v", err)
            // Alert monitoring system
        } else {
            log.Println("Service is healthy")
        }
    }
}
```

## Monitoring

### Circuit Breaker State Monitoring

```go
// Check circuit breaker state (conceptual - actual API may vary)
func monitorCircuitBreaker(client httpc.Client) {
    // This is a conceptual example - actual monitoring API may differ
    stats := client.GetCircuitBreakerStats("api.example.com")
    
    log.Printf("Circuit Breaker Stats for api.example.com:")
    log.Printf("  State: %s", stats.State)
    log.Printf("  Failure Rate: %.2f%%", stats.FailureRate)
    log.Printf("  Total Requests: %d", stats.TotalRequests)
    log.Printf("  Failed Requests: %d", stats.FailedRequests)
    log.Printf("  Last Failure: %v", stats.LastFailure)
}
```

### Logging Circuit Breaker Events

```go
func setupLogging() {
    // Log circuit breaker state changes
    log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func makeRequest(client httpc.Client, url string) {
    resp, err := client.Get(url)
    
    if err != nil {
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("[CIRCUIT BREAKER] Service %s is unavailable", url)
        } else {
            log.Printf("[ERROR] Request failed: %v", err)
        }
        return
    }
    
    log.Printf("[SUCCESS] Request to %s completed: %d", url, resp.StatusCode)
}
```

## Best Practices

### ✅ DO

1. **Implement Fallback Strategies**
   ```go
   if strings.Contains(err.Error(), "circuit breaker is open") {
       return useFallbackData()
   }
   ```

2. **Use Appropriate Thresholds**
   ```go
   // For critical services - more sensitive
   config.CircuitBreaker.FailureThreshold = 30.0
   
   // For non-critical services - less sensitive
   config.CircuitBreaker.FailureThreshold = 70.0
   ```

3. **Cache Successful Responses**
   ```go
   if resp.IsSuccess() {
       cache.Set(key, data, ttl)
   }
   ```

4. **Monitor Circuit Breaker State**
   ```go
   // Log circuit breaker events for monitoring
   if strings.Contains(err.Error(), "circuit breaker is open") {
       metrics.IncrementCounter("circuit_breaker_open")
   }
   ```

5. **Set Reasonable Timeouts**
   ```go
   // Don't set timeouts too high - it delays circuit breaker detection
   httpc.WithTimeout(10*time.Second) // Good
   ```

### ❌ DON'T

1. **Don't Ignore Circuit Breaker Errors**
   ```go
   // Bad - ignoring circuit breaker
   resp, _ := client.Get(url)
   
   // Good - handle circuit breaker
   resp, err := client.Get(url)
   if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
       return handleServiceDown()
   }
   ```

2. **Don't Set Thresholds Too Low**
   ```go
   // Bad - too sensitive
   config.CircuitBreaker.FailureThreshold = 5.0
   
   // Good - reasonable threshold
   config.CircuitBreaker.FailureThreshold = 50.0
   ```

3. **Don't Disable Circuit Breaker for External Services**
   ```go
   // Bad - no protection against external service failures
   config.CircuitBreaker = nil
   ```

4. **Don't Use Circuit Breaker for Client Errors**
   ```go
   // Circuit breaker doesn't trigger on 4xx errors (this is correct behavior)
   // Don't try to force it to trigger on client errors
   ```

## Configuration Reference

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `FailureThreshold` | `float64` | 50.0 | Failure rate percentage to open circuit |
| `MinRequests` | `int` | 10 | Minimum requests before evaluating failure rate |
| `TimeWindow` | `time.Duration` | 60s | Time window for failure rate calculation |
| `OpenTimeout` | `time.Duration` | 30s | How long to keep circuit open |
| `MaxTestRequests` | `int` | 3 | Max test requests in half-open state |
| `SlowCallThreshold` | `time.Duration` | 0 | Response time threshold (0 = disabled) |
| `SlowCallRate` | `float64` | 0 | Slow call rate to open circuit (0 = disabled) |

## Error Messages

| Error Message | Meaning | Action |
|---------------|---------|--------|
| `"circuit breaker is open"` | Service is down | Use fallback or cached data |
| `"circuit breaker: too many test requests"` | Half-open state overloaded | Wait and retry later |

## Related Documentation

- [Error Handling](error-handling.md) - Comprehensive error handling patterns
- [Configuration](configuration.md) - Client configuration options
- [Best Practices](best-practices.md) - Production usage patterns

---
