package servicekit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
	"gopkg.in/yaml.v3"
)

const defaultManagedConfigPrefix = "configs/service"

// ConfigLoaderOptions configures the standard bootstrap/managed config loader.
type ConfigLoaderOptions struct {
	Root                string
	Key                 string
	ManagedConfigPrefix string
	DisableEtcdAutoSeed bool
}

type configLoaderDeps struct {
	newFileProvider func(root string) runtimeconfig.ConfigProvider
	newEtcdProvider func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool)
}

type configSeeder interface {
	PutIfAbsent(ctx context.Context, key string, value []byte) (created bool, err error)
}

// NewConfigLoader creates a standard servicekit config loader.
//
// The loader first reads a local bootstrap config from Root/Key. File-mode
// configs are returned directly. Etcd-mode configs are used to load the managed
// config from the configured config center. When the etcd config is missing,
// the loader seeds it with the local complete service config by using
// PutIfAbsent, then reads the managed config from etcd.
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
		return loadManagedConfigWithDeps(ctx, service, opts, deps)
	}
}

func loadManagedConfigWithDeps(ctx context.Context, service string, opts ConfigLoaderOptions, deps configLoaderDeps) (Config, error) {
	if deps.newFileProvider == nil {
		deps.newFileProvider = defaultConfigLoaderDeps().newFileProvider
	}
	if deps.newEtcdProvider == nil {
		deps.newEtcdProvider = defaultConfigLoaderDeps().newEtcdProvider
	}

	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	managedPrefix := managedConfigPrefix(opts.ManagedConfigPrefix)
	key := strings.TrimSpace(opts.Key)
	if key == "" {
		var err error
		key, err = defaultManagedConfigKey(service, managedPrefix)
		if err != nil {
			return Config{}, err
		}
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
	managedKey := strings.TrimSpace(cfg.Runtime.Config.Key)
	if managedKey == "" {
		managedKey = key
		cfg.Runtime.Config.Key = managedKey
	}

	etcdProvider, closer, ok := deps.newEtcdProvider(cfg)
	if !ok {
		return cfg, nil
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	data, err = etcdProvider.Load(ctx, managedKey)
	if err != nil {
		if !errors.Is(err, runtimeconfig.ErrConfigNotFound) || opts.DisableEtcdAutoSeed {
			return Config{}, fmt.Errorf("load etcd config: %w", err)
		}
		if err := seedEtcdConfig(ctx, etcdProvider, service, managedKey, managedPrefix, cfg); err != nil {
			return Config{}, fmt.Errorf("seed etcd config: %w", err)
		}
		data, err = etcdProvider.Load(ctx, managedKey)
		if err != nil {
			return Config{}, fmt.Errorf("load seeded etcd config: %w", err)
		}
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

func seedEtcdConfig(ctx context.Context, provider runtimeconfig.ConfigProvider, service string, key string, prefix string, cfg Config) error {
	if err := validateSeedConfig(service, key, prefix, cfg); err != nil {
		return err
	}
	seeder, ok := provider.(configSeeder)
	if !ok {
		return fmt.Errorf("etcd config provider does not support PutIfAbsent")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode local service config: %w", err)
	}
	if _, err := seeder.PutIfAbsent(ctx, key, data); err != nil {
		return err
	}
	return nil
}

func validateSeedConfig(service string, key string, prefix string, cfg Config) error {
	service = strings.TrimSpace(service)
	if service == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(cfg.Service.Name) == "" {
		return fmt.Errorf("local config service.name is required")
	}
	if cfg.Service.Name != service {
		return fmt.Errorf("local config service.name %q does not match service %q", cfg.Service.Name, service)
	}
	if strings.TrimSpace(cfg.Service.GRPCAddr) == "" {
		return fmt.Errorf("local config service.grpc_addr is required")
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Runtime.Config.Provider), "etcd") {
		return fmt.Errorf("local config runtime.config.provider must be etcd")
	}
	if len(cfg.Runtime.Config.Etcd.Endpoints) == 0 {
		return fmt.Errorf("local config runtime.config.etcd.endpoints is required")
	}
	if strings.TrimSpace(cfg.Runtime.Config.Etcd.Prefix) == "" {
		return fmt.Errorf("local config runtime.config.etcd.prefix is required")
	}
	if !logicalKeyHasPrefix(key, prefix) {
		return fmt.Errorf("managed config key %q must be under %q", key, prefix)
	}
	return nil
}

func defaultManagedConfigKey(service string, prefix string) (string, error) {
	service = strings.Trim(strings.TrimSpace(service), "/")
	if service == "" {
		return "", fmt.Errorf("service name is required for default config key")
	}
	return path.Join(prefix, service+".yaml"), nil
}

func managedConfigPrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return defaultManagedConfigPrefix
	}
	return prefix
}

func logicalKeyHasPrefix(key string, prefix string) bool {
	key = strings.Trim(strings.TrimSpace(key), "/")
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	return key == prefix || strings.HasPrefix(key, prefix+"/")
}
