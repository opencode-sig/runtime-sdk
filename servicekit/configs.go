package servicekit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type configLister interface {
	List(ctx context.Context, prefix string) ([]runtimeconfig.Entry, error)
}

// Configs exposes runtime config-center reads to service initialization code.
//
// It is read-oriented on purpose. Services should load global/service config in
// Init or InitDistributed and keep write operations in platform services such as
// runtimeadmin.
type Configs struct {
	provider runtimeconfig.ConfigProvider
	lister   configLister
}

type configsOptions struct {
	etcd *clientv3.Client
}

type ConfigsOption func(*configsOptions)

// WithConfigsEtcdClient reuses an externally managed etcd client for config reads.
func WithConfigsEtcdClient(client *clientv3.Client) ConfigsOption {
	return func(opts *configsOptions) {
		opts.etcd = client
	}
}

// NewConfigs creates a read-oriented config accessor matching cfg.Runtime.Config.
func NewConfigs(cfg Config, options ...ConfigsOption) (*Configs, error) {
	var opts configsOptions
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Runtime.Config.Provider)) {
	case "file", "":
		provider := runtimeconfig.NewFileProvider(fileConfigRoot(cfg.Runtime.Config.Key))
		return &Configs{provider: provider, lister: provider}, nil
	case "etcd":
		var provider *runtimeconfig.EtcdProvider
		if opts.etcd != nil {
			provider = runtimeconfig.NewEtcdProviderWithClient(opts.etcd, cfg.Runtime.Config.Etcd.Prefix)
		} else {
			provider = runtimeconfig.NewEtcdProvider(cfg.Runtime.Config.Etcd.Endpoints, cfg.Runtime.Config.Etcd.Prefix)
		}
		return &Configs{provider: provider, lister: provider}, nil
	default:
		return nil, fmt.Errorf("unsupported config provider %q", cfg.Runtime.Config.Provider)
	}
}

// Load reads one logical config key.
func (c *Configs) Load(ctx context.Context, key string) ([]byte, error) {
	if c == nil || c.provider == nil {
		return nil, fmt.Errorf("configs accessor is not configured")
	}
	data, err := c.provider.Load(ctx, key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", runtimeconfig.ErrConfigNotFound, key)
		}
		return nil, err
	}
	return data, nil
}

// Decode reads one config key and decodes YAML or JSON into out.
func (c *Configs) Decode(ctx context.Context, key string, out any) error {
	data, err := c.Load(ctx, key)
	if err != nil {
		return err
	}
	if err := runtimeconfig.DecodeInto(data, out); err != nil {
		return fmt.Errorf("decode config %s: %w", key, err)
	}
	return nil
}

// List returns entries under prefix when the backing provider supports listing.
func (c *Configs) List(ctx context.Context, prefix string) ([]runtimeconfig.Entry, error) {
	if c == nil || c.lister == nil {
		return nil, fmt.Errorf("configs lister is not configured")
	}
	return c.lister.List(ctx, prefix)
}

func fileConfigRoot(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || !filepath.IsAbs(key) {
		return "."
	}
	dir := filepath.Dir(filepath.Clean(key))
	if filepath.Base(dir) == "configs" {
		return filepath.Dir(dir)
	}
	return dir
}
