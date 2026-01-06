// Package handler provides HTTP handlers that work across all runtimes
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/joeblew999/deckfs/internal/processor"
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
	writeJSON(w, map[string]any{
		"service":   "deckfs",
		"version":   Version,
		"endpoints": []string{"/health", "/process", "/slides/:key", "/manifest/:name", "/decks", "/upload/:key", "/status/:key"},
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"status":  "ok",
		"version": Version,
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

	cfg := processor.DefaultConfig()

	if ws := r.URL.Query().Get("width"); ws != "" {
		fmt.Sscanf(ws, "%d", &cfg.Width)
	}
	if hs := r.URL.Query().Get("height"); hs != "" {
		fmt.Sscanf(hs, "%d", &cfg.Height)
	}

	result, err := processor.ProcessDeckSH(source, cfg)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	slides := make([]string, len(result.Slides))
	for i, s := range result.Slides {
		slides[i] = string(s)
	}

	writeJSON(w, map[string]any{
		"success":    true,
		"title":      result.Title,
		"slideCount": result.SlideCount,
		"slides":     slides,
	})
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/upload/")
	if key == "" || !strings.HasSuffix(key, ".dsh") {
		writeError(w, "Key must end with .dsh", http.StatusBadRequest)
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

	// Process
	cfg := processor.DefaultConfig()
	result, err := processor.ProcessDeckSH(source, cfg)
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

	writeJSON(w, map[string]any{
		"success":    true,
		"key":        key,
		"slideCount": result.SlideCount,
		"manifest":   manifestKey,
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
	if key == "" {
		writeError(w, "Missing key", http.StatusBadRequest)
		return
	}

	data, err := runtime.KV().Get(r.Context(), "status:"+key)
	if err != nil || data == nil {
		writeJSON(w, map[string]any{
			"key":    key,
			"status": "unknown",
		})
		return
	}

	var status map[string]any
	if err := json.Unmarshal(data, &status); err != nil {
		writeError(w, "Invalid status data", http.StatusInternalServerError)
		return
	}

	status["key"] = key
	writeJSON(w, status)
}

func handleListDecks(w http.ResponseWriter, r *http.Request) {
	result, err := runtime.Output().List(r.Context(), "", "/")
	if err != nil {
		writeError(w, fmt.Sprintf("List failed: %v", err), http.StatusInternalServerError)
		return
	}

	decks := make([]string, 0)
	for _, prefix := range result.DelimitedPrefixes {
		decks = append(decks, strings.TrimSuffix(prefix, "/"))
	}

	writeJSON(w, map[string]any{
		"decks": decks,
	})
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": message,
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
