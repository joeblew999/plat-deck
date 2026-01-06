//go:build cloudflare

package runtime

import (
	"bytes"
	"context"
	"io"

	"github.com/syumai/workers/cloudflare/r2"
)

// R2Storage implements Storage using Cloudflare R2
type R2Storage struct {
	bucketName string
	bucket     *r2.Bucket
}

// NewR2Storage creates a new R2 storage
func NewR2Storage(bucketBinding string) (*R2Storage, error) {
	bucket, err := r2.NewBucket(bucketBinding)
	if err != nil {
		return nil, err
	}
	return &R2Storage{
		bucketName: bucketBinding,
		bucket:     bucket,
	}, nil
}

func (s *R2Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.bucket.Get(key)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, io.EOF
	}
	// Wrap the io.Reader in a NopCloser to satisfy io.ReadCloser interface
	return io.NopCloser(obj.Body), nil
}

func (s *R2Storage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	opts := &r2.PutOptions{}
	if contentType != "" {
		opts.HTTPMetadata = r2.HTTPMetadata{
			ContentType: contentType,
		}
	}
	// Create a ReadCloser from the byte slice
	reader := io.NopCloser(bytes.NewReader(data))
	_, err := s.bucket.Put(key, reader, opts)
	return err
}

func (s *R2Storage) List(ctx context.Context, prefix string, delimiter string) (*ListResult, error) {
	// Note: The simple List() doesn't support prefix/delimiter filtering
	// TODO: Use ListWithOptions if available in newer versions
	result, err := s.bucket.List()
	if err != nil {
		return nil, err
	}

	lr := &ListResult{
		Keys:              make([]string, 0, len(result.Objects)),
		DelimitedPrefixes: result.DelimitedPrefixes,
	}
	for _, obj := range result.Objects {
		// Filter by prefix if specified
		if prefix == "" || len(obj.Key) >= len(prefix) && obj.Key[:len(prefix)] == prefix {
			lr.Keys = append(lr.Keys, obj.Key)
		}
	}
	return lr, nil
}

func (s *R2Storage) Delete(ctx context.Context, key string) error {
	return s.bucket.Delete(key)
}
