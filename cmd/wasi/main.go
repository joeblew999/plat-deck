//go:build wasi || wasip1

// WASI entry point - for running in wazero or other WASI runtimes
// Uses stdin/stdout for I/O, designed to be called as a CLI tool
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/joeblew999/deckfs/pkg/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "process":
		doProcess()
	case "version":
		fmt.Println("deckfs-wasm v0.1.0 (wasi)")
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: deckfs <command>")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  process  Read decksh from stdin, write JSON result to stdout")
	fmt.Fprintln(os.Stderr, "  version  Print version")
	fmt.Fprintln(os.Stderr, "  help     Print this help")
}

func doProcess() {
	// Read decksh source from stdin
	source, err := io.ReadAll(os.Stdin)
	if err != nil {
		outputError(fmt.Sprintf("Failed to read stdin: %v", err))
		return
	}

	// Process (WASI uses WASMPipeline)
	p := pipeline.NewWASMPipeline()

	// Check for config in env vars
	width, height := 1920, 1080
	if w := os.Getenv("DECKFS_WIDTH"); w != "" {
		fmt.Sscanf(w, "%d", &width)
	}
	if h := os.Getenv("DECKFS_HEIGHT"); h != "" {
		fmt.Sscanf(h, "%d", &height)
	}
	p.WithDimensions(width, height)

	result, err := p.Process(context.Background(), source, pipeline.FormatSVG)
	if err != nil {
		outputError(err.Error())
		return
	}

	// Convert slides to strings
	slides := make([]string, len(result.Slides))
	for i, s := range result.Slides {
		slides[i] = string(s)
	}

	// Output JSON result
	output := map[string]any{
		"success":    true,
		"title":      result.Title,
		"slideCount": result.SlideCount,
		"slides":     slides,
	}

	json.NewEncoder(os.Stdout).Encode(output)
}

func outputError(msg string) {
	output := map[string]any{
		"success": false,
		"error":   msg,
	}
	json.NewEncoder(os.Stdout).Encode(output)
}
