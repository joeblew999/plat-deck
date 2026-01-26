//go:build !js && !tinygo

// Package pipeline provides native pipeline implementation using ajstarks' CLI tools
package pipeline

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ajstarks/deck"
)

// NativePipeline implements Pipeline for native environments (CLI, wazero host)
// It uses os/exec to pipe to ajstarks' binaries (decksh, svgdeck, pngdeck, pdfdeck)
// Supports SVG, PNG, and PDF output
type NativePipeline struct {
	deckshBin  string
	svgdeckBin string
	pngdeckBin string
	pdfdeckBin string
}

// NewNativePipeline creates a new native pipeline
// If binDir is empty, it looks for binaries in .bin/deck/ relative to working directory
func NewNativePipeline(binDir string) (*NativePipeline, error) {
	if binDir == "" {
		// Default to .bin/deck/
		binDir = ".bin/deck"
	}

	// Convert to absolute path to handle working directory changes
	absBinDir, err := filepath.Abs(binDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for binDir: %w", err)
	}

	p := &NativePipeline{
		deckshBin:  filepath.Join(absBinDir, "decksh"),
		svgdeckBin: filepath.Join(absBinDir, "svgdeck"),
		pngdeckBin: filepath.Join(absBinDir, "pngdeck"),
		pdfdeckBin: filepath.Join(absBinDir, "pdfdeck"),
	}

	// Verify decksh exists (required for all formats)
	if _, err := os.Stat(p.deckshBin); err != nil {
		return nil, fmt.Errorf("decksh binary not found at %s: %w", p.deckshBin, err)
	}

	return p, nil
}

// Process implements Pipeline.Process
// For sources with imports, use ProcessFile or ProcessWithWorkDir instead
func (p *NativePipeline) Process(ctx context.Context, source []byte, format OutputFormat) (*Result, error) {
	return p.ProcessWithWorkDir(ctx, source, format, "")
}

// ProcessWithWorkDir processes decksh source with a working directory for resolving imports
// If workDir is empty, uses stdin piping (imports won't work)
// If workDir is set, writes source to a temp file in that directory
func (p *NativePipeline) ProcessWithWorkDir(ctx context.Context, source []byte, format OutputFormat, workDir string) (*Result, error) {
	var xmlData []byte
	var err error

	if workDir != "" {
		// Convert workDir to absolute path
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for workDir: %w", err)
		}
		// File-based processing for imports
		xmlData, err = p.runDeckshFile(ctx, source, absWorkDir)
	} else {
		// Stdin-based processing (no imports)
		xmlData, err = p.runDeckshStdin(ctx, source)
	}

	if err != nil {
		return nil, err
	}

	// Parse deck XML to get slide count and title
	var d deck.Deck
	if err := xml.Unmarshal(xmlData, &d); err != nil {
		return nil, fmt.Errorf("failed to parse deck XML: %w", err)
	}

	// Step 2: Pipe to appropriate renderer
	var rendererBin string
	switch format {
	case FormatSVG:
		rendererBin = p.svgdeckBin
	case FormatPNG:
		rendererBin = p.pngdeckBin
	case FormatPDF:
		rendererBin = p.pdfdeckBin
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	// Verify renderer exists
	if _, err := os.Stat(rendererBin); err != nil {
		return nil, fmt.Errorf("%s binary not found at %s: %w", format, rendererBin, err)
	}

	// For PNG and PDF, we need to generate all slides
	// For SVG, we generate each slide separately
	// Pass workDir so renderers can find image assets
	var assetDir string
	if workDir != "" {
		// Convert to absolute path
		absAssetDir, err := filepath.Abs(workDir)
		if err == nil {
			assetDir = absAssetDir
		}
	}
	slides, err := p.renderSlides(ctx, rendererBin, xmlData, len(d.Slide), format, assetDir)
	if err != nil {
		return nil, err
	}

	return &Result{
		Slides:     slides,
		Format:     format,
		Title:      d.Title,
		SlideCount: len(d.Slide),
	}, nil
}

// renderSlides renders all slides using the specified renderer
// assetDir is the directory where image assets can be found (empty if none)
func (p *NativePipeline) renderSlides(ctx context.Context, rendererBin string, xmlData []byte, slideCount int, format OutputFormat, assetDir string) ([][]byte, error) {
	// Create temp directory for processing
	tmpDir, err := os.MkdirTemp("", "deckfs-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write XML to temp file
	xmlFile := filepath.Join(tmpDir, "deck.xml")
	if err := os.WriteFile(xmlFile, xmlData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write XML file: %w", err)
	}

	// Get fontdir from environment or default to .src/deckfonts
	fontDir := os.Getenv("DECKFONTS")
	if fontDir == "" {
		fontDir = ".src/deckfonts"
	}

	// Convert to absolute path
	absFontDir, err := filepath.Abs(fontDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for fontDir: %w", err)
	}

	// PDF needs special handling: generate all pages in one command
	if format == FormatPDF {
		// Generate single multi-page PDF
		cmd := exec.CommandContext(ctx, rendererBin, "-pages", fmt.Sprintf("1-%d", slideCount), "-fontdir", absFontDir, "-outdir", tmpDir, xmlFile)
		if assetDir != "" {
			cmd.Dir = assetDir // Set working directory to find image assets
		}
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("pdf failed: %w\nstderr: %s", err, errBuf.String())
		}

		// Read the single PDF file
		pdfFile := filepath.Join(tmpDir, "deck.pdf")
		pdfData, err := os.ReadFile(pdfFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read generated pdf: %w", err)
		}

		// Return as single-element slice (multi-page PDF document)
		return [][]byte{pdfData}, nil
	}

	// SVG/PNG: Generate each slide separately
	slides := make([][]byte, slideCount)
	for i := 0; i < slideCount; i++ {
		pageNum := i + 1
		var cmd *exec.Cmd

		switch format {
		case FormatSVG:
			cmd = exec.CommandContext(ctx, rendererBin, "-pages", fmt.Sprintf("%d-%d", pageNum, pageNum), "-outdir", tmpDir, xmlFile)
		case FormatPNG:
			cmd = exec.CommandContext(ctx, rendererBin, "-pages", fmt.Sprintf("%d-%d", pageNum, pageNum), "-fontdir", absFontDir, "-outdir", tmpDir, xmlFile)
		}

		if assetDir != "" {
			cmd.Dir = assetDir // Set working directory to find image assets
		}

		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%s failed on slide %d: %w\nstderr: %s", format, pageNum, err, errBuf.String())
		}

		// Read the generated file (format: deck-00001.{svg|png})
		var ext string
		if format == FormatSVG {
			ext = "svg"
		} else {
			ext = "png"
		}
		outputFile := filepath.Join(tmpDir, fmt.Sprintf("deck-%05d.%s", pageNum, ext))
		fileData, err := os.ReadFile(outputFile)
		if err != nil {
			stderr := errBuf.String()
			if stderr != "" {
				return nil, fmt.Errorf("failed to read generated %s for slide %d: %w\nstderr: %s", format, pageNum, err, stderr)
			}
			return nil, fmt.Errorf("failed to read generated %s for slide %d: %w", format, pageNum, err)
		}

		slides[i] = fileData
	}

	return slides, nil
}

// SupportedFormats implements Pipeline.SupportedFormats
func (p *NativePipeline) SupportedFormats() []OutputFormat {
	formats := []OutputFormat{}

	// Check which binaries are available
	if _, err := os.Stat(p.svgdeckBin); err == nil {
		formats = append(formats, FormatSVG)
	}
	if _, err := os.Stat(p.pngdeckBin); err == nil {
		formats = append(formats, FormatPNG)
	}
	if _, err := os.Stat(p.pdfdeckBin); err == nil {
		formats = append(formats, FormatPDF)
	}

	return formats
}

// runDeckshStdin runs decksh with stdin (no imports support)
func (p *NativePipeline) runDeckshStdin(ctx context.Context, source []byte) ([]byte, error) {
	deckshCmd := exec.CommandContext(ctx, p.deckshBin)
	deckshCmd.Stdin = bytes.NewReader(source)

	// Add .bin/deck to PATH for dchart and other deck tools
	binDir := filepath.Dir(p.deckshBin)
	if currentPath := os.Getenv("PATH"); currentPath != "" {
		deckshCmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s:%s", binDir, currentPath))
	} else {
		deckshCmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", binDir))
	}

	var xmlBuf bytes.Buffer
	deckshCmd.Stdout = &xmlBuf
	var stderrBuf bytes.Buffer
	deckshCmd.Stderr = &stderrBuf

	if err := deckshCmd.Run(); err != nil {
		return nil, fmt.Errorf("decksh failed: %w\nstderr: %s", err, stderrBuf.String())
	}

	return xmlBuf.Bytes(), nil
}

// runDeckshFile runs decksh with a file in a working directory (supports imports)
func (p *NativePipeline) runDeckshFile(ctx context.Context, source []byte, workDir string) ([]byte, error) {
	// Write source to temp file in working directory
	tmpFile := filepath.Join(workDir, "input.dsh")
	if err := os.WriteFile(tmpFile, source, 0644); err != nil {
		return nil, fmt.Errorf("failed to write source file: %w", err)
	}
	defer os.Remove(tmpFile)

	// Run decksh with the file path
	deckshCmd := exec.CommandContext(ctx, p.deckshBin, tmpFile)
	deckshCmd.Dir = workDir // Set working directory

	// Add .bin/deck to PATH for dchart and other deck tools
	binDir := filepath.Dir(p.deckshBin)
	if currentPath := os.Getenv("PATH"); currentPath != "" {
		deckshCmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s:%s", binDir, currentPath))
	} else {
		deckshCmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", binDir))
	}

	var xmlBuf bytes.Buffer
	deckshCmd.Stdout = &xmlBuf
	var stderrBuf bytes.Buffer
	deckshCmd.Stderr = &stderrBuf

	if err := deckshCmd.Run(); err != nil {
		return nil, fmt.Errorf("decksh failed: %w\nstderr: %s", err, stderrBuf.String())
	}

	return xmlBuf.Bytes(), nil
}

// ProcessFile processes a decksh file by path (supports imports)
func (p *NativePipeline) ProcessFile(ctx context.Context, filePath string, format OutputFormat) (*Result, error) {
	// Read the file
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Get the directory for resolving imports
	workDir := filepath.Dir(filePath)

	return p.ProcessWithWorkDir(ctx, source, format, workDir)
}
