//go:build js || tinygo || cloudflare

// Package pipeline provides import resolution for WASM environments
//
// Decksh's import system loads function definitions (def/edef blocks) from files.
// In WASM environments without file system access, we pre-expand imports by:
// 1. Finding import "file" statements
// 2. Loading the referenced file from storage
// 3. Extracting def/edef function definitions
// 4. Inlining those definitions before the import statement
// 5. Removing the import statement (function already defined)
package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// ImportResolver resolves decksh import statements for WASM environments
type ImportResolver struct {
	// Loader loads file content by path
	Loader func(ctx context.Context, path string) ([]byte, error)

	// BasePath is the base directory for resolving relative imports
	BasePath string

	// funcDefs tracks loaded function definitions to prevent duplicates
	funcDefs map[string]string // funcName -> def...edef block
}

// NewImportResolver creates a new import resolver
func NewImportResolver(loader func(ctx context.Context, path string) ([]byte, error), basePath string) *ImportResolver {
	return &ImportResolver{
		Loader:   loader,
		BasePath: basePath,
		funcDefs: make(map[string]string),
	}
}

var (
	importRegex  = regexp.MustCompile(`^\s*import\s+"([^"]+)"\s*$`)
	includeRegex = regexp.MustCompile(`^\s*include\s+"([^"]+)"\s*$`)
	defRegex     = regexp.MustCompile(`^\s*def\s+(\w+)`)
	edefRegex    = regexp.MustCompile(`^\s*edef\s*$`)
)

// Expand recursively expands all imports in the source
// It extracts function definitions from imported files and inlines them
func (r *ImportResolver) Expand(ctx context.Context, source []byte, sourcePath string) ([]byte, error) {
	// Normalize the source path
	fullPath := sourcePath
	if !filepath.IsAbs(sourcePath) && r.BasePath != "" {
		fullPath = filepath.Join(r.BasePath, sourcePath)
	}

	var result bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(source))

	for scanner.Scan() {
		line := scanner.Text()

		// Check if this line is an import statement (function definition)
		if match := importRegex.FindStringSubmatch(line); match != nil {
			importPath := match[1]
			resolvedPath := r.resolvePath(importPath, fullPath)

			// Load imported file
			importedContent, err := r.Loader(ctx, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load import %q: %w", importPath, err)
			}

			// Extract function definitions from imported file
			funcDef, funcName, err := r.extractFunctionDef(importedContent)
			if err != nil {
				return nil, fmt.Errorf("failed to extract function from %q: %w", importPath, err)
			}

			// Only inline if we haven't seen this function before
			if _, exists := r.funcDefs[funcName]; !exists {
				r.funcDefs[funcName] = funcDef
				// Inline the function definition with a comment
				result.WriteString(fmt.Sprintf("// Function imported from: %s\n", importPath))
				result.WriteString(funcDef)
				result.WriteString("\n")
			}
			// Skip the import statement itself (it's replaced by the inlined def)
			continue
		}

		// Check if this line is an include statement (full content)
		if match := includeRegex.FindStringSubmatch(line); match != nil {
			includePath := match[1]
			resolvedPath := r.resolvePath(includePath, fullPath)

			// Load included file
			includedContent, err := r.Loader(ctx, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load include %q: %w", includePath, err)
			}

			// Recursively expand any imports/includes in the included file
			expandedContent, err := r.Expand(ctx, includedContent, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to expand includes in %q: %w", includePath, err)
			}

			// Inline the full content with a comment
			result.WriteString(fmt.Sprintf("// BEGIN INCLUDE: %s\n", includePath))
			result.Write(expandedContent)
			result.WriteString(fmt.Sprintf("// END INCLUDE: %s\n", includePath))
			continue
		}

		// Regular line, just copy it
		result.WriteString(line)
		result.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan source: %w", err)
	}

	return result.Bytes(), nil
}

// resolvePath resolves a file path relative to the source file directory
func (r *ImportResolver) resolvePath(filePath, sourcePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	// Get directory of current source file
	currentDir := filepath.Dir(sourcePath)
	return filepath.Join(currentDir, filePath)
}

// extractFunctionDef extracts a def/edef block from source
// Returns: (function definition, function name, error)
func (r *ImportResolver) extractFunctionDef(source []byte) (string, string, error) {
	var defBlock bytes.Buffer
	var funcName string
	inDef := false
	scanner := bufio.NewScanner(bytes.NewReader(source))

	for scanner.Scan() {
		line := scanner.Text()

		// Check for def start
		if match := defRegex.FindStringSubmatch(line); match != nil {
			if inDef {
				return "", "", fmt.Errorf("nested def blocks not supported")
			}
			funcName = match[1]
			inDef = true
			defBlock.WriteString(line)
			defBlock.WriteString("\n")
			continue
		}

		// Check for def end
		if edefRegex.MatchString(line) {
			if !inDef {
				return "", "", fmt.Errorf("edef without matching def")
			}
			defBlock.WriteString(line)
			// Found complete function definition
			return defBlock.String(), funcName, nil
		}

		// Inside def block
		if inDef {
			defBlock.WriteString(line)
			defBlock.WriteString("\n")
		}
		// Lines outside def blocks are ignored (comments, etc.)
	}

	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("failed to scan source: %w", err)
	}

	if inDef {
		return "", "", fmt.Errorf("unclosed def block for function %q", funcName)
	}

	if funcName == "" {
		return "", "", fmt.Errorf("no function definition found")
	}

	return defBlock.String(), funcName, nil
}

// HasImports checks if source contains any import or include statements
func HasImports(source []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(source))
	for scanner.Scan() {
		line := scanner.Text()
		if importRegex.MatchString(line) || includeRegex.MatchString(line) {
			return true
		}
	}
	return false
}

// StorageLoader creates a loader function that reads from a storage interface
func StorageLoader(storage interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}) func(ctx context.Context, path string) ([]byte, error) {
	return func(ctx context.Context, path string) ([]byte, error) {
		// Normalize path (remove leading slash for storage keys)
		key := strings.TrimPrefix(path, "/")

		reader, err := storage.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("storage get failed: %w", err)
		}
		defer reader.Close()

		content, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read content: %w", err)
		}

		return content, nil
	}
}
