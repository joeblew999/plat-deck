# ADR 001: Deck Processing Architecture Refactoring

**Status**: Proposed
**Date**: 2026-01-27
**Authors**: Development Team
**Deciders**: Project Maintainers

## Context

### Current Problems

We've encountered multiple production bugs caused by duplicated logic across handler endpoints:

1. **WorkDir Bug**: Examples with external data files (`.d`, `.kml`) failed to render because WorkDir support was implemented in `/deck/slide/` but not `/process`
   - Files: `aapl/aapl.dsh`, `africa/geoscale.dsh`, `astrobang/astrobang.dsh`
   - Root cause: WorkDir calculation duplicated in two places

2. **Image Path Bug**: Images in demo UI didn't load because SVG link rewriting was in `/deck/slide/` but not `/process`
   - File: `aid/aid.dsh`
   - Root cause: `rewriteSVGLinks()` called in one place but not the other

3. **Share Button Bug**: Demo UI share button broke due to flag-based state management
   - Root cause: Event handlers need to know about programmatic vs user-initiated changes

### Current Architecture

```
handler/handler.go
├── handleProcess()          ← Used by demo UI
│   ├── Read source
│   ├── Expand imports
│   ├── Calculate workDir         ← Added to fix bug #1
│   ├── Call pipeline
│   ├── Rewrite SVG links         ← Added to fix bug #2
│   └── Return JSON
│
└── handleDeckSlide()        ← Used by /deck/ endpoint
    ├── Read source
    ├── Expand imports
    ├── Calculate workDir         ← Added earlier
    ├── Call pipeline
    ├── Rewrite SVG links         ← Added earlier
    └── Return raw SVG
```

**Problem Pattern**: When logic needs to be added, it must be duplicated in both handlers. Forgetting one causes a bug.

### Why This Happened

The handler package violates Single Responsibility Principle:
- HTTP concerns (routes, CORS, content-type)
- Business logic (import expansion, workDir calculation)
- Data transformation (SVG link rewriting)
- Output formatting (JSON vs raw SVG)

All mixed together with no clear boundaries.

## Decision Drivers

1. **Prevent duplicate logic bugs** - Primary concern
2. **Maintainability** - Easy to understand where logic lives
3. **Testability** - Can test business logic without HTTP
4. **Cross-platform compatibility** - Works on Cloudflare WASM, wazero, native CLI
5. **Risk** - Can't break production during refactor

## Options Considered

### Option 1: Quick Fix - Extract ProcessCompleteDeck()

**Approach**: Create single function that does all deck processing

```go
// handler/processing.go
func ProcessCompleteDeck(ctx context.Context, source []byte, sourcePath string, format runtime.Format) (*ProcessedDeck, error) {
    // 1. Expand imports (WASM only)
    source, err := expandImports(ctx, source, sourcePath)
    if err != nil {
        return nil, err
    }

    // 2. Calculate workDir (native only)
    workDir := calculateWorkDir(sourcePath)

    // 3. Process through pipeline
    result, err := runtime.GetPipeline().ProcessWithWorkDir(ctx, source, format, workDir)
    if err != nil {
        return nil, err
    }

    // 4. Rewrite links and images
    if sourcePath != "" {
        for i := range result.Slides {
            result.Slides[i] = rewriteSVGLinks(result.Slides[i], sourcePath)
        }
    }

    return &ProcessedDeck{Result: result, SourcePath: sourcePath}, nil
}
```

Then both handlers just call this:
```go
func handleProcess(w http.ResponseWriter, r *http.Request) {
    source, sourcePath := readRequest(r)
    deck, err := ProcessCompleteDeck(r.Context(), source, sourcePath, runtime.FormatSVG)
    // Format as JSON
}

func handleDeckSlide(w http.ResponseWriter, r *http.Request, examplePath string, slideNum int) {
    source := readSource(examplePath)
    deck, err := ProcessCompleteDeck(r.Context(), source, examplePath, runtime.FormatSVG)
    // Return raw SVG
}
```

**Pros**:
- Low risk - just extracting existing code
- Prevents future bugs - logic in one place
- Can implement in 1-2 hours
- Easy to test in isolation
- Doesn't change external APIs

**Cons**:
- Still mixed concerns in handler package
- Doesn't solve demo UI state management
- Band-aid on deeper architecture issues

**Effort**: Low (2-4 hours)
**Risk**: Low
**Impact**: Medium (prevents bug recurrence)

### Option 2: Medium Refactor - Domain Layer

**Approach**: Separate business logic from HTTP handling

```
handler/
├── domain/          ← NEW: Business logic
│   ├── deck.go             type Deck, ProcessDeck()
│   ├── imports.go          expandImports()
│   └── transforms.go       rewriteSVGLinks()
├── http/            ← NEW: HTTP-specific
│   ├── routes.go           RegisterHandlers()
│   ├── process.go          handleProcess()
│   ├── deck.go             handleDeck*()
│   └── middleware.go       CORS, etc
└── handler.go       ← KEEP: Backward compat exports
```

**Pros**:
- Clear separation of concerns
- Business logic testable without HTTP
- Easier to add new output formats (gRPC, CLI)
- Standard Go project layout
- Still prevents duplicate logic

**Cons**:
- More files to navigate
- Migration effort for existing code
- Need to maintain backward compat during transition
- Overkill if this is a small/stable project

**Effort**: Medium (1-2 days)
**Risk**: Medium (imports change, tests need updates)
**Impact**: High (better structure for future work)

### Option 3: Long-Term - Domain-Driven Design

**Approach**: Full DDD with domain objects and transformers

```go
// domain/deck.go
type Deck struct {
    Source      []byte
    SourcePath  string
    Slides      []Slide
    Assets      map[string]Asset
    Metadata    DeckMetadata
}

type Slide struct {
    Number      int
    SVG         []byte
    Links       []Link
    Images      []Image
}

// Processing pipeline
func (d *Deck) Process(opts ...ProcessOption) error {
    for _, opt := range opts {
        if err := opt.Apply(d); err != nil {
            return err
        }
    }
    return nil
}

// Transform options
func WithImportExpansion() ProcessOption { ... }
func WithWorkDir(dir string) ProcessOption { ... }
func WithLinkRewriting() ProcessOption { ... }
func WithAssetDiscovery() ProcessOption { ... }

// Usage
deck := &Deck{Source: source, SourcePath: path}
err := deck.Process(
    WithImportExpansion(),
    WithWorkDir(dir),
    WithLinkRewriting(),
    WithAssetDiscovery(),
)
```

**Pros**:
- Cleanest architecture - each concern isolated
- Composable transformations
- Easy to add new features (caching, validation, etc.)
- Test each transformation independently
- Explicit about what processing happens
- Supports advanced features (lazy loading, streaming)

**Cons**:
- Significant refactor effort
- Potential performance overhead (copying data between structures)
- Over-engineering if requirements are stable
- Learning curve for new contributors
- Risk of breaking existing functionality

**Effort**: High (3-5 days)
**Risk**: High (touching core processing logic)
**Impact**: Very High (sets foundation for future features)

### Option 4: Status Quo - Just Document

**Approach**: Keep current code, add comments and documentation

```go
// handleProcess processes decksh source to SVG slides
// IMPORTANT: Keep in sync with handleDeckSlide:
//  - Import expansion
//  - WorkDir calculation
//  - SVG link rewriting
// See docs/processing-pipeline.md for details
func handleProcess(w http.ResponseWriter, r *http.Request) {
    // ... existing code ...
}
```

**Pros**:
- Zero refactor effort
- No risk of breaking changes
- Code works as-is

**Cons**:
- Will have bugs again in the future
- Comments go stale
- New contributors won't read docs
- Doesn't scale as project grows
- Technical debt accumulates

**Effort**: Very Low (1 hour for docs)
**Risk**: None (to codebase)
**Impact**: Very Low (documentation only)

## Decision Matrix

| Criteria | Option 1 (Extract) | Option 2 (Domain) | Option 3 (DDD) | Option 4 (Document) |
|----------|-------------------|-------------------|----------------|---------------------|
| Prevents bugs | ✅ High | ✅ High | ✅ Very High | ❌ None |
| Maintainability | ✅ Good | ✅ Very Good | ✅ Excellent | ❌ Poor |
| Testability | ✅ Good | ✅ Very Good | ✅ Excellent | ❌ Current |
| Implementation effort | ✅ Low | ⚠️ Medium | ❌ High | ✅ Very Low |
| Risk | ✅ Low | ⚠️ Medium | ❌ High | ✅ None |
| Future flexibility | ⚠️ Limited | ✅ Good | ✅ Excellent | ❌ None |
| Learning curve | ✅ Easy | ⚠️ Moderate | ❌ Steep | ✅ None |

## Recommendation

**Choose Option 3 (Long-Term DDD)** with **phased implementation**:

### Phase 1 (Week 1): Foundation
Implement Option 1 to stop the bleeding:
- Extract `ProcessCompleteDeck()`
- Consolidate duplicate logic
- Ship to production
- **Goal**: Prevent bugs while planning bigger refactor

### Phase 2 (Week 2-3): Domain Layer
Implement Option 2 structure:
- Create `domain/` package with Deck type
- Move business logic out of handlers
- Keep HTTP handlers thin
- Migrate gradually, one handler at a time
- **Goal**: Clear separation of concerns

### Phase 3 (Week 4+): Transformers
Add composable processing from Option 3:
- Implement `ProcessOption` pattern
- Add transformer functions
- Support custom processing pipelines
- **Goal**: Flexible, testable architecture

### Why Phased Approach?

1. **De-risks Option 3**: Each phase delivers value independently
2. **Proves the concept**: Can stop after Phase 1 if that's sufficient
3. **Allows learning**: Discover issues early, adjust approach
4. **Maintains velocity**: Ship features while refactoring
5. **Reversible**: Can rollback any phase without losing others

### Why Not Just Option 1?

While Option 1 is low-risk and solves immediate bugs, it doesn't address root causes:

- **Scalability**: Adding PDF/PNG output will duplicate logic again
- **Testing**: Still hard to test business logic without HTTP mocks
- **Clarity**: New contributors still confused about handler responsibilities
- **Growth**: Project will outgrow this as features are added

Option 3 (via phases) sets up for long-term success.

### Why Not Stop at Option 2?

Option 2 is good, but we're already doing the refactor work. The incremental effort from Option 2 → Option 3 is small compared to the benefits:

- **Explicit transformations**: Code shows exactly what processing happens
- **Testability**: Each transformer tested in isolation
- **Composability**: Can create custom pipelines for different use cases
- **Future features**: Easy to add caching, validation, streaming, etc.

## Consequences

### Positive

- **Bug Prevention**: Impossible to forget logic in one handler (single code path)
- **Better Testing**: Can test deck processing without HTTP layer
- **Clearer Code**: Each layer has single responsibility
- **Easier Onboarding**: New contributors understand structure quickly
- **Future-Proof**: Easy to add new features (PDF, PNG, streaming, caching)
- **Cross-Platform**: Same logic works in Cloudflare, wazero, CLI

### Negative

- **Initial Effort**: 3-5 days of refactoring work
- **Migration Risk**: Could break existing functionality if not careful
- **More Files**: More navigation required (mitigated by clear structure)
- **Learning Curve**: Team needs to understand new patterns
- **Over-Engineering?**: May be overkill if project stays small

### Mitigation Strategies

1. **Phased rollout**: Implement in 3 phases, can stop anytime
2. **Comprehensive tests**: Add tests before refactoring, ensure they still pass
3. **Feature flags**: Use flags to toggle between old/new implementations
4. **Documentation**: Document new structure in CLAUDE.md
5. **Code review**: All phases reviewed before merging
6. **Rollback plan**: Keep old handlers until new code proven in production

## Implementation Plan

### Phase 1: Extract Function (Days 1-2)

**Goals**:
- Stop duplicate logic bugs
- No API changes
- Ship to production quickly

**Tasks**:
1. Extract `ProcessCompleteDeck()` in handler package
2. Update `handleProcess()` to use it
3. Update `handleDeckSlide()` to use it
4. Add unit tests for `ProcessCompleteDeck()`
5. Deploy to production
6. Monitor for issues

**Success Criteria**:
- All tests pass
- No regressions in production
- Both endpoints use same code path

### Phase 2: Domain Layer (Days 3-10)

**Goals**:
- Separate business logic from HTTP
- Improve testability
- Standard Go project structure

**Tasks**:
1. Create `handler/domain/` package
2. Define `Deck` type with methods
3. Move `ProcessCompleteDeck()` → `domain.ProcessDeck()`
4. Move `expandImports()` → `domain/imports.go`
5. Move `rewriteSVGLinks()` → `domain/transforms.go`
6. Create `handler/http/` package
7. Move HTTP handlers to `handler/http/`
8. Update handler.go exports for backward compat
9. Update tests
10. Deploy to production

**Success Criteria**:
- Business logic has no HTTP dependencies
- Can test domain logic without http.ResponseWriter
- All existing tests still pass
- No import changes for external consumers

### Phase 3: Transformers (Days 11-15)

**Goals**:
- Composable processing pipeline
- Explicit transformations
- Support custom processing flows

**Tasks**:
1. Define `ProcessOption` interface
2. Implement transformers:
   - `WithImportExpansion()`
   - `WithWorkDir(dir)`
   - `WithLinkRewriting(basePath)`
   - `WithAssetDiscovery()`
3. Update `Deck.Process()` to use options
4. Migrate handlers to use transformer pattern
5. Add tests for each transformer
6. Add integration tests for common pipelines
7. Document transformer usage
8. Deploy to production

**Success Criteria**:
- Can compose custom processing pipelines
- Each transformer tested independently
- Easy to add new transformations
- Documentation shows common patterns

## Testing Strategy

### Phase 1 Tests
```go
func TestProcessCompleteDeck(t *testing.T) {
    tests := []struct{
        name       string
        source     []byte
        sourcePath string
        wantErr    bool
    }{
        {"simple deck", []byte("deck\nslide\neslide\nedeck"), "", false},
        {"with images", readFile("aid/aid.dsh"), "aid/aid.dsh", false},
        {"with data", readFile("aapl/aapl.dsh"), "aapl/aapl.dsh", false},
    }
    // ...
}
```

### Phase 2 Tests
```go
func TestDeckProcess(t *testing.T) {
    deck := &domain.Deck{
        Source: []byte("deck\nslide\neslide\nedeck"),
    }
    err := deck.Process()
    require.NoError(t, err)
    assert.Len(t, deck.Slides, 1)
}
```

### Phase 3 Tests
```go
func TestTransformers(t *testing.T) {
    t.Run("WithImportExpansion", func(t *testing.T) {
        deck := &domain.Deck{Source: sourceWithImports}
        err := deck.Process(domain.WithImportExpansion())
        // Assert imports expanded
    })

    t.Run("composed pipeline", func(t *testing.T) {
        deck := &domain.Deck{Source: source}
        err := deck.Process(
            domain.WithImportExpansion(),
            domain.WithWorkDir("/tmp"),
            domain.WithLinkRewriting("path/to/deck"),
        )
        // Assert all transformations applied
    })
}
```

## Monitoring

### Success Metrics
- **Bug count**: Zero duplicate logic bugs after Phase 1
- **Test coverage**: >80% for domain package after Phase 2
- **Build time**: <5% increase after all phases
- **Response time**: No degradation in API latency
- **Error rate**: No increase in production errors

### Rollback Triggers
- Test coverage drops below 70%
- >2 critical bugs in production
- API latency increases >20%
- Error rate doubles
- Team velocity decreases >30%

## Alternatives Not Chosen

### Functional Core, Imperative Shell
Keep handlers as-is but make processing pure functions. Rejected because it doesn't solve duplication problem.

### Event-Driven Architecture
Use events to trigger processing stages. Rejected as over-engineered for current scale.

### Microservices
Split processing into separate service. Rejected as adds operational complexity without benefits.

## References

- [Issue: Images not working in demo UI](related bugs)
- [Issue: WorkDir support for external data files](related bugs)
- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [DDD in Go](https://www.citefast.com/styleguide.php)
- [Hexagonal Architecture](https://netflixtechblog.com/ready-for-changes-with-hexagonal-architecture-b315ec967749)

## Appendix: Code Examples

### Before (Current State)
```go
// handler.go - ~800 lines, mixed concerns
func handleProcess(w http.ResponseWriter, r *http.Request) {
    // 50 lines of HTTP + business logic
    source, err := io.ReadAll(r.Body)
    sourcePath := r.URL.Query().Get("source")

    // Import expansion
    source, err = expandImports(r.Context(), source, sourcePath)

    // WorkDir calculation
    workDir := ""
    if sourcePath != "" {
        if fsStorage, ok := runtime.Input().(runtime.FilesystemStorage); ok {
            // ... 10 lines ...
        }
    }

    // Processing
    result, err := runtime.GetPipeline().ProcessWithWorkDir(...)

    // Link rewriting
    for i, s := range result.Slides {
        if sourcePath != "" {
            s = rewriteSVGLinks(s, sourcePath)
        }
        slides[i] = string(s)
    }

    // JSON response
    writeJSON(w, ProcessResponse{...})
}
```

### After Phase 3 (Proposed)
```go
// handler/http/process.go - ~20 lines, HTTP only
func handleProcess(w http.ResponseWriter, r *http.Request) {
    source, err := io.ReadAll(r.Body)
    sourcePath := r.URL.Query().Get("source")

    deck, err := domain.ProcessDeck(r.Context(), source, sourcePath,
        domain.WithImportExpansion(),
        domain.WithWorkDir(),
        domain.WithLinkRewriting(),
    )

    writeJSON(w, deck.ToProcessResponse())
}

// domain/deck.go - ~50 lines, business logic
func ProcessDeck(ctx context.Context, source []byte, path string, opts ...ProcessOption) (*Deck, error) {
    deck := &Deck{Source: source, SourcePath: path}
    for _, opt := range opts {
        if err := opt.Apply(ctx, deck); err != nil {
            return nil, err
        }
    }
    return deck, nil
}

// domain/transforms.go - ~100 lines, each transformer tested independently
func WithImportExpansion() ProcessOption { ... }
func WithWorkDir() ProcessOption { ... }
func WithLinkRewriting() ProcessOption { ... }
```

**Result**: Clear separation, each piece testable, no duplication.

---

**Next Steps**: Review this ADR with team, get consensus, start Phase 1 implementation.
