# CLAUDE

## Architecture

Converts decksh markup to SVG.

```
PRIMARY PATH (use this):
  task run:wrangler  → local dev with Cloudflare emulator (:8787)
  task cf:deploy     → production on Cloudflare Workers

ALTERNATIVE (if you need standalone server without Cloudflare):
  task pc:up         → wazero server (:8080) + demo (:3000)

TESTING:
  task test:e2e      → uses native CLI
```

## cmd/ directories

```
cmd/cloudflare/  ← PRIMARY: Cloudflare Workers (TinyGo WASM)
cmd/cli/         ← Testing: native binary
cmd/wazero/      ← Alternative: standalone Go server
```

## What needs what

```
DEVELOPMENT:
  task run:wrangler    → build:cloudflare (long-running)
  task run:demo        → nothing (long-running)

DEPLOY:
  task cf:deploy     → build:cloudflare

TESTING:
  task test:unit     → nothing
  task test:e2e      → build:cli
  task test:decksh   → build:cli + test:clone
  task test:deckviz  → build:cli + test:clone

BUILD CHAIN:
  build:host       → cmd/wazero/  → .bin/wazero/deckfs-host (Go binary)
  build:cloudflare → cmd/cloudflare/ → .bin/cloudflare/app.wasm (TinyGo WASM)
  build:cli        → cmd/cli/     → .bin/deckfs (Go binary)
```

## Build outputs

```
.bin/wazero/deckfs-host  ← Go binary (task build:host)
.bin/cloudflare/app.wasm ← Cloudflare worker (task build:cloudflare)
```

## Rules

You MUST dogfood your own code using xplat. xplat runs everything. you MUST test everything.

You MUST not just assume things work but run things using xplat.

You MUST use GOWORK=off, so you do no need a go.work. again modelled in xplat task file.

You MUST use the Decksh test repos. use the .src folder for this.

you MUST NOT take shortcuts.

.bin is used for binaries.
.src is used for source.

## Architecture Decisions

See [docs/adr/](docs/adr/) for Architecture Decision Records.

**Active ADRs**:
- [ADR-001: Deck Processing Architecture](docs/adr/001-deck-processing-architecture.md) - Addresses duplicate logic bugs and proposes phased refactoring

## Known Issues & Gotchas

### Handler Logic Duplication

**Problem**: The `/process` and `/deck/slide/` endpoints duplicate processing logic. When adding features, you MUST update both.

**Why**: Historical - handlers grew organically without shared abstraction.

**What to check when modifying handlers**:
1. Import expansion - Must happen in both `handleProcess()` and `handleDeckSlide()`
2. WorkDir calculation - Must handle FilesystemStorage in both places
3. SVG link rewriting - Must call `rewriteSVGLinks()` in both places

**Current workaround**: Manually ensure both code paths stay in sync.

**Future fix**: See ADR-001 for refactoring plan.

### Build Dependencies

**cmd/wazero embeds demo/index.html**:
- When you change `demo/index.html`, you MUST run `task build:host`
- The wazero binary embeds the HTML via `//go:embed` in `demo/embed.go`
- Just restarting the server won't pick up HTML changes

**cmd/cloudflare embeds demo/index.html**:
- When you change `demo/index.html`, you MUST run `task build:cloudflare` and `task cf:deploy`
- The Cloudflare worker also embeds the HTML

**Quick rebuild checklist**:
```bash
# Changed demo/index.html?
task build:host && task pc:restart PROC=wazero  # Local
task cf:deploy                                   # Cloudflare

# Changed handler/*.go?
task build:host && task pc:restart PROC=wazero  # Local
task cf:deploy                                   # Cloudflare

# Changed runtime/*.go?
task build:host && task pc:restart PROC=wazero  # Local
task cf:deploy                                   # Cloudflare
```

### Demo UI State Management

**Problem**: The `isLoadingExample` flag prevents input event handlers from firing during programmatic updates.

**Why**: Setting `textarea.value` triggers the `input` event, which clears `currentSourcePath`.

**Pattern**: When programmatically updating form fields:
```javascript
isLoadingExample = true;
currentSourcePath = examplePath;
sourceEl.value = content;
isLoadingExample = false;
```

**Future fix**: See ADR-001 Phase 1 for better state management approach.

