# ADR 0006: Cross-Platform Consistency Architecture

**Status:** Proposed
**Date:** 2026-01-27
**Deciders:** Development Team

## Context

We support three runtime environments:
1. **Cloudflare Workers** (TinyGo WASM) - Production deployment
2. **Wazero Server** (Go + WASM) - Local development, native tools
3. **CLI** (Go native) - Testing, direct rendering

### Current Issues

Multiple cross-platform inconsistencies have emerged:

**Issue 1: API Response Format Mismatch**
- Handler returned `{count, examples}`
- Wazero server returned `[examples]` array directly
- Frontend failed to load examples on wazero

**Issue 2: Missing Endpoints**
- `/deck/` endpoints existed in wazero but not in shared handler
- Caused 404 errors on Cloudflare production
- Share button didn't work in production

**Issue 3: Code Duplication**
- Deck slide rendering logic duplicated between wazero and handler
- Same processing steps repeated in different places
- Risk of divergence over time

**Issue 4: Build System Gaps**
- Taskfile didn't track demo/*.html as source for cloudflare build
- Changes to frontend required manual rebuild
- Variable naming inconsistency (SRC_DIR vs SOURCE_DIR)

**Issue 5: Platform-Specific Code Paths**
```go
// handler/handler.go - WASM pipeline
p := pipeline.NewWASMPipeline()
p.WithDimensions(1920, 1080)
result, err := p.Process(r.Context(), source, pipeline.FormatSVG)

// cmd/wazero/main.go - Native pipeline
result, err := s.pipeline.ProcessWithWorkDir(r.Context(), source, format, workDir)
```

Different code paths for same functionality increase maintenance burden.

## Decision Drivers

1. **DRY Principle** - Don't Repeat Yourself across platforms
2. **Single Source of Truth** - One handler implementation for all platforms
3. **Type Safety** - Compile-time guarantees of API compatibility
4. **Testing** - Easier to test when logic isn't duplicated
5. **Maintainability** - Changes apply to all platforms automatically

## Considered Options

### Option 1: Shared Handler Library (Recommended)

**Structure:**
```
handler/
  handler.go         # All HTTP handlers
  routes.go          # Route registration
  deck.go            # Deck-specific handlers
  examples.go        # Example listing/fetching
  process.go         # Deck processing
  types.go           # Shared request/response types

runtime/
  runtime.go         # Storage + Pipeline abstraction
  storage_*.go       # Storage implementations
  pipeline_*.go      # Pipeline implementations (new)
```

**Changes:**
1. Add `runtime.Pipeline` interface alongside `runtime.Storage`
2. Shared handler uses runtime abstractions exclusively
3. Each platform initializes runtime with appropriate implementations
4. Remove all platform-specific handler code

**Benefits:**
- Single handler codebase for all platforms
- Compile-time API compatibility
- Runtime abstractions prevent code duplication
- Easy to add new platforms

**Drawbacks:**
- Requires refactor of existing wazero handlers
- Need to unify pipeline interfaces

### Option 2: Code Generation

Generate platform-specific handlers from shared templates.

**Benefits:**
- Flexibility per platform
- Can optimize for each runtime

**Drawbacks:**
- Build complexity
- Generated code harder to debug
- Still requires unified API design

### Option 3: Acceptance Testing Only

Just add comprehensive E2E tests for each platform.

**Benefits:**
- No refactor needed
- Catch issues in testing

**Drawbacks:**
- Doesn't prevent duplication
- Tests catch issues late
- Maintenance burden remains

## Decision

**Choose Option 1: Shared Handler Library**

Rationale:
1. Already using runtime.Storage abstraction successfully
2. Extending to runtime.Pipeline is natural evolution
3. Prevents future inconsistencies at source
4. Aligns with existing architecture patterns

## Implementation Plan

### Phase 1: Pipeline Abstraction (High Priority)

Create `runtime.Pipeline` interface:

```go
// runtime/pipeline.go
type Pipeline interface {
    Process(ctx context.Context, source []byte, format Format) (*Result, error)
    ProcessWithWorkDir(ctx context.Context, source []byte, format Format, workDir string) (*Result, error)
    SupportedFormats() []Format
}

type Format string
const (
    FormatSVG Format = "svg"
    FormatPNG Format = "png"
    FormatPDF Format = "pdf"
)

type Result struct {
    Slides     [][]byte
    SlideCount int
    Title      string
}
```

Implementations:
- `WASMPipeline` - Cloudflare Workers, uses internal WASM processors
- `NativePipeline` - Wazero/CLI, shells out to decksh/svgdeck/pngdeck/pdfdeck

### Phase 2: Handler Consolidation

1. Move all wazero-specific handlers to shared handler package
2. Update to use `runtime.Pipeline()` instead of direct pipeline access
3. Remove duplicated code from cmd/wazero/main.go
4. Ensure handler works with both pipeline types

### Phase 3: Response Type Safety

Define shared types:

```go
// handler/types.go
type ExamplesResponse struct {
    Examples []Example `json:"examples"`
    Count    int       `json:"count"`
}

type Example struct {
    Name       string `json:"name"`
    Path       string `json:"path"`
    Renderable bool   `json:"renderable"`
}
```

Use consistently in all handlers.

### Phase 4: Build System Improvements

1. Add validation that shared handler doesn't import platform-specific packages
2. Enforce consistent variable naming in Taskfile
3. Add pre-commit hook to verify handler builds for all platforms

### Phase 5: Testing Strategy

1. Shared handler unit tests (mock runtime)
2. Integration tests for each runtime implementation
3. E2E tests covering all platforms with same test cases

## Consequences

### Positive

- **Consistency**: All platforms share same handler logic
- **Maintainability**: Fix once, works everywhere
- **Type Safety**: Compiler catches API mismatches
- **Testing**: Easier to test shared code
- **Onboarding**: New developers see clear architecture

### Negative

- **Upfront Work**: Requires refactoring existing wazero handlers
- **Abstraction Cost**: Extra layer of indirection
- **Runtime Flexibility**: Harder to optimize per-platform

### Risks

1. **Breaking Changes**: Refactor might introduce bugs
   - Mitigation: Comprehensive test coverage before/after

2. **Performance**: Runtime abstraction might have overhead
   - Mitigation: Benchmark before/after, optimize if needed

3. **Platform Limitations**: Some features might not work on all platforms
   - Mitigation: Feature flags in runtime interface

## Success Metrics

1. Zero API format mismatches between platforms
2. All handlers in shared handler package
3. No duplicated endpoint logic
4. E2E tests pass on all platforms
5. Build system catches demo/ changes automatically

## References

- ADR 0003: Runtime Storage Abstraction (established pattern)
- ADR 0004: Font Fetching (uses storage abstraction)
- Current handler: `handler/handler.go`
- Wazero server: `cmd/wazero/main.go`

## Next Steps

1. Review and approve this ADR
2. Create runtime.Pipeline interface (Phase 1)
3. Implement WASMPipeline wrapper around existing code
4. Implement NativePipeline wrapper around existing code
5. Update handler to use runtime.Pipeline
6. Remove duplicated handlers from wazero
7. Add comprehensive tests
