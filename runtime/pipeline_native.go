//go:build !js && !tinygo && !cloudflare

package runtime

import (
	"context"

	"github.com/joeblew999/deckfs/pkg/pipeline"
)

// NativePipeline implements Pipeline using native CLI tools
type NativePipeline struct {
	internal *pipeline.NativePipeline
}

// NewNativePipeline creates a new native pipeline
func NewNativePipeline(binDir string) (*NativePipeline, error) {
	internal, err := pipeline.NewNativePipeline(binDir)
	if err != nil {
		return nil, err
	}

	return &NativePipeline{
		internal: internal,
	}, nil
}

func (p *NativePipeline) Process(ctx context.Context, source []byte, format Format) (*ProcessResult, error) {
	return p.ProcessWithWorkDir(ctx, source, format, "")
}

func (p *NativePipeline) ProcessWithWorkDir(ctx context.Context, source []byte, format Format, workDir string) (*ProcessResult, error) {
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

	// Process with or without workDir
	var result *pipeline.Result
	var err error

	if workDir != "" {
		result, err = p.internal.ProcessWithWorkDir(ctx, source, internalFormat, workDir)
	} else {
		result, err = p.internal.Process(ctx, source, internalFormat)
	}

	if err != nil {
		return nil, err
	}

	// Convert result
	return &ProcessResult{
		Slides:     result.Slides,
		SlideCount: result.SlideCount,
		Title:      "",
	}, nil
}

func (p *NativePipeline) SupportedFormats() []Format {
	formats := p.internal.SupportedFormats()
	result := make([]Format, len(formats))
	for i, f := range formats {
		result[i] = Format(f)
	}
	return result
}
