# plat-deck

Cross-platform decksh presentation renderer

Demo: https://deckfs.gedw99.workers.dev/

Process [decksh](https://github.com/ajstarks/decksh) presentations to **SVG, PNG, and PDF** using two deployment modes:

## Deployment Modes

### Native Pipeline (Recommended for Development)
Uses [ajstarks' deck tools](https://github.com/ajstarks/deck) via Go server
- ✅ **Formats**: SVG, PNG, PDF
- ✅ **File system access** for imports/includes
- ✅ **Font rendering** with custom TTF fonts
- ✅ **275 example presentations** built-in
- ✅ **Fast**: Direct binary execution

### WASM Pipeline (Cloudflare Workers)
TinyGo WASM for serverless edge deployment
- ✅ **Formats**: SVG
- ✅ **R2 storage** for inputs/outputs
- ✅ **Edge compute**: Sub-100ms global latency
- ⏳ PNG/PDF support coming soon

## Quick Start

### Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/)
- [TinyGo](https://tinygo.org/) (for Cloudflare builds)
- [Bun](https://bun.sh/) (for demo server)

### 1. Clone and Setup

```bash
git clone https://github.com/joeblew999/plat-deck.git
cd plat-deck

# Clone ajstarks' repos for binaries and examples
task test:clone

# Build ajstarks' binaries and wazero host
task build:host
task build:tools
```

### 2. Run Native Server

```bash
# Option A: Everything together (recommended)
task pc:up
# Starts wazero (:8080) + demo (:3000)

# Option B: Individual services
task run:wazero    # API only on :8080
task run:demo      # Demo UI only on :3000
```

Open http://localhost:8080 or http://localhost:3000 to try it.

### 3. Run Cloudflare Worker Locally

```bash
task run:wrangler
# Starts local Cloudflare emulator on :8787
```

## Output Formats

All formats use ajstarks' original deck renderers:

| Format | Multi-slide | Font Support | Use Case |
|--------|-------------|--------------|----------|
| **SVG** | Individual files | Basic | Web display, editing |
| **PNG** | Individual images | Full TTF | Social media, thumbnails |
| **PDF** | Single document | Full TTF | Print, distribution |

## API

### Native Server (localhost:8080)

```bash
# Health check
curl http://localhost:8080/health

# API info
curl http://localhost:8080/api | jq

# Process to SVG
curl -X POST 'http://localhost:8080/process?format=svg' \
  --data-binary @presentation.dsh | jq -r '.slides[0]' > slide1.svg

# Process to PNG (returns base64)
curl -X POST 'http://localhost:8080/process?format=png' \
  --data-binary @presentation.dsh | jq -r '.slides[0]' | base64 -d > slide1.png

# Process to PDF (multi-page document)
curl -X POST 'http://localhost:8080/process?format=pdf' \
  --data-binary @presentation.dsh | jq -r '.document' | base64 -d > deck.pdf

# List examples
curl http://localhost:8080/examples | jq
curl 'http://localhost:8080/examples?renderable=true' | jq  # Only complete decks

# Get example content
curl http://localhost:8080/examples/go/go.dsh
```

#### Deck Routing (Shareable URLs)

Direct access to rendered decks with proper URL routing for sharing and navigation.

**Slide Access:** `GET /deck/:examplePath/slide/:num.svg`

Serves a specific slide with rewritten links for in-deck navigation.

```bash
# View B-17 casualty rates deck
curl http://localhost:8080/deck/b17/b17.dsh/slide/1.svg

# View election results (7 slides)
curl http://localhost:8080/deck/elections/elections.dsh/slide/1.svg
```

Returns SVG with navigation links rewritten to proper URLs (e.g., `/deck/b17/b17.dsh/slide/2.svg`).

**Asset Access:** `GET /deck/:examplePath/asset/:filename`

Serves images and assets referenced in decks.

```bash
# Access deck image
curl http://localhost:8080/deck/b17/b17.dsh/asset/iza-vailable.png
```

**Deck Redirect:** `GET /deck/:examplePath` → Redirects to slide 1

**Features:**
- Shareable URLs for specific slides
- In-memory caching for fast access
- Asset serving with correct paths
- Link rewriting (temp paths → proper URLs)
- Path traversal security

### Cloudflare Worker (localhost:8787 or production)

```bash
# Health check
curl https://deckfs.gedw99.workers.dev/health

# Process to SVG
curl -X POST 'https://deckfs.gedw99.workers.dev/process' \
  --data-binary @presentation.dsh | jq -r '.slides[0]' > slide1.svg
```

## Demo UI

Interactive demo at `http://localhost:3000` (when using `task pc:up`)

**Features:**
- Browse all renderable example decks (116 complete presentations)
- Render custom decksh code in the editor
- Navigate multi-slide presentations with arrow buttons
- **Share button** for opening decks in new tabs with shareable URLs
- **Click SVG links** to navigate between slides within the demo
- Filter examples by name
- Grouped examples by directory

**File Types:**

Decksh files come in two types:
1. **Complete Decks** (116 files) - Contain `deck`/`edeck` structure, can be rendered directly
2. **Library Files** (159 files) - Contain only `def`/`edef` blocks, used as imports

The demo filters to show only renderable complete decks. See [docs/DECKSH-FILES.md](docs/DECKSH-FILES.md) for details.

## Project Structure

```
plat-deck/
├── cmd/
│   ├── cloudflare/    # Cloudflare Workers entry (TinyGo WASM)
│   ├── wazero/        # Native server using ajstarks binaries
│   ├── cli/           # CLI tool for testing
│   ├── wasi/          # WASM module for wazero (alternative)
│   └── browser/       # Browser WASM (experimental)
├── pkg/
│   └── pipeline/
│       ├── native.go      # Uses ajstarks binaries (SVG/PNG/PDF)
│       └── types.go       # Shared types
├── handler/           # Shared HTTP handlers
├── runtime/           # Platform abstraction (R2, KV, etc)
├── internal/
│   └── processor/     # decksh → XML conversion
├── demo/
│   └── index.html     # Interactive demo UI
├── .bin/              # Built binaries
│   ├── deck/          # ajstarks tools (decksh, svgdeck, pngdeck, pdfdeck)
│   └── wazero/        # deckfs-host
├── .src/              # Source repos and resources
│   ├── decksh/        # ajstarks/decksh
│   ├── deck/          # ajstarks/deck
│   ├── deckviz/       # 275 example presentations
│   └── deckfonts/     # TTF fonts for PNG/PDF
└── taskfiles/         # Task definitions
```

## Architecture

### Native Pipeline

```
┌─────────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  decksh     │────▶│  decksh  │────▶│ svgdeck/ │────▶│ SVG/PNG/ │
│  source     │     │  binary  │     │ pngdeck/ │     │   PDF    │
│  (.dsh)     │     │          │     │ pdfdeck  │     │          │
└─────────────┘     └──────────┘     └──────────┘     └──────────┘
                         │                 ▲
                         │                 │
                         └─────XML─────────┘
```

1. **Parse**: `decksh` converts `.dsh` → deck XML
2. **Render**: Format-specific renderer converts XML → output
   - `svgdeck`: Individual SVG files per slide
   - `pngdeck`: Individual PNG images per slide (with fonts)
   - `pdfdeck`: Single multi-page PDF document (with fonts)

### WASM Pipeline

```
┌─────────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  R2 Input   │────▶│Cloudflare│────▶│   SVG    │────▶│ R2 Output│
│   (.dsh)    │     │  Worker  │     │ renderer │     │  (.svg)  │
└─────────────┘     └──────────┘     └──────────┘     └──────────┘
```

## Available Tasks

Run `task --list` to see all tasks. Key tasks:

```bash
# Development
task pc:up              # Start all services (wazero + demo)
task run:wrangler       # Cloudflare worker locally (:8787)
task run:wazero         # Wazero server locally (:8080)
task run:demo           # Demo server only (:3000)

# Building
task build:host         # Build wazero host binary
task build:tools        # Build ajstarks binaries (decksh, svgdeck, etc)
task build:cloudflare   # Build Cloudflare worker WASM
task build:cli          # Build CLI tool

# Testing
task test:unit          # Run Go tests
task test:e2e           # End-to-end tests
task test:decksh        # Test against decksh examples
task test:deckviz       # Test against deckviz examples

# Deployment
task cf:deploy          # Deploy to Cloudflare Workers
task cf:setup           # Create R2 buckets, KV, Queue
task cf:teardown        # Remove Cloudflare resources

# Setup
task test:clone         # Clone ajstarks repos
```

## Environment Variables

### Native Server

- `DECKFONTS` - Path to font directory (default: `.src/deckfonts`)
  - Required for PNG/PDF rendering with custom fonts
  - Should contain TTF files (SansSerif, Serif, Mono variants)

## Deployment

### Deploy to Cloudflare Workers

```bash
# 1. Set up environment
cp .env.example .env
# Edit .env with your CLOUDFLARE_API_TOKEN

# 2. Create resources
task cf:setup
# Update wrangler.toml with KV namespace ID from output

# 3. Deploy
task cf:deploy

# 4. Test
curl https://deckfs.YOUR-SUBDOMAIN.workers.dev/health
```

### Deploy Native Server

Build and run anywhere Go runs:

```bash
# Build
task build:host
task build:tools

# Run
DECKFONTS=.src/deckfonts \
  .bin/wazero/deckfs-host \
  -addr :8080 \
  -bin .bin/deck \
  -examples .src/deckviz
```

## Examples

### Basic Presentation

```bash
cat > hello.dsh << 'EOF'
deck
  slide "white" "black"
    ctext "Hello, World!" 50 50 5
  eslide
  slide "lightblue" "navy"
    text "Bullet 1" 20 40 3
    text "Bullet 2" 20 50 3
    text "Bullet 3" 20 60 3
  eslide
edeck
EOF

# Generate SVG
curl -X POST 'http://localhost:8080/process?format=svg' \
  --data-binary @hello.dsh | jq -r '.slides[0]' > slide1.svg

# Generate PNG
curl -X POST 'http://localhost:8080/process?format=png' \
  --data-binary @hello.dsh | jq -r '.slides[0]' | base64 -d > slide1.png

# Generate PDF (all slides in one document)
curl -X POST 'http://localhost:8080/process?format=pdf' \
  --data-binary @hello.dsh | jq -r '.document' | base64 -d > hello.pdf
```

### With Custom Fonts

```bash
cat > fonts.dsh << 'EOF'
deck
  slide "white" "black"
    text "Sans Serif Font" 10 30 3 "sans"
    text "Serif Font" 10 50 3 "serif"
    text "Monospace Font" 10 70 3 "mono"
  eslide
edeck
EOF

# Fonts are automatically used for PNG/PDF
curl -X POST 'http://localhost:8080/process?format=png' \
  --data-binary @fonts.dsh | jq -r '.slides[0]' | base64 -d > fonts.png
```

### Using Imports

```bash
# Create reusable definitions
cat > defs.dsh << 'EOF'
def companylogo
  image "logo.png" 90 5 128 128
edef
EOF

# Use in presentation
cat > main.dsh << 'EOF'
include "defs.dsh"

deck
  slide "white" "black"
    companylogo
    ctext "Company Presentation" 50 50 5
  eslide
edeck
EOF

# Process with working directory for import resolution
curl -X POST 'http://localhost:8080/process?format=svg&source=main.dsh' \
  --data-binary @main.dsh | jq -r '.slides[0]' > slide1.svg
```

## Contributing

1. Fork the repo
2. Create a feature branch
3. Make your changes
4. Run tests: `task test:unit test:e2e`
5. Submit a pull request

## License

MIT

## Credits

- [ajstarks/decksh](https://github.com/ajstarks/decksh) - Original decksh implementation
- [ajstarks/deck](https://github.com/ajstarks/deck) - SVG/PNG/PDF renderers
- [syumai/workers](https://github.com/syumai/workers) - Cloudflare Workers Go runtime
