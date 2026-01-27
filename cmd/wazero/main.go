//go:build !js

// Host server - uses ajstarks' native tools for rendering
// Supports SVG, PNG, PDF via piped CLI tools
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/joeblew999/deckfs/handler"
	"github.com/joeblew999/deckfs/runtime"
)

func main() {
	var (
		addr        = flag.String("addr", ":8080", "Listen address")
		binDir      = flag.String("bin", ".bin/deck", "Directory containing deck binaries (decksh, svgdeck, etc.)")
		examplesDir = flag.String("examples", ".src/deckviz", "Directory containing .dsh examples")
	)
	flag.Parse()

	// Initialize runtime pipeline
	runtimePipe, err := runtime.NewNativePipeline(*binDir)
	if err != nil {
		log.Fatalf("Failed to create runtime pipeline: %v", err)
	}
	runtime.SetPipeline(runtimePipe)

	// Initialize local file storage for examples
	inputStorage, err := runtime.NewLocalFileStorage(*examplesDir)
	if err != nil {
		log.Fatalf("Failed to create input storage: %v", err)
	}

	// Set runtime with both pipeline and storage
	runtime.SetRuntime(&runtime.Runtime{
		InputStorage:  inputStorage,
		OutputStorage: nil, // wazero does not need output storage
		KV:            nil, // wazero does not use KV
		Publisher:     nil, // wazero does not use pub/sub
	})

	// Create HTTP server with shared handlers
	mux := http.NewServeMux()
	handler.RegisterHandlers(mux)

	formats := runtimePipe.SupportedFormats()
	log.Printf("Starting server on %s", *addr)
	log.Printf("Binaries directory: %s", *binDir)
	log.Printf("Examples directory: %s", *examplesDir)
	log.Printf("Supported formats: %v", formats)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
