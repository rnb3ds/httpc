---
type: "always_apply"
---

# Project Structure & Code Organization

## Critical Architecture Constraints

### Package Structure Rules
- ❌ **NEVER create new root-level packages** — only internal/ subdirectories allowed
- ❌ **NEVER import root package from internal** — avoids circular dependencies
- ✅ **Root package = public API only** — all implementation goes in internal/
- ✅ **Zero external dependencies** — standard library only
- ✅ **Single responsibility** — one feature per file, clear separation of concerns
- ✅ **Thread-safe by default** — all public types safe for concurrent use

### Directory Layout
```
project/
├── *.go                   # Public API files (root package only)
├── *_test.go              # Co-located unit tests
├── config.go              # Configuration structs
├── errors.go              # All error definitions
├── test_helpers.go        # Shared test utilities
├── CHANGES.md             # Release version logs
├── change_logs.md         # Development tracking logs
├── internal/              # Private implementation packages
│   ├── cache/             # Caching mechanisms
│   ├── engine/            # Core processing logic
│   └── parser/            # Parsing functionality
├── examples/              # Runnable examples with `main()`
└── docs/                  # Documentation files
```

## File Placement Decision Matrix

### Root Package (Public API)
**Include in root when:**
- Defining public types (Processor, Config, interfaces)
- Exposing public methods/functions to users
- Package-level convenience functions

**Required root files:**
- `feature.go` - main feature implementation
- `config.go` - configuration structs with validation
- `errors.go` - all error definitions
- `helpers.go` - package-level utilities

### Internal Package (Implementation)
**Include in `internal/` when:**
- Implementing core logic and algorithms
- Utilities, helpers, and supporting services
- Performance-critical implementations

**Rules:**
- Single responsibility per package
- Co-locate tests with implementation
- Never expose internal types in public API
- Use descriptive package names

### Development Tracking
- Record all modifications in `change_logs.md`
- Include: date, description, files modified, reason

## Naming Standards

### File Naming
- Implementation: `feature_name.go` (snake_case)
- Tests: `feature_name_test.go`
- Examples: `descriptive_name.go`

### Code Element Naming
- **Public types**: `PascalCase` (`Processor`, `Config`)
- **Private types**: `camelCase` (`tokenInfo`, `cacheEntry`)
- **Methods**: Verb form (`Process()`, `Validate()`, `Close()`)
- **Constants**: `PascalCase` for exported, `camelCase` for internal

### Forbidden Patterns
- ❌ Generic prefixes: "enhanced", "optimized", "new", "improved"
- ❌ Variable names matching imported packages
- ✅ Descriptive, purpose-driven names

## Implementation Workflow

### Adding a New Feature
1. Design public API in root feature.go
2. Implement core logic in appropriate internal/ package
3. Add configuration in config.go with validation
4. Define errors in errors.go
5. Create tests in feature_test.go
6. Add examples in examples/
7. Update documentation and README

### Modifying Existing Feature
1. Identify location: root for API changes, internal/ for implementation
2. Maintain backward compatibility
3. Update tests and examples
4. Document changes

## Code Placement Quick Reference

| What | Location | Example File |
|------|----------|--------------|
| Public API | Root `feature.go` | `processor.go`, `json.go` |
| Core logic | `internal/engine/` | `internal/engine/pipeline.go` |
| Caching | `internal/cache/` | `internal/cache/memory.go` |
| Utilities | `internal/utils/` | `internal/utils/helpers.go` |
| Unit tests | Adjacent `*_test.go` | `processor_test.go` |
| Test utilities | `test_helpers.go` | Root level only |
| Examples | `examples/name.go` | `examples/basic_usage.go` |
| Error definitions | `errors.go` | Root level only |
| Configuration | `config.go` | Root level only |

## Quality Validation Checklist
- [ ] Correct package placement
- [ ] No new root-level packages
- [ ] No circular dependencies
- [ ] Thread-safety maintained
- [ ] All errors defined in errors.go
- [ ] Tests co-located
- [ ] Examples demonstrate usage
- [ ] Comprehensive test passed
- [ ] Project built successfully
- [ ] Documentation update
- [ ] Update `change_logs.md`

## Critical Anti-Patterns

### Architecture Violations
- Standalone functions instead of methods
- Exposing internal implementation in public API
- Creating new root-level packages
- Importing root from internal

### Organization Mistakes
- Mixing public API and implementation
- Forgetting tests
- Not updating examples
- Defining errors inline

### File Management
- Files >1000 lines without logical split
- Multiple responsibilities per file
- Generic or unclear file names
- Missing co-located tests

## AI-Specific Guidelines

### When Writing Code
- Check existing structure before creating new files
- Follow existing patterns and naming conventions
- Maintain single responsibility per file

### When Modifying Code
- Preserve architecture patterns
- Update all related files
- Maintain backward compatibility
- Document significant changes

---

**Key Principle**: Maintain clean separation between public API (root) and implementation (internal), with zero external dependencies and consistent organization patterns.