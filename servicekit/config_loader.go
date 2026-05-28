package servicekit

import (
	"context"
	"fmt"
	"io"
	"strings"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

// ConfigLoaderOptions configures the standard bootstrap/managed config loader.
type ConfigLoaderOptions struct {
	Root string
	Key  string
}

type configLoaderDeps struct {
	newFileProvider func(root string) runtimeconfig.ConfigProvider
	newEtcdProvider func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool)
}

// NewConfigLoader creates a standard servicekit config loader.
//
// The loader first reads a local bootstrap config from Root/Key. File-mode
// configs are returned directly. Etcd-mode configs are used to load the managed
// config from the configured config center.
func NewConfigLoader(opts ConfigLoaderOptions) ConfigLoader {
	return newConfigLoaderWithDeps(opts, defaultConfigLoaderDeps())
}

// ManagedConfigLoader returns the standard bootstrap/managed config loader.
func ManagedConfigLoader(root string, key string) ConfigLoader {
	return NewConfigLoader(ConfigLoaderOptions{
		Root: root,
		Key:  key,
	})
}

func defaultConfigLoaderDeps() configLoaderDeps {
	return configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return runtimeconfig.NewFileProvider(root)
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			provider, ok := cfg.EtcdConfigStore()
			if !ok {
				return nil, nil, false
			}
			return provider, provider, true
		},
	}
}

func newConfigLoaderWithDeps(opts ConfigLoaderOptions, deps configLoaderDeps) ConfigLoader {
	return func(ctx context.Context, service string) (Config, error) {
		return loadManagedConfigWithDeps(ctx, opts.Root, opts.Key, deps)
	}
}

func loadManagedConfigWithDeps(ctx context.Context, root string, key string, deps configLoaderDeps) (Config, error) {
	if deps.newFileProvider == nil {
		deps.newFileProvider = defaultConfigLoaderDeps().newFileProvider
	}
	if deps.newEtcdProvider == nil {
		deps.newEtcdProvider = defaultConfigLoaderDeps().newEtcdProvider
	}

	fileProvider := deps.newFileProvider(root)
	data, err := fileProvider.Load(ctx, key)
	if err != nil {
		return Config{}, fmt.Errorf("load bootstrap config: %w", err)
	}

	cfg, err := runtimeconfig.Decode[Config](data)
	if err != nil {
		return Config{}, fmt.Errorf("decode bootstrap config: %w", err)
	}
	if cfg.Runtime.Config.Root == "" {
		cfg.Runtime.Config.Root = root
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Runtime.Config.Provider), "etcd") {
		return cfg, nil
	}

	etcdProvider, closer, ok := deps.newEtcdProvider(cfg)
	if !ok {
		return cfg, nil
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	data, err = etcdProvider.Load(ctx, cfg.Runtime.Config.Key)
	if err != nil {
		return Config{}, fmt.Errorf("load etcd config: %w", err)
	}
	managed, err := runtimeconfig.Decode[Config](data)
	if err != nil {
		return Config{}, fmt.Errorf("decode etcd config: %w", err)
	}

	if managed.Runtime.Config.Root == "" &&
		(managed.Runtime.Config.Provider == "" ||
			strings.EqualFold(strings.TrimSpace(managed.Runtime.Config.Provider), "file")) {
		managed.Runtime.Config.Root = root
	}
	return managed, nil
}
