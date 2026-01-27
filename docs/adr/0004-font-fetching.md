# ADR 0004: Font Fetching and Caching

## Status

**Accepted** - Strategy decided using existing storage abstraction

## Date

2026-01-27

## Context

### Problem Statement

To render PNG/PDF correctly, we need:
1. Actual TTF font files (not just font names)
2. Cache mechanism to avoid re-downloading
3. Works in both Cloudflare Workers and native server
4. Unified approach with Google Fonts API

### Key Insight

**Font files needed for:**
- ❌ **SVG**: Only needs font names (see ADR 0002)
- ✅ **PNG**: pngdeck requires TTF files via `-fontdir`
- ✅ **PDF**: pdfdeck requires TTF files via `-fontdir`

**Platform support:**
- **PNG in Cloudflare**: ✅ Will work (pngdeck is Go → compile to WASM)
- **PDF in Cloudflare**: ✅ Will work later (future ADR for WASM compilation)
- **Both in Native**: ✅ Already works via native binaries

**Therefore:** Font fetching MUST work in Cloudflare Workers, not just native server!

### Existing Infrastructure

**plat-deck already has storage abstraction** (runtime/ package):

```go
type Storage interface {
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Put(ctx context.Context, key string, data []byte, contentType string) error
    List(ctx context.Context, prefix string, delimiter string) (*ListResult, error)
    Delete(ctx context.Context, key string) error
}
```

**Implementations:**
- `R2Storage` - Cloudflare R2 buckets (runtime/storage_cloudflare.go)
- `R2HTTPStorage` - Native Go → R2 via S3 API (runtime/storage_http.go)
- `LocalStorage` - Local filesystem (to be added for native)

**Current R2 buckets:**
- DECKFS_INPUT, DECKFS_OUTPUT, DECKFS_WASM
- Need: DECKFS_FONTS

## Decision

**Use Google Fonts API + runtime.Storage abstraction**

Design for ALL formats (SVG/PNG/PDF) working in Cloudflare Workers.

### Strategy

1. **Single source of fonts:** Google Fonts API
2. **Storage abstraction:** Use existing runtime.Storage
3. **Platform-specific storage:**
   - **Cloudflare Workers**: R2Storage (DECKFS_FONTS bucket) ← Primary use case!
   - **Native Server**: LocalStorage (.fonts/ directory) for fast local cache
   - **Native Server**: Can also use R2HTTPStorage to share cache with Workers

4. **SVG font modes** (user choice):
   - `fallback` - CSS fallback lists (no files needed)
   - `cdn` - Inject Google Fonts @import (no files needed)
   - `embed` - Base64 embed fonts (downloads fonts, large files)

5. **PNG/PDF fonts** (automatic):
   - Always downloads from Google Fonts
   - Caches in R2 (Workers) or local FS (native)
   - In Workers: Passes fonts to pngdeck/pdfdeck WASM modules
   - In Native: Passes via `-fontdir` to native binaries

### Architecture for Cloudflare Workers

```
User Request (PNG)
    ↓
Cloudflare Worker
    ↓
Detect fonts from deck XML
    ↓
Font Manager
    ├─ Check R2 cache (DECKFS_FONTS)
    │   ├─ Hit → Return cached font
    │   └─ Miss ↓
    └─ Download from Google Fonts API
        └─ Cache in R2 for next time
    ↓
Pass fonts to pngdeck WASM
    ↓
Render PNG
```

**Benefits:**
- Font cache shared across ALL Worker instances globally
- One download → cached for all users
- No local filesystem needed
- Same code works in Workers and native

## Implementation

### Add DECKFS_FONTS Bucket

**wrangler.toml:**
```toml
[[r2_buckets]]
binding = "DECKFS_FONTS"
bucket_name = "deckfs-fonts"
```

**Create bucket:**
```bash
task cf:setup  # Creates all buckets including DECKFS_FONTS
```

### Add LocalStorage Implementation (Native Only)

**runtime/storage_local.go:**
```go
//go:build !cloudflare

package runtime

import (
    "context"
    "io"
    "os"
    "path/filepath"
)

type LocalStorage struct {
    basePath string
}

func NewLocalStorage(basePath string) (*LocalStorage, error) {
    if err := os.MkdirAll(basePath, 0755); err != nil {
        return nil, err
    }
    return &LocalStorage{basePath: basePath}, nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
    return os.Open(filepath.Join(s.basePath, key))
}

func (s *LocalStorage) Put(ctx context.Context, key string, data []byte, contentType string) error {
    path := filepath.Join(s.basePath, key)
    os.MkdirAll(filepath.Dir(path), 0755)
    return os.WriteFile(path, data, 0644)
}

func (s *LocalStorage) List(ctx context.Context, prefix string, delimiter string) (*ListResult, error) {
    // Implementation: filepath.Walk
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
    return os.Remove(filepath.Join(s.basePath, key))
}
```

### Update Runtime Struct

**runtime/runtime.go:**
```go
type Runtime struct {
    InputStorage  Storage
    OutputStorage Storage
    FontStorage   Storage  // ← Add this
    KV            KVStore
    Publisher     Publisher
}

func Fonts() Storage {
    if Current == nil || Current.FontStorage == nil {
        return &noopStorage{}
    }
    return Current.FontStorage
}
```

### Font Manager (Cross-Platform)

**pkg/fonts/manager.go:**
```go
package fonts

import (
    "context"
    "io"
    
    "github.com/joeblew99/plat-deck/runtime"
)

type Manager struct {
    storage runtime.Storage
    api     *GoogleFontsAPI
}

func NewManager(storage runtime.Storage) *Manager {
    return &Manager{
        storage: storage,
        api:     NewGoogleFontsAPI(),
    }
}

// GetFont returns font data, from cache or downloads
func (m *Manager) GetFont(ctx context.Context, fontName string) ([]byte, error) {
    key := "fonts/" + fontName + "-Regular.ttf"
    
    // Check cache first
    r, err := m.storage.Get(ctx, key)
    if err == nil {
        defer r.Close()
        return io.ReadAll(r)
    }
    
    // Cache miss - download from Google Fonts
    fontData, err := m.api.Download(ctx, fontName, "regular")
    if err != nil {
        return nil, err
    }
    
    // Cache for next time (async, don't fail on cache error)
    go m.storage.Put(context.Background(), key, fontData, "font/ttf")
    
    return fontData, nil
}

// GetFonts returns multiple fonts (for PNG/PDF rendering)
func (m *Manager) GetFonts(ctx context.Context, fontNames []string) (map[string][]byte, error) {
    fonts := make(map[string][]byte)
    for _, name := range fontNames {
        data, err := m.GetFont(ctx, name)
        if err != nil {
            // Skip missing fonts, don't fail entire request
            continue
        }
        fonts[name] = data
    }
    return fonts, nil
}
```

### Cloudflare Worker Initialization

**cmd/cloudflare/main.go:**
```go
func init() {
    inputStorage, _ := runtime.NewR2Storage("DECKFS_INPUT")
    outputStorage, _ := runtime.NewR2Storage("DECKFS_OUTPUT")
    fontStorage, _ := runtime.NewR2Storage("DECKFS_FONTS")  // ← New!
    kvStore, _ := runtime.NewCloudflareKV("DECKFS_STATUS")

    runtime.SetRuntime(&runtime.Runtime{
        InputStorage:  inputStorage,
        OutputStorage: outputStorage,
        FontStorage:   fontStorage,  // ← Required for PNG/PDF
        KV:            kvStore,
    })
}
```

### Native Server Initialization

**cmd/wazero/main.go (or similar):**
```go
func main() {
    // Option 1: Local filesystem cache (faster, no R2 costs)
    fontStorage, _ := runtime.NewLocalStorage(".fonts/")

    // Option 2: Use R2 (shared cache with Workers, slower)
    // fontStorage := runtime.NewR2HTTPStorage(runtime.R2HTTPConfig{...})

    runtime.SetRuntime(&runtime.Runtime{
        InputStorage:  localInput,
        OutputStorage: localOutput,
        FontStorage:   fontStorage,
    })
}
```

## Font Variants Strategy

**Start simple:**
- Only download Regular variant (~150KB)
- Key format: `fonts/Roboto-Regular.ttf`
- If pngdeck needs Bold/Italic, download on-demand with keys:
  - `fonts/Roboto-Bold.ttf`
  - `fonts/Roboto-Italic.ttf`

**Future enhancement:**
- Detect which variants deck actually uses (parse XML for bold/italic text)
- Download only required variants
- Font family manifests

## Google Fonts API Integration

### Discovery API

**No API key required for public API:**

```bash
GET https://www.googleapis.com/webfonts/v1/webfonts?key=YOUR_KEY
# OR for public use:
GET https://fonts.google.com/metadata/fonts
```

Response:
```json
{
  "familyMetadataList": [
    {
      "family": "Roboto",
      "fonts": {
        "400": {
          "thickness": 400,
          "style": "normal",
          "url": "https://fonts.gstatic.com/s/roboto/v30/KFOmCnqEu92Fr1Mu4mxK.ttf"
        },
        "700": { ... }
      }
    }
  ]
}
```

### Download Flow

```go
type GoogleFontsAPI struct {
    // No auth needed for downloads
}

func (api *GoogleFontsAPI) Download(ctx context.Context, fontFamily, variant string) ([]byte, error) {
    // 1. Get font metadata (optional - can hardcode URLs)
    // Most Google Fonts follow pattern:
    url := fmt.Sprintf("https://fonts.gstatic.com/s/%s/v30/Font-File-Name.ttf", 
        strings.ToLower(fontFamily))
    
    // 2. Download TTF
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("font download failed: %s", resp.Status)
    }
    
    return io.ReadAll(resp.Body)
}
```

**Alternative:** Use official CSS API to find TTF URLs:
```bash
GET https://fonts.googleapis.com/css2?family=Roboto
# Returns CSS with @font-face and TTF URLs
```

## Integration with PNG/PDF Pipeline

### WASM Pipeline (Cloudflare)

```go
// pkg/pipeline/wasm.go
func (p *WASMPipeline) Process(ctx context.Context, source []byte, format OutputFormat) (*Result, error) {
    if format == FormatPNG || format == FormatPDF {
        // 1. Detect fonts from deck XML
        fonts := DetectFonts(deckXML)
        
        // 2. Fetch fonts from R2 cache / Google Fonts
        fontMgr := fonts.NewManager(runtime.Fonts())
        fontData, _ := fontMgr.GetFonts(ctx, fonts)
        
        // 3. Pass fonts to pngdeck/pdfdeck WASM
        // (Future: when PNG/PDF WASM ready)
        // pngdeck.RenderWithFonts(deckXML, fontData)
    }
    // ... existing SVG logic
}
```

### Native Pipeline

```go
// pkg/pipeline/native.go
func (p *NativePipeline) renderSlides(...) {
    // 1. Detect fonts
    fonts := DetectFonts(xmlData)
    
    // 2. Fetch fonts
    fontMgr := fonts.NewManager(runtime.Fonts())
    fontData, _ := fontMgr.GetFonts(ctx, fonts)
    
    // 3. Write to temp dir
    tmpDir, _ := os.MkdirTemp("", "deckfs-fonts-")
    defer os.RemoveAll(tmpDir)
    
    for name, data := range fontData {
        os.WriteFile(filepath.Join(tmpDir, name+".ttf"), data, 0644)
    }
    
    // 4. Pass to pngdeck/pdfdeck
    cmd := exec.Command(rendererBin, "-fontdir", tmpDir, ...)
}
```

## Storage Costs & Performance

**Cloudflare R2 Free Tier:**
- Storage: 10 GB/month
- Class A ops (writes): 1M/month
- Class B ops (reads): 10M/month

**Font sizes:**
- Average font: ~150KB
- 10GB = ~66,000 fonts (way more than needed!)
- Top 100 fonts = ~15MB total

**Expected usage:**
- Popular fonts (Roboto, Open Sans, etc.) cached once globally
- Cache hit ratio: >95% after warmup
- Occasional cache miss → download → R2 PUT
- All Worker instances share same R2 cache

**Performance:**
- R2 read: ~50ms
- Google Fonts download: ~200ms
- After first download: Always R2 cache hit (fast)

## Success Criteria

- ✅ Can download fonts from Google Fonts API
- ✅ Fonts cached in R2 (Cloudflare) or local FS (native)
- ✅ PNG rendering works with custom fonts in Workers
- ✅ PDF rendering works with custom fonts (future)
- ✅ Same code works in both Cloudflare and native
- ✅ Graceful fallback if fonts unavailable
- ✅ No API keys or auth required

## Implementation Phases

**Phase 1: Core Infrastructure**
1. Add `runtime/storage_local.go` (native filesystem)
2. Add `FontStorage` field to Runtime struct
3. Create DECKFS_FONTS bucket (`task cf:setup`)
4. Update Cloudflare/native initialization with FontStorage

**Phase 2: Font Manager**
1. Create `pkg/fonts/manager.go` with Manager type
2. Implement Google Fonts API client (no auth)
3. Add caching logic using runtime.Storage
4. Add font detection from deck XML (from ADR 0002)

**Phase 3: Native Pipeline Integration**
1. Integrate FontManager with native pipeline
2. Create temp fontdir with cached fonts
3. Pass `-fontdir` to pngdeck/pdfdeck
4. Handle missing fonts gracefully

**Phase 4: WASM Pipeline Integration (When PNG/PDF WASM Ready)**
1. Integrate FontManager with WASM pipeline
2. Pass font data directly to pngdeck/pdfdeck WASM modules
3. Test in Cloudflare Workers
4. Performance testing

**Phase 5: SVG Embed Mode (Optional)**
1. Add font embedding logic for SVG
2. Support `svgFontMode=embed` in API
3. Test file sizes (expect ~150KB increase per font)

## References

- [Google Fonts](https://fonts.google.com/)
- [Google Fonts Developer API](https://developers.google.com/fonts/docs/developer_api)
- [Cloudflare R2](https://developers.cloudflare.com/r2/)
- runtime/storage_cloudflare.go (R2 implementation)
- runtime/storage_http.go (R2 HTTP/S3 client)
- pngdeck source: .src/deck/cmd/pngdeck/
- pdfdeck source: .src/deck/cmd/pdfdeck/

## Related ADRs

- ADR 0002: Font Discovery (detection strategy)
- ADR 0003: Font Display (Web GUI UX)
- Future ADR: PDF in Cloudflare Workers (WASM compilation)
