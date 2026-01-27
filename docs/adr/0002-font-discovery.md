# ADR 0002: Font Discovery and SVG Font Delivery

## Status

**Accepted** - Strategy decided based on research

## Date

2026-01-27

## Context

### Problem Statement

To render decks correctly, we need to understand:
1. What fonts does a deck use?
2. How do we ensure those fonts are available when SVG is viewed?
3. How does this work in both Cloudflare Workers and native server?

### Current Facts

**SVG rendering:**
- SVG output contains font references: `<text font-family="Roboto">`
- Does NOT embed font files
- Browser/viewer uses its own fonts to render
- **Fonts must be available to browser** ⚠️

**PNG/PDF rendering:**
- Requires actual TTF font files
- pngdeck/pdfdeck need `-fontdir` pointing to .ttf files
- **Font files REQUIRED for PNG/PDF** ⚠️

**Decksh in Cloudflare:**
- DOES work - `github.com/ajstarks/decksh` package compiled to WASM ✅
- Can parse decksh source → deck XML
- **CAN detect fonts from XML in Cloudflare** ✅

### Research: What ajstarks' svgdeck Actually Does

**Tested svgdeck behavior** (.src/deck/cmd/svgdeck/svgdeck.go):

1. **Font output** (line 437):
   ```go
   doc.Text(x, y, s, `xml:space="preserve"`, 
       fmt.Sprintf("font-family:%s", fontlookup(font)))
   ```
   - Just writes `font-family:FontName` into SVG
   - No font embedding (would be 150KB+ per font)
   - No Google Fonts `@import` injection
   - No font file references

2. **Font mapping** (lines 34-36, 159-165, 757-759):
   ```go
   var fontmap = map[string]string{}
   
   func fontlookup(s string) string {
       font, ok := fontmap[s]
       if ok {
           return font
       }
       return "sans"  // Fallback to sans
   }
   
   // In main():
   fontmap["sans"] = *sansfont    // Default: "Helvetica"
   fontmap["serif"] = *serifont   // Default: "Times-Roman"
   fontmap["mono"] = *monofont    // Default: "Courier"
   ```
   - Unmapped fonts fall back to "Helvetica"
   - Can override via `-sans Roboto` flag

3. **Actual test results:**
   ```bash
   $ echo 'deck
       slide
         text "Test" 50 50 5 "Roboto"
       eslide
     edeck' | decksh | svgdeck
   ```
   Output:
   ```xml
   <text style="font-family:sans">Test</text>
   ```
   "Roboto" → falls back to "sans" → "Helvetica"

**What plat-deck currently does:**

1. **WASM pipeline** (pkg/pipeline/wasm.go:44-51):
   ```go
   sansFont:  "Helvetica, Arial, sans-serif"
   serifFont: "Georgia, Times, serif"
   monoFont:  "Monaco, Consolas, monospace"
   ```
   ✅ Uses CSS font fallback lists (smart!)
   ✅ If Helvetica unavailable, tries Arial, then system sans-serif

2. **Native pipeline** (pkg/pipeline/native.go:23):
   ```go
   svgdeckBin string  // Calls svgdeck binary
   ```
   ⚠️ Uses svgdeck defaults: "Helvetica", "Times-Roman", "Courier"
   ⚠️ No fallback lists

### The Real SVG Font Problem

**When browser renders SVG with `<text font-family="Roboto">`:**

1. **User's local fonts** (current behavior)
   - Different systems have different fonts
   - Roboto may not be installed
   - Falls back to default sans-serif
   - ❌ Looks different on different systems
   - ❌ May not render as intended

2. **Google Fonts CDN injection** (we could do this)
   - Inject `<style>@import url('fonts.googleapis.com/css2?family=Roboto');</style>` into SVG
   - Browser fetches font when viewing SVG
   - ✅ Consistent rendering across systems
   - ✅ Small SVG file size
   - ❌ Requires internet connection
   - ❌ Google Fonts CDN dependency
   - ⚠️ Need to detect fonts to inject

3. **Embedded font data** (possible but large)
   - Base64 encode TTF into SVG `<defs>`
   - Works offline
   - ✅ Self-contained SVG
   - ✅ Consistent rendering
   - ❌ Each SVG becomes ~150KB+ larger per font
   - ❌ Multi-slide deck duplicates fonts per slide
   - ⚠️ Need font files available

## Decision

**Use CSS font fallback lists for SVG**

Reasons:
1. **WASM pipeline already does this correctly**
2. **Simple and works without font fetching**
3. **Reasonable cross-platform rendering**
4. **No external dependencies (CDN)**
5. **Small SVG file size**
6. **Works offline**

### Strategy

**For SVG (both WASM and Native):**
- Map generic names to fallback lists:
  - `sans` → "Helvetica, Arial, sans-serif"
  - `serif` → "Georgia, Times, serif"
  - `mono` → "Monaco, Consolas, monospace"
- Custom fonts (e.g., "Roboto") → map to similar generic + fallback:
  - "Roboto" → "Roboto, Helvetica, Arial, sans-serif"
- Browser tries fonts in order until one is found

**For PNG/PDF (Native only):**
- Requires actual font files (TTF)
- See ADR 0004: Font Fetching for font management strategy

### Font Discovery

**Two approaches for detecting fonts:**

#### Option A: XML-Only (Recommended for MVP)

```
decksh source → deck XML → Parse XML → Extract fonts from Text/List elements
```

**Pros:**
- 100% accurate
- Single code path
- Uses canonical deck format
- Works in Cloudflare (decksh package compiles to WASM)

**Cons:**
- Must run decksh conversion first
- Slightly slower than regex

**Use for:** 
- Backend validation before PNG/PDF rendering
- API endpoint `/detect-fonts`

#### Option B: Regex for Quick Preview (Optional enhancement)

```
Scan decksh source with regex for `"fontname"` patterns
```

**Pros:**
- Instant results (no decksh conversion)
- Good for real-time Web GUI as user types

**Cons:**
- Inaccurate (misses variables, loops, conditionals)
- May over/under detect fonts
- Regex may disagree with XML

**Use for:**
- Optional: Web GUI instant preview (debounced)
- Show approximate font list while typing
- Re-validate with XML before rendering

### Implementation Plan

**Phase 1: MVP (XML-only detection)**
1. Create font detector using `github.com/ajstarks/deck` package
2. Parse deck XML, extract unique fonts from:
   - `<text font="...">`
   - `<list font="...">`
   - List items with custom fonts
3. Map fonts to fallback lists
4. Return font names + fallback CSS

**Phase 2: Optional (Regex quick preview)**
1. Build regex patterns for decksh source
2. Test accuracy on 20 deckviz files
3. If >90% accurate, add to Web GUI
4. Always re-validate with XML before rendering

## Success Criteria

- ✅ Can detect fonts from deck XML
- ✅ SVG renders consistently with fallback fonts
- ✅ Works in Cloudflare Workers
- ✅ No external dependencies for SVG
- ✅ Fast enough for Web GUI (<500ms)

## Implementation Notes

### Font Mapping (for SVG)

```go
var fontFallbacks = map[string]string{
    "sans":       "Helvetica, Arial, sans-serif",
    "serif":      "Georgia, Times, serif",
    "mono":       "Monaco, Consolas, monospace",
    "helvetica":  "Helvetica, Arial, sans-serif",
    "roboto":     "Roboto, Helvetica, Arial, sans-serif",
    "opensans":   "Open Sans, Helvetica, Arial, sans-serif",
    // Add more as needed
}
```

### XML Font Detection

```go
func DetectFonts(deckXML []byte) ([]string, error) {
    var d deck.Deck
    if err := xml.Unmarshal(deckXML, &d); err != nil {
        return nil, err
    }
    
    fonts := make(map[string]bool)
    for _, slide := range d.Slide {
        for _, text := range slide.Text {
            if text.Font != "" {
                fonts[strings.ToLower(text.Font)] = true
            }
        }
        for _, list := range slide.List {
            if list.Font != "" {
                fonts[strings.ToLower(list.Font)] = true
            }
        }
    }
    
    result := make([]string, 0, len(fonts))
    for font := range fonts {
        result = append(result, font)
    }
    return result, nil
}
```

## References

- [ajstarks/deck package](https://github.com/ajstarks/deck)
- [svgdeck source](https://github.com/ajstarks/deck/blob/main/cmd/svgdeck/svgdeck.go)
- [CSS font-family fallbacks](https://developer.mozilla.org/en-US/docs/Web/CSS/font-family)
- Current WASM pipeline: pkg/pipeline/wasm.go
- Current native pipeline: pkg/pipeline/native.go

## Related ADRs

- ADR 0003: Font Display (Web GUI UX)
- ADR 0004: Font Fetching (PNG/PDF font management)
