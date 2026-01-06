//go:build !cloudflare

package runtime

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// R2HTTPStorage implements Storage using R2's S3-compatible HTTP API
// Works from wazero, browser, or any environment with HTTP access
type R2HTTPStorage struct {
	endpoint    string // e.g., https://<account>.r2.cloudflarestorage.com
	bucketName  string
	accessKeyID string
	secretKey   string
	httpClient  *http.Client
}

// R2HTTPConfig holds configuration for R2 HTTP storage
type R2HTTPConfig struct {
	Endpoint    string
	BucketName  string
	AccessKeyID string
	SecretKey   string
}

// NewR2HTTPStorage creates storage that accesses R2 via HTTP/S3 API
func NewR2HTTPStorage(cfg R2HTTPConfig) *R2HTTPStorage {
	return &R2HTTPStorage{
		endpoint:    strings.TrimSuffix(cfg.Endpoint, "/"),
		bucketName:  cfg.BucketName,
		accessKeyID: cfg.AccessKeyID,
		secretKey:   cfg.SecretKey,
		httpClient:  &http.Client{},
	}
}

func (s *R2HTTPStorage) url(key string) string {
	return fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucketName, key)
}

func (s *R2HTTPStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.url(key), nil)
	if err != nil {
		return nil, err
	}

	s.signRequest(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, io.EOF
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("R2 GET failed: %s", resp.Status)
	}

	return resp.Body, nil
}

func (s *R2HTTPStorage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", s.url(key), bytes.NewReader(data))
	if err != nil {
		return err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.ContentLength = int64(len(data))

	s.signRequest(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("R2 PUT failed: %s", resp.Status)
	}

	return nil
}

func (s *R2HTTPStorage) List(ctx context.Context, prefix string, delimiter string) (*ListResult, error) {
	url := fmt.Sprintf("%s/%s?list-type=2", s.endpoint, s.bucketName)
	if prefix != "" {
		url += "&prefix=" + prefix
	}
	if delimiter != "" {
		url += "&delimiter=" + delimiter
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	s.signRequest(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("R2 LIST failed: %s", resp.Status)
	}

	// Parse S3 ListObjectsV2 response
	var listResp struct {
		Contents []struct {
			Key string `xml:"Key"`
		} `xml:"Contents"`
		CommonPrefixes []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, err
	}

	result := &ListResult{
		Keys:              make([]string, len(listResp.Contents)),
		DelimitedPrefixes: make([]string, len(listResp.CommonPrefixes)),
	}

	for i, c := range listResp.Contents {
		result.Keys[i] = c.Key
	}
	for i, p := range listResp.CommonPrefixes {
		result.DelimitedPrefixes[i] = p.Prefix
	}

	return result, nil
}

func (s *R2HTTPStorage) Delete(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", s.url(key), nil)
	if err != nil {
		return err
	}

	s.signRequest(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("R2 DELETE failed: %s", resp.Status)
	}

	return nil
}

// signRequest adds AWS Signature V4 authentication
// Simplified version - for production use github.com/aws/aws-sdk-go-v2
func (s *R2HTTPStorage) signRequest(req *http.Request) {
	// For now, if credentials are provided, use basic auth header approach
	// In production, implement proper AWS Sig V4 or use presigned URLs
	if s.accessKeyID != "" && s.secretKey != "" {
		// R2 also supports Authorization header with access key
		// This is a simplified approach - real implementation needs AWS Sig V4
		req.SetBasicAuth(s.accessKeyID, s.secretKey)
	}
}

// PublicR2Storage accesses public R2 buckets (no auth required)
type PublicR2Storage struct {
	publicURL string // e.g., https://pub-xxx.r2.dev
}

// NewPublicR2Storage creates storage for public R2 buckets
func NewPublicR2Storage(publicURL string) *PublicR2Storage {
	return &PublicR2Storage{
		publicURL: strings.TrimSuffix(publicURL, "/"),
	}
}

func (s *PublicR2Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/%s", s.publicURL, key)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, io.EOF
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET failed: %s", resp.Status)
	}

	return resp.Body, nil
}

func (s *PublicR2Storage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	return fmt.Errorf("public R2 storage is read-only")
}

func (s *PublicR2Storage) List(ctx context.Context, prefix string, delimiter string) (*ListResult, error) {
	return nil, fmt.Errorf("public R2 storage does not support listing")
}

func (s *PublicR2Storage) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("public R2 storage is read-only")
}
