//go:build js || tinygo || cloudflare

package pipeline

// Tests for import resolver (WASM-only functionality)
// Run with: go test -tags tinygo ./pkg/pipeline/

import (
	"context"
	"strings"
	"testing"
)

func TestHasImports(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "no imports",
			source: "deck\n  slide\n    text \"hello\"\n  eslide\nedeck",
			want:   false,
		},
		{
			name:   "has import",
			source: "import \"common.dsh\"\ndeck\n  slide\n  eslide\nedeck",
			want:   true,
		},
		{
			name:   "has include",
			source: "include \"header.dsh\"\ndeck\n  slide\n  eslide\nedeck",
			want:   true,
		},
		{
			name:   "has import with spaces",
			source: "  import  \"styles.dsh\"  \ndeck\nedeck",
			want:   true,
		},
		{
			name:   "comment not import",
			source: "// import \"fake.dsh\"\ndeck\nedeck",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasImports([]byte(tt.source)); got != tt.want {
				t.Errorf("HasImports() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImportResolver(t *testing.T) {
	// Mock file storage - import loads function definitions
	files := map[string]string{
		"main.dsh": `import "redcircle.dsh"
deck
  slide
    redcircle 50 50
  eslide
edeck`,
		"redcircle.dsh": `def redcircle X Y
	circle X Y 10 "red"
	text "Point" X Y 2
edef`,
	}

	loader := func(ctx context.Context, path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, &testError{"file not found: " + path}
		}
		return []byte(content), nil
	}

	resolver := NewImportResolver(loader, "")

	result, err := resolver.Expand(context.Background(), []byte(files["main.dsh"]), "main.dsh")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	resultStr := string(result)

	// Check that import is replaced with function definition
	if strings.Contains(resultStr, `import "redcircle.dsh"`) {
		t.Error("Import statement not replaced")
	}

	// Check that function definition is included
	if !strings.Contains(resultStr, "def redcircle") {
		t.Error("Function definition missing")
	}
	if !strings.Contains(resultStr, "edef") {
		t.Error("Function definition end missing")
	}
	if !strings.Contains(resultStr, "deck") {
		t.Error("Original content missing")
	}

	// Check for comment marker
	if !strings.Contains(resultStr, "Function imported from: redcircle.dsh") {
		t.Error("Import comment marker missing")
	}
}

func TestImportResolver_DuplicateImport(t *testing.T) {
	// Test that same function imported multiple times is only inlined once
	files := map[string]string{
		"main.dsh": `import "util.dsh"
import "util.dsh"
deck
  slide
    helper 10 10
  eslide
edeck`,
		"util.dsh": `def helper X Y
	circle X Y 5 "blue"
edef`,
	}

	loader := func(ctx context.Context, path string) ([]byte, error) {
		return []byte(files[path]), nil
	}

	resolver := NewImportResolver(loader, "")

	result, err := resolver.Expand(context.Background(), []byte(files["main.dsh"]), "main.dsh")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	// Count occurrences of "def helper" - should only appear once
	resultStr := string(result)
	count := strings.Count(resultStr, "def helper")
	if count != 1 {
		t.Errorf("Expected function to be inlined once, got %d times", count)
	}
}

func TestImportResolver_RelativePaths(t *testing.T) {
	files := map[string]string{
		"decks/main.dsh": `import "common/header.dsh"
deck
  slide
    header 50 10
  eslide
edeck`,
		"decks/common/header.dsh": `def header X Y
	text "Header" X Y 3
edef`,
	}

	loader := func(ctx context.Context, path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, &testError{"file not found: " + path}
		}
		return []byte(content), nil
	}

	resolver := NewImportResolver(loader, "")

	result, err := resolver.Expand(context.Background(), []byte(files["decks/main.dsh"]), "decks/main.dsh")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	if !strings.Contains(string(result), "def header") {
		t.Error("Relative import not resolved")
	}
	if !strings.Contains(string(result), "Header") {
		t.Error("Imported function content missing")
	}
}

func TestImportResolver_Include(t *testing.T) {
	files := map[string]string{
		"main.dsh": `deck
  slide
    include "slide-content.dsh"
  eslide
edeck`,
		"slide-content.dsh": `text "Included content" 50 50 2
circle 50 70 5 "red"`,
	}

	loader := func(ctx context.Context, path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, &testError{"file not found: " + path}
		}
		return []byte(content), nil
	}

	resolver := NewImportResolver(loader, "")

	result, err := resolver.Expand(context.Background(), []byte(files["main.dsh"]), "main.dsh")
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}

	resultStr := string(result)

	// Check that include is expanded
	if strings.Contains(resultStr, `include "slide-content.dsh"`) {
		t.Error("Include statement not replaced")
	}
	if !strings.Contains(resultStr, "Included content") {
		t.Error("Included content missing")
	}
	if !strings.Contains(resultStr, "BEGIN INCLUDE:") {
		t.Error("BEGIN INCLUDE marker missing")
	}
	if !strings.Contains(resultStr, "END INCLUDE:") {
		t.Error("END INCLUDE marker missing")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
