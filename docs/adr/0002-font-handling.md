# ADR 0002: Dynamic Font Management with Google Fonts

## Status

Draft - Needs Review

## Date

2026-01-27

## Context

### Problem

All deck rendering (SVG, PNG, PDF) requires fonts, but:
- Users limited to fonts in `.src/deckfonts/` Git repo
- Manual font installation required
- Font files checked into Git (bloat)
- No way to use Google Fonts or custom fonts

### Critical Requirement

**We must detect fonts in BOTH decksh source AND deck XML:**

1. **Decksh Source** (.dsh files):
   - Needed for pre-caching before processing
   - Required for Cloudflare Workers (can't run decksh binary there)
   - Allows early validation

2. **Deck XML** (after decksh conversion):
   - 100% accurate (canonical format)
   - Handles variables, loops, conditionals
   - Used for final validation before rendering

## Decision

NOT READY - needs both detection approaches working

### What We Need

#### 1. Dual Font Detection

**A. Decksh Source Parser**
- Regex-based detection of font names in .dsh files
- Fast but approximate (may miss complex cases)
- Good enough for pre-caching

**B. Deck XML Parser**  
- Use `github.com/ajstarks/deck` package
- Parse XML after decksh conversion
- 100% accurate

#### 2. Google Fonts Integration

- Query Google Fonts API for font metadata
- Download TTF files on demand
- Cache locally (.fonts/) or in R2 (Cloudflare)

#### 3. Multi-Environment Support

- Native: Filesystem cache + direct binary execution
- Cloudflare: R2 cache + WASM pipeline

## Open Questions

1. How accurate does decksh regex detection need to be?
2. Should we pre-cache aggressively or fetch on-demand?
3. What happens when Google Fonts API is unavailable?
4. How do we handle font variants (bold, italic)?
5. Cache eviction strategy?
6. API rate limiting concerns?

## Next Steps

1. Implement decksh source detector (regex-based)
2. Test regex accuracy on real deckviz examples
3. Implement deck XML detector (using deck package)
4. Compare results from both detectors
5. Design font resolver interface
6. Prototype Google Fonts API client
7. Design cache structure

## References

- [Google Fonts API](https://developers.google.com/fonts/docs/developer_api)
- [ajstarks/deck package](https://github.com/ajstarks/deck)
- [ADR 0001: Pipeline Architecture](done/0001-pipeline-architecture.md)

## Use Cases

### 1. Web GUI Font Display

**Requirement**: Demo UI should show what fonts a deck uses

**User Experience**:
```
┌─────────────────────────────────────┐
│ DeckFS Demo                         │
├─────────────────────────────────────┤
│ Editor                   │ Preview  │
│                          │          │
│ deck                     │  [SVG]   │
│   slide                  │          │
│     text "Hi" 50 50 5    │          │
│       "Roboto"           │          │
│   eslide                 │          │
│ edeck                    │          │
│                          │          │
├─────────────────────────────────────┤
│ 📝 Fonts Used:                      │
│    • Roboto (will be fetched)       │
│    ⚠️ Not in cache - first render   │
│       will download ~150KB          │
└─────────────────────────────────────┘
```

**Why This Matters**:
- User knows what fonts will be downloaded
- Can see font dependencies before rendering
- Helps debug font issues
- Educational (shows font detection working)

**Technical Requirements**:
- Must detect from decksh source (user typing in editor)
- Real-time detection as user types (debounced)
- Show font status (cached, needs download, unavailable)
- Works in Cloudflare Workers environment

**API Endpoint**:
```bash
POST /detect-fonts
Content-Type: text/plain

deck
  slide
    text "Hello" 50 50 5 "Roboto"
  eslide
edeck

# Response:
{
  "fonts": ["Roboto"],
  "cached": [],
  "needs_download": ["Roboto"],
  "estimated_size": "~150KB"
}
```

### 2. Pre-flight Font Caching

Before expensive PNG/PDF rendering, pre-cache fonts:

```bash
POST /process?format=png
X-Preflight: detect-fonts

# First response (fast):
{
  "fonts_needed": ["Roboto", "Open Sans"],
  "status": "caching",
  "estimated_time": "2s"
}

# Then auto-retry or poll:
GET /process/status/{job-id}

# Final response:
{
  "fonts_cached": true,
  "ready": true,
  "slides": [...]
}
```

### 3. Font Validation

Validate deck before rendering:

```bash
POST /validate
{
  "source": "deck\n  slide...",
  "format": "png"
}

# Response:
{
  "valid": true,
  "fonts": {
    "required": ["Roboto"],
    "available": true,
    "will_download": ["Roboto"]
  },
  "warnings": [
    "Roboto not cached - first render will download 150KB"
  ]
}
```

This use case makes it clear: **Decksh source detection is not optional** - it's required for the web GUI.

## Font Family Complexity

### The Font Family Problem

A "font" is not a single file - it's a family of variants:

```
Roboto Family:
├── Roboto-Regular.ttf       (400 weight, normal)
├── Roboto-Bold.ttf          (700 weight, normal)  
├── Roboto-Italic.ttf        (400 weight, italic)
├── Roboto-BoldItalic.ttf    (700 weight, italic)
├── Roboto-Light.ttf         (300 weight, normal)
└── ...more weights (100-900)
```

### Questions to Answer

1. **Which variants to download?**
   - All variants? (complete but wasteful)
   - Just Regular + Bold + Italic? (most common)
   - On-demand per variant? (complex)

2. **How does deck use variants?**
   - Does decksh specify variants explicitly?
   - Or does the renderer auto-select (bold for headings, etc.)?
   - Check ajstarks' deck code for variant handling

3. **Storage implications**
   - Full Roboto family: ~10 files, ~1MB
   - Just Regular: ~150KB
   - Cache entire families or individual files?

4. **Google Fonts API**
   - API returns all variants for a family
   - Download URLs for each variant
   - How to map deck font request to variants?

### Example: User Types "Roboto"

**Option A - Minimal** (just regular):
```
Cache: Roboto-Regular.ttf (150KB)
Risk: Missing bold/italic if deck uses them
```

**Option B - Common Variants**:
```
Cache: 
  - Roboto-Regular.ttf (150KB)
  - Roboto-Bold.ttf (150KB)
  - Roboto-Italic.ttf (150KB)
Total: ~450KB
Risk: Downloaded unused variants
```

**Option C - Full Family**:
```
Cache: All 18 Roboto variants (1MB+)
Risk: Wasteful bandwidth/storage
```

**Option D - On-Demand**:
```
1. Download Regular first
2. Detect which variants actually used in rendering
3. Download those on-demand
Risk: Complex, multiple round trips
```

### Research Needed

1. How do ajstarks' renderers handle font families?
2. Can we detect which variants a deck will use?
3. What's the Google Fonts API response format?
4. How do other systems handle this (browsers, PDF tools)?

### Proposed Solution (TBD)

Need to research and test before deciding. Likely:
- Start with Common Variants (B) for safety
- Add variant detection later for optimization
- Cache at family level, not individual files
