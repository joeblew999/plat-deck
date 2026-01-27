//go:build !cloudflare

package runtime

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LocalFileStorage implements Storage using the local file system
// Used by wazero server for local development
type LocalFileStorage struct {
	baseDir string // Base directory for all file operations
}

// NewLocalFileStorage creates storage that accesses local file system
func NewLocalFileStorage(baseDir string) (*LocalFileStorage, error) {
	// Ensure base directory exists
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, err
	}

	return &LocalFileStorage{
		baseDir: absPath,
	}, nil
}

// fullPath returns the absolute path for a key, ensuring it's within baseDir
func (s *LocalFileStorage) fullPath(key string) (string, error) {
	// Clean the key to prevent path traversal
	cleanKey := filepath.Clean(key)
	if strings.HasPrefix(cleanKey, "..") {
		return "", fs.ErrInvalid
	}

	fullPath := filepath.Join(s.baseDir, cleanKey)

	// Security check: ensure path is within baseDir
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absPath, s.baseDir) {
		return "", fs.ErrInvalid
	}

	return absPath, nil
}

// FullPath returns the absolute file system path for a storage key
// This is used by native pipelines that need actual file paths for working directories
func (s *LocalFileStorage) FullPath(key string) (string, error) {
	return s.fullPath(key)
}

func (s *LocalFileStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := s.fullPath(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, io.EOF
		}
		return nil, err
	}

	return file, nil
}

func (s *LocalFileStorage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	path, err := s.fullPath(key)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (s *LocalFileStorage) List(ctx context.Context, prefix string, delimiter string) (*ListResult, error) {
	result := &ListResult{
		Keys:              make([]string, 0),
		DelimitedPrefixes: make([]string, 0),
	}

	// Determine the directory to walk
	searchDir := s.baseDir
	if prefix != "" {
		prefixPath, err := s.fullPath(prefix)
		if err != nil {
			return result, nil // Return empty result for invalid prefix
		}

		// If prefix is a directory, search within it
		if info, err := os.Stat(prefixPath); err == nil && info.IsDir() {
			searchDir = prefixPath
		} else {
			// If prefix is a file path, search its parent directory
			searchDir = filepath.Dir(prefixPath)
		}
	}

	// Handle delimiter for hierarchical listing
	if delimiter != "" {
		// List only immediate children when delimiter is specified
		entries, err := os.ReadDir(searchDir)
		if err != nil {
			if os.IsNotExist(err) {
				return result, nil // Return empty result for non-existent directory
			}
			return nil, err
		}

		prefixesMap := make(map[string]bool)

		for _, entry := range entries {
			fullPath := filepath.Join(searchDir, entry.Name())
			relPath, err := filepath.Rel(s.baseDir, fullPath)
			if err != nil {
				continue
			}

			// Normalize to forward slashes for consistency
			relPath = filepath.ToSlash(relPath)

			// Apply prefix filter
			if prefix != "" && !strings.HasPrefix(relPath, prefix) {
				continue
			}

			if entry.IsDir() {
				// Add directory as delimited prefix
				dirPrefix := relPath + "/"
				if !prefixesMap[dirPrefix] {
					prefixesMap[dirPrefix] = true
					result.DelimitedPrefixes = append(result.DelimitedPrefixes, dirPrefix)
				}
			} else {
				result.Keys = append(result.Keys, relPath)
			}
		}

		return result, nil
	}

	// Recursive listing when no delimiter specified
	err := filepath.WalkDir(searchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if d.IsDir() {
			return nil // Skip directories in recursive listing
		}

		relPath, err := filepath.Rel(s.baseDir, path)
		if err != nil {
			return nil
		}

		// Normalize to forward slashes for consistency
		relPath = filepath.ToSlash(relPath)

		// Apply prefix filter
		if prefix != "" && !strings.HasPrefix(relPath, prefix) {
			return nil
		}

		result.Keys = append(result.Keys, relPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *LocalFileStorage) Delete(ctx context.Context, key string) error {
	path, err := s.fullPath(key)
	if err != nil {
		return err
	}

	err = os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil // Deletion of non-existent file is success
	}

	return err
}
