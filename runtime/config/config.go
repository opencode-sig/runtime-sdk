package config

import "context"

// ConfigProvider abstracts runtime configuration sources.
//
// File providers load local files, and etcd providers load remote config-center values.
type ConfigProvider interface {
	Load(ctx context.Context, key string) ([]byte, error)
}
