# ADR 0004: Font Fetching and Caching (PNG/PDF Only)

## Status

Draft - Needs Architecture

## Date

2026-01-27

## Context

### Key Insight

**Font files only needed for PNG/PDF rendering, NOT SVG!**

- **SVG**: References font names (`font-family="Roboto"`), browser renders with its fonts
- **PNG/PDF**: Requires actual TTF files for pngdeck/pdfdeck binaries

**This means:**
- Cloudflare Workers (SVG only) doesn't need font fetching
- Native server (PNG/PDF) needs font fetching
- Simpler problem than initially thought

### When We Need Fonts

Only for native server PNG/PDF rendering:
```bash
# SVG - no fonts needed
curl POST /process?format=svg
→ Returns SVG with font-family references
→ No font files required ✅

# PNG - needs font files
curl POST /process?format=png
→ Requires TTF files in DECKFONTS directory
→ Must fetch/cache fonts ⚠️
```

## Decision

NOT READY - need to research font families and caching

## Key Questions

### 1. Font Family Variants

When user requests "Roboto" for PNG, which variants to fetch?

**Current behavior:**
- pngdeck looks in `-fontdir` for matching TTF files
- Need to determine: does it look for Roboto-Regular.ttf? Roboto.ttf? How are variants selected?

**Options:**
- **Minimal**: Just Regular (~150KB)
- **Common**: Regular + Bold + Italic (~450KB)
- **Full Family**: All variants (~1MB)

**Research needed:**
- How does pngdeck select font variants?
- Test with different variant file names
- Check ajstarks' deck code for variant logic

### 2. Caching Strategy

**Native Server Only** (Cloudflare doesn't need it):

```
.fonts/
├── roboto/
│   ├── regular.ttf
│   ├── bold.ttf
│   └── italic.ttf
└── open-sans/
    └── regular.ttf
```

**Questions:**
- Cache full families or individual files?
- Eviction strategy (LRU, size limit)?
- Update strategy (font versions)?

### 3. Google Fonts API

**Need to research:**
- API response format
- What file formats available (TTF, WOFF2)?
- Which format does pngdeck/pdfdeck support?
- Rate limits
- Download URLs

### 4. Fallback Strategy

What if Google Fonts unavailable?

**Options:**
- Use existing .src/deckfonts/ as fallback
- Error and refuse to render
- System fonts (but which ones?)
- Substitute similar font

## Scope Simplification

**IMPORTANT:** Font fetching is ONLY for:
- Native server
- PNG/PDF formats
- ~100 users vs millions (Cloudflare SVG)

This changes priorities:
- Don't need to over-optimize
- Can start simple (just Regular variant)
- Add complexity later if needed
- Cloudflare Workers unaffected

## Research Tasks

1. Test: How does pngdeck find font files?
2. Test: Does it support bold/italic variants?
3. Call Google Fonts API, inspect response
4. Download sample fonts, test with pngdeck
5. Prototype local .fonts/ cache
6. Measure cache size for common fonts

## Success Criteria

- Can fetch fonts from Google Fonts API
- pngdeck/pdfdeck can find cached fonts
- Works with current DECKFONTS env variable
- Simple cache structure
- Clear error when fonts missing

## References

- [Google Fonts API](https://developers.google.com/fonts/docs/developer_api)
- pngdeck source: .src/deck/cmd/pngdeck/
- Current font dir: .src/deckfonts/
