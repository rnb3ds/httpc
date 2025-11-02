# Error Handling

This guide covers comprehensive error handling patterns and best practices for HTTPC.

## Table of Contents

- [Overview](#overview)
- [Error Types](#error-types)
- [Response Status Checking](#response-status-checking)
- [Error Handling Patterns](#error-handling-patterns)
- [Circuit Breaker Errors](#circuit-breaker-errors)
- [Timeout Errors](#timeout-errors)
- [Network Errors](#network-errors)
- [Best Practices](#best-practices)

## Overview

HTTPC provides multiple layers of error handling:

1. **Network-level errors** - Connection failures, timeouts, DNS errors
2. **HTTP-level errors** - Status codes (4xx, 5xx)
3. **Application-level errors** - Circuit breaker, validation errors
4. **Response parsing errors** - JSON/XML unmarshaling failures

## Error Types

### Network Errors

```go
resp, err := client.Get(url)
if err != nil {
    // Network error occurred
    log.Printf("Request failed: %v", err)
    return err
}
```

**Common network errors:**
- Connection refused
- DNS lookup failed
- Connection timeout
- TLS handshake failed
- Context canceled

### HTTP Status Errors

```go
resp, err := client.Get(url)
if err != nil {
    return err
}

// Check HTTP status
if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

### Parsing Errors

```go
var data MyStruct
if err := resp.JSON(&data); err != nil {
    return fmt.Errorf("failed to parse JSON: %w", err)
}
```

## Response Status Checking

### Status Helper Methods

```go
resp, err := client.Get(url)
if err != nil {
    return err
}

// Check status categories
if resp.IsSuccess() {        // 2xx
    fmt.Println("Success!")
}

if resp.IsClientError() {    // 4xx
    fmt.Println("Client error")
}

if resp.IsServerError() {    // 5xx
    fmt.Println("Server error")
}
```

### Specific Status Codes

```go
switch resp.StatusCode {
case 200:
    // OK
case 201:
    // Created
case 400:
    // Bad Request
case 401:
    // Unauthorized
case 403:
    // Forbidden
case 404:
    // Not Found
case 429:
    // Too Many Requests
case 500:
    // Internal Server Error
case 503:
    // Service Unavailable
default:
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

### Status Code Ranges

```go
if resp.StatusCode >= 200 && resp.StatusCode < 300 {
    // Success
} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
    // Client error
} else if resp.StatusCode >= 500 {
    // Server error
}
```

## Error Handling Patterns

### Pattern 1: Basic Error Handling

```go
func fetchUser(client httpc.Client, userID int) (*User, error) {
    url := fmt.Sprintf("https://api.example.com/users/%d", userID)
    
    resp, err := client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("API returned status %d: %s", 
            resp.StatusCode, resp.Body)
    }
    
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    return &user, nil
}
```

### Pattern 2: Detailed Error Information

```go
type APIError struct {
    StatusCode int
    Message    string
    Body       string
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

func fetchData(client httpc.Client, url string) ([]byte, error) {
    resp, err := client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, &APIError{
            StatusCode: resp.StatusCode,
            Message:    resp.Status,
            Body:       resp.Body,
        }
    }
    
    return resp.RawBody, nil
}
```

### Pattern 3: Error with Retry Logic

```go
func fetchWithRetry(client httpc.Client, url string) (*httpc.Response, error) {
    resp, err := client.Get(url,
        httpc.WithMaxRetries(3),
        httpc.WithTimeout(10*time.Second),
    )
    
    if err != nil {
        // Check if it's a temporary error
        if strings.Contains(err.Error(), "timeout") {
            return nil, fmt.Errorf("request timed out after retries: %w", err)
        }
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    return resp, nil
}
```

### Pattern 4: Context-Aware Error Handling

```go
func fetchWithContext(ctx context.Context, client httpc.Client, url string) ([]byte, error) {
    resp, err := client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithTimeout(30*time.Second),
    )
    
    if err != nil {
        // Check if context was canceled
        if ctx.Err() == context.Canceled {
            return nil, fmt.Errorf("request canceled: %w", err)
        }
        if ctx.Err() == context.DeadlineExceeded {
            return nil, fmt.Errorf("request deadline exceeded: %w", err)
        }
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("status %d: %s", resp.StatusCode, resp.Body)
    }
    
    return resp.RawBody, nil
}
```

### Pattern 5: Fallback on Error

```go
func fetchWithFallback(client httpc.Client, primaryURL, fallbackURL string) ([]byte, error) {
    // Try primary
    resp, err := client.Get(primaryURL)
    if err == nil && resp.IsSuccess() {
        return resp.RawBody, nil
    }
    
    // Log primary failure
    log.Printf("Primary failed: %v, trying fallback", err)
    
    // Try fallback
    resp, err = client.Get(fallbackURL)
    if err != nil {
        return nil, fmt.Errorf("both primary and fallback failed: %w", err)
    }
    
    if !resp.IsSuccess() {
        return nil, fmt.Errorf("fallback returned status %d", resp.StatusCode)
    }
    
    return resp.RawBody, nil
}
```

## Circuit Breaker Errors

### Detecting Circuit Breaker Errors

```go
resp, err := client.Get(url)
if err != nil {
    if strings.Contains(err.Error(), "circuit breaker is open") {
        // Circuit is open - service is down
        log.Printf("Circuit breaker open for %s", url)
        return useCachedData()
    }
    return nil, err
}
```

### Handling Circuit Breaker State

```go
func fetchWithCircuitBreaker(client httpc.Client, url string) ([]byte, error) {
    resp, err := client.Get(url)
    
    if err != nil {
        // Check for circuit breaker
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("[CIRCUIT OPEN] Service unavailable: %s", url)
            
            // Use fallback strategy
            return getFallbackData(), nil
        }
        
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    return resp.RawBody, nil
}
```

## Timeout Errors

### Handling Timeouts

```go
resp, err := client.Get(url,
    httpc.WithTimeout(10*time.Second),
)

if err != nil {
    if strings.Contains(err.Error(), "timeout") ||
       strings.Contains(err.Error(), "deadline exceeded") {
        log.Printf("Request timed out after 10s")
        return nil, fmt.Errorf("request timeout: %w", err)
    }
    return nil, err
}
```

### Timeout with Retry

```go
func fetchWithTimeoutRetry(client httpc.Client, url string) ([]byte, error) {
    const maxAttempts = 3
    timeout := 5 * time.Second
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        resp, err := client.Get(url,
            httpc.WithTimeout(timeout),
        )
        
        if err == nil && resp.IsSuccess() {
            return resp.RawBody, nil
        }
        
        if err != nil && strings.Contains(err.Error(), "timeout") {
            log.Printf("Attempt %d timed out, retrying...", attempt)
            timeout *= 2  // Exponential backoff
            continue
        }
        
        return nil, err
    }
    
    return nil, fmt.Errorf("all attempts timed out")
}
```

## Network Errors

### Connection Errors

```go
resp, err := client.Get(url)
if err != nil {
    if strings.Contains(err.Error(), "connection refused") {
        return nil, fmt.Errorf("service is not running: %w", err)
    }
    if strings.Contains(err.Error(), "no such host") {
        return nil, fmt.Errorf("DNS lookup failed: %w", err)
    }
    if strings.Contains(err.Error(), "TLS handshake") {
        return nil, fmt.Errorf("TLS error: %w", err)
    }
    return nil, err
}
```

### Network Error Recovery

```go
func fetchWithNetworkRetry(client httpc.Client, url string) ([]byte, error) {
    const maxAttempts = 3
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        resp, err := client.Get(url)
        
        if err == nil && resp.IsSuccess() {
            return resp.RawBody, nil
        }
        
        // Check if it's a recoverable network error
        if err != nil {
            if strings.Contains(err.Error(), "connection refused") ||
               strings.Contains(err.Error(), "connection reset") {
                log.Printf("Network error on attempt %d: %v", attempt, err)
                time.Sleep(time.Second * time.Duration(attempt))
                continue
            }
            // Non-recoverable error
            return nil, err
        }
        
        // HTTP error
        if !resp.IsSuccess() {
            return nil, fmt.Errorf("status %d", resp.StatusCode)
        }
    }
    
    return nil, fmt.Errorf("max attempts reached")
}
```

## Best Practices

1. **Always check errors**
   ```go
   resp, err := client.Get(url)
   if err != nil {
       return err
   }
   ```

2. **Check HTTP status codes**
   ```go
   if !resp.IsSuccess() {
       return fmt.Errorf("status %d", resp.StatusCode)
   }
   ```

3. **Wrap errors with context**
   ```go
   if err != nil {
       return fmt.Errorf("failed to fetch user %d: %w", userID, err)
   }
   ```

4. **Use appropriate error types**
   ```go
   type APIError struct {
       StatusCode int
       Message    string
   }
   ```

5. **Log errors appropriately**
   ```go
   if err != nil {
       log.Printf("Request failed: %v", err)
       return err
   }
   ```

6. **Handle circuit breaker errors**
   ```go
   if strings.Contains(err.Error(), "circuit breaker is open") {
       return useFallback()
   }
   ```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "strings"
    "time"
    
    "github.com/cybergodev/httpc"
)

type User struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func fetchUser(ctx context.Context, client httpc.Client, userID int) (*User, error) {
    url := fmt.Sprintf("https://api.example.com/users/%d", userID)
    
    // Make request with context and timeout
    resp, err := client.Get(url,
        httpc.WithContext(ctx),
        httpc.WithTimeout(10*time.Second),
        httpc.WithMaxRetries(2),
    )
    
    // Handle network errors
    if err != nil {
        // Check for circuit breaker
        if strings.Contains(err.Error(), "circuit breaker is open") {
            log.Printf("Circuit breaker open for user service")
            return nil, fmt.Errorf("user service unavailable: %w", err)
        }
        
        // Check for timeout
        if strings.Contains(err.Error(), "timeout") {
            return nil, fmt.Errorf("request timed out: %w", err)
        }
        
        // Check for context cancellation
        if ctx.Err() != nil {
            return nil, fmt.Errorf("request canceled: %w", err)
        }
        
        return nil, fmt.Errorf("request failed: %w", err)
    }
    
    // Handle HTTP errors
    if !resp.IsSuccess() {
        switch resp.StatusCode {
        case 404:
            return nil, fmt.Errorf("user %d not found", userID)
        case 401:
            return nil, fmt.Errorf("unauthorized")
        case 429:
            return nil, fmt.Errorf("rate limit exceeded")
        case 500, 502, 503:
            return nil, fmt.Errorf("server error: %d", resp.StatusCode)
        default:
            return nil, fmt.Errorf("unexpected status %d: %s", 
                resp.StatusCode, resp.Body)
        }
    }
    
    // Parse response
    var user User
    if err := resp.JSON(&user); err != nil {
        return nil, fmt.Errorf("failed to parse user data: %w", err)
    }
    
    return &user, nil
}

func main() {
    client, err := httpc.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    ctx := context.Background()
    user, err := fetchUser(ctx, client, 123)
    if err != nil {
        log.Printf("Error: %v", err)
        return
    }
    
    fmt.Printf("User: %+v\n", user)
}
```

---