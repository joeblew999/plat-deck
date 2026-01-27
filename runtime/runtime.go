// Package runtime provides a unified interface for different WASM runtimes
package runtime

import (
	"context"
	"io"
)

// Storage abstracts file storage (R2, local filesystem, etc.)
type Storage interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, data []byte, contentType string) error
	List(ctx context.Context, prefix string, delimiter string) (*ListResult, error)
	Delete(ctx context.Context, key string) error
}

// FilesystemStorage is an optional interface for storage backends that map to local filesystems
// This allows native pipelines to get actual filesystem paths for workDir support
type FilesystemStorage interface {
	Storage
	FullPath(key string) (string, error)
}

// ListResult holds storage listing results
type ListResult struct {
	Keys              []string
	DelimitedPrefixes []string
}

// KVStore abstracts key-value storage
type KVStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
}

// Publisher abstracts event publishing (NATS, etc.)
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// Runtime holds all platform-specific dependencies
type Runtime struct {
	InputStorage  Storage
	OutputStorage Storage
	KV            KVStore
	Publisher     Publisher
}

// Global runtime instance - set by platform-specific init
var Current *Runtime

// SetRuntime sets the global runtime
func SetRuntime(r *Runtime) {
	Current = r
}

// Input returns the input storage
func Input() Storage {
	if Current == nil || Current.InputStorage == nil {
		return &noopStorage{}
	}
	return Current.InputStorage
}

// Output returns the output storage
func Output() Storage {
	if Current == nil || Current.OutputStorage == nil {
		return &noopStorage{}
	}
	return Current.OutputStorage
}

// KV returns the KV store
func KV() KVStore {
	if Current == nil || Current.KV == nil {
		return &noopKV{}
	}
	return Current.KV
}

// noopStorage is a no-op implementation for when storage isn't configured
type noopStorage struct{}

func (s *noopStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, io.EOF
}

func (s *noopStorage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	return nil
}

func (s *noopStorage) List(ctx context.Context, prefix string, delimiter string) (*ListResult, error) {
	return &ListResult{}, nil
}

func (s *noopStorage) Delete(ctx context.Context, key string) error {
	return nil
}

// noopKV is a no-op implementation for when KV isn't configured
type noopKV struct{}

func (k *noopKV) Get(ctx context.Context, key string) ([]byte, error) {
	return nil, nil
}

func (k *noopKV) Put(ctx context.Context, key string, value []byte) error {
	return nil
}

func (k *noopKV) Delete(ctx context.Context, key string) error {
	return nil
}
