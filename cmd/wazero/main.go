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
	"regexp"
	"strings"
	"sync"

	"github.com/joeblew999/deckfs/pkg/pipeline"
	"github.com/joeblew999/deckfs/runtime"
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

	// Initialize runtime pipeline
	runtimePipe, err := runtime.NewNativePipeline(*binDir)
	if err != nil {
		log.Fatalf("Failed to create runtime pipeline: %v", err)
	}
	runtime.SetPipeline(runtimePipe)

	// Create HTTP server
	server := &Server{
		pipeline:    pipe,
		examplesDir: *examplesDir,
		deckCache:   NewDeckCache(),
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
	deckCache   *DeckCache
}

// DeckCache stores rendered decks in memory
type DeckCache struct {
	mu    sync.RWMutex
	decks map[string]*CachedDeck
}

// CachedDeck stores the rendered slides for a deck
type CachedDeck struct {
	ExamplePath string
	Slides      [][]byte
	SlideCount  int
}

func NewDeckCache() *DeckCache {
	return &DeckCache{
		decks: make(map[string]*CachedDeck),
	}
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

	case strings.HasPrefix(r.URL.Path, "/deck/"):
		// Deck routing: /deck/:examplePath/slide/:num.svg or /deck/:examplePath/asset/:filename
		s.handleDeckRoute(w, r)

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
		for i, slide := range result.Slides {
			// Rewrite links if source path is provided (for demo UI navigation)
			if sourcePath != "" {
				rewritten := s.rewriteSVGLinks(slide, sourcePath)
				slides[i] = string(rewritten)
			} else {
				slides[i] = string(slide)
			}
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
		Name       string `json:"name"`
		Path       string `json:"path"`
		Renderable bool   `json:"renderable"`
	}

	// Check if we should filter to only renderable decks
	filterRenderable := r.URL.Query().Get("renderable") == "true"

	var examples []Example

	err := filepath.WalkDir(s.examplesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".dsh") {
			relPath, _ := filepath.Rel(s.examplesDir, path)
			
			// Always check if file is renderable (contains "deck" keyword)
			isRenderable := false
			content, err := os.ReadFile(path)
			if err == nil {
				// Check if file contains deck/edeck structure
				contentStr := string(content)
				isRenderable = strings.HasPrefix(contentStr, "deck\n") ||
					strings.HasPrefix(contentStr, "deck ") ||
					strings.Contains(contentStr, "\ndeck\n") ||
					strings.Contains(contentStr, "\ndeck ")
			}
			
			// Skip non-renderable files if filter is enabled
			if filterRenderable && !isRenderable {
				return nil
			}

			name := strings.TrimSuffix(filepath.Base(path), ".dsh")
			dir := filepath.Dir(relPath)
			if dir != "." {
				name = dir + "/" + name
			}
			examples = append(examples, Example{
				Name:       name,
				Path:       relPath,
				Renderable: isRenderable,
			})
		}
		return nil
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list examples: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"examples": examples,
		"count":    len(examples),
	})
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

func (s *Server) handleDeckRoute(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /deck/:examplePath/slide/:num.svg or /deck/:examplePath/asset/:filename
	path := strings.TrimPrefix(r.URL.Path, "/deck/")

	// Extract examplePath and route type
	var examplePath string
	var routeType string
	var routeParam string

	if strings.Contains(path, "/slide/") {
		parts := strings.SplitN(path, "/slide/", 2)
		examplePath = parts[0]
		routeType = "slide"
		routeParam = parts[1]
	} else if strings.Contains(path, "/asset/") {
		parts := strings.SplitN(path, "/asset/", 2)
		examplePath = parts[0]
		routeType = "asset"
		routeParam = parts[1]
	} else {
		// Just the deck path - redirect to slide 1
		examplePath = path
		http.Redirect(w, r, fmt.Sprintf("/deck/%s/slide/1.svg", examplePath), http.StatusFound)
		return
	}

	// Security: prevent path traversal
	if strings.Contains(examplePath, "..") || strings.Contains(routeParam, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	switch routeType {
	case "slide":
		s.handleDeckSlide(w, r, examplePath, routeParam)
	case "asset":
		s.handleDeckAsset(w, r, examplePath, routeParam)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDeckSlide(w http.ResponseWriter, r *http.Request, examplePath string, slideParam string) {
	// Parse slide number from "1.svg" -> 1
	slideNum := 0
	fmt.Sscanf(slideParam, "%d.svg", &slideNum)
	if slideNum < 1 {
		http.Error(w, "Invalid slide number", http.StatusBadRequest)
		return
	}

	// Get or render deck
	deck, err := s.getOrRenderDeck(r, examplePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render deck: %v", err), http.StatusInternalServerError)
		return
	}

	// Check slide exists
	if slideNum > deck.SlideCount {
		http.Error(w, "Slide not found", http.StatusNotFound)
		return
	}

	// Get slide (1-indexed)
	slide := deck.Slides[slideNum-1]

	// Rewrite links in SVG
	rewrittenSlide := s.rewriteSVGLinks(slide, examplePath)

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write(rewrittenSlide)
}

func (s *Server) handleDeckAsset(w http.ResponseWriter, r *http.Request, examplePath string, filename string) {
	// Serve asset file from deck directory
	deckDir := filepath.Join(s.examplesDir, filepath.Dir(examplePath))
	assetPath := filepath.Join(deckDir, filename)

	// Security: ensure asset is within deck directory
	absAssetPath, err := filepath.Abs(assetPath)
	if err != nil {
		http.Error(w, "Invalid asset path", http.StatusBadRequest)
		return
	}
	absDeckDir, err := filepath.Abs(deckDir)
	if err != nil {
		http.Error(w, "Invalid deck path", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(absAssetPath, absDeckDir) {
		http.Error(w, "Asset path outside deck directory", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, assetPath)
}

func (s *Server) getOrRenderDeck(r *http.Request, examplePath string) (*CachedDeck, error) {
	// Check cache first
	s.deckCache.mu.RLock()
	cached, ok := s.deckCache.decks[examplePath]
	s.deckCache.mu.RUnlock()
	if ok {
		return cached, nil
	}

	// Render deck
	s.deckCache.mu.Lock()
	defer s.deckCache.mu.Unlock()

	// Double-check after acquiring write lock
	if cached, ok := s.deckCache.decks[examplePath]; ok {
		return cached, nil
	}

	// Read deck source
	fullPath := filepath.Join(s.examplesDir, examplePath)
	source, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deck: %w", err)
	}

	// Get working directory for import resolution
	workDir := filepath.Dir(fullPath)

	// Render with SVG format
	result, err := s.pipeline.ProcessWithWorkDir(r.Context(), source, pipeline.FormatSVG, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to process deck: %w", err)
	}

	// Cache the rendered deck
	deck := &CachedDeck{
		ExamplePath: examplePath,
		Slides:      result.Slides,
		SlideCount:  result.SlideCount,
	}
	s.deckCache.decks[examplePath] = deck

	return deck, nil
}

func (s *Server) rewriteSVGLinks(svg []byte, examplePath string) []byte {
	svgStr := string(svg)

	// Rewrite temporary file path links to deck URLs
	// Pattern: /var/folders/.../T/deckfs-NNNN/deck-00001.svg
	linkPattern := regexp.MustCompile(`xlink:href="[^"]*(/deck-(\d{5})\.svg)"`)

	rewritten := linkPattern.ReplaceAllStringFunc(svgStr, func(match string) string {
		// Extract slide number from deck-00001.svg
		submatches := linkPattern.FindStringSubmatch(match)
		if len(submatches) >= 3 {
			slideNum := submatches[2]
			// Convert to integer to remove leading zeros
			num := 0
			fmt.Sscanf(slideNum, "%d", &num)
			return fmt.Sprintf(`xlink:href="/deck/%s/slide/%d.svg"`, examplePath, num)
		}
		return match
	})

	// Rewrite image asset references
	// Pattern: xlink:href="filename.png"
	assetPattern := regexp.MustCompile(`xlink:href="([^"/][^"]*\.(png|jpg|jpeg|gif|svg))"`)

	rewritten = assetPattern.ReplaceAllStringFunc(rewritten, func(match string) string {
		submatches := assetPattern.FindStringSubmatch(match)
		if len(submatches) >= 2 {
			filename := submatches[1]
			return fmt.Sprintf(`xlink:href="/deck/%s/asset/%s"`, examplePath, filename)
		}
		return match
	})

	return []byte(rewritten)
}
