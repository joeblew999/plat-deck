# Decksh File Types

Decksh supports two types of `.dsh` files for code reusability:

## 1. Complete Deck Files (Renderable)

Files that contain the `deck`/`edeck` structure and can be directly rendered.

**Structure:**
```decksh
deck
  slide "white" "black"
    ctext "Hello" 50 50 5
  eslide
edeck
```

**Examples:**
- `go/go.dsh` - Complete Go logo presentation
- `warlife/warlife.dsh` - Complete war timeline visualization
- `elections/elections.dsh` - Complete election results

**Count:** 116 files in deckviz

## 2. Library/Definition Files (Non-renderable)

Files that only contain `def`/`edef` blocks for reusable components. Cannot be rendered standalone.

**Structure:**
```decksh
def redcircle X Y
  circle X Y 10 "red"
  text "Point" X Y 2
edef
```

**Examples:**
- `warlife/a.dsh` - Arc definitions for warlife visualization
- `warlife/c.dsh` - Circle definitions
- `walk/route.dsh` - Route line definitions

**Count:** 159 files in deckviz

## Usage in Complete Decks

Library files are included/imported into complete decks:

```decksh
deck
  slide "white" "black"
    grid "a.dsh" 10 50 5 5 80  // Import arc definitions
    grid "c.dsh" 10 60 5 5 80  // Import circle definitions
  eslide
edeck
```

## API Filtering

The `/examples` endpoint supports filtering:

- `GET /examples` - Returns all 275 .dsh files
- `GET /examples?renderable=true` - Returns only 116 renderable files

**Demo UI:** Uses `?renderable=true` to show only files that can be directly rendered.

## Detection Logic

A file is considered renderable if it contains:
- `deck\n` at start
- `deck ` at start
- `\ndeck\n` anywhere
- `\ndeck ` anywhere

This ensures files with the complete deck structure are identified correctly.

## Implications for Authoring

When building an authoring system:

1. **File Browser**: Should distinguish between library and complete files
2. **Render Button**: Only enable for complete deck files
3. **Imports**: Show library files as importable components
4. **Templates**: Provide both complete and library file templates
5. **Validation**: Check for deck/edeck structure before rendering

## Test Results

After filtering implementation:
- decksh tests: 13/18 passing (72%) - 5 are definition-only files
- deckviz tests: 153/275 passing (56%) - 122 include definition files or missing data

Complete files render successfully; definition-only files correctly error with "EOF" (no deck structure).
