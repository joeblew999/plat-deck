
// Package handler provides HTTP handlers that work across all runtimes
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joeblew999/deckfs/demo"
	"github.com/joeblew999/deckfs/pkg/pipeline"
	"github.com/joeblew999/deckfs/runtime"
)

const Version = "0.1.0"

// RegisterHandlers registers all HTTP handlers
func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/", cors(handleRoot))
	mux.HandleFunc("/health", cors(handleHealth))
	mux.HandleFunc("/process", cors(handleProcess))
	mux.HandleFunc("/slides/", cors(handleGetSlide))
	mux.HandleFunc("/manifest/", cors(handleGetManifest))
	mux.HandleFunc("/decks", cors(handleListDecks))
	mux.HandleFunc("/upload/", cors(handleUpload))
	mux.HandleFunc("/status/", cors(handleStatus))
	mux.HandleFunc("/examples", cors(handleListExamples))
	mux.HandleFunc("/examples/", cors(handleGetExample))
	mux.HandleFunc("/deck/", cors(handleDeckRoute))
}

// cors wraps a handler with CORS headers
func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h(w, r)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Check if request is from browser (Accept: text/html)
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/html") {
		// Serve demo HTML
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(demo.HTML)
		return
	}

	// Serve API info for non-browser requests
	formats := runtime.GetPipeline().SupportedFormats()
	formatStrs := make([]string, len(formats))
	for i, f := range formats {
		formatStrs[i] = string(f)
	}

	writeJSON(w, RootResponse{
		Service:   "deckfs",
		Version:   Version,
		Runtime:   "wasm",
		Endpoints: []string{"/health", "/process", "/slides/:key", "/manifest/:name", "/decks", "/upload/:key", "/status/:key", "/examples", "/examples/:path"},
		Formats:   formatStrs,
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, HealthResponse{
		Status:  "ok",
		Runtime: "wasm",
	})
}

func handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Validate sourcePath if provided
	sourcePath := r.URL.Query().Get("source")
	if sourcePath != "" {
		v := NewValidator()
		v.RequireNoPathTraversal("source", sourcePath)
		if !v.IsValid() {
			writeError(w, v.Error(), http.StatusBadRequest)
			return
		}
	}

	// Expand imports if needed (WASM only)
	source, err = expandImports(r.Context(), source, sourcePath)
	if err != nil {
		writeError(w, fmt.Sprintf("Import resolution failed: %v", err), http.StatusBadRequest)
		return
	}

	// Process using runtime pipeline
	// TODO: Support custom dimensions from query params
	result, err := runtime.GetPipeline().Process(r.Context(), source, runtime.FormatSVG)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	slides := make([]string, len(result.Slides))
	for i, s := range result.Slides {
		slides[i] = string(s)
	}

	writeJSON(w, ProcessResponse{
		Success:    true,
		Title:      result.Title,
		SlideCount: result.SlideCount,
		Slides:     slides,
		Format:     "svg",
	})
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/upload/")

	// Validate key
	v := NewValidator()
	v.RequireNonEmpty("key", key)
	v.RequireNoPathTraversal("key", key)
	if !strings.HasSuffix(key, ".dsh") {
		writeError(w, "Key must end with .dsh", http.StatusBadRequest)
		return
	}
	if !v.IsValid() {
		writeError(w, v.Error(), http.StatusBadRequest)
		return
	}

	source, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	input := runtime.Input()
	output := runtime.Output()

	// Store source
	if err := input.Put(ctx, key, source, "text/plain"); err != nil {
		writeError(w, fmt.Sprintf("Failed to store source: %v", err), http.StatusInternalServerError)
		return
	}

	// Expand imports if needed (WASM only)
	processSource, err := expandImports(ctx, source, key)
	if err != nil {
		writeError(w, fmt.Sprintf("Import resolution failed: %v", err), http.StatusBadRequest)
		return
	}

	// Process using runtime pipeline
	result, err := runtime.GetPipeline().Process(ctx, processSource, runtime.FormatSVG)
	if err != nil {
		writeError(w, fmt.Sprintf("Processing failed: %v", err), http.StatusBadRequest)
		return
	}

	// Store slides
	baseName := strings.TrimSuffix(key, ".dsh")
	for i, slide := range result.Slides {
		slideKey := fmt.Sprintf("%s/slide-%04d.svg", baseName, i+1)
		if err := output.Put(ctx, slideKey, slide, "image/svg+xml"); err != nil {
			writeError(w, fmt.Sprintf("Failed to store slide %d: %v", i+1, err), http.StatusInternalServerError)
			return
		}
	}

	// Store manifest
	manifest := map[string]any{
		"sourceKey":   key,
		"processedAt": time.Now().UTC().Format(time.RFC3339),
		"title":       result.Title,
		"slideCount":  result.SlideCount,
		"slides":      makeSlideList(baseName, result.SlideCount),
	}
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	manifestKey := fmt.Sprintf("%s/manifest.json", baseName)

	if err := output.Put(ctx, manifestKey, manifestJSON, "application/json"); err != nil {
		writeError(w, fmt.Sprintf("Failed to store manifest: %v", err), http.StatusInternalServerError)
		return
	}

	// Build slide URL list
	slides := make([]string, result.SlideCount)
	for i := 0; i < result.SlideCount; i++ {
		slides[i] = fmt.Sprintf("%s/slide-%04d.svg", baseName, i+1)
	}

	writeJSON(w, UploadResponse{
		Success:    true,
		Key:        key,
		SlideCount: result.SlideCount,
		Slides:     slides,
	})
}

func handleGetSlide(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/slides/")
	if key == "" {
		writeError(w, "Missing key", http.StatusBadRequest)
		return
	}

	reader, err := runtime.Output().Get(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	io.Copy(w, reader)
}

func handleGetManifest(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/manifest/")
	name = strings.TrimSuffix(name, ".dsh")
	if name == "" {
		writeError(w, "Missing name", http.StatusBadRequest)
		return
	}

	key := fmt.Sprintf("%s/manifest.json", name)
	reader, err := runtime.Output().Get(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, reader)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/status/")

	// Validate key
	v := NewValidator()
	v.RequireNonEmpty("key", key)
	v.RequireNoPathTraversal("key", key)
	if !v.IsValid() {
		writeError(w, v.Error(), http.StatusBadRequest)
		return
	}

	data, err := runtime.KV().Get(r.Context(), "status:"+key)
	if err != nil || data == nil {
		writeJSON(w, StatusResponse{
			Status: "unknown",
		})
		return
	}

	var status StatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		writeError(w, "Invalid status data", http.StatusInternalServerError)
		return
	}

	writeJSON(w, status)
}

func handleListDecks(w http.ResponseWriter, r *http.Request) {
	result, err := runtime.Output().List(r.Context(), "", "/")
	if err != nil {
		writeError(w, fmt.Sprintf("List failed: %v", err), http.StatusInternalServerError)
		return
	}

	decks := make([]DeckInfo, 0)
	for _, prefix := range result.DelimitedPrefixes {
		key := strings.TrimSuffix(prefix, "/")
		decks = append(decks, DeckInfo{
			Key: key,
			// TODO: Optionally read manifest.json to populate SlideCount and ProcessedAt
		})
	}

	writeJSON(w, DecksResponse{
		Decks: decks,
		Count: len(decks),
	})
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   message,
		Success: false,
	})
}

func makeSlideList(baseName string, count int) []map[string]any {
	slides := make([]map[string]any, count)
	for i := 0; i < count; i++ {
		slides[i] = map[string]any{
			"number": i + 1,
			"key":    fmt.Sprintf("%s/slide-%04d.svg", baseName, i+1),
		}
	}
	return slides
}

// expandImports pre-expands import/include statements for WASM environments
func expandImports(ctx context.Context, source []byte, sourcePath string) ([]byte, error) {
	// Check if source has imports and expand them
	if !pipeline.HasImports(source) || sourcePath == "" {
		return source, nil
	}

	// Create import resolver with R2 input storage
	resolver := pipeline.NewImportResolver(
		pipeline.StorageLoader(runtime.Input()),
		"", // R2 keys are already absolute-like
	)

	// Expand imports
	return resolver.Expand(ctx, source, sourcePath)
}

// handleListExamples lists all example deck files from storage
func handleListExamples(w http.ResponseWriter, r *http.Request) {
	// List all .dsh files from INPUT storage
	listResult, err := runtime.Input().List(r.Context(), "", "")
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to list examples: %v", err), http.StatusInternalServerError)
		return
	}

	var examples []Example

	for _, key := range listResult.Keys {
		if !strings.HasSuffix(key, ".dsh") {
			continue
		}

		// Extract name from path
		name := strings.TrimSuffix(key, ".dsh")

		// Assume all .dsh files are renderable
		// Actual renderability will be determined when file is accessed
		examples = append(examples, Example{
			Name:       name,
			Path:       key,
			Renderable: true, // Optimistic assumption
		})
	}

	writeJSON(w, ExamplesResponse{
		Examples: examples,
		Count:    len(examples),
	})
}

// handleGetExample returns the content of a specific example file
func handleGetExample(w http.ResponseWriter, r *http.Request) {
	examplePath := strings.TrimPrefix(r.URL.Path, "/examples/")

	// Validate path
	v := NewValidator()
	v.RequireNonEmpty("path", examplePath)
	v.RequireNoPathTraversal("path", examplePath)
	if !v.IsValid() {
		writeError(w, v.Error(), http.StatusBadRequest)
		return
	}

	reader, err := runtime.Input().Get(r.Context(), examplePath)
	if err != nil {
		writeError(w, "Example not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, "Failed to read example", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(content)
}

// handleDeckRoute routes deck requests to slide or asset handlers
// Supports: /deck/:examplePath/slide/:num.svg or /deck/:examplePath/asset/:filename
func handleDeckRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/deck/")

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
		writeError(w, "Invalid path", http.StatusBadRequest)
		return
	}

	switch routeType {
	case "slide":
		handleDeckSlide(w, r, examplePath, routeParam)
	case "asset":
		handleDeckAsset(w, r, examplePath, routeParam)
	default:
		http.NotFound(w, r)
	}
}

func handleDeckSlide(w http.ResponseWriter, r *http.Request, examplePath string, slideParam string) {
	// Validate inputs
	v := NewValidator()
	v.RequireNonEmpty("examplePath", examplePath)
	v.RequireNoPathTraversal("examplePath", examplePath)
	v.RequireNonEmpty("slideParam", slideParam)
	if !v.IsValid() {
		writeError(w, v.Error(), http.StatusBadRequest)
		return
	}

	// Parse slide number from "1.svg" -> 1
	slideNumStr := strings.TrimSuffix(slideParam, ".svg")
	slideNum, err := strconv.Atoi(slideNumStr)
	if err != nil || slideNum < 1 {
		writeError(w, "Invalid slide number", http.StatusBadRequest)
		return
	}

	// Read deck source from storage
	reader, err := runtime.Input().Get(r.Context(), examplePath)
	if err != nil {
		writeError(w, "Deck not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	source, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, "Failed to read deck", http.StatusInternalServerError)
		return
	}

	// Expand imports if needed
	source, err = expandImports(r.Context(), source, examplePath)
	if err != nil {
		writeError(w, fmt.Sprintf("Import resolution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Process using runtime pipeline
	result, err := runtime.GetPipeline().Process(r.Context(), source, runtime.FormatSVG)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to render deck: %v", err), http.StatusInternalServerError)
		return
	}

	// Check slide exists
	if slideNum > len(result.Slides) {
		writeError(w, "Slide not found", http.StatusNotFound)
		return
	}

	// Get slide (1-indexed)
	slide := result.Slides[slideNum-1]

	// Rewrite links in SVG
	rewrittenSlide := rewriteSVGLinks(slide, examplePath)

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write(rewrittenSlide)
}

func handleDeckAsset(w http.ResponseWriter, r *http.Request, examplePath string, filename string) {
	// Validate inputs
	v := NewValidator()
	v.RequireNonEmpty("examplePath", examplePath)
	v.RequireNoPathTraversal("examplePath", examplePath)
	v.RequireNonEmpty("filename", filename)
	v.RequireNoPathTraversal("filename", filename)
	if strings.Contains(filename, "/") {
		writeError(w, "Filename cannot contain path separators", http.StatusBadRequest)
		return
	}
	if !v.IsValid() {
		writeError(w, v.Error(), http.StatusBadRequest)
		return
	}

	// Get directory of the deck file
	lastSlash := strings.LastIndex(examplePath, "/")
	var assetPath string
	if lastSlash >= 0 {
		deckDir := examplePath[:lastSlash]
		assetPath = deckDir + "/" + filename
	} else {
		assetPath = filename
	}

	// Read asset from storage
	reader, err := runtime.Input().Get(r.Context(), assetPath)
	if err != nil {
		writeError(w, "Asset not found", http.StatusNotFound)
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, "Failed to read asset", http.StatusInternalServerError)
		return
	}

	// Set content type based on extension
	contentType := "application/octet-stream"
	if strings.HasSuffix(filename, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg") {
		contentType = "image/jpeg"
	} else if strings.HasSuffix(filename, ".gif") {
		contentType = "image/gif"
	} else if strings.HasSuffix(filename, ".svg") {
		contentType = "image/svg+xml"
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(content)
}

func rewriteSVGLinks(svg []byte, examplePath string) []byte {
	svgStr := string(svg)

	// Rewrite temporary file path links to deck URLs
	// Pattern: /var/folders/.../T/deckfs-NNNN/deck-00001.svg
	linkPattern := regexp.MustCompile(`xlink:href="[^"]*(/deck-(\d{5})\.svg)"`)

	rewritten := linkPattern.ReplaceAllStringFunc(svgStr, func(match string) string {
		submatches := linkPattern.FindStringSubmatch(match)
		if len(submatches) >= 3 {
			slideNum := submatches[2]
			num, _ := strconv.Atoi(slideNum)
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
