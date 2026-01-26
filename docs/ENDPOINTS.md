# DeckFS Endpoints

## Development Servers

### Wazero Server (Native)
```bash
task run:wazero
# or
.bin/wazero/deckfs-host -addr :8080 -examples .src/deckviz
```

**URL:** http://localhost:8080

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Demo HTML interface |
| `/api` | GET | API information (JSON) |
| `/health` | GET | Health check |
| `/process` | POST | Process decksh source to SVG |
| `/examples` | GET | List available examples |
| `/examples/{path}` | GET | Get example source content |

**Features:**
- Full SVG, PNG, PDF support (uses ajstarks CLI tools)
- Import/include support via file system
- Serves demo HTML at root

---

### Wrangler (Cloudflare Local)
```bash
task run:wrangler
```

**URL:** http://localhost:8787

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Demo HTML interface (from browser: text/html) |
| `/` | GET | API information (from API clients: application/json) |
| `/health` | GET | Health check |
| `/process` | POST | Process decksh source to SVG |
| `/upload/{key}` | PUT/POST | Upload source to R2 and process |
| `/slides/{key}` | GET | Get rendered slide |
| `/manifest/{name}` | GET | Get deck manifest |
| `/decks` | GET | List all processed decks |
| `/status/{key}` | GET | Get processing status |

**Features:**
- SVG only (WASM-based rendering)
- Import/include support via R2 storage pre-expansion
- Queue-based reactive processing
- Serves demo HTML at root (content negotiation)

---

### Demo Server (Static)
```bash
task run:demo
```

**URL:** http://localhost:3000

- Serves `demo/index.html` statically
- Frontend only, connects to backend API

---

## Production

### Cloudflare Workers
```bash
task cf:deploy
```

**URL:** https://deckfs.gedw99.workers.dev

Same endpoints as Wrangler, deployed to Cloudflare's global network.

---

## Content Negotiation

The root `/` endpoint serves different content based on the `Accept` header:

**Browser request** (Accept: text/html):
```bash
curl -H "Accept: text/html" http://localhost:8080/
# Returns: demo HTML interface
```

**API request** (Accept: application/json):
```bash
curl -H "Accept: application/json" http://localhost:8080/
# Returns: {"service": "deckfs", "version": "...", "endpoints": [...]}
```

**Note:** Wazero also provides `/api` endpoint for explicit API info.

---

## Demo Usage

1. **Open browser:** Navigate to http://localhost:8080 (wazero) or http://localhost:8787 (wrangler)
2. **Edit source:** Modify decksh markup in the textarea
3. **Process:** Click "Process" or press Ctrl+Enter
4. **Navigate:** Use arrow keys or buttons to view slides
5. **Select examples:** Choose from dropdown to load pre-made decks

---

## Import/Include Support

### Native (Wazero)
Imports work automatically via file system:
```
import "coord.dsh"
include "header.dsh"
```

### WASM (Cloudflare)
Imports pre-expanded from R2 storage:
1. Upload source files to R2 input bucket
2. API loads and inlines imports before processing
3. Decksh receives fully expanded source

Query parameter for import resolution:
```
POST /process?source=decks/main.dsh
```
