# ADR 0004: Font Fetching and Caching

## Status

Draft - Needs Architecture

## Date

2026-01-27

## Context

Once we know what fonts are needed, we must:
- Fetch them from Google Fonts (or other source)
- Handle font families (regular, bold, italic variants)
- Cache for future use
- Work in both native and Cloudflare environments

## Decision

NOT READY - need to research font families and caching

## Key Questions

### 1. Font Family Variants

When user requests "Roboto", which variants to fetch?

**Option A - Minimal (Regular only):**
- 1 file, ~150KB
- Risk: Missing bold/italic if used

**Option B - Common Variants:**
- Regular + Bold + Italic = 3 files, ~450KB
- Covers 95% of use cases
- Some wasted bandwidth if not all used

**Option C - Full Family:**
- All 10+ variants, ~1MB
- Complete but wasteful

**Option D - On-Demand:**
- Start with Regular
- Fetch Bold/Italic as needed
- Complex, requires variant detection

**Need to research:**
- How do pngdeck/pdfdeck request variants?
- Can we detect which variants a deck uses?
- What does Google Fonts API return?

### 2. Caching Strategy

**Local (Native Server):**
```
.fonts/
├── roboto/
│   ├── regular.ttf
│   ├── bold.ttf
│   └── italic.ttf
└── open-sans/
    └── regular.ttf
```

**Cloudflare (R2 Bucket):**
```
R2: deckfs-fonts
├── roboto/regular.ttf
├── roboto/bold.ttf
└── open-sans/regular.ttf
```

**Questions:**
- Cache entire families or individual files?
- Eviction strategy (LRU, size limit)?
- How to handle updates (font version changes)?

### 3. Google Fonts API

**Need to research:**
- API response format
- Rate limits
- Download URLs format
- Font file formats available (TTF, WOFF2, etc.)
- API key requirements

### 4. Fallback Strategy

What if Google Fonts unavailable?
- System fonts only?
- Cached fonts only?
- Error and refuse to render?
- Substitute similar font?

## Research Tasks

1. Call Google Fonts API, inspect response
2. Download sample font family, measure sizes
3. Test pngdeck with different variants
4. Prototype local cache implementation
5. Prototype R2 cache implementation
6. Test API rate limits

## Success Criteria

- Can fetch fonts from Google Fonts API
- Fonts cached efficiently (no duplicates)
- Works in both native and Cloudflare
- Graceful fallback when API unavailable
- Clear error messages when fonts missing

## References

- [Google Fonts API](https://developers.google.com/fonts/docs/developer_api)
- [Google Fonts](https://fonts.google.com/)
