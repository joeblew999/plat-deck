# ADR 0002: Font Discovery from Decksh and Deck XML

## Status

Draft - Needs Research

## Date

2026-01-27

## Context

We need to detect what fonts a deck uses for:
- Pre-caching before rendering
- Showing users what fonts their deck requires
- Validating font availability before expensive operations
- Working in environments without decksh binary (Cloudflare Workers)

## Decision

NOT READY - need to research and test detection approaches

## Approaches to Research

### 1. Decksh Source Detection (Regex-based)

**Pros:**
- Fast, no conversion needed
- Works in Cloudflare Workers (no decksh binary)
- Can detect before any processing

**Cons:**
- Approximate, may miss edge cases
- Doesn't handle variables, loops, conditionals
- May over-detect (fonts in comments/strings)

**Need to answer:**
- What regex patterns cover common cases?
- How accurate is it on real deckviz examples?
- What's acceptable false positive/negative rate?

### 2. Deck XML Detection (deck package)

**Pros:**
- 100% accurate (canonical format)
- Type-safe using ajstarks' deck package
- Handles all decksh features after expansion

**Cons:**
- Requires decksh conversion first
- Not available in Cloudflare Workers
- Slower (must run decksh)

**Need to answer:**
- Which XML elements contain fonts?
- How are font defaults handled?
- What about inherited fonts?

### 3. Hybrid Approach

Use both methods:
- Decksh detection for fast preview (Web GUI)
- XML detection for validation before rendering

**Need to answer:**
- When to use which method?
- How to merge results?
- What if they disagree?

## Research Tasks

1. Build regex detector, test on 20 random deckviz files
2. Build XML detector using deck package
3. Compare results, measure accuracy
4. Document false positives/negatives
5. Decide: hybrid vs single approach

## Success Criteria

- Can detect fonts from decksh source with >90% accuracy
- Can detect fonts from deck XML with 100% accuracy
- Fast enough for real-time Web GUI (<100ms)

## References

- [ajstarks/deck package](https://github.com/ajstarks/deck)
- [decksh syntax](https://github.com/ajstarks/decksh)
