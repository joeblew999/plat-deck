//go:build !js && !wasi

// Native CLI entry point for testing and local use
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/joeblew999/deckfs/internal/processor"
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

	// Check if file argument provided
	if len(os.Args) > 2 {
		filePath := os.Args[2]

		// Get absolute path and directory
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			outputError(fmt.Sprintf("Failed to resolve path: %v", err))
			os.Exit(1)
		}

		fileDir := filepath.Dir(absPath)

		// Change to file's directory so includes resolve correctly
		origDir, err := os.Getwd()
		if err != nil {
			outputError(fmt.Sprintf("Failed to get working directory: %v", err))
			os.Exit(1)
		}

		if err := os.Chdir(fileDir); err != nil {
			outputError(fmt.Sprintf("Failed to change directory: %v", err))
			os.Exit(1)
		}
		defer os.Chdir(origDir)

		// Read the file
		source, err = os.ReadFile(filepath.Base(absPath))
		if err != nil {
			outputError(fmt.Sprintf("Failed to read file: %v", err))
			os.Exit(1)
		}
	} else {
		// Read from stdin (legacy mode - includes won't work properly)
		source, err = io.ReadAll(os.Stdin)
		if err != nil {
			outputError(fmt.Sprintf("Failed to read stdin: %v", err))
			os.Exit(1)
		}
	}

	// Process
	cfg := processor.DefaultConfig()

	// Check for config in env vars
	if w := os.Getenv("DECKFS_WIDTH"); w != "" {
		fmt.Sscanf(w, "%d", &cfg.Width)
	}
	if h := os.Getenv("DECKFS_HEIGHT"); h != "" {
		fmt.Sscanf(h, "%d", &cfg.Height)
	}

	result, err := processor.ProcessDeckSH(source, cfg)
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
