# ADR 0003: Font Display in Web GUI

## Status

Draft - Needs UX Design

## Date

2026-01-27

## Context

Web GUI users need to see what fonts their deck uses:
- Know what will be downloaded
- See cache status
- Get size estimates
- Debug font issues

## Decision

NOT READY - need to design UX and API

## Design Questions

### 1. Where to display font info?

**Option A - Inline warnings:**
```
Editor:
deck
  slide
    text "Hello" 50 50 5 "Roboto"  ⚠️ Roboto not cached (150KB)
  eslide
edeck
```

**Option B - Status panel:**
```
┌─────────────────────────────────────┐
│ Editor              │ Preview       │
├─────────────────────────────────────┤
│ Status: 📝 Fonts                    │
│   • Roboto (needs download, ~150KB)│
│   • Open Sans (cached)              │
└─────────────────────────────────────┘
```

**Option C - On-demand tooltip:**
Hover over font name in editor to see status

### 2. When to detect?

- On every keystroke? (too aggressive)
- Debounced (wait 500ms after typing stops)?
- On demand (button click)?
- On render attempt?

### 3. What info to show?

Minimal:
- Font name
- Cached yes/no

Detailed:
- Font name
- Cache status
- Download size
- Variants included
- Availability (Google Fonts vs system vs unavailable)

### 4. API Design

```bash
# Simple version
POST /detect-fonts
Content-Type: text/plain
[decksh source]

Response:
{
  "fonts": ["Roboto", "Open Sans"]
}

# Detailed version
POST /detect-fonts?details=true
Content-Type: text/plain
[decksh source]

Response:
{
  "fonts": [
    {
      "name": "Roboto",
      "cached": false,
      "available": true,
      "size_estimate": "~450KB",
      "variants": ["regular", "bold", "italic"]
    },
    {
      "name": "Open Sans",
      "cached": true
    }
  ]
}
```

## Research Tasks

1. Design mockup of UI
2. Test debounce timing (100ms, 500ms, 1s)
3. Measure font detection performance
4. Prototype API endpoint
5. Get user feedback on mockup

## Success Criteria

- Font info visible without being intrusive
- Updates feel responsive (<500ms after typing stops)
- Clear indication of download requirements
- Works well on mobile screens

## References

- Current demo UI: https://deckfs.gedw99.workers.dev/
