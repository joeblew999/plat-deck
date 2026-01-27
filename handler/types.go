//go:build js || tinygo || cloudflare

package handler

// Response types for consistent API contracts across all platforms

// ExamplesResponse is returned by /examples endpoint
type ExamplesResponse struct {
	Examples []Example `json:"examples"`
	Count    int       `json:"count"`
}

// Example represents a single deck example file
type Example struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Renderable bool   `json:"renderable"`
}

// ProcessResponse is returned by /process endpoint
type ProcessResponse struct {
	Success    bool     `json:"success"`
	Title      string   `json:"title,omitempty"`
	SlideCount int      `json:"slideCount"`
	Slides     []string `json:"slides"`
	Format     string   `json:"format,omitempty"`
}

// UploadResponse is returned by /upload endpoint
type UploadResponse struct {
	Success    bool     `json:"success"`
	Key        string   `json:"key"`
	SlideCount int      `json:"slideCount"`
	Slides     []string `json:"slides,omitempty"`
}

// StatusResponse is returned by /status endpoint
type StatusResponse struct {
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DecksResponse is returned by /decks endpoint
type DecksResponse struct {
	Decks []DeckInfo `json:"decks"`
	Count int        `json:"count"`
}

// DeckInfo represents metadata about a deck
type DeckInfo struct {
	Key        string `json:"key"`
	SlideCount int    `json:"slideCount,omitempty"`
	ProcessedAt string `json:"processedAt,omitempty"`
}

// ManifestResponse is returned by /manifest endpoint
type ManifestResponse struct {
	SourceKey   string `json:"sourceKey"`
	ProcessedAt string `json:"processedAt"`
	Title       string `json:"title,omitempty"`
	SlideCount  int    `json:"slideCount"`
}

// ErrorResponse is returned for all error cases
type ErrorResponse struct {
	Error   string `json:"error"`
	Success bool   `json:"success"`
}

// HealthResponse is returned by /health endpoint
type HealthResponse struct {
	Status  string `json:"status"`
	Runtime string `json:"runtime,omitempty"`
}

// RootResponse is returned by / endpoint
type RootResponse struct {
	Service   string   `json:"service"`
	Version   string   `json:"version"`
	Runtime   string   `json:"runtime"`
	Endpoints []string `json:"endpoints"`
	Formats   []string `json:"formats,omitempty"`
}
