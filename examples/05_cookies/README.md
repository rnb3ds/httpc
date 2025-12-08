# Cookie Management Examples

This directory contains examples demonstrating various cookie handling patterns in httpc.

## Examples

### 1. cookie_jar_management.go
**Automatic Cookie Management with Cookie Jar**

Demonstrates how the cookie jar automatically manages cookies across requests:
- Automatic cookie persistence
- Cookie accumulation across requests
- Cookie override behavior

```bash
go run -tags examples examples/05_cookies/cookie_jar_management.go
```

**Key Features:**
- Enable cookie jar with `config.EnableCookies = true`
- Cookies automatically persist across requests
- New cookies are added to existing ones
- Override cookies by setting new values

---

### 2. cookie_string_example.go
**Parse Cookie Strings from Browser**

Shows how to use `WithCookieString()` to parse cookie strings copied from browser developer tools:
- Parse multiple cookies from a single string
- Combine with other cookie methods
- Handle empty cookie strings

```bash
go run -tags examples examples/05_cookies/cookie_string_example.go
```

**Key Features:**
- Parse format: `"name1=value1; name2=value2; name3=value3"`
- Useful for copying cookies from browser DevTools
- Automatically creates cookies with secure defaults

---

### 3. quick_cookie_inspection.go
**Quick Reference for Cookie Inspection**

A concise example showing the difference between request and response cookies:
- Request cookies (sent TO server)
- Response cookies (received FROM server)
- Helper methods for both types

```bash
go run -tags examples examples/05_cookies/quick_cookie_inspection.go
```

**Key Concepts:**
- **Request Cookies**: `resp.GetRequestCookies()`, `resp.GetRequestCookie(name)`, `resp.HasRequestCookie(name)`
- **Response Cookies**: `resp.Response.Cookies`, `resp.GetCookie(name)`, `resp.HasCookie(name)`

---

### 4. request_cookies_inspection.go
**Comprehensive Request Cookie Inspection**

Detailed examples of inspecting cookies that were actually sent in HTTP requests:
- Direct inspection via RequestHeaders
- Using helper functions to parse cookies
- Comparing request vs response cookies
- Debugging cookie issues
- Cookie jar automatic cookies

```bash
go run -tags examples examples/05_cookies/request_cookies_inspection.go
```

**Use Cases:**
- Debugging: Verify cookies were sent correctly
- Testing: Validate cookie behavior
- Monitoring: Track cookie flow in requests

---

## Cookie Concepts

### Request Cookies vs Response Cookies

**Request Cookies** (sent TO server):
- Set using `WithCookieValue()`, `WithCookie()`, `WithCookies()`, `WithCookieString()`
- Accessed via `resp.GetRequestCookies()` or `resp.Request.Headers.Get("Cookie")`
- These are the cookies your client sends to the server

**Response Cookies** (received FROM server):
- Set by server using `Set-Cookie` header
- Accessed via `resp.Response.Cookies` or `resp.GetCookie(name)`
- These are the cookies the server wants you to store

### Cookie Jar

When `config.EnableCookies = true`:
- Cookies are automatically stored and managed
- Cookies from responses are saved
- Cookies from request options are also persisted
- Subsequent requests automatically include stored cookies
- Domain and path rules are enforced

### Manual Cookie Management

Without cookie jar:
- Cookies must be explicitly set for each request
- No automatic persistence
- Full control over cookie behavior
- Useful for testing or specific scenarios

## Best Practices

1. **Use Cookie Jar for Session Management**
   ```go
   config := httpc.DefaultConfig()
   config.EnableCookies = true
   client, _ := httpc.New(config)
   ```

2. **Parse Browser Cookies Easily**
   ```go
   httpc.WithCookieString("session=abc; token=xyz")
   ```

3. **Inspect Cookies for Debugging**
   ```go
   // Check what was sent
   requestCookies := resp.GetRequestCookies()
   
   // Check what was received
   responseCookies := resp.Response.Cookies
   ```

4. **Secure Cookie Defaults**
   ```go
   cookie := http.Cookie{
       Name:     "session",
       Value:    "secret",
       HttpOnly: true,
       Secure:   true,
       SameSite: http.SameSiteStrictMode,
   }
   ```

## Related Examples

- **[02_core_features/cookies.go](../02_core_features/cookies.go)** - Basic cookie usage
- **[06_domain_client/](../06_domain_client/)** - Automatic cookie management per domain
- **[04_real_world/rest_api_client.go](../04_real_world/rest_api_client.go)** - Real-world API client with cookies

## Common Patterns

### Login Flow with Cookies
```go
config := httpc.DefaultConfig()
config.EnableCookies = true
client, _ := httpc.New(config)

// Login - server sets session cookie
client.Post("/login", httpc.WithJSON(credentials))

// Subsequent requests automatically include session cookie
client.Get("/profile")
client.Get("/data")
```

### Copy Cookies from Browser
```go
// Copy from browser DevTools -> Application -> Cookies
cookieString := "SESSIONID=abc123; XSRF-TOKEN=xyz789"

resp, _ := client.Get("/api/data",
    httpc.WithCookieString(cookieString),
)
```

### Debug Cookie Issues
```go
resp, _ := client.Get("/api/endpoint",
    httpc.WithCookieValue("debug", "true"),
)

// Verify cookie was sent
if resp.HasRequestCookie("debug") {
    fmt.Println("✓ Cookie sent successfully")
} else {
    fmt.Println("✗ Cookie was not sent")
}
```
