# Request Options

This guide covers all available request options in HTTPC for customizing HTTP requests.

> **Prerequisite**: Before diving into request options, make sure you understand the [Common Patterns from Getting Started](getting-started.md#common-patterns), especially the Client Setup and Error Handling patterns.

## Table of Contents

- [Overview](#overview)
- [Headers](#headers)
- [Authentication](#authentication)
- [Query Parameters](#query-parameters)
- [Request Body](#request-body)
- [Timeout & Context](#timeout--context)
- [Retry Options](#retry-options)
- [Cookies](#cookies)
- [Complete Reference](#complete-reference)

## Overview

Request options are functions that modify request behavior. They follow the functional options pattern:

```go
resp, err := client.Get(url,
    httpc.WithTimeout(10*time.Second),
    httpc.WithHeader("X-Custom", "value"),
    httpc.WithBearerToken("token"),
)
```

**Key Concepts:**
- Options are applied in order
- Later options can override earlier ones
- Options are reusable across requests
- All options are optional (sensible defaults apply)

## Headers

### Single Header

```go
resp, err := client.Get(url,
    httpc.WithHeader("X-API-Key", "your-key"),
)
```

### Multiple Headers

```go
resp, err := client.Get(url,
    httpc.WithHeader("X-API-Key", "key"),
    httpc.WithHeader("X-Request-ID", "123"),
)
```

### Header Map

```go
headers := map[string]string{
    "X-API-Key":    "your-key",
    "X-Request-ID": "123",
    "X-Client":     "MyApp",
}

resp, err := client.Get(url,
    httpc.WithHeaders(headers),
)
```

### Common Headers

```go
// User-Agent
httpc.WithUserAgent("MyApp/1.0")

// Content-Type (set via body options)
httpc.WithJSON(data)  // Sets Content-Type: application/json
httpc.WithXML(data)   // Sets Content-Type: application/xml
```

## Authentication

### Bearer Token

```go
resp, err := client.Get(url,
    httpc.WithBearerToken("your-jwt-token"),
)
```

### Basic Authentication

```go
resp, err := client.Get(url,
    httpc.WithBasicAuth("username", "password"),
)
```

### API Key

```go
// Header-based API key
resp, err := client.Get(url,
    httpc.WithHeader("X-API-Key", "your-api-key"),
)

// Query parameter API key
resp, err := client.Get(url,
    httpc.WithQuery("api_key", "your-api-key"),
)

// Note: Use WithCookie for cookie-based authentication
resp, err := client.Get(url,
    httpc.WithCookie(http.Cookie{Name: "session", Value: "your-session"}),
)
```

### Custom Authentication

```go
// Custom auth header
resp, err := client.Get(url,
    httpc.WithHeader("Authorization", "Custom "+token),
)
```

## Query Parameters

### Single Parameter

```go
resp, err := client.Get(url,
    httpc.WithQuery("page", 1),
    httpc.WithQuery("limit", 20),
)
```

### Query Map

```go
params := map[string]interface{}{
    "page":   1,
    "limit":  20,
    "sort":   "name",
    "active": true,
}

resp, err := client.Get(url,
    httpc.WithQueryMap(params),
)
```

### Complex Query Parameters

```go
// Arrays/slices
resp, err := client.Get(url,
    httpc.WithQuery("tags", []string{"go", "http"}),
)

// Multiple values for same key
resp, err := client.Get(url,
    httpc.WithQuery("id", 1),
    httpc.WithQuery("id", 2),
    httpc.WithQuery("id", 3),
)
```

## Request Body

### JSON Body

```go
user := map[string]interface{}{
    "name":  "John Doe",
    "email": "john@example.com",
    "age":   30,
}

resp, err := client.Post(url,
    httpc.WithJSON(user),
)
```

**Automatic Features:**
- Sets `Content-Type: application/json`
- Marshals data to JSON automatically
- Works with structs, maps, slices

### XML Body

```go
type User struct {
    XMLName xml.Name `xml:"user"`
    Name    string   `xml:"name"`
    Email   string   `xml:"email"`
}

user := User{Name: "John", Email: "john@example.com"}

resp, err := client.Post(url,
    httpc.WithXML(user),
)
```

### Form Data

```go
formData := map[string]string{
    "username": "john",
    "password": "secret",
    "remember": "true",
}

resp, err := client.Post(url,
    httpc.WithForm(formData),
)
```

**Sets:** `Content-Type: application/x-www-form-urlencoded`

### Binary Data

```go
imageData, _ := os.ReadFile("image.png")

resp, err := client.Post(url,
    httpc.WithBinary(imageData, "image/png"),
)
```

### Raw Body

```go
resp, err := client.Post(url,
    httpc.WithBody([]byte("raw data")),
    httpc.WithHeader("Content-Type", "application/octet-stream"),
)
```

## File Upload

### Single File

```go
fileContent, _ := os.ReadFile("document.pdf")

resp, err := client.Post(url,
    httpc.WithFile("file", "document.pdf", fileContent),
)
```

### Multiple Files with Form Fields

```go
file1, _ := os.ReadFile("doc1.pdf")
file2, _ := os.ReadFile("doc2.pdf")

formData := &httpc.FormData{
    Fields: map[string]string{
        "title":       "My Documents",
        "description": "Important files",
    },
    Files: map[string]*httpc.FileData{
        "document1": {
            Filename:    "doc1.pdf",
            Content:     file1,
            ContentType: "application/pdf",
        },
        "document2": {
            Filename:    "doc2.pdf",
            Content:     file2,
            ContentType: "application/pdf",
        },
    },
}

resp, err := client.Post(url,
    httpc.WithFormData(formData),
)
```

## Timeout & Context

### Request Timeout

```go
// Timeout for this specific request
resp, err := client.Get(url,
    httpc.WithTimeout(30*time.Second),
)
```

### Context

```go
// With cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

resp, err := client.Get(url,
    httpc.WithContext(ctx),
)

// With deadline
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()

resp, err := client.Get(url,
    httpc.WithContext(ctx),
)
```

### Context with Values

```go
ctx := context.WithValue(context.Background(), "request-id", "12345")

resp, err := client.Get(url,
    httpc.WithContext(ctx),
)
```

## Retry Options

### Max Retries

```go
resp, err := client.Get(url,
    httpc.WithMaxRetries(3),
)
```

### Retry with Timeout

```go
resp, err := client.Get(url,
    httpc.WithMaxRetries(3),
    httpc.WithTimeout(10*time.Second),
)
```

**Note:** Retry behavior is also configured at the client level. Request-level options override client configuration.

## Cookies

### Send Cookie

```go
cookie := http.Cookie{
    Name:  "session_id",
    Value: "abc123",
}

resp, err := client.Get(url,
    httpc.WithCookie(cookie),
)
```

### Multiple Cookies

```go
// Use multiple WithCookie calls for multiple cookies
resp, err := client.Get(url,
    httpc.WithCookie(http.Cookie{Name: "session_id", Value: "abc123"}),
    httpc.WithCookie(http.Cookie{Name: "user_pref", Value: "dark_mode"}),
)
```

### Cookie Map

Convenient way to set multiple simple cookies from a map:

```go
cookies := map[string]string{
    "session_id": "abc123",
    "user_pref":  "dark_mode",
    "lang":       "en",
}

resp, err := client.Get(url,
    httpc.WithCookieMap(cookies),
)
```

### Cookie String (From Browser)

Parse and send multiple cookies from a cookie string (e.g., copied from browser dev tools):

```go
resp, err := client.Get(url,
    httpc.WithCookieString("session=abc123; token=xyz789; user_id=12345"),
)
```

## Complete Reference

### All Request Options

| Option                           | Description          | Example                                 |
|----------------------------------|----------------------|-----------------------------------------|
| `WithHeader(key, value)`         | Set single header    | `WithHeader("X-API-Key", "key")`        |
| `WithHeaderMap(headers)`         | Set multiple headers | `WithHeaderMap(map[string]string{...})` |
| `WithUserAgent(ua)`              | Set User-Agent       | `WithUserAgent("MyApp/1.0")`            |
| `WithBearerToken(token)`         | Bearer auth          | `WithBearerToken("jwt-token")`          |
| `WithBasicAuth(u, p)`            | Basic auth           | `WithBasicAuth("user", "pass")`         |
| `WithQuery(key, value)`          | Add query param      | `WithQuery("page", 1)`                  |
| `WithQueryMap(params)`           | Add multiple params  | `WithQueryMap(map[string]any{...})`     |
| `WithJSON(data)`                 | JSON body            | `WithJSON(struct{...})`                 |
| `WithXML(data)`                  | XML body             | `WithXML(struct{...})`                  |
| `WithForm(data)`                 | Form data            | `WithForm(map[string]string{...})`      |
| `WithBinary(data, ct)`           | Binary data          | `WithBinary([]byte{...}, "image/png")`  |
| `WithBody(data)`                 | Raw body             | `WithBody([]byte{...})`                 |
| `WithFile(field, name, content)` | Single file          | `WithFile("file", "doc.pdf", data)`     |
| `WithFormData(fd)`               | Multipart form       | `WithFormData(&FormData{...})`          |
| `WithTimeout(duration)`          | Request timeout      | `WithTimeout(30*time.Second)`           |
| `WithContext(ctx)`               | Request context      | `WithContext(ctx)`                      |
| `WithMaxRetries(n)`              | Max retry attempts   | `WithMaxRetries(3)`                     |
| `WithCookie(cookie)`             | Add cookie           | `WithCookie(http.Cookie{Name: "n", Value: "v"})` |
| `WithCookieMap(cookies)`         | Add multiple cookies | `WithCookieMap(map[string]string{...})` |
| `WithCookieString(cookieStr)`    | Parse cookie string  | `WithCookieString("a=1; b=2")`          |
| `WithFollowRedirects(follow)`    | Redirect policy      | `WithFollowRedirects(false)`            |
| `WithMaxRedirects(n)`            | Max redirects        | `WithMaxRedirects(5)`                   |

## Best Practices

1. **Use appropriate content types**
   ```go
   httpc.WithJSON(data)  // Automatically sets Content-Type
   ```

2. **Set timeouts for external APIs**
   ```go
   httpc.WithTimeout(30*time.Second)
   ```

3. **Use context for cancellation**
   ```go
   httpc.WithContext(ctx)
   ```

4. **Reuse option patterns**
   ```go
   authOption := httpc.WithBearerToken(token)
   resp1, _ := client.Get(url1, authOption)
   resp2, _ := client.Get(url2, authOption)
   ```

---
