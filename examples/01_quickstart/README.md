# Quick Start Examples

**Time to complete**: 5 minutes

Start here to learn the basics of httpc!

## What You'll Learn

- ✅ Simplest GET/POST/PUT/DELETE requests
- ✅ Package-level functions vs client instances
- ✅ Basic JSON handling
- ✅ When to use what approach

## Example

**[basic_usage.go](basic_usage.go)** - Complete quick start guide

### Key Concepts

1. **Package-level functions** (simplest)
   ```go
   resp, err := httpc.Get("https://api.example.com/data")
   ```

2. **Client instances** (recommended for multiple requests)
   ```go
   client, err := httpc.New()
   defer client.Close()
   resp, err := client.Get("https://api.example.com/data")
   ```

3. **JSON handling**
   ```go
   var result MyStruct
   err := resp.JSON(&result)
   ```

## Running

```bash
go run -tags examples examples/01_quickstart/basic_usage.go
```

## Next Steps

After completing this example, move on to:
- **[02_core_features/](../02_core_features/)** - Learn essential features
- **[03_advanced/](../03_advanced/)** - Master advanced patterns

---

**Estimated time**: 5 minutes | **Difficulty**: Beginner
