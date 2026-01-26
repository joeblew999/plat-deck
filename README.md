# plat-deck

Universal WASM Presentation Processing

Demo: https://deckfs.gedw99.workers.dev/

Process [decksh](https://github.com/ajstarks/decksh) presentations to SVG, running the same Go code on:

- **Cloudflare Workers** (via syumai/workers)
- **Wazero** (server-side WASM runtime)
- **Browser** (standard Go WASM)

All using **R2 as the universal filesystem** for both decksh files and WASM modules.

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │              R2 Storage              │
                    │  ┌─────────┐ ┌─────────┐ ┌───────┐ │
                    │  │  Input  │ │ Output  │ │ WASM  │ │
                    │  │  .dsh   │ │  .svg   │ │modules│ │
                    │  └────┬────┘ └────┬────┘ └───┬───┘ │
                    └───────┼───────────┼─────────┼──────┘
                            │           │         │
        ┌───────────────────┼───────────┼─────────┼────────────────────┐
        │                   ▼           ▼         ▼                    │
        │   ┌─────────────────────────────────────────────────────┐   │
        │   │              deckfs WASM Module                      │   │
        │   │   (same Go code, different build tags)               │   │
        │   └─────────────────────────────────────────────────────┘   │
        │                           │                                  │
        │     ┌─────────────────────┼─────────────────────┐           │
        │     ▼                     ▼                     ▼           │
        │ ┌────────┐          ┌──────────┐          ┌─────────┐       │
        │ │Cloudflare         │  Wazero  │          │ Browser │       │
        │ │Workers │          │  Host    │          │  WASM   │       │
        │ └────────┘          └──────────┘          └─────────┘       │
        └─────────────────────────────────────────────────────────────┘
```

## Build Targets

| Target | Build Tag | Output | Use Case |
|--------|-----------|--------|----------|
| `build-cloudflare` | `cloudflare` | `build/cloudflare/app.wasm` | Cloudflare Workers |
| `build-browser` | (none) | `build/browser/deckfs.wasm` | Browser via JS |
| `build-wasi` | `wasi` | `build/wasi/deckfs.wasm` | Wazero, wasmtime |
| `build-wazero-host` | - | `build/wazero/deckfs-host` | Host binary |

## Quick Start

### Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/)
- [TinyGo](https://tinygo.org/) (for WASI builds)
- [xplat](https://github.com/user/xplat) (for dev environment)
- [Bun](https://bun.sh/) (for demo server)

### Run Locally (Fastest)

```bash
git clone https://github.com/joeblew999/plat-deck.git
cd plat-deck
task util:deps
task dev:up
```

This starts:
- **API** at http://localhost:8080
- **Demo UI** at http://localhost:3000

Open http://localhost:3000 to try it.

### Alternative: Run Services Individually

```bash
task dev:wazero   # API only on :8080
task dev:demo     # Demo UI only on :3000
```

### Build All Targets

```bash
task build:all
```

### Deploy to Cloudflare

```bash
# Set up environment
cp .env.example .env
# Edit .env with your CLOUDFLARE_API_TOKEN

# Deploy
task cf:setup          # Create R2 buckets, KV, Queue
# Update wrangler.toml with KV namespace ID from output
task cf:notifications  # Enable R2 events
task cf:deploy         # Deploy worker
task cf:r2-upload-wasm # Upload WASM modules to R2
```

### Use in Browser

```html
<script src="https://pub-xxx.r2.dev/browser/wasm_exec.js"></script>
<script>
const go = new Go();
WebAssembly.instantiateStreaming(
  fetch("https://pub-xxx.r2.dev/browser/deckfs.wasm"),
  go.importObject
).then(result => {
  go.run(result.instance);
  
  // Now use deckfs
  const result = JSON.parse(deckfs.process(`
    deck
    slide
      ctext "Hello!" 50 50 5
    eslide
    edeck
  `));
  console.log(result.slides[0]); // SVG
});
</script>
```

## R2 Bucket Structure

```
deckfs-input/          # Source .dsh files
  presentations/
    intro.dsh
    
deckfs-output/         # Generated SVGs + manifests
  presentations/
    intro/
      slide-0001.svg
      slide-0002.svg
      manifest.json

deckfs-wasm/           # WASM modules (runtime loading)
  browser/
    deckfs.wasm
    wasm_exec.js
  wasi/
    deckfs.wasm
```

## Project Structure

```
deckfs/
├── cmd/
│   ├── cloudflare/    # Cloudflare Workers entry
│   ├── browser/       # Browser WASM entry
│   ├── wasi/          # WASI entry (stdin/stdout)
│   └── wazero/        # Wazero host binary
├── handler/           # Shared HTTP handlers
├── runtime/           # Platform abstraction
│   ├── runtime.go     # Storage interface
│   ├── storage_cloudflare.go  # R2 via syumai/workers
│   └── storage_http.go        # R2 via S3 HTTP API
├── internal/
│   └── processor/     # decksh → SVG conversion
├── wrangler.toml
└── Taskfile.yaml
```

## API

All runtimes expose the same API:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/process` | POST | Process decksh, return SVG |
| `/upload/:key` | PUT | Upload, process, store in R2 |
| `/slides/:key` | GET | Get SVG from R2 |
| `/manifest/:name` | GET | Get manifest |
| `/decks` | GET | List processed decks |
| `/status/:key` | GET | Get processing status (from KV) |

## Browser Demo

Open `demo/index.html` in a browser to try the API interactively.

## Tasks

Run `task --list` to see all available tasks:

```
build-*         Build targets (cloudflare, browser, wasi, wazero-host, cli)
cf-*            Cloudflare management (setup, deploy, list, teardown)
r2-*            R2 storage operations (upload, upload-wasm)
test-*          Tests (unit, e2e, decksh, deckviz)
dev-*           Local development servers
```

## License

MIT
