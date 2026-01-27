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
