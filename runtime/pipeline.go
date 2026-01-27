package runtime

import "context"

// Pipeline abstracts deck processing across different runtimes
type Pipeline interface {
	// Process converts decksh source to output format
	Process(ctx context.Context, source []byte, format Format) (*ProcessResult, error)

	// ProcessWithWorkDir processes with a working directory for import resolution
	ProcessWithWorkDir(ctx context.Context, source []byte, format Format, workDir string) (*ProcessResult, error)

	// SupportedFormats returns the formats this pipeline can produce
	SupportedFormats() []Format
}

// Format represents output format
type Format string

const (
	FormatSVG Format = "svg"
	FormatPNG Format = "png"
	FormatPDF Format = "pdf"
)

// ProcessResult contains the output of deck processing
type ProcessResult struct {
	Slides     [][]byte // Slide content (SVG, PNG, or single PDF)
	SlideCount int      // Number of slides
	Title      string   // Deck title (if available)
}

var globalPipeline Pipeline

// SetPipeline sets the global pipeline implementation
func SetPipeline(p Pipeline) {
	globalPipeline = p
}

// GetPipeline returns the global pipeline implementation
func GetPipeline() Pipeline {
	return globalPipeline
}
