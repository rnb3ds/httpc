# Technology Stack

## Language & Version

- **Go 1.24+** - Modern Go with latest features and performance improvements

## Architecture

- **Layered architecture** with clear separation of concerns:
  - Public API layer (`client.go`, `types.go`, `public_options.go`)
  - Internal engine layer (`internal/engine/`)
  - Specialized internal packages for specific functionality

## Core Dependencies

- Standard library only - no external dependencies for core functionality
- Uses `net/http`, `crypto/tls`, `context` packages extensively

## Internal Package Structure

- `internal/engine/` - Core HTTP engine and request/response processing
- `internal/cache/` - Caching mechanisms
- `internal/circuitbreaker/` - Circuit breaker implementation
- `internal/concurrency/` - Concurrency control and semaphore management
- `internal/connection/` - Connection pooling
- `internal/memory/` - Memory management and buffer pooling
- `internal/pool/` - Object pooling for performance
- `internal/security/` - Security validation and protection

## Build & Test Commands

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test categories
go test ./... -run TestSecurity_
go test ./... -run TestRetry_
go test ./... -run TestOptions_

# Run comprehensive tests
go test ./internal/engine/ -run Comprehensive
```

### Building

```bash
# Build the module
go build ./...

# Verify module
go mod verify
go mod tidy
```

### Linting & Quality

```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run staticcheck (if available)
staticcheck ./...
```

## Performance Considerations

- Zero-allocation buffer pooling to reduce GC pressure
- Connection reuse and pooling for efficiency
- Atomic operations for thread-safe counters
- Goroutine-safe design throughout
