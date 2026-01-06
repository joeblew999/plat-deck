//go:build cloudflare

package runtime

import (
	"context"

	"github.com/syumai/workers/cloudflare/kv"
)

// CloudflareKV implements KVStore using Cloudflare KV
type CloudflareKV struct {
	namespace *kv.Namespace
}

// NewCloudflareKV creates a new Cloudflare KV store
func NewCloudflareKV(binding string) (*CloudflareKV, error) {
	ns, err := kv.NewNamespace(binding)
	if err != nil {
		return nil, err
	}
	return &CloudflareKV{namespace: ns}, nil
}

func (k *CloudflareKV) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := k.namespace.GetString(key, nil)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func (k *CloudflareKV) Put(ctx context.Context, key string, value []byte) error {
	return k.namespace.PutString(key, string(value), nil)
}

func (k *CloudflareKV) Delete(ctx context.Context, key string) error {
	return k.namespace.Delete(key)
}
