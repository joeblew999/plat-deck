// Package pipeline defines the interface for converting decksh to various formats
package pipeline

import "context"

// OutputFormat represents the target output format
type OutputFormat string

const (
	FormatSVG OutputFormat = "svg"
	FormatPNG OutputFormat = "png"
	FormatPDF OutputFormat = "pdf"
)

// Pipeline defines the interface for processing decksh markup
type Pipeline interface {
	// Process converts decksh source to the specified format
	Process(ctx context.Context, source []byte, format OutputFormat) (*Result, error)

	// SupportedFormats returns the formats this pipeline can generate
	SupportedFormats() []OutputFormat
}

// Result holds the output of pipeline processing
type Result struct {
	// Slides contains the rendered output for each slide
	Slides [][]byte

	// Format is the output format
	Format OutputFormat

	// Title from the deck metadata
	Title string

	// SlideCount is the number of slides
	SlideCount int
}
