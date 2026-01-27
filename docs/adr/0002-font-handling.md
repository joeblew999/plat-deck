# ADR 0002: Dynamic Font Management with Google Fonts

## Status

Proposed

## Date

2026-01-27

## Context

### Current State

All deck rendering (SVG, PNG, PDF) requires fonts:

1. **SVG Rendering**: References fonts via `font-family` in XML
   - Fonts can be system fonts or web fonts
   - No embedding required, just references

2. **PNG/PDF Rendering**: Requires actual TTF font files
   - `pngdeck` needs `-fontdir` pointing to TTF files
   - `pdfdeck` needs `-fontdir` pointing to TTF files
   - Currently uses `.src/deckfonts/` directory with limited font set

3. **Current Limitations**:
   - Users limited to fonts in `.src/deckfonts/` Git repo
   - Manual font installation required for new fonts
   - No automatic font discovery or fetching
   - Font files checked into Git (bloat)

### Existing Tools

ajstarks/deck provides:
- **`deck.Deck` package**: Parses deck XML with full type definitions
- **`deckinfo` tool**: Inspects deck structure and extracts metadata
- **Font attributes**: `Text.Font` and `ListItem.Font` fields in XML

Font detection strategy:
```go
import "github.com/ajstarks/deck"

// After decksh converts .dsh → XML
var d deck.Deck
xml.Unmarshal(xmlData, &d)

// Extract fonts from all slides
for _, slide := range d.Slide {
    for _, text := range slide.Text {
        if text.Font != "" {
            fonts[text.Font] = true  // Collect unique fonts
        }
    }
    for _, list := range slide.List {
        for _, item := range list.Li {
            if item.Font != "" {
                fonts[item.Font] = true
            }
        }
    }
}
```

## Decision

Implement **dynamic font discovery and fetching** using Google Fonts API with multi-environment caching.

**Key Insight**: Use deck XML (not decksh source) for font detection since we already convert .dsh→XML in the pipeline, and ajstarks provides robust XML parsing.

### Architecture

```
┌─────────────┐
│ Deck Source │
│   (.dsh)    │
└──────┬──────┘
       │
       ▼
┌─────────────┐      ┌──────────────┐
│   decksh    │─────▶│   Deck XML   │
│   (binary)  │      │              │
└─────────────┘      └──────┬───────┘
                            │
                            ▼
                    ┌────────────────┐
                    │ Font Detector  │
                    │ - Parse XML    │
                    │ - Extract fonts│
                    │ - Use deck pkg │
                    └────────┬───────┘
                            │
                            ▼
┌─────────────────────┐         ┌──────────────┐
│  Font Resolver      │────────▶│ Google Fonts │
│  - Check cache      │         │     API      │
│  - Fetch if missing │◀────────│              │
│  - Store in cache   │         └──────────────┘
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│  Font Cache         │
│  - Local: .fonts/   │
│  - CF: R2 bucket    │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│  Renderer           │
│  - svgdeck: refs    │
│  - pngdeck: files   │
│  - pdfdeck: files   │
└─────────────────────┘
```

### Components

#### 1. Font Detector (using deck package)

Extract fonts from deck XML after decksh conversion:

```go
package fonts

import (
	"encoding/xml"
	"github.com/ajstarks/deck"
)

type FontDetector struct{}

func (d *FontDetector) DetectFonts(xmlData []byte) ([]string, error) {
	var deckXML deck.Deck
	if err := xml.Unmarshal(xmlData, &deckXML); err != nil {
		return nil, err
	}

	fontSet := make(map[string]bool)
	
	// Extract fonts from all slides
	for _, slide := range deckXML.Slide {
		// Text elements
		for _, text := range slide.Text {
			if text.Font != "" {
				fontSet[text.Font] = true
			} else {
				fontSet["sans"] = true // default font
			}
		}
		
		// List item fonts
		for _, list := range slide.List {
			if list.Font != "" {
				fontSet[list.Font] = true
			}
			for _, item := range list.Li {
				if item.Font != "" {
					fontSet[item.Font] = true
				}
			}
		}
	}
	
	// Convert set to slice
	fonts := make([]string, 0, len(fontSet))
	for font := range fontSet {
		fonts = append(fonts, font)
	}
	
	return fonts, nil
}
```

**Advantages**:
- Uses ajstarks' canonical deck XML format
- Leverages existing type-safe parsing
- No fragile regex on decksh source
- Works even with complex decksh (variables, loops, conditionals)

#### 2. Font Resolver

Fetches fonts from Google Fonts and caches them:

```go
package fonts

type FontResolver interface {
	Resolve(ctx context.Context, fontNames []string) (map[string]FontFiles, error)
}

type FontFiles struct {
	Regular []byte
	Bold    []byte
	Italic  []byte
	// Additional variants as needed
}

type GoogleFontsResolver struct {
	apiKey string
	cache  FontCache
}

func (r *GoogleFontsResolver) Resolve(ctx context.Context, fontNames []string) (map[string]FontFiles, error) {
	result := make(map[string]FontFiles)
	
	for _, name := range fontNames {
		// Resolve alias (sans → Open Sans, serif → Merriweather, etc.)
		resolvedName := r.resolveAlias(name)
		
		// Check cache first
		if cached, err := r.cache.Get(ctx, resolvedName); err == nil {
			result[name] = cached
			continue
		}
		
		// Fetch from Google Fonts API
		fetched, err := r.fetchFromGoogleFonts(ctx, resolvedName)
		if err != nil {
			// Log warning but don't fail - renderer will use fallback
			log.Printf("Warning: couldn't fetch font %q: %v", resolvedName, err)
			continue
		}
		
		// Store in cache
		if err := r.cache.Put(ctx, resolvedName, fetched); err != nil {
			return nil, err
		}
		
		result[name] = fetched
	}
	
	return result, nil
}

var fontAliases = map[string]string{
	"sans":       "Open Sans",
	"serif":      "Merriweather", 
	"mono":       "Roboto Mono",
	"sans-serif": "Open Sans",
	"monospace":  "Roboto Mono",
}

func (r *GoogleFontsResolver) resolveAlias(name string) string {
	if resolved, ok := fontAliases[name]; ok {
		return resolved
	}
	return name
}
```

#### 3. Font Cache

Environment-specific font storage:

```go
package fonts

type FontCache interface {
	Get(ctx context.Context, fontName string) (FontFiles, error)
	Put(ctx context.Context, fontName string, files FontFiles) error
	List(ctx context.Context) ([]string, error)
}

// Native environment: filesystem cache
type LocalFontCache struct {
	dir string // ".fonts/" by default
}

// Cloudflare Workers: R2 bucket
type R2FontCache struct {
	bucket runtime.Bucket
}
```

**Cache Structure**:
```
.fonts/                           # Local cache
├── open-sans/
│   ├── regular.ttf
│   ├── bold.ttf
│   └── italic.ttf
├── roboto-mono/
│   └── regular.ttf
├── merriweather/
│   ├── regular.ttf
│   └── bold.ttf
└── .index.json                   # Font metadata

R2 Bucket: deckfs-fonts          # Cloudflare cache
├── open-sans/regular.ttf
├── open-sans/bold.ttf
├── roboto-mono/regular.ttf
└── merriweather/regular.ttf
```

#### 4. Google Fonts API Integration

**API Endpoints**:
```
GET https://www.googleapis.com/webfonts/v1/webfonts?key=API_KEY
```

**Response Example**:
```json
{
  "items": [
    {
      "family": "Open Sans",
      "variants": ["regular", "italic", "700", "700italic"],
      "files": {
        "regular": "http://fonts.gstatic.com/s/opensans/v18/regular.ttf",
        "700": "http://fonts.gstatic.com/s/opensans/v18/bold.ttf"
      }
    }
  ]
}
```

**Font Download Process**:
1. Query API for font family
2. Download TTF files for regular, bold, italic variants
3. Store in cache with normalized name (lowercase, hyphens)

### Pipeline Integration

#### Native Pipeline (pkg/pipeline/native.go)

```go
// Modified ProcessWithWorkDir
func (p *NativePipeline) ProcessWithWorkDir(ctx context.Context, source []byte, format OutputFormat, workDir string) (*Result, error) {
	var xmlData []byte
	var err error

	// Step 1: decksh → XML (existing code)
	if workDir != "" {
		xmlData, err = p.runDeckshFile(ctx, source, workDir)
	} else {
		xmlData, err = p.runDeckshStdin(ctx, source)
	}
	if err != nil {
		return nil, err
	}

	// Step 2: Parse XML (existing code)
	var d deck.Deck
	if err := xml.Unmarshal(xmlData, &d); err != nil {
		return nil, fmt.Errorf("failed to parse deck XML: %w", err)
	}

	// Step 3: NEW - Detect and resolve fonts (PNG/PDF only)
	if format == FormatPNG || format == FormatPDF {
		detector := &fonts.FontDetector{}
		fontNames, _ := detector.DetectFonts(xmlData)
		
		if len(fontNames) > 0 && p.fontResolver != nil {
			_, err := p.fontResolver.Resolve(ctx, fontNames)
			if err != nil {
				// Log warning but continue - renderers will use fallbacks
				log.Printf("Font resolution warning: %v", err)
			}
			
			// Point renderer to cache directory
			os.Setenv("DECKFONTS", p.fontCache.Dir())
		}
	}

	// Step 4: Render (existing code)
	slides, err := p.renderSlides(ctx, rendererBin, xmlData, len(d.Slide), format, assetDir)
	// ...
}
```

#### Cloudflare Workers (for future PNG/PDF support)

```go
func (w *Worker) handleProcess(r *http.Request) (*Response, error) {
	source, _ := io.ReadAll(r.Body)
	
	// Convert to XML
	xmlData, _ := w.convertToXML(source)
	
	// Detect and pre-cache fonts
	detector := &fonts.FontDetector{}
	fontNames, _ := detector.DetectFonts(xmlData)
	
	if len(fontNames) > 0 && w.fontResolver != nil {
		_, _ = w.fontResolver.Resolve(r.Context(), fontNames)
		// Fonts now cached in R2 for future use
	}
	
	// Process (SVG for now, PNG/PDF when WASM supports it)
	result, _ := w.pipeline.Process(r.Context(), source, pipeline.FormatSVG)
	
	return result, nil
}
```

### API Usage

**Automatic font resolution** (transparent to user):
```bash
curl -X POST 'http://localhost:8080/process?format=png' \
  --data 'deck
  slide
    text "Hello" 50 50 5 "Roboto"
    text "World" 50 50 3 "Lato"
  eslide
edeck'
# Roboto and Lato automatically fetched and cached
```

**Font management endpoints** (optional):
```bash
# List cached fonts
GET /fonts
{
  "cached": ["Open Sans", "Roboto", "Merriweather"],
  "aliases": {"sans": "Open Sans", "serif": "Merriweather"}
}

# Pre-cache fonts
POST /fonts/cache
{"fonts": ["Roboto", "Lato", "Poppins"]}

# Clear cache
DELETE /fonts/cache
```

## Consequences

### Positive

1. **Robust font detection** - Uses canonical deck XML, not fragile regex
2. **No Git bloat** - Font files not in repository
3. **1000+ fonts available** - Access to entire Google Fonts library
4. **Automatic resolution** - Transparent to users
5. **Caching** - Fast subsequent renders
6. **Cloudflare ready** - R2 cache for Workers
7. **Graceful degradation** - Missing fonts use system defaults
8. **Reuses existing code** - Leverages ajstarks' deck package

### Negative

1. **API dependency** - Requires Google Fonts API access
2. **Initial latency** - First render slower (downloading fonts)
3. **API key needed** - Must configure GOOGLE_FONTS_API_KEY
4. **Network required** - Can't work fully offline (first time)
5. **Storage costs** - R2 storage for Cloudflare fonts (~pennies)

### Neutral

1. **Cache size** - Font files ~50-200KB each
2. **Variant handling** - Downloads regular, bold, italic variants
3. **Font licensing** - Google Fonts are open source (no issues)

## Alternatives Considered

### 1. Parse decksh source with regex

Rejected because:
- decksh has complex syntax (variables, loops, conditionals)
- Regex would be fragile and miss edge cases
- deck XML is canonical format, already parsed

### 2. Embed all fonts in binary

Rejected because:
- Binary size would be huge (100+ MB)
- Can't include every possible font
- Users still limited to embedded set

### 3. User provides font directory

Rejected because:
- Manual setup burden
- Not cloud-friendly (Cloudflare Workers)
- Doesn't scale for many users

## Implementation Plan

### Phase 1: Font Detection (Week 1)
- [ ] Implement FontDetector using deck package
- [ ] Extract fonts from deck XML
- [ ] Handle default fonts (sans, serif, mono)
- [ ] Unit tests for detection

### Phase 2: Local Cache (Week 1-2)
- [ ] Implement LocalFontCache
- [ ] Create `.fonts/` directory structure
- [ ] Add cache index/metadata
- [ ] Test cache read/write

### Phase 3: Google Fonts Integration (Week 2-3)
- [ ] Implement GoogleFontsResolver
- [ ] Query Google Fonts API
- [ ] Download TTF files
- [ ] Handle API errors gracefully
- [ ] Font alias mapping (sans→Open Sans, etc.)

### Phase 4: Native Pipeline Integration (Week 3)
- [ ] Update NativePipeline with font resolver
- [ ] Set DECKFONTS to cache directory
- [ ] Add /fonts API endpoints (optional)
- [ ] Test with real decks

### Phase 5: Cloudflare Integration (Week 4)
- [ ] Implement R2FontCache
- [ ] Update Worker with font resolver
- [ ] Test font caching in R2
- [ ] Document Cloudflare setup

### Phase 6: Documentation & Testing (Week 4)
- [ ] Update README with font documentation
- [ ] Add examples using various Google Fonts
- [ ] Integration tests
- [ ] Performance benchmarks

## Configuration

### Environment Variables

```bash
# Google Fonts API key (required for auto-fetch)
GOOGLE_FONTS_API_KEY=your_api_key_here

# Font cache directory (native server, default: .fonts)
FONT_CACHE_DIR=.fonts

# R2 bucket for fonts (Cloudflare)
FONT_BUCKET_NAME=deckfs-fonts
```

### Wrangler Configuration

```toml
# wrangler.toml
[[r2_buckets]]
binding = "FONTS"
bucket_name = "deckfs-fonts"

[vars]
GOOGLE_FONTS_API_KEY = "your_api_key_here"
```

## References

- [Google Fonts API](https://developers.google.com/fonts/docs/developer_api)
- [Google Fonts](https://fonts.google.com/)
- [ajstarks/deck package](https://github.com/ajstarks/deck)
- [ajstarks/deck deckinfo tool](https://github.com/ajstarks/deck/tree/master/cmd/deckinfo)
- [ADR 0001: Pipeline Architecture](done/0001-pipeline-architecture.md)
