# Core Features Examples

**Time to complete: 10 minutes**

This directory demonstrates the essential features you'll use in everyday HTTP client operations.

## What You'll Learn

- Request headers and authentication methods
- Query parameters and URL building
- Different request body formats (JSON, XML, Form, Text, Binary)
- Response parsing and error handling
- Status code checking

## Examples Overview

### 1. Headers and Authentication (`headers_auth.go`)

Learn how to set headers and authenticate requests:

- **Custom Headers**: Add any header to your requests
- **Bearer Token**: JWT authentication
- **Basic Auth**: Username/password authentication
- **API Keys**: Custom API key headers
- **User-Agent**: Set custom user agent strings

**Key Functions:**
- `WithHeader(key, value)` - Single header
- `WithHeaderMap(map)` - Multiple headers
- `WithBearerToken(token)` - JWT authentication
- `WithBasicAuth(user, pass)` - Basic authentication
- `WithUserAgent(ua)` - Custom user agent

### 2. Request Body Formats (`body_formats.go`)

Master different ways to send data:

- **JSON**: Structs and maps
- **Form Data**: URL-encoded forms
- **Plain Text**: Text content
- **XML**: XML structures
- **Binary**: Binary data with content types
- **Raw Body**: Custom formats

**Key Functions:**
- `WithJSON(data)` - JSON body
- `WithForm(data)` - Form data
- `WithText(content)` - Plain text
- `WithXML(data)` - XML body
- `WithBinary(data, contentType)` - Binary data
- `WithBody(data)` + `WithContentType(type)` - Custom formats

### 3. Query Parameters (`query_params.go`)

Build URLs with query parameters:

- Single parameters
- Multiple parameters using maps
- Different value types (strings, numbers, booleans)
- URL encoding

**Key Functions:**
- `WithQuery(key, value)` - Single parameter
- `WithQueryMap(params)` - Multiple parameters (recommended)

### 4. Response Parsing (`response_parsing.go`)

Handle and parse responses:

- JSON parsing
- XML parsing
- Status code checking
- Header access
- Response metadata

**Key Methods:**
- `resp.JSON(&result)` - Parse JSON
- `resp.XML(&result)` - Parse XML
- `resp.IsSuccess()` - Check 2xx status
- `resp.IsClientError()` - Check 4xx status
- `resp.IsServerError()` - Check 5xx status
- `resp.Headers.Get(key)` - Get header value

### 5. Error Handling (`error_handling.go`)

Proper error handling patterns:

- Network errors
- Timeout errors
- HTTP errors
- Parsing errors
- Context cancellation

## Common Patterns

### Pattern 1: JSON API Request

```go
type User struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

// Send JSON
user := User{Name: "John", Email: "john@example.com"}
resp, err := client.Post("https://api.example.com/users",
    httpc.WithJSON(user),
    httpc.WithBearerToken("your-token"),
)
if err != nil {
    return fmt.Errorf("request failed: %w", err)
}

// Parse JSON response
var result User
if err := resp.JSON(&result); err != nil {
    return fmt.Errorf("parse failed: %w", err)
}
```

### Pattern 2: Form Submission

```go
formData := map[string]string{
    "username": "john",
    "password": "secret",
}

resp, err := client.Post("https://api.example.com/login",
    httpc.WithForm(formData),
)
```

### Pattern 3: Authenticated GET with Query Params

```go
params := map[string]interface{}{
    "page":  1,
    "limit": 20,
    "sort":  "date",
}

resp, err := client.Get("https://api.example.com/data",
    httpc.WithQueryMap(params),
    httpc.WithBearerToken("your-token"),
    httpc.WithJSONAccept(),
)
```

### Pattern 4: Custom Headers

```go
headers := map[string]string{
    "X-API-Version":  "v1",
    "X-Request-ID":   "12345",
    "X-Client-ID":    "my-app",
}

resp, err := client.Get("https://api.example.com/data",
    httpc.WithHeaderMap(headers),
    httpc.WithBearerToken("your-token"),
)
```

## Request Options Reference

### Headers
```go
WithHeader(key, value string)               // Single header
WithHeaderMap(headers map[string]string)    // Multiple headers
WithUserAgent(userAgent string)             // User-Agent header
WithContentType(contentType string)         // Content-Type header
WithAccept(accept string)                   // Accept header
WithJSONAccept()                            // Accept: application/json
WithXMLAccept()                             // Accept: application/xml
```

### Authentication
```go
WithBasicAuth(username, password string)    // Basic authentication
WithBearerToken(token string)               // Bearer token (JWT)
```

### Query Parameters
```go
WithQuery(key string, value interface{})        // Single parameter
WithQueryMap(params map[string]interface{})     // Multiple parameters
```

### Body
```go
WithJSON(data interface{})                      // JSON body
WithXML(data interface{})                       // XML body
WithText(content string)                        // Plain text
WithForm(data map[string]string)                // Form data
WithBinary(data []byte, contentType ...string)  // Binary data
WithBody(body interface{})                      // Raw body
```

## Response Methods

```go
// Status checking
resp.IsSuccess()      // 2xx
resp.IsRedirect()     // 3xx
resp.IsClientError()  // 4xx
resp.IsServerError()  // 5xx

// Parsing
resp.JSON(&result)    // Parse JSON
resp.XML(&result)     // Parse XML

// Access
resp.StatusCode       // HTTP status code
resp.Status           // HTTP status message
resp.Headers          // Response headers
resp.Body             // Response body (string)
resp.RawBody          // Response body ([]byte)
resp.Duration         // Request duration
resp.Attempts         // Number of attempts
```

## Best Practices

1. **Always check errors**: Never ignore error returns
2. **Use WithJSON for JSON**: Automatically sets Content-Type
3. **Use WithQueryMap for multiple params**: More readable than multiple WithQuery calls
4. **Check response status**: Use `resp.IsSuccess()` before parsing
5. **Use proper authentication**: Choose the right auth method for your API
6. **Set Accept headers**: Use `WithJSONAccept()` for JSON APIs
7. **Handle parsing errors**: Always check errors from `resp.JSON()` and `resp.XML()`

## Next Steps

After mastering these core features, move on to:
- **[Advanced Usage](../03_advanced)** - Timeouts, retries, file uploads, concurrent requests
- **[Real-World Examples](../04_real_world)** - Complete API client implementations

## Tips

- The echo.hoppscotch.io endpoint echoes back your request, perfect for testing
- Use `WithJSONAccept()` when you expect JSON responses
- Combine multiple options in a single request
- Headers are case-insensitive but conventionally use Title-Case
- Query parameters are automatically URL-encoded

