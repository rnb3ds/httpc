# Circuit Breaker

The circuit breaker pattern protects your application from cascading failures and improves resilience by automatically detecting and recovering from service failures.

## Table of Contents

- [Overview](#overview)
- [How It Works](#how-it-works)
- [Circuit States](#circuit-states)
- [Configuration](#configuration)
- [Usage Examples](#usage-examples)
- [Best Practices](#best-practices)

## Overview

### What is a Circuit Breaker?

A circuit breaker is a design pattern that prevents an application from repeatedly trying to execute an operation that's likely to fail. It acts like an electrical circuit breaker - when failures exceed a threshold, the circuit "opens" and requests fail fast without hitting the failing service.

### Why Use Circuit Breakers?

- **Prevent Cascading Failures** - Stop failures from spreading through your system
- **Fail Fast** - Don't waste time on requests that will likely fail
- **Automatic Recovery** - Test and restore connections automatically
- **Resource Protection** - Prevent overwhelming a struggling service
- **Improved User Experience** - Faster error responses instead of timeouts

### Key Features

- ✅ **Automatic per-host** - Circuit breakers are created automatically for each host
- ✅ **Zero configuration** - Works out of the box with sensible defaults
- ✅ **Transparent** - No code changes needed to enable
- ✅ **Isolated** - Failures on one host don't affect others
- ✅ **Self-healing** - Automatically tests and recovers connections

## How It Works

The circuit breaker monitors requests to each host and tracks success/failure rates:

1. **Normal Operation (Closed)**
   - All requests pass through
   - Failures are counted
   - If failure rate exceeds threshold → Open

2. **Failure Mode (Open)**
   - Requests fail immediately with circuit breaker error
   - No requests reach the failing service
   - After timeout period → Half-Open

3. **Testing Recovery (Half-Open)**
   - Limited requests are allowed through
   - If successful → Closed (recovered)
   - If failed → Open (still failing)

## Circuit States

### State Diagram

```
┌─────────────┐
│   CLOSED    │  Normal operation - all requests pass through
│  (Normal)   │  Failures are counted
└──────┬──────┘
       │ Failure rate > 50% (min 10 requests)
       ▼
┌─────────────┐
│    OPEN     │  Failure mode - requests fail immediately
│  (Tripped)  │  No requests reach the failing service
└──────┬──────┘
       │ After 30 seconds
       ▼
┌─────────────┐
│ HALF-OPEN   │  Testing recovery - limited requests allowed
│  (Testing)  │  If successful → CLOSED, if fail → OPEN
└─────────────┘
```

### State Transitions

| From          | To            | Condition                                    |
|---------------|---------------|----------------------------------------------|
| **Closed**    | **Open**      | Failure rate > 50% with at least 10 requests |
| **Open**      | **Half-Open** | After 30 seconds timeout                     |
| **Half-Open** | **Closed**    | After 5 consecutive successful requests      |
| **Half-Open** | **Open**      | On any failure                               |

### State Details

#### Closed (Normal Operation)

- All requests are allowed
- Success and failure counts are tracked
- Counts reset every 60 seconds
- Transitions to Open when failure threshold is exceeded

**Failure Threshold:**
- Minimum 10 requests in the interval
- Failure rate > 50%

#### Open (Circuit Tripped)

- All requests fail immediately with circuit breaker error
- No requests reach the backend service
- Protects both your app and the failing service
- Automatically transitions to Half-Open after timeout

**Timeout:** 30 seconds

#### Half-Open (Testing Recovery)

- Limited number of requests are allowed (default: 5)
- Tests if the service has recovered
- If all test requests succeed → Closed
- If any test request fails → Open

**Max Requests:** 5 concurrent requests

## Configuration

### Default Settings

Circuit breakers work automatically with these defaults:

```go
// Default circuit breaker configuration:
// - MaxRequests: 5 (in half-open state)
// - Interval: 60 seconds (reset period in closed state)
// - Timeout: 30 seconds (before transitioning to half-open)
// - Failure threshold: 50% with minimum 10 requests
```

### No Configuration Needed

Circuit breakers are created automatically per host:

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Circuit breaker works automatically
resp, err := client.Get("https://api.example.com/data")
```

## Usage Examples

### Example 1: Basic Usage

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Circuit breaker works automatically per host
for i := 0; i < 100; i++ {
    resp, err := client.Get("https://unreliable-api.example.com/data")
    if err != nil {
        // After multiple failures, you'll get:
        // "circuit breaker is open for host unreliable-api.example.com"
        log.Printf("Request %d failed: %v", i, err)
        
        // Circuit is open - requests fail fast without hitting the server
        // This protects both your app and the failing service
        continue
    }
    
    fmt.Printf("Request %d succeeded: %d\n", i, resp.StatusCode)
}

// After 30 seconds, circuit automatically enters half-open state
// and tests if the service has recovered
```

### Example 2: Microservices with Fallback

```go
func fetchUserData(client httpc.Client, userID int) (*User, error) {
    url := fmt.Sprintf("https://user-service.internal/users/%d", userID)
    
    resp, err := client.Get(url)
    if err != nil {
        // Check if circuit breaker is open
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("User service circuit is open, using cache")
            // Use cached data or default
            return getCachedUser(userID)
        }
        return nil, err
    }
    
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, err
    }
    
    return &user, nil
}
```

### Example 3: External API with Retry

```go
func callExternalAPI(client httpc.Client, endpoint string) ([]byte, error) {
    resp, err := client.Get(endpoint,
        httpc.WithTimeout(10*time.Second),
        httpc.WithMaxRetries(2),
    )
    
    if err != nil {
        // Circuit breaker error
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("Circuit open for %s, will retry later", endpoint)
            return nil, fmt.Errorf("service temporarily unavailable")
        }
        
        // Other errors
        return nil, err
    }
    
    return resp.RawBody, nil
}
```

### Example 4: High-Availability with Primary/Secondary

```go
func fetchData(client httpc.Client) ([]byte, error) {
    // Try primary service
    resp, err := client.Get("https://primary.example.com/data")
    if err != nil {
        // If circuit is open or other error, try secondary
        log.Printf("Primary failed: %v, trying secondary", err)
        
        resp, err = client.Get("https://secondary.example.com/data")
        if err != nil {
            return nil, fmt.Errorf("both primary and secondary failed: %w", err)
        }
    }
    
    return resp.RawBody, nil
}
```

## Best Practices

### ✅ DO

1. **Implement Fallback Logic**
   ```go
   if strings.Contains(err.Error(), "circuit breaker is open") {
       return fallbackData, nil
   }
   ```

2. **Log Circuit Breaker Events**
   ```go
   if strings.Contains(err.Error(), "circuit breaker is open") {
       log.Printf("[CIRCUIT] Open for host: %s", host)
   }
   ```

3. **Use Multiple Backends**
   - Have primary and secondary services
   - Switch to secondary when primary circuit opens

4. **Monitor Circuit State**
   - Track circuit breaker errors
   - Alert when circuits open frequently
   - Investigate root causes

5. **Cache Data**
   - Cache successful responses
   - Use cached data when circuit is open

### ❌ DON'T

1. **Don't Retry Immediately**
   ```go
   // Bad - retrying won't help when circuit is open
   for i := 0; i < 3; i++ {
       resp, err := client.Get(url)
       if err == nil {
           break
       }
   }
   
   // Good - check for circuit breaker and use fallback
   resp, err := client.Get(url)
   if err != nil && strings.Contains(err.Error(), "circuit breaker is open") {
       return fallbackData, nil
   }
   ```

2. **Don't Ignore Circuit Breaker Errors**
   - Circuit breaker errors indicate service issues
   - Implement proper fallback logic
   - Don't treat them like temporary errors

3. **Don't Disable Circuit Breakers**
   - They protect your application
   - They protect failing services
   - They improve overall system resilience

## Troubleshooting

### "circuit breaker is open"

**Cause:** The service has exceeded the failure threshold.

**Solution:**
1. Check if the service is actually down
2. Implement fallback logic
3. Wait for automatic recovery (30 seconds)
4. Check service logs for root cause

### Circuit Opens Too Frequently

**Cause:** Service is unstable or threshold is too low.

**Solution:**
1. Investigate service stability
2. Check network connectivity
3. Review service logs
4. Consider increasing timeout values

### Circuit Never Opens

**Cause:** Not enough requests or failures below threshold.

**Solution:**
- This is normal if service is stable
- Circuit breaker only opens when needed
- No action required

## Related Documentation

- [Error Handling](error-handling.md) - Comprehensive error handling patterns
- [Configuration](configuration.md) - Client configuration options
- [Best Practices](best-practices.md) - Recommended usage patterns

---

