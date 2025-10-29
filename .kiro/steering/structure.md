# Project Structure

## Root Level Organization

### Core Library Files
- `client.go` - Main client interface and implementation
- `types.go` - Core types, Response, Config, and validation
- `public_options.go` - Public API option methods (WithHeader, WithJSON, etc.)
- `download.go` - File download functionality
- `config_presets.go` - Security configuration presets

### Testing Strategy
- `*_test.go` - Comprehensive test coverage with categorized test functions
- Test naming convention: `TestCategory_SpecificFeature` (e.g., `TestSecurity_URLValidation`)
- `test_helpers.go` - Shared testing utilities
- Comprehensive test files for complex scenarios

### Documentation Structure
- `README.md` - Main documentation with examples
- `docs/` - Detailed documentation by topic
- `examples/` - Organized example code by complexity level
- `USAGE_GUIDE.md` - Complete usage reference
- `QUICK_REFERENCE.md` - Cheat sheet for common tasks

## Internal Architecture

### Package Organization
```
internal/
├── engine/          # Core HTTP engine
├── cache/           # Caching mechanisms  
├── circuitbreaker/  # Circuit breaker pattern
├── concurrency/     # Concurrency control
├── connection/      # Connection pooling
├── memory/          # Memory management
├── pool/            # Object pooling
└── security/        # Security validation
```

### File Naming Conventions
- `*.go` - Implementation files
- `*_test.go` - Unit tests
- `*_comprehensive_test.go` - Integration/comprehensive tests
- `manager.go` - Management/coordination logic
- `validator.go` - Validation logic

## Code Organization Principles

### Public API Design
- Clean separation between request methods and option methods
- All public types in `types.go`
- Option methods follow `With*` naming pattern
- Security-first validation in all public interfaces

### Internal Implementation
- Each internal package has single responsibility
- Comprehensive error handling and validation
- Thread-safe operations throughout
- Performance-optimized with pooling and reuse

### Testing Structure
- Tests organized by functionality categories
- Security tests for all validation logic
- Performance tests for concurrency scenarios
- Edge case testing for robustness

### Documentation Standards
- Examples progress from simple to complex
- Security considerations highlighted
- Performance implications documented
- Common patterns and best practices included