# DeckFS Deployment Guide

## Production Deployment

**Live URL:** https://deckfs.gedw99.workers.dev

### Quick Deploy

```bash
task cf:deploy
```

This will:
1. Build the Cloudflare Worker WASM binary
2. Deploy to Cloudflare Workers global network
3. Configure R2 buckets, KV namespace, and queue bindings

### Deployment Details

**Worker Size:** 1.6 MB (572 KB gzipped)
**Startup Time:** ~14ms
**Runtime:** TinyGo WASM

### Bindings

| Binding | Type | Purpose |
|---------|------|---------|
| `DECKFS_INPUT` | R2 Bucket | Source .dsh files and imports |
| `DECKFS_OUTPUT` | R2 Bucket | Rendered slides and manifests |
| `DECKFS_WASM` | R2 Bucket | WASM modules (future) |
| `DECKFS_STATUS` | KV Namespace | Processing status tracking |
| `deckfs-events` | Queue | R2 event notifications |

---

## Local Development

### Wazero Server (Full Features)

```bash
task run:wazero
# or
task pc:up  # with process-compose
```

**Features:**
- SVG, PNG, PDF rendering (native binaries)
- Import/include via file system
- Demo at http://localhost:8080
- Examples from `.src/deckviz`

### Wrangler (Cloudflare Emulation)

```bash
task run:wrangler
```

**Features:**
- SVG rendering only (WASM)
- Import/include via R2 pre-expansion
- Demo at http://localhost:8787
- Local R2 buckets and KV

---

## Features Deployed

### 1. **Import/Include Support** ✅

Works in all environments:

**Native (Wazero, CLI):**
- Decksh handles imports naturally via file system
- `import "file.dsh"` loads function definitions
- `include "file.dsh"` includes full content

**WASM (Cloudflare):**
- Pre-expansion from R2 storage
- `import` → Extracts and inlines `def/edef` blocks
- `include` → Recursively expands and inlines content
- Prevents duplicate function definitions

### 2. **Demo HTML** ✅

Served at root `/` with content negotiation:

**Browser request:**
```bash
curl https://deckfs.gedw99.workers.dev/ -H "Accept: text/html"
# Returns: Full demo HTML interface
```

**API request:**
```bash
curl https://deckfs.gedw99.workers.dev/ -H "Accept: application/json"
# Returns: {"service": "deckfs", "endpoints": [...]}
```

### 3. **Processing Endpoints** ✅

#### Direct Processing
```bash
POST /process
Content-Type: text/plain

deck
  slide "white" "black"
    ctext "Hello World" 50 50 5
  eslide
edeck
```

#### With Imports
```bash
POST /process?source=decks/main.dsh
Content-Type: text/plain

import "common/utils.dsh"
deck
  slide
    myFunction 50 50
  eslide
edeck
```

#### Upload and Process
```bash
PUT /upload/my-deck.dsh
Content-Type: text/plain

[decksh source]
```

Automatically:
1. Stores to R2 input bucket
2. Processes to SVG
3. Stores slides to R2 output bucket
4. Updates KV status

---

## Testing

### Unit Tests
```bash
task test:unit
```

### Import Resolver Tests
```bash
task test:imports
```

### End-to-End Tests
```bash
task test:e2e
```

### Integration Tests with Real Examples
```bash
task test:clone    # Clone ajstarks' test repos
task test:decksh   # Test against decksh examples
task test:deckviz  # Test against deckviz examples
```

---

## Architecture

### Build Artifacts

```
.bin/
├── deck/               # ajstarks CLI tools (for native)
│   ├── decksh         # DSL parser
│   ├── svgdeck        # SVG renderer
│   ├── pngdeck        # PNG renderer
│   └── pdfdeck        # PDF renderer
├── cloudflare/        # Cloudflare Worker
│   └── app.wasm       # TinyGo WASM (1.6 MB)
├── wazero/            # Host server
│   └── deckfs-host    # Go binary
└── deckfs             # CLI tool
```

### Pipeline Implementations

**Native Pipeline:** [pkg/pipeline/native.go](../pkg/pipeline/native.go)
- Uses `os/exec` to pipe to ajstarks' binaries
- Supports SVG, PNG, PDF
- Import support via file system

**WASM Pipeline:** [pkg/pipeline/wasm.go](../pkg/pipeline/wasm.go)
- Uses decksh package in-process
- Supports SVG only
- Import support via pre-expansion

**Import Resolver:** [pkg/pipeline/imports.go](../pkg/pipeline/imports.go)
- Pre-processes imports for WASM
- Extracts `def/edef` blocks
- Recursively expands includes
- Storage-agnostic loader

---

## Monitoring

### Health Check
```bash
curl https://deckfs.gedw99.workers.dev/health
# {"status":"ok","version":"0.1.0"}
```

### Processing Status
```bash
curl https://deckfs.gedw99.workers.dev/status/my-deck.dsh
# {"key":"my-deck.dsh","status":"complete","updatedAt":"..."}
```

### List Processed Decks
```bash
curl https://deckfs.gedw99.workers.dev/decks
# {"decks":["my-deck","another-deck"]}
```

---

## Troubleshooting

### Import Resolution Fails

**Problem:** `Import resolution failed: failed to load import "file.dsh"`

**Solution:** Ensure the imported file exists in R2 input bucket:
```bash
# Upload imported files first
wrangler r2 object put deckfs-input/common/utils.dsh --file=./common/utils.dsh

# Then upload main file with source path
curl -X POST https://deckfs.gedw99.workers.dev/process?source=decks/main.dsh \
  -d @decks/main.dsh
```

### WASM Out of Memory

**Problem:** Worker exceeds memory limit

**Solution:**
- Reduce source file complexity
- Split into smaller decks
- Avoid deeply nested includes

### Slow Processing

**Problem:** Processing takes > 50ms

**Solution:**
- Check if imports are being expanded multiple times
- Ensure function definitions are cached (no duplicates)
- Consider pre-expanding imports before upload

---

## Next Steps

1. **Upload Examples to R2:**
   ```bash
   task cf:r2-upload-examples
   ```

2. **Configure Custom Domain:**
   ```bash
   wrangler domains add deckfs.example.com
   ```

3. **Monitor Logs:**
   ```bash
   wrangler tail
   ```

4. **Update Workers:**
   ```bash
   task cf:deploy
   ```

---

## Resources

- **Live Demo:** https://deckfs.gedw99.workers.dev
- **API Docs:** [ENDPOINTS.md](./ENDPOINTS.md)
- **Architecture:** [ADR 0001](./adr/0001-pipeline-architecture.md)
- **Source:** https://github.com/joeblew99/deckfs
