//go:build !js

// Host server - uses ajstarks' native tools for rendering
// Supports SVG, PNG, PDF via piped CLI tools
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/deckfs/pkg/pipeline"
)

func main() {
	var (
		addr        = flag.String("addr", ":8080", "Listen address")
		binDir      = flag.String("bin", ".bin/deck", "Directory containing deck binaries (decksh, svgdeck, etc.)")
		examplesDir = flag.String("examples", ".src/deckviz", "Directory containing .dsh examples")
	)
	flag.Parse()

	// Create native pipeline
	pipe, err := pipeline.NewNativePipeline(*binDir)
	if err != nil {
		log.Fatalf("Failed to create pipeline: %v", err)
	}

	// Create HTTP server
	server := &Server{
		pipeline:    pipe,
		examplesDir: *examplesDir,
	}

	log.Printf("Starting server on %s", *addr)
	log.Printf("Binaries directory: %s", *binDir)
	log.Printf("Supported formats: %v", pipe.SupportedFormats())
	if err := http.ListenAndServe(*addr, server); err != nil {
		log.Fatal(err)
	}
}

type Server struct {
	pipeline    *pipeline.NativePipeline
	examplesDir string
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch {
	case r.URL.Path == "/":
		// Serve demo HTML
		s.handleDemo(w, r)

	case r.URL.Path == "/api":
		// API info
		s.handleRoot(w, r)

	case r.URL.Path == "/health":
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","runtime":"native"}`))

	case r.URL.Path == "/process" && r.Method == "POST":
		s.handleProcess(w, r)

	case r.URL.Path == "/examples":
		s.handleExamplesList(w, r)

	case strings.HasPrefix(r.URL.Path, "/examples/"):
		s.handleExampleContent(w, r)

	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	formats := make([]string, len(s.pipeline.SupportedFormats()))
	for i, f := range s.pipeline.SupportedFormats() {
		formats[i] = string(f)
	}

	info := map[string]any{
		"service":   "deckfs",
		"version":   "0.2.0",
		"runtime":   "native",
		"endpoints": []string{"/health", "/process", "/examples"},
		"formats":   formats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	// Read input
	source, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get format from query param (default: svg)
	formatStr := r.URL.Query().Get("format")
	if formatStr == "" {
		formatStr = "svg"
	}

	format := pipeline.OutputFormat(formatStr)

	// Check if format is supported
	supported := false
	for _, f := range s.pipeline.SupportedFormats() {
		if f == format {
			supported = true
			break
		}
	}
	if !supported {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   fmt.Sprintf("unsupported format: %s", format),
		})
		return
	}

	// Check if source path is provided (for import resolution)
	sourcePath := r.URL.Query().Get("source")
	var result *pipeline.Result

	if sourcePath != "" && s.examplesDir != "" {
		// Security: prevent path traversal
		if strings.Contains(sourcePath, "..") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "invalid source path",
			})
			return
		}

		// Get working directory from source path
		workDir := filepath.Join(s.examplesDir, filepath.Dir(sourcePath))
		result, err = s.pipeline.ProcessWithWorkDir(r.Context(), source, format, workDir)
	} else {
		// No source path, use stdin mode (no imports)
		result, err = s.pipeline.Process(r.Context(), source, format)
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Return result based on format
	switch format {
	case pipeline.FormatSVG:
		// SVG returns slides as strings
		slides := make([]string, len(result.Slides))
		for i, s := range result.Slides {
			slides[i] = string(s)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"slideCount": result.SlideCount,
			"slides":     slides,
			"format":     "svg",
		})

	case pipeline.FormatPNG:
		// PNG returns slides as base64
		slides := make([]string, len(result.Slides))
		for i, s := range result.Slides {
			slides[i] = base64.StdEncoding.EncodeToString(s)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"slideCount": result.SlideCount,
			"slides":     slides,
			"format":     "png",
			"encoding":   "base64",
		})

	case pipeline.FormatPDF:
		// PDF returns single document as base64
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"slideCount": result.SlideCount,
			"document":   base64.StdEncoding.EncodeToString(result.Slides[0]),
			"format":     "pdf",
			"encoding":   "base64",
		})
	}
}

func (s *Server) handleExamplesList(w http.ResponseWriter, r *http.Request) {
	type Example struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var examples []Example

	err := filepath.WalkDir(s.examplesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".dsh") {
			relPath, _ := filepath.Rel(s.examplesDir, path)
			name := strings.TrimSuffix(filepath.Base(path), ".dsh")
			dir := filepath.Dir(relPath)
			if dir != "." {
				name = dir + "/" + name
			}
			examples = append(examples, Example{
				Name: name,
				Path: relPath,
			})
		}
		return nil
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list examples: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(examples)
}

func (s *Server) handleExampleContent(w http.ResponseWriter, r *http.Request) {
	examplePath := strings.TrimPrefix(r.URL.Path, "/examples/")

	// Security: prevent path traversal
	if strings.Contains(examplePath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(s.examplesDir, examplePath)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("File not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(content)
}

func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile("demo/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Demo not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}
