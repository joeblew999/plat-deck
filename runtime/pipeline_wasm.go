//go:build js || tinygo || cloudflare

package runtime

import (
	"context"

	"github.com/joeblew999/deckfs/pkg/pipeline"
)

// WASMPipeline implements Pipeline using internal WASM processors
type WASMPipeline struct {
	width  int
	height int
}

// NewWASMPipeline creates a new WASM pipeline
func NewWASMPipeline() *WASMPipeline {
	return &WASMPipeline{
		width:  1920,
		height: 1080,
	}
}

// WithDimensions sets the output dimensions
func (p *WASMPipeline) WithDimensions(width, height int) *WASMPipeline {
	p.width = width
	p.height = height
	return p
}

func (p *WASMPipeline) Process(ctx context.Context, source []byte, format Format) (*ProcessResult, error) {
	return p.ProcessWithWorkDir(ctx, source, format, "")
}

func (p *WASMPipeline) ProcessWithWorkDir(ctx context.Context, source []byte, format Format, workDir string) (*ProcessResult, error) {
	// Create internal pipeline
	internalPipeline := pipeline.NewWASMPipeline()
	internalPipeline.WithDimensions(p.width, p.height)

	// Convert format
	var internalFormat pipeline.OutputFormat
	switch format {
	case FormatSVG:
		internalFormat = pipeline.FormatSVG
	case FormatPNG:
		internalFormat = pipeline.FormatPNG
	case FormatPDF:
		internalFormat = pipeline.FormatPDF
	default:
		internalFormat = pipeline.FormatSVG
	}

	// Process
	result, err := internalPipeline.Process(ctx, source, internalFormat)
	if err != nil {
		return nil, err
	}

	// Convert result
	return &ProcessResult{
		Slides:     result.Slides,
		SlideCount: result.SlideCount,
		Title:      result.Title,
	}, nil
}

func (p *WASMPipeline) SupportedFormats() []Format {
	return []Format{FormatSVG}
}
