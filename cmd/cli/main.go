//go:build !js && !wasi

// Native CLI entry point for testing and local use
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
		fmt.Println("deckfs v0.1.0 (native)")
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: deckfs <command> [file]")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  process [file]  Process decksh file (or stdin if no file)")
	fmt.Fprintln(os.Stderr, "                  When file is provided, includes are resolved relative to it")
	fmt.Fprintln(os.Stderr, "  version         Print version")
	fmt.Fprintln(os.Stderr, "  help            Print this help")
}

func doProcess() {
	var source []byte
	var err error
	var workDir string

	// Initialize pipeline BEFORE changing directories
	// This ensures binary paths are resolved from current directory
	binDir := os.Getenv("DECKFS_BIN_DIR")
	if binDir == "" {
		binDir = ".bin/deck"
	}
	p, err := pipeline.NewNativePipeline(binDir)
	if err != nil {
		outputError(fmt.Sprintf("Failed to initialize pipeline: %v", err))
		os.Exit(1)
	}

	// Check if file argument provided
	if len(os.Args) > 2 {
		filePath := os.Args[2]

		// Read the file
		source, err = os.ReadFile(filePath)
		if err != nil {
			outputError(fmt.Sprintf("Failed to read file: %v", err))
			os.Exit(1)
		}

		// Get directory for resolving imports
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			outputError(fmt.Sprintf("Failed to resolve path: %v", err))
			os.Exit(1)
		}
		workDir = filepath.Dir(absPath)
	} else {
		// Read from stdin (legacy mode - includes won't work properly)
		source, err = io.ReadAll(os.Stdin)
		if err != nil {
			outputError(fmt.Sprintf("Failed to read stdin: %v", err))
			os.Exit(1)
		}
	}

	// Process with working directory for import resolution
	var result *pipeline.Result
	if workDir != "" {
		result, err = p.ProcessWithWorkDir(context.Background(), source, pipeline.FormatSVG, workDir)
	} else {
		result, err = p.Process(context.Background(), source, pipeline.FormatSVG)
	}
	if err != nil {
		outputError(err.Error())
		os.Exit(1)
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
