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

