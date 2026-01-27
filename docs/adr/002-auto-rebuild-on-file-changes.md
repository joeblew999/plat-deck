# ADR 002: Auto-Rebuild on File Changes

**Status**: Proposed
**Date**: 2026-01-27
**Authors**: Development Team
**Related**: ADR-001 (mentions build dependency issues)

## Context

### The Problem

When developing locally, file changes don't automatically trigger rebuilds, causing confusion and wasted time:

**Scenario 1: HTML Changes**
```bash
# Developer edits demo/index.html
vim demo/index.html

# Refreshes browser at http://localhost:8080
# Sees OLD HTML because wazero binary still has old embedded content

# Must manually:
task build:host
task pc:restart PROC=wazero

# Easy to forget, leads to debugging phantom issues
```

**Scenario 2: Handler Changes**
```bash
# Developer fixes bug in handler/handler.go
vim handler/handler.go

# Calls API endpoint
# Sees OLD behavior

# Must manually:
task build:host
task pc:restart PROC=wazero
```

### Root Causes

1. **Embedded Files**: `demo/index.html` is embedded via `//go:embed` in both:
   - `cmd/wazero/` → wazero binary
   - `cmd/cloudflare/` → Cloudflare worker WASM

   Embedded files are baked into binary at compile time.

2. **Missing Task Dependencies**: Taskfile `sources:` don't include `demo/` files
   ```yaml
   host:
     sources:
       - cmd/wazero/**/*.go
       # ❌ MISSING: demo/**/*.html
   ```

3. **No File Watcher**: process-compose doesn't watch for file changes and rebuild

4. **Manual Restart Required**: Even after rebuild, must manually restart via `task pc:restart`

### Impact

**Developer Experience**:
- Wastes 5-10 minutes per change debugging "why isn't my fix working?"
- Breaks flow - context switch between code and manual rebuild
- Easy to forget, especially when switching between multiple changes
- New contributors don't know about this gotcha

**Production Risk**:
- Developers test old code locally
- Push "working" code that's actually broken
- Bugs make it to production

**Current Workarounds**:
- Documentation in CLAUDE.md (developers don't read it)
- Muscle memory to always rebuild (developers forget)
- Nuclear option: `task build:host` before every test (slow)

## Decision Drivers

1. **Developer Experience** - Primary concern, this is painful daily
2. **Fast Feedback Loop** - Changes visible in <2 seconds
3. **No Manual Steps** - Zero cognitive load to rebuild
4. **Production Parity** - Local dev behaves like production (both use embedded files)
5. **Low Overhead** - File watching shouldn't slow down development
6. **Cross-Platform** - Works on macOS, Linux, Windows

## Options Considered

### Option 1: Fix Taskfile Dependencies Only

**Approach**: Add `demo/` to `sources:` in taskfiles/build.yaml

```yaml
host:
  desc: Build wazero host binary
  sources:
    - cmd/wazero/**/*.go
    - handler/**/*.go
    - runtime/**/*.go
    - internal/**/*.go
    - pkg/**/*.go
    - demo/**/*.go      # ADD
    - demo/**/*.html    # ADD
    - go.mod
    - go.sum
  generates:
    - "{{.BUILD_DIR}}/wazero/deckfs-host"
```

**Result**: `task build:host` will rebuild if HTML is newer than binary

**Pros**:
- 5 minute fix
- Uses existing Task functionality
- No new dependencies
- Works with `task --watch` if we add it

**Cons**:
- Still requires manual `task build:host` or remembering `--watch`
- Still requires manual `task pc:restart PROC=wazero`
- Doesn't help with Cloudflare (must remember `task cf:deploy`)

**Developer UX**:
```bash
# Edit file
vim demo/index.html

# Must remember:
task build:host          # Auto-detects change and rebuilds
task pc:restart PROC=wazero  # Still manual
```

**Rating**: Better than current, but still manual steps.

### Option 2: Add File Watcher to process-compose

**Approach**: Use `watchexec` to rebuild on file changes, integrate with process-compose

```yaml
# process-compose.yaml
processes:
  wazero:
    command: task run:wazero
    # ...existing config...

  wazero-watcher:
    command: |
      watchexec \
        --watch demo \
        --watch handler \
        --watch runtime \
        --watch cmd/wazero \
        --exts go,html \
        --debounce 500 \
        -- task build:host && task pc:restart PROC=wazero
```

**Pros**:
- Fully automatic - zero manual steps
- Fast feedback (rebuilds in <2s)
- Integrated with existing `task pc:up` workflow
- Can watch multiple directories
- Debouncing prevents rebuild storms

**Cons**:
- Requires `watchexec` installed (new dependency)
- Adds complexity to process-compose
- Watcher process uses ~10MB RAM
- Might rebuild too often (e.g., vim swap files)

**Developer UX**:
```bash
# Start once
task pc:up

# Edit files
vim demo/index.html
# <save file>
# [2 seconds later]
# Browser automatically shows new content

# Zero manual steps!
```

**Rating**: Best UX, small dependency cost.

### Option 3: Runtime Filesystem Read (Dev Mode)

**Approach**: Read `demo/index.html` from disk in dev, embed in prod

```go
// cmd/wazero/main.go
var devMode = flag.Bool("dev", false, "Dev mode: read HTML from filesystem")

func serveHTML(w http.ResponseWriter, r *http.Request) {
    var html []byte
    if *devMode {
        // Read from filesystem - always fresh
        html, _ = os.ReadFile("demo/index.html")
    } else {
        // Use embedded version
        html = demo.HTML
    }
    w.Write(html)
}
```

**Pros**:
- Zero rebuild for HTML changes in dev mode
- Instant feedback
- No dependencies
- Same approach as many web frameworks

**Cons**:
- Dev/prod behavior differs (risky)
- Doesn't help with Go code changes (still need rebuild)
- Doesn't help with Cloudflare (can't read filesystem)
- Adds complexity to code
- Risk: forget to test embedded version before deploy

**Developer UX**:
```bash
# Start with dev flag
task pc:up DEV=true

# Edit HTML
vim demo/index.html
# <save file>
# <refresh browser>
# Instant update!

# Edit Go code
vim handler/handler.go
# Still need: task build:host && task pc:restart PROC=wazero
```

**Rating**: Good for HTML, but incomplete solution.

### Option 4: Hybrid (Taskfile + Watcher + Dev Mode)

**Approach**: Combine all three options

**Layer 1**: Fix taskfile dependencies (always good)
**Layer 2**: Add file watcher for auto-rebuild in development
**Layer 3**: Optional dev mode for instant HTML changes

```yaml
# taskfiles/build.yaml
host:
  sources:
    - demo/**/*.html  # Layer 1

# process-compose.yaml
wazero-watcher:        # Layer 2
  command: watchexec ...

# cmd/wazero/main.go
if devMode {           # Layer 3
  readFromFS()
}
```

**Pros**:
- Layer 1: Works immediately, no changes needed to workflow
- Layer 2: Full auto-rebuild for all file types
- Layer 3: Instant feedback for HTML (optional)
- Flexible: developers choose level of automation

**Cons**:
- Most complex solution
- Multiple systems to maintain
- Potential confusion about which layer is helping

**Developer UX**:
```bash
# Normal dev: Layer 1 + 2
task pc:up
vim demo/index.html
# Auto-rebuilds in 2s

# Fast HTML iteration: Layer 3
task pc:up DEV=true
vim demo/index.html
# Instant updates, no rebuild
```

**Rating**: Most complete, but complex.

## Decision

**Choose Option 2: File Watcher with process-compose**

With **Option 1 as prerequisite** (fix taskfile dependencies first).

### Why Not Option 3 or 4?

The dev mode (reading from filesystem) creates dev/prod parity issues:
- Might work in dev but fail in production
- Cloudflare can't use it (no filesystem)
- Adds code complexity for marginal benefit (2s rebuild is fast enough)

**Philosophy**: Dev should match prod as closely as possible.

### Why Not Just Option 1?

Fixing taskfile dependencies helps, but doesn't eliminate manual steps. We want **zero cognitive load** for developers.

### Implementation Plan

**Phase 1: Fix Dependencies** (Today)
1. Update `taskfiles/build.yaml` to include `demo/` in sources
2. Test that `task build:host` detects HTML changes
3. Commit and push

**Phase 2: Add Watcher** (Today)
1. Install `watchexec` in development dependencies
2. Add `watcher-wazero` process to `process-compose.yaml`
3. Configure to watch: `demo/`, `handler/`, `runtime/`, `cmd/wazero/`
4. Add `--debounce 500ms` to prevent rebuild storms
5. Test: edit HTML, verify auto-rebuild
6. Update CLAUDE.md

**Phase 3: Documentation** (Today)
1. Document required dependencies (`watchexec`)
2. Add install instructions for macOS/Linux/Windows
3. Document how to disable watcher if needed
4. Update onboarding docs

## Consequences

### Positive

- **Developer Experience**: Zero manual rebuilds, instant feedback
- **Fewer Bugs**: Developers always test latest code
- **Faster Iteration**: Edit-save-refresh instead of edit-rebuild-restart-refresh
- **Cognitive Load**: One less thing to remember
- **Onboarding**: New developers don't need to learn rebuild commands

### Negative

- **New Dependency**: Requires `watchexec` installed
- **Resource Usage**: Watcher process uses ~10MB RAM, minimal CPU
- **Rebuild Noise**: Console shows rebuilds (can be filtered)
- **Potential Over-rebuilding**: vim swap files might trigger rebuilds
- **Startup Time**: process-compose starts one more process

### Mitigation

1. **Document watchexec install** in README and util:deps task
2. **Add .gitignore patterns** to prevent swap files triggering rebuilds
3. **Debounce settings** prevent rebuild storms
4. **Optional watcher** - can disable by commenting out in process-compose.yaml
5. **Log filtering** - watcher logs to separate file

## Alternatives Not Chosen

### Air (Cosmtrek/air)
Go-specific live reload tool. Rejected because:
- Only works for Go files, not HTML
- Another tool to learn
- watchexec is more general-purpose

### Tilt
Kubernetes-native development tool. Rejected because:
- Overkill for our use case
- We're not using k8s
- Steep learning curve

### Custom Go Watcher
Write our own file watcher in Go. Rejected because:
- Reinventing the wheel
- watchexec is battle-tested
- More code to maintain

### Docker Volumes
Use Docker with volume mounts. Rejected because:
- Adds Docker complexity
- Doesn't solve embed problem
- Slower rebuild cycle

## Implementation Details

### watchexec Installation

```bash
# macOS
brew install watchexec

# Linux (Debian/Ubuntu)
apt install watchexec

# Linux (Arch)
pacman -S watchexec

# Windows
scoop install watchexec
# or
choco install watchexec

# Cargo (all platforms)
cargo install watchexec-cli
```

### process-compose.yaml Addition

```yaml
processes:
  # Existing processes...
  wazero:
    command: task run:wazero
    # ... existing config ...

  # New watcher process
  watcher-wazero:
    command: |
      watchexec \
        --watch demo \
        --watch handler \
        --watch runtime \
        --watch cmd/wazero \
        --exts go,html \
        --ignore '*.swp' \
        --ignore '*.swo' \
        --ignore '.git' \
        --debounce 500ms \
        --clear \
        --on-busy-update restart \
        -- sh -c 'echo "🔄 Rebuilding wazero..." && task build:host && task pc:restart PROC=wazero'
    depends_on:
      wazero:
        condition: process_started
    availability:
      restart: on_failure
      backoff_seconds: 5
    log_location: .logs/watcher-wazero.log
```

### Taskfile Update

```yaml
# taskfiles/build.yaml
host:
  desc: Build wazero host binary
  cmds:
    - mkdir -p {{.BUILD_DIR}}/wazero
    - go build -o {{.BUILD_DIR}}/wazero/deckfs-host ./cmd/wazero
  sources:
    - cmd/wazero/**/*.go
    - handler/**/*.go
    - runtime/**/*.go
    - internal/**/*.go
    - pkg/**/*.go
    - demo/**/*.go     # ADDED
    - demo/**/*.html   # ADDED
    - go.mod
    - go.sum
  generates:
    - "{{.BUILD_DIR}}/wazero/deckfs-host"

cloudflare:
  desc: Build for Cloudflare Workers (TinyGo)
  sources:
    # ... existing ...
    - demo/**/*.go     # ALREADY HAS THIS
    - demo/**/*.html   # ALREADY HAS THIS
```

### util:deps Task Update

```yaml
# taskfiles/util.yaml
deps:
  desc: Install dependencies
  cmds:
    - echo "Installing development dependencies..."
    - |
      # Check for watchexec
      if ! command -v watchexec &> /dev/null; then
        echo "⚠️  watchexec not found. Install with:"
        echo "   macOS:   brew install watchexec"
        echo "   Linux:   apt install watchexec (or pacman -S watchexec)"
        echo "   Windows: scoop install watchexec"
        echo "   Cargo:   cargo install watchexec-cli"
        exit 1
      fi
      echo "✓ watchexec installed"
    # ... other dependencies ...
```

## Testing Strategy

### Manual Testing

```bash
# Test 1: HTML changes
task pc:up
# Wait for startup
vim demo/index.html  # Change title
# Save file
# Wait ~2 seconds
# Refresh browser
# Verify new title appears

# Test 2: Handler changes
vim handler/handler.go  # Add log statement
# Save file
# Wait ~2 seconds
# Make API request
# Verify log appears

# Test 3: Multiple rapid changes
vim demo/index.html  # Change 1
# Save
vim demo/index.html  # Change 2
# Save immediately
# Should only rebuild once (debounce working)

# Test 4: Invalid code
vim handler/handler.go  # Introduce syntax error
# Save file
# Should see build error in logs
# Fix syntax error
# Save file
# Should rebuild successfully
```

### Success Criteria

- ✅ HTML changes visible in browser within 3 seconds of save
- ✅ Go changes visible in API within 3 seconds of save
- ✅ No more than 1 rebuild per 500ms (debounce working)
- ✅ Build errors shown in console
- ✅ Watcher survives build errors
- ✅ Can disable watcher without breaking pc:up
- ✅ Works on macOS and Linux

## Rollback Plan

If watcher causes issues:

1. **Disable watcher**: Comment out `watcher-wazero` in process-compose.yaml
2. **Fallback to manual**: Use `task build:host && task pc:restart PROC=wazero`
3. **Uninstall watchexec**: `brew uninstall watchexec` (or OS equivalent)

The fixed taskfile dependencies (Option 1) remain and provide value.

## Future Enhancements

### Could Add Later

1. **Browser LiveReload**: Auto-refresh browser on HTML changes
   ```bash
   watchexec --watch demo/*.html -- browser-sync reload
   ```

2. **Cloudflare Watcher**: Auto-deploy to CF on handler changes
   ```yaml
   watcher-cloudflare:
     command: watchexec --watch handler -- task cf:deploy
   ```

3. **Test Runner**: Auto-run tests on code changes
   ```yaml
   watcher-tests:
     command: watchexec --watch handler --watch runtime -- task test:unit
   ```

4. **Diff Previews**: Show what changed before rebuilding
   ```bash
   watchexec -- sh -c 'git diff && task build:host'
   ```

## References

- [watchexec](https://github.com/watchexec/watchexec) - File watcher tool
- [process-compose](https://github.com/F1bonacc1/process-compose) - Process orchestration
- [Task](https://taskfile.dev) - Task runner documentation
- [ADR-001](001-deck-processing-architecture.md) - Related architecture decisions

## Appendix: Benchmark

```bash
# Rebuild time (without watcher)
$ time task build:host
real    0m2.341s

# Rebuild time (with watcher detecting change)
# File change detected: ~50ms
# Task execution: ~2.3s
# Process restart: ~200ms
# Total: ~2.5s

# This is acceptable for development workflow
```

---

**Decision**: Implement Option 2 with Option 1 as prerequisite.
**Next Steps**: Begin Phase 1 implementation (fix taskfile dependencies).
