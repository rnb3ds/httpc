# Core Features Examples

**Time to complete**: 15 minutes

Master the essential features of httpc.

## What You'll Learn

- ✅ All request body formats (JSON, Form, XML, Binary, Text)
- ✅ Headers and authentication methods
- ✅ Query parameters handling
- ✅ Response parsing and status checking
- ✅ Error handling patterns
- ✅ Compression support

## Examples

### 1. Request Options
**[request_options.go](request_options.go)** - Comprehensive request configuration

**Covers:**
- Body formats: JSON, Form, XML, Binary, Text, File upload
- Headers: Single, multiple, custom
- Authentication: Bearer token, Basic auth, API key
- Query parameters: Single, map, special characters

**Key patterns:**
```go
// JSON body
httpc.WithJSON(data)

// Authentication
httpc.WithBearerToken("token")
httpc.WithBasicAuth("user", "pass")

// Query parameters
httpc.WithQueryMap(params)
```

### 2. Response Handling
**[response_handling.go](response_handling.go)** - Complete response processing

**Covers:**
- Result API structure (Request/Response/Meta)
- JSON/XML parsing
- Status code checking (IsSuccess, IsClientError, etc.)
- Header access
- Response metadata (duration, attempts, redirects)
- Response formatting (String, Html)

**Key patterns:**
```go
// Parse JSON
var result MyStruct
err := resp.JSON(&result)

// Check status
if resp.IsSuccess() {
    // Process response
}

// Access metadata
fmt.Printf("Duration: %v\n", resp.Meta.Duration)
```

### 3. Error Handling
**[error_handling.go](error_handling.go)** - Robust error handling

**Covers:**
- Basic error patterns
- HTTP status errors (4xx, 5xx)
- Timeout and context cancellation
- Parsing errors
- Comprehensive error handling pattern

**Key patterns:**
```go
resp, err := client.Get(url)
if err != nil {
    return fmt.Errorf("request failed: %w", err)
}

if !resp.IsSuccess() {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode())
}
```

### 4. Compression
**[compression.go](compression.go)** - Automatic compression handling

**Covers:**
- Automatic gzip/deflate decompression
- Accept-Encoding headers
- Transparent handling

## Running Examples

```bash
# Request options
go run -tags examples examples/02_core_features/request_options.go

# Response handling
go run -tags examples examples/02_core_features/response_handling.go

# Error handling
go run -tags examples examples/02_core_features/error_handling.go

# Compression
go run -tags examples examples/02_core_features/compression.go
```

## Key Takeaways

1. **Request Options**: Use `With*` functions to configure requests
2. **Response Handling**: Always check status before parsing
3. **Error Handling**: Handle errors at every step
4. **Compression**: Automatic, no configuration needed

## Common Patterns

### Complete Request Pattern
```go
resp, err := client.Post(url,
    httpc.WithJSON(data),
    httpc.WithBearerToken(token),
    httpc.WithQueryMap(params),
    httpc.WithTimeout(30*time.Second),
)
if err != nil {
    return fmt.Errorf("request failed: %w", err)
}

if !resp.IsSuccess() {
    return fmt.Errorf("status %d: %s", resp.StatusCode(), resp.Body())
}

var result MyStruct
if err := resp.JSON(&result); err != nil {
    return fmt.Errorf("parse failed: %w", err)
}
```

## Next Steps

After mastering core features, explore:
- **[03_advanced/](../03_advanced/)** - Advanced patterns and configurations

---

**Estimated time**: 15 minutes | **Difficulty**: Intermediate

