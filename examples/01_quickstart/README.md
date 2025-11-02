# Quick Start Examples

**Time to complete: 5 minutes**

This directory contains the simplest examples to get you started with httpc immediately.

## What You'll Learn

- How to make basic HTTP requests (GET, POST, PUT, DELETE)
- Package-level functions vs client instances
- Simple JSON handling
- Basic error handling

## Examples Included

### 1. Simple GET Request
The absolute simplest way to make an HTTP request:

```go
resp, err := httpc.Get("https://api.example.com/data")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Status: %d\n", resp.StatusCode)
```

### 2. POST with JSON
Send JSON data to an API:

```go
user := User{Name: "John", Email: "john@example.com"}
resp, err := httpc.Post("https://api.example.com/users",
    httpc.WithJSON(user),
)
```

### 3. PUT Request
Update existing resources:

```go
updateData := map[string]interface{}{"status": "active"}
resp, err := httpc.Put("https://api.example.com/users/123",
    httpc.WithJSON(updateData),
)
```

### 4. DELETE Request
Delete resources:

```go
resp, err := httpc.Delete("https://api.example.com/users/123",
    httpc.WithBearerToken("your-token"),
)
```

### 5. Using a Client Instance
For production code, create a client instance:

```go
client, err := httpc.New()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

resp, err := client.Get("https://api.example.com/data")
```


## Key Takeaways

1. **Package-level functions** (`httpc.Get()`, `httpc.Post()`, etc.) are perfect for quick testing and simple scripts
2. **Client instances** (`httpc.New()`) are recommended for production code
3. **Always close clients** with `defer client.Close()`
4. **Check errors** - never ignore the error return value
5. **Use `WithJSON()`** for sending JSON data
6. **Check response status** with `resp.IsSuccess()`

## Common Patterns

### Quick Testing
```go
// Perfect for quick API testing
resp, _ := httpc.Get("https://api.example.com/health")
fmt.Println(resp.StatusCode)
```

### Production Code
```go
// Better for production with proper error handling
client, err := httpc.New()
if err != nil {
    return fmt.Errorf("failed to create client: %w", err)
}
defer client.Close()

resp, err := client.Get(url)
if err != nil {
    return fmt.Errorf("request failed: %w", err)
}

if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
```

## Next Steps

Once you're comfortable with these basics, move on to:
- **[Core Features](../02_core_features)** - Learn about headers, authentication, and more request options
- **[Advanced Usage](../03_advanced)** - Master timeouts, retries, and concurrent requests
- **[Real-World Examples](../04_real_world)** - See practical implementations


## Tips

- Start with package-level functions to learn the API
- Use client instances for applications with multiple requests
- The echo.hoppscotch.io endpoint is perfect for testing
- Check the response status before parsing the body
- Use `resp.JSON(&result)` to parse JSON responses

---
