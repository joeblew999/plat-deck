# ADR 0001: Pipeline Architecture for Multi-Environment Rendering

## Status

Accepted

## Date

2026-01-26

## Context

DeckFS converts decksh (a presentation DSL by ajstarks) to various output formats (SVG, PNG, PDF). The system needs to run in multiple environments:

1. **Native host** - Full system access, can spawn processes
2. **Wazero runtime** - WASI sandbox, limited capabilities
3. **Cloudflare Workers** - Serverless WASM, no file system
4. **Browser WASM** - Client-side, no file system

ajstarks designed his deck ecosystem as piped CLI tools:
```bash
decksh < input.dsh | svgdeck > output.svg
decksh < input.dsh | pngdeck -dir /tmp/out
decksh < input.dsh | pdfdeck -o output.pdf
```

The current codebase has duplicated/modified renderer code that doesn't respect this design.

## Decision

We will implement a **Pipeline interface** with environment-specific implementations:

### Pipeline Interface

```go
package pipeline

type OutputFormat string

const (
    FormatSVG OutputFormat = "svg"
    FormatPNG OutputFormat = "png"
    FormatPDF OutputFormat = "pdf"
)

type Pipeline interface {
    Process(ctx context.Context, source []byte, format OutputFormat) (*Result, error)
    SupportedFormats() []OutputFormat
}

type Result struct {
    Slides [][]byte
    Format OutputFormat
}
```

### Implementations

#### 1. NativePipeline (Host environments)

Uses `os/exec` to pipe to ajstarks' binaries:

- **Supports:** SVG, PNG, PDF
- **Requires:** decksh, svgdeck, pngdeck, pdfdeck binaries in PATH or configured location
- **Build tag:** `//go:build !js && !tinygo`

```go
// Basic processing (no imports)
func (p *NativePipeline) Process(ctx context.Context, source []byte, format OutputFormat) (*Result, error) {
    // Uses stdin mode - no import support
}

// With import support and asset resolution
func (p *NativePipeline) ProcessWithWorkDir(ctx context.Context, source []byte, format OutputFormat, workDir string) (*Result, error) {
    // Writes source to temp file in workDir
    // decksh can resolve relative imports
    // Renderers run with workDir set to find image assets
    // PATH includes .bin/deck for dchart and other tools
}

// File-based processing
func (p *NativePipeline) ProcessFile(ctx context.Context, filePath string, format OutputFormat) (*Result, error) {
    // Reads file and uses its directory for imports
}
```

**Import Resolution:**
- Stdin mode (`Process`): No import support - imports fail
- File mode (`ProcessWithWorkDir`, `ProcessFile`): Full import support via working directory context
- Binary paths converted to absolute to handle directory changes
- Used for examples with `import "file.dsh"` statements

#### 2. WASMPipeline (WASM environments)

Uses ajstarks' packages directly for in-memory processing:

- **Supports:** SVG only
- **Packages:** `github.com/ajstarks/decksh`, `github.com/ajstarks/svgo/float`
- **Build tag:** `//go:build js || tinygo`

```go
func (p *WASMPipeline) Process(ctx context.Context, source []byte, format OutputFormat) (*Result, error) {
    // 1. Use decksh package to parse to XML
    // 2. Use svgdeck logic to render to SVG
    // PNG/PDF return ErrUnsupportedFormat
}
```

**Import/Include Support:**
WASM environments lack file system access, but we work around this:
- `ImportResolver` pre-processes source before passing to decksh
- Loads imported/included files from R2 storage
- `import "file.dsh"` → Extracts `def/edef` function blocks and inlines them
- `include "file.dsh"` → Recursively expands and inlines full content
- Decksh receives expanded source with all dependencies inlined

### Environment Capabilities Matrix

| Environment       | Pipeline    | SVG | PNG | PDF | Imports | Notes                           |
|-------------------|-------------|-----|-----|-----|---------|--------------------------------|
| Native CLI        | Native      | Yes | Yes | Yes | Yes     | Full capability                |
| Wazero Host       | Native      | Yes | Yes | Yes | Yes     | Executes ajstarks tools        |
| Cloudflare Worker | WASM        | Yes | No  | No  | Yes*    | *Via pre-expansion from R2     |
| Browser           | WASM        | Yes | No  | No  | Yes*    | *Via pre-expansion from storage|

### Build Artifacts

```
.bin/
├── deck/               # ajstarks CLI binaries (built from .src/, NOT the deckfs runtime)
│   ├── decksh          # DSL parser
│   ├── dshfmt          # DSL formatter
│   ├── dshlint         # DSL linter
│   ├── svgdeck         # SVG renderer
│   ├── pngdeck         # PNG renderer (needs DECKFONTS)
│   ├── pdfdeck         # PDF renderer (needs DECKFONTS)
│   └── dchart          # Chart tool (called by decksh for chart rendering)
├── cloudflare/         # Worker WASM (WASMPipeline, deckfs runtime)
├── browser/            # Browser WASM (WASMPipeline, deckfs runtime)
└── wazero/             # Host binary (NativePipeline, deckfs runtime)
```

## Consequences

### Positive

1. **Respects upstream design** - Uses ajstarks' tools as intended
2. **No code duplication** - Single source of truth in .src/
3. **Clear boundaries** - Each environment knows its capabilities
4. **Easy updates** - `git pull` in .src/ updates tools
5. **Testable** - Each pipeline implementation can be tested independently
6. **Import support** - Native pipeline supports decksh import statements

### Negative

1. **WASM limitation** - PNG/PDF not available in browser/Cloudflare (font files too large)
2. **Binary dependency** - Native pipeline requires built tools
3. **Disk space** - Need to build and store 4 binaries
4. **Import pre-expansion** - WASM requires extra step to load/inline imports from storage

### Neutral

1. **Font handling** - PNG/PDF still require DECKFONTS environment
2. **Output format** - API returns base64 for binary formats
3. **Processing modes** - Native pipeline has two modes: stdin (fast) and file (import support)

## Alternatives Considered

### 1. Embed all rendering code

Rejected because:
- Duplicates ajstarks' code
- Hard to maintain
- Different versions could drift

### 2. WASM for all formats

Rejected because:
- PNG/PDF need font files
- Embedding fonts makes WASM huge (~20MB+)
- Not practical for Cloudflare Workers

### 3. Server-side rendering only

Rejected because:
- Cloudflare Workers are cost-effective
- Browser-side reduces server load
- SVG works perfectly in WASM

### 4. Virtual file system for imports in WASM

Rejected because:
- Complex to implement
- Would require bundling all possible imports
- Import resolution is dynamic, can't predict all files needed

## Implementation Notes

### Import Support (Native Pipeline)

The native pipeline supports imports by switching between two execution modes:

1. **Stdin Mode** (default): Fast, but imports fail
   - Used when no working directory provided
   - Source piped directly to decksh stdin
   - Relative imports cannot be resolved

2. **File Mode** (import support): Writes temp file with working directory
   - Used when `ProcessWithWorkDir` or `ProcessFile` called
   - Source written to temp file in target directory
   - decksh runs with working directory set
   - Relative imports resolved correctly

The wazero server automatically uses file mode when the `source` query parameter is provided, enabling imports for all examples.

### PATH and Asset Resolution (Native Pipeline)

The native pipeline handles two critical runtime dependencies:

1. **PATH Environment Setup**:
   - `.bin/deck` directory is added to PATH for all decksh executions
   - This allows decksh to find `dchart` when processing chart data files (*.d)
   - Without this, decks using charts would fail with "command not found"
   - PATH is set for both stdin and file modes

2. **Asset Directory for Renderers**:
   - Renderers (svgdeck, pngdeck, pdfdeck) need to find image assets (*.png, *.jpg)
   - `cmd.Dir` is set to the source file's directory (workDir)
   - Allows relative image paths in deck XML to resolve correctly
   - Example: `<image xlink:href="logo.png"/>` works when renderer runs from correct directory

3. **Working Directory Flow**:
   ```
   Source: .src/deckviz/b17/b17.dsh
   ↓
   workDir: .src/deckviz/b17/ (absolute path)
   ↓
   decksh runs with PATH including .bin/deck
   ↓
   pngdeck runs with cmd.Dir = .src/deckviz/b17/
   ↓
   Image assets (iza-vailable.png, etc.) found successfully
   ```

### Import Support (WASM Pipeline)

The WASM pipeline uses `ImportResolver` to work around the lack of file system:

1. **Pre-processing Stage**: Before passing source to decksh
   - Scans for `import "file.dsh"` and `include "file.dsh"` statements
   - Loads referenced files from R2 storage (Cloudflare) or other storage backends
   - For `import`: Extracts `def/edef` function definition blocks
   - For `include`: Recursively expands full file content
   - Inlines content and removes import/include statements

2. **Decksh Processing**: Expanded source passed to decksh
   - All dependencies already inlined
   - No file I/O attempted by decksh
   - Functions available for calling

3. **Storage Integration**:
   - `StorageLoader` adapts any storage backend with `Get(ctx, key) (io.ReadCloser, error)`
   - Cloudflare Workers: R2 input bucket
   - Future: Could support embedded files, HTTP URLs, etc.

**Example transformation:**
```
// Original source with import
import "coord.dsh"
deck
  slide
    coord 50 50 2 3
  eslide
edeck

// After expansion (passed to decksh)
// Function imported from: coord.dsh
def coord Xc Yc Radius TextSize
  circle Xc Yc Radius "blue"
  text "Point" Xc Yc TextSize
edef

deck
  slide
    coord 50 50 2 3
  eslide
edeck
```

## References

- [ajstarks/deck](https://github.com/ajstarks/deck) - Core library
- [ajstarks/decksh](https://github.com/ajstarks/decksh) - DSL
- [ADR format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
