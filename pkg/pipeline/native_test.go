//go:build !js && !tinygo

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativePipeline(t *testing.T) {
	// Test input
	input := []byte(`deck
  slide
    text "Hello from Pipeline" 50 50 5
  eslide
edeck
`)

	// Find project root by looking for go.mod
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	
	// Walk up to find go.mod
	projectRoot := wd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Skip("Could not find project root")
			return
		}
		projectRoot = parent
	}

	binDir := filepath.Join(projectRoot, ".bin", "deck")
	p, err := NewNativePipeline(binDir)
	if err != nil {
		t.Skipf("Skipping test, binaries not available: %v", err)
		return
	}

	t.Logf("Supported formats: %v", p.SupportedFormats())

	result, err := p.Process(context.Background(), input, FormatSVG)
	if err != nil {
		t.Fatalf("Failed to process: %v", err)
	}

	if result.SlideCount != 1 {
		t.Errorf("Expected 1 slide, got %d", result.SlideCount)
	}

	if len(result.Slides) != 1 {
		t.Errorf("Expected 1 slide in results, got %d", len(result.Slides))
	}

	if len(result.Slides[0]) == 0 {
		t.Error("Expected non-empty slide data")
	}

	t.Logf("Successfully generated %d slides", result.SlideCount)
	t.Logf("First slide size: %d bytes", len(result.Slides[0]))
}
