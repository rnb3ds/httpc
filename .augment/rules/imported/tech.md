---
type: "always_apply"
---

# Go Technical Specifications & Development Guidelines

## Technology Stack

### Core Constraints
- **Go Version**: 1.24+ (use modern features including generics)
- **Zero Dependencies**: Standard library ONLY - never add external packages
- **Package Structure**: Only internal/ subdirectories; never create new root-level packages
- **Thread Safety**: ALL public APIs must be goroutine-safe with documented guarantees
- **Module Path**: `github.com/cybergodev/*`
- **Critical Rule**: Never suggest or add third-party packages under any circumstances

### Standard Library Usage
Prefer these stdlib packages:
- `sync`: RWMutex, Pool, Once, atomic operations
- `fmt`, `errors`: Error handling with `%w` wrapping
- `strings`, `bytes`: String/byte manipulation
- `encoding/json`: JSON operations
- `crypto/sha256`: Content-addressable hashing
- `testing`: Tests, benchmarks, examples

## Development Change Log
- Record changes in `change_logs.md`.
- Don’t overwrite.
- Insert at the appropriate top position.
- All log contents are in English.
- Clear, concise technical writing.
- Summarize after every change.

### Format Description
Each entry should include the following information:
- **Date**: The date when the change was completed
- **Version**: The related version number (if applicable)
- **Type**: Type of change (optimization / new feature / bug fix / refactor, etc.)
- **Affected Files**: List of the main files modified
- **Summary**: A brief description of the change
- **Details**: Detailed explanation of the change and its impact

### Document Maintenance
- This document should be updated in sync with code changes
- Major architectural changes require detailed explanations of design decisions
- Performance optimizations must include benchmark results
- Security fixes must describe the vulnerability impact and the remediation approach

## Go Language Specifications

### Naming Conventions
- **Exported**: PascalCase (`Processor`, `NewClient`)
- **Unexported**: camelCase (`buildRequest`, `validateURL`)
- **Method receivers**: Single letter matching type (`p *Processor`)
- **Variables**: Short in small scopes (`i`, `err`), descriptive in large scopes
- **CRITICAL**: Never name variables same as imported packages

### Test Functions
- Format: `TestFeature_Scenario` (e.g., `TestRetry_ExponentialBackoff`)
- Benchmarks: `BenchmarkOperation` (e.g., `BenchmarkMarshal`)

### Error Handling
- Define ALL errors in `errors.go`
- Use typed errors for programmatic handling
- Always wrap with context: `fmt.Errorf("failed to parse: %w", err)`
- Return errors, NEVER panic (except unrecoverable init failures)
- Check errors immediately after function calls
- Return early on errors to avoid deep nesting
- Validate ALL inputs at API boundaries

```go
result, err := operation()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
```

### Configuration Pattern
- Use explicit config structs, never functional options
```go
config := Config{
    MaxInputSize: 5 * 1024 * 1024,
    CacheEnabled: true,
    Timeout:      30 * time.Second,
}
processor := New(config)
```
- Validate configuration before use
- Config must be immutable after creation
- Provide sensible defaults and document all fields

## Concurrency

### Thread Safety
- `atomic`: Simple counters/flags in hot paths
- `sync.RWMutex`: Read-heavy workloads (prefer read locks)
- `sync.Mutex`: Exclusive access (avoid in hot paths)
- `sync.Pool`: Object reuse for frequent allocations
- `sync.Once`: One-time initialization

### Rules
- Keep critical sections minimal
- Use read locks when possible
- Never use Mutex in hot paths; prefer atomic operations
- Document thread-safety guarantees in godoc
```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]CacheEntry
}

func (c *Cache) Get(key string) (CacheEntry, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    entry, ok := c.items[key]
    return entry, ok
}
```

## Testing

### Testing Requirements
- Unit tests: `<feature>_test.go` alongside implementation
- Integration tests: `<feature>_comprehensive_test.go`
- Test helpers: `test_helpers.go` for shared utilities
- Use table-driven tests with parallel execution
- Aim for >80% coverage and test edge cases, concurrent scenarios, and errors
```go
func TestFeature(t *testing.T) {
    tests := []struct { name, input, want string; wantErr bool }{
        {"valid input", "test", "result", false},
        {"empty input", "", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got, err := Feature(tt.input)
            if (err != nil) != tt.wantErr { t.Errorf("error = %v, wantErr %v", err, tt.wantErr) }
            if got != tt.want { t.Errorf("got %v, want %v", got, tt.want) }
        })
    }
}
```

## Security

### Input Validation
- Validate and sanitize all inputs at API boundaries
- Enforce maximum input size limits
- Use `crypto/sha256` for hashing cache keys or content addressing

## Development Commands
```bash
go fmt ./...
go vet ./...
go test -v -cover ./...
go test -bench=. -benchmem ./...
go build
go mod tidy
```

## Critical Rules for AI

### NEVER Do
- Add external dependencies
- Create root-level packages
- Use functional options instead of config structs
- Panic instead of returning errors
- Name variables same as imported packages
- Use fmt.Sprintf or lock mutexes in hot paths
- Allocate new slices/maps unnecessarily
- Break backward compatibility in minor versions

### ALWAYS Do
- Maintain thread-safety for all public APIs
- Validate inputs and configurations at boundaries
- Use explicit config structs
- Document thread-safety guarantees
- Pre-allocate buffers with known sizes
- Benchmark performance-critical code
- Write tests for edge cases and concurrency

---

**Note**: This document serves as a comprehensive guide for AI-assisted Go development. Follow these guidelines strictly to ensure consistent, high-quality, performant, and maintainable Go code.