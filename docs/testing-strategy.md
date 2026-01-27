# Testing Strategy

## Overview

The plat-deck project has three runtime targets with different testing approaches:

1. **Native Go** (cmd/cli, cmd/wazero, pkg/*, runtime/*_native.go)
2. **TinyGo WASM** (cmd/cloudflare, runtime/*_wasm.go)
3. **Shared Handler** (handler/*, requires WASM build tags)

## Testing Challenges

### Handler Package Build Constraints

The handler package uses:
```go
//go:build js || tinygo || cloudflare
```

This prevents normal `go test` execution because:
- Handler code only compiles for WASM targets
- Standard `go test` uses native Go compiler
- TinyGo test support is limited

## Testing Approaches

### 1. Unit Tests (Native Packages)

**Location:** `pkg/pipeline/*_test.go`, other native packages

**Approach:** Standard Go testing
```bash
go test ./pkg/...
task test:unit
```

**Coverage:**
- ✅ Native pipeline implementation
- ✅ Import resolver
- ✅ Utility functions

### 2. Integration Tests (Wazero)

**Location:** `cmd/wazero/` (future: `cmd/wazero/*_test.go`)

**Approach:** Test handler via wazero server
- Start wazero server in test
- Make HTTP requests to handlers
- Verify responses

**Benefits:**
- Tests actual handler code paths
- Validates runtime integration
- No build tag issues

**Example:**
```go
//go:build !js && !tinygo

package main

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestWazeroHandlers(t *testing.T) {
    // Initialize runtime with test storage
    // Register handlers
    // Test HTTP requests
}
```

### 3. E2E Tests (CLI + Script)

**Location:** `test-e2e.sh`, `taskfiles/test.yaml`

**Approach:** End-to-end CLI testing
```bash
task test:e2e
```

**Coverage:**
- ✅ Full processing pipeline
- ✅ API response structure
- ✅ Error handling
- ✅ Multi-slide decks

### 4. Browser Tests (Playwright MCP)

**Approach:** Test deployed endpoints with browser automation

**Local (Wazero):**
```bash
task pc:up
# Use Playwright MCP to test localhost:8080
```

**Cloudflare (Production):**
```bash
task cf:deploy
# Use Playwright MCP to test production URL
```

**Coverage:**
- ✅ Full HTTP stack
- ✅ Frontend integration
- ✅ Real user flows
- ✅ Cross-browser compatibility

### 5. Repository Tests (Real Decks)

**Location:** `taskfiles/test.yaml`

**Approach:** Test with real-world deck files
```bash
task test:decksh   # Test with decksh examples
task test:deckviz  # Test with deckviz examples
task test:all      # Run all tests
```

## Recommended Testing Workflow

### During Development

1. Run unit tests: `task test:unit`
2. Run E2E tests: `task test:e2e`
3. Run lint checks: `task lint:all`

### Before Commit

Pre-commit hook automatically runs:
- Lint checks if handler/runtime changed
- Cloudflare build verification

### In CI/CD

GitHub Actions runs:
- Lint checks
- Build validation (Cloudflare + CLI)
- Unit tests
- E2E tests

### Before Deploy

1. Test local wazero: `task pc:up` + Playwright MCP
2. Deploy to Cloudflare: `task cf:deploy`
3. Test production with Playwright MCP

## Handler Testing Strategy

The handler package has build constraints (`//go:build js || tinygo || cloudflare`) that prevent standard Go testing. Here are the practical options:

### Option 1: E2E + Browser Testing (Current Approach ✅)

Test handlers indirectly through:
1. **E2E CLI tests** (`test-e2e.sh`) - Test processing and API responses
2. **Browser automation** (Playwright MCP) - Test live endpoints with real HTTP stack

**Pros:**
- ✅ Actually works with current architecture
- ✅ Tests real request/response flow
- ✅ Validates full stack including HTTP, CORS, content-type
- ✅ No build constraints issues
- ✅ Type-safe responses catch many bugs at compile time

**Cons:**
- ❌ Slower than unit tests
- ❌ Harder to test edge cases
- ❌ Less granular feedback on failures

**Implementation:**
```bash
# E2E tests (automated)
task test:e2e

# Local browser testing (manual with Playwright MCP)
task pc:up
# Navigate to localhost:8080 with Playwright

# Production browser testing (manual with Playwright MCP)
task cf:deploy
# Navigate to production URL with Playwright
```

**Why this works:**
- Handlers are tested through actual HTTP requests
- Response types enforce consistency
- Validation utilities ensure security
- Lint checks prevent platform-specific code leakage

### Option 2: Wazero Integration Tests (Doesn't Work ❌)

**Status:** ⛔ **Blocked** - Cannot import handler package from native Go tests

The handler package build tags exclude it from native builds, so even native test code cannot import it:

```go
// This fails because handler package is excluded on native platforms
import "github.com/joeblew999/deckfs/handler"
```

**Why it doesn't work:**
- Handler build tags: `//go:build js || tinygo || cloudflare`
- Native tests run with native Go compiler
- Go build system excludes entire handler package
- No way to import excluded packages

### Option 3: TinyGo Test Target (Experimental ⚠️)

**Status:** ⚠️ **Not Implemented** - TinyGo test support is experimental

Run tests using TinyGo compiler:
```bash
tinygo test -tags cloudflare ./handler/
```

**Pros:**
- Direct handler testing
- Same compilation target as production

**Cons:**
- TinyGo test support incomplete (no httptest, limited mocking)
- Very slow compilation (~30s per test run)
- May not support all Go testing features
- Complex debugging

### Option 4: Architectural Change (Not Planned ⬜)

Remove build tags from handler package:
- Split handler into `handler/core` (no build tags)
- Create `handler/wasm` for WASM-specific initialization
- Create `handler/native` for native-specific initialization

**Status:** ⬜ **Not planned** - Current approach works well enough

**Pros:**
- Standard Go testing works
- Fast test execution
- Full testing toolkit available

**Cons:**
- Major refactoring required
- Risk of platform leakage
- Current E2E + browser testing is sufficient

### Recommended Approach

**Use Option 1 (E2E + Browser Testing):**

The combination of:
1. **Type-safe responses** - Compile-time API contracts
2. **Validation utilities** - Runtime input validation
3. **E2E tests** - Automated API response testing
4. **Browser tests** - Manual real-world flow testing
5. **Lint checks** - Platform consistency enforcement

...provides sufficient coverage without fighting the build system.

## Current Implementation

**Implemented:**
- ✅ Unit tests for native pipeline (`pkg/pipeline/*_test.go`)
- ✅ E2E tests via CLI (`test-e2e.sh`)
- ✅ Real-world deck validation (`task test:decksh`, `task test:deckviz`)
- ✅ CI/CD automation (`.github/workflows/ci.yml`)
- ✅ Input validation in all handlers
- ✅ Type-safe API responses
- ✅ Pre-commit hooks (`.githooks/pre-commit`)
- ✅ Lint checks (`taskfiles/lint.yaml`)

**Handler Testing:**
- ✅ E2E tests (automated)
- ⬜ Browser tests with Playwright MCP (manual)
- ⛔ Direct unit tests (blocked by build constraints)

**Future Work:**
- ⬜ Performance benchmarks
- ⬜ Test coverage reporting
- ⬜ Automated Playwright MCP tests
- ⬜ Load testing for Cloudflare Workers

## Testing Checklist

Before merging:
- [ ] `task test:unit` passes
- [ ] `task test:e2e` passes
- [ ] `task lint:all` passes
- [ ] `task build:cloudflare` succeeds
- [ ] `task build:cli` succeeds

Before deploying:
- [ ] Test local wazero with browser
- [ ] Test Cloudflare deployment with browser
- [ ] Verify all endpoints work
- [ ] Check error handling

## See Also

- [ADR 0006: Cross-Platform Consistency](adr/0006-cross-platform-consistency.md)
- [Pre-commit hooks](.githooks/pre-commit)
- [CI/CD configuration](.github/workflows/ci.yml)
