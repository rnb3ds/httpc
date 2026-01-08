---
type: "always_apply"
---

# Product Guidelines for cybergodev/httpc

**cybergodev/httpc** is a high-performance Go HTTP client library for Go with zero external dependencies, type safety, enhancements, and optimized performance.

## Product Vision

### Core Value Proposition
- **Two-Tier Architecture**: Convenience functions + configurable Processor API
- **Backward Compatibility**: No breaking changes in minor versions
- **Minimal Code**: Only essential code, avoid verbosity
- **Zero Learning Curve**: Intuitive, idiomatic Go APIs
- **Production Ready**: Thread-safe, comprehensive error handling
- **Type Safety**: Generic support with compile-time checks
- **Zero Dependencies**: Standard library only

## Design Principles

### 1. Simplicity
- Minimal, intuitive APIs
- Sensible defaults for common use cases
- Explicit, self-documenting code

### 2. Security
- Validate all inputs at API boundaries
- Enforce resource limits to prevent abuse
- No unsafe operations; sanitize outputs

### 3. Performance
- Optimized for production workloads
- Smart caching, memory pooling
- Benchmark critical paths

### 4. Thread Safety
- All public methods goroutine-safe
- Concurrency guarantees documented

### 5. Backward Compatibility
- Maintain API stability across minor versions
- Deprecate features gracefully
- No breaking changes without major version bump
- Provide clear migration paths

## API Design Patterns

### 1. Convenience API (Simple, No Limits)
```go
result, err := Process(input)
```
- Quick usage, minimal boilerplate
- No configuration required

### 2. Processor API (Advanced, Configurable)
```go
config := Config{
    MaxSize:     5 * 1024 * 1024,
    CacheEnabled: true,
    Timeout:     30 * time.Second,
}
processor := NewProcessor(config)
```
- Fine-grained control, explicit lifecycle
- Thread-safe, resource pooling

### 3. Configuration Pattern
```go
type Config struct {
    MaxSize     int           // Default: 10MB
    Timeout     time.Duration // Default: 30s
    Concurrency int           // Default: runtime.NumCPU()
}
```
- Explicit config structs with validation
- Sensible, safe defaults

### 4. Error Handling
```go
var (
    ErrInvalidInput = errors.New("invalid input")
    ErrTimeout      = errors.New("operation timeout")
)
```
- Return errors, never panic
- Clear, actionable messages
- Wrap errors with %w

## Quality Requirements
- **Security**: Validate inputs, enforce limits, sanitize outputs
- **Performance**: Minimize allocations, benchmark hot paths
- **Reliability**: Graceful degradation, resource leak prevention
- **Type Safety**: Leverage generics, avoid interface{} in public APIs
- **Compatibility**: Maintain backward compatibility, document minimum Go version

## Language
- English for all code, comments, and examples
- Clear, concise technical writing

## Development Best Practices

### Always
- Goroutine-safe APIs with documented concurrency
- Validate inputs at API boundaries
- Return errors, never panic
- Use explicit config structs
- Proper cleanup (defer processor.Close())
- Extend types via methods

### Never
- Add external dependencies
- Break backward compatibility without major version bump
- Use functional options
- Omit resource cleanup or thread-safety guarantees
- Write verbose or redundant code
- Name variables same as imported packages

## Common Mistakes to Avoid
- ❌ Functional options for configuration
- ❌ Standalone stateful functions
- ❌ Exposing internal details
- ❌ Panics instead of errors
- ❌ Ignoring resource cleanup
- ❌ Undocumented concurrency
- ❌ Overly verbose implementations

## Development Workflow
1. Design public API first
2. Implement both convenience and processor tiers
3. Add explicit config structs
4. Validate inputs and enforce limits
5. Handle errors explicitly
6. Ensure thread safety
7. Write minimal, essential code
8. Test thoroughly (unit, integration, concurrency)
9. Document with Godoc and examples
10. Benchmark critical paths

## Code Review Checklist
- [ ] No external dependencies
- [ ] Godoc for all public APIs
- [ ] Thread-safety documented
- [ ] Errors returned, no panics
- [ ] Proper resource cleanup
- [ ] Tests cover edge cases and concurrency
- [ ] Examples demonstrate correct usage
- [ ] Performance verified
- [ ] Input validation at API boundaries
- [ ] Backward compatible
- [ ] Security reviewed

## Success Criteria
- ✅ Intuitive, documented public API
- ✅ Both convenience and processor APIs implemented
- ✅ Validated inputs with clear errors
- ✅ Thread-safe
- ✅ >80% test coverage
- ✅ Performance benchmarks met
- ✅ No external dependencies
- ✅ Backward compatible
- ✅ README updated
- ✅ Examples demonstrate correct usage

## Success Metrics
- Ease of Use: <5 lines for basic usage
- Performance: 2-3x faster than alternatives
- Reliability: Zero critical production bugs
- Maintainability: Minimal, clean code
- Security: No vulnerabilities
- Adoption: Positive feedback, growing usage

---

**Remember**: Prioritize developer experience, security, and performance. APIs must be intuitive, safe, and production-optimized.