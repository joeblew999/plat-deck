# ADR 0002: Font Discovery from Decksh and Deck XML

## Status

Draft - Needs Research

## Date

2026-01-27

## Context

### Current Facts

**SVG rendering:**
- SVG output contains font references: `<text font-family="Roboto">`
- Does NOT embed font files
- Browser/viewer uses its own fonts to render
- **Font files NOT needed for SVG** ✅

**PNG/PDF rendering:**
- Requires actual TTF font files
- pngdeck/pdfdeck need `-fontdir` pointing to .ttf files
- **Font files REQUIRED for PNG/PDF** ⚠️

**Decksh in Cloudflare:**
- DOES work - `github.com/ajstarks/decksh` package compiled to WASM
- Can parse decksh source → deck XML
- **CAN detect fonts from XML in Cloudflare** ✅

### Why We Need Font Discovery

1. **PNG/PDF Pre-caching** - Fetch font files before rendering
2. **Web GUI Display** - Show users what fonts their deck uses
3. **Validation** - Check if required fonts are available
4. **Size Estimates** - Tell users download size before rendering

## Decision

NOT READY - need to research detection approaches

## Detection Strategy

Since decksh works in Cloudflare (compiled to WASM), we have options:

### Option A: Always Use Deck XML

**Flow:**
1. Convert decksh → XML (works everywhere)
2. Parse XML using deck package
3. Extract fonts from Text/List elements

**Pros:**
- 100% accurate
- Single code path
- Uses canonical format

**Cons:**
- Must run decksh first
- Slightly slower

### Option B: Regex for Quick Preview, XML for Accuracy

**Flow:**
1. Regex decksh source for instant preview (Web GUI as user types)
2. XML detection for validation before rendering

**Pros:**
- Fast preview
- Accurate validation

**Cons:**
- Two code paths
- Regex may disagree with XML

### Option C: Regex Only (Fast but Risky)

**Pros:**
- Fastest
- Works without conversion

**Cons:**
- Inaccurate (misses variables, loops, conditionals)
- May over/under detect fonts

## Research Tasks

1. Measure decksh→XML conversion time (is it fast enough for real-time?)
2. If fast enough: Option A (XML only)
3. If too slow: Build regex detector, measure accuracy
4. Test on 20 random deckviz files
5. Decide which option

## Success Criteria

- Can detect fonts needed for PNG/PDF rendering
- Fast enough for Web GUI (<500ms)
- Works in Cloudflare Workers
- Clear error when fonts unavailable

## Key Insight

**Font files only needed for PNG/PDF, not SVG!**

This simplifies the problem:
- SVG: No font fetching needed (just names in XML)
- PNG/PDF: Font fetching required
- Focus font management on PNG/PDF only

## References

- [ajstarks/deck package](https://github.com/ajstarks/deck)
- [decksh package](https://github.com/ajstarks/decksh)
- Current WASM pipeline: pkg/pipeline/wasm.go

## CRITICAL QUESTION: SVG Font Delivery

### Current Assumption (Needs Validation)

I assumed SVG "just works" because it references font names. **This is wrong!**

When we generate:
```xml
<text font-family="Roboto">Hello</text>
```

**The font must come from somewhere:**

1. **User's local fonts** (unreliable)
   - Different systems have different fonts
   - Roboto may not be installed
   - Falls back to default (looks wrong)

2. **Google Fonts CDN** (need to inject reference)
   - Add `@import url('fonts.googleapis.com/...')` to SVG
   - Browser fetches font when viewing
   - Requires internet
   - Consistent rendering ✅

3. **Embedded font data** (self-contained but large)
   - Base64 encode font into SVG
   - Works offline
   - Each SVG becomes ~150KB+ larger per font
   - Self-contained ✅

### What Does ajstarks' svgdeck Do?

**NEED TO RESEARCH:**
- Check svgdeck source code
- Generate SVG with font, inspect output
- Does it inject Google Fonts references?
- Does it embed fonts?
- Or does it assume local fonts?

### Testing Needed

1. Generate SVG with "Roboto" font
2. View on system without Roboto installed
3. What happens? (probably falls back)
4. Check if ajstarks' svgdeck has font embedding option
5. Decide our strategy

### Implications

**If we inject Google Fonts CDN references:**
- Pro: Consistent rendering
- Pro: Small SVG size
- Con: Requires internet
- Con: Depends on Google CDN
- Need: Modify svgdeck output or post-process

**If we embed fonts:**
- Pro: Self-contained
- Pro: Works offline
- Con: Large files (150KB+ per font per SVG)
- Con: Multiple slides = font duplicated per slide
- Need: Fetch font, base64 encode, inject

**If we do nothing (current):**
- Pro: Small SVG size
- Pro: Simple
- Con: Looks different on different systems
- Con: May not render correctly

This is a major architecture decision we missed!
