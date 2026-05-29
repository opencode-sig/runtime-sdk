package servicekit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/opencode-sig/runtime-sdk/logger"
	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

const (
	defaultConventionRuntimeKey = "configs/runtime.yaml"
	defaultConventionPrefix     = "configs"
)

// ConventionConfigLoaderOptions configures the convention-based config loader.
type ConventionConfigLoaderOptions struct {
	Root                string
	RuntimeKey          string
	ManagedConfigPrefix string
	DisableEtcdAutoSeed bool
}

type conventionRuntimeFragment struct {
	Config   ConfigSourceConfig `json:"config" yaml:"config"`
	Control  ControlConfig      `json:"control" yaml:"control"`
	Metadata MetadataConfig     `json:"metadata" yaml:"metadata"`
}

type serviceFragmentConfig struct {
	ServiceConfig `json:",inline" yaml:",inline"`
	Settings      map[string]any `json:"settings" yaml:"settings"`
}

type conventionConfigLoaderDeps struct {
	newFileProvider func(root string) runtimeconfig.ConfigProvider
	newEtcdProvider func(cfg ConfigSourceConfig) (runtimeconfig.ConfigProvider, io.Closer, bool)
}

// NewConventionConfigLoader creates a loader for split, convention-based config.
//
// It reads configs/runtime.yaml, configs/logger.yaml, configs/registry.yaml,
// configs/infra/*.yaml and configs/service/<service>.yaml, then composes a
// complete servicekit Config for the current service. In etcd mode it reads the
// same logical keys from the configured config center and seeds missing keys
// with local files using PutIfAbsent.
func NewConventionConfigLoader(opts ConventionConfigLoaderOptions) ConfigLoader {
	return newConventionConfigLoaderWithDeps(opts, defaultConventionConfigLoaderDeps())
}

func defaultConventionConfigLoaderDeps() conventionConfigLoaderDeps {
	return conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return runtimeconfig.NewFileProvider(root)
		},
		newEtcdProvider: func(cfg ConfigSourceConfig) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			if !strings.EqualFold(strings.TrimSpace(cfg.Provider), "etcd") && !hasEtcdConfig(cfg.Etcd) {
				return nil, nil, false
			}
			provider := runtimeconfig.NewEtcdProvider(cfg.Etcd.Endpoints, cfg.Etcd.Prefix)
			return provider, provider, true
		},
	}
}

func newConventionConfigLoaderWithDeps(opts ConventionConfigLoaderOptions, deps conventionConfigLoaderDeps) ConfigLoader {
	return func(ctx context.Context, service string) (Config, error) {
		return loadConventionConfigWithDeps(ctx, service, opts, deps)
	}
}

func loadConventionConfigWithDeps(ctx context.Context, service string, opts ConventionConfigLoaderOptions, deps conventionConfigLoaderDeps) (Config, error) {
	if deps.newFileProvider == nil {
		deps.newFileProvider = defaultConventionConfigLoaderDeps().newFileProvider
	}
	if deps.newEtcdProvider == nil {
		deps.newEtcdProvider = defaultConventionConfigLoaderDeps().newEtcdProvider
	}

	service = strings.TrimSpace(service)
	if service == "" {
		return Config{}, fmt.Errorf("service name is required")
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	managedPrefix := conventionManagedPrefix(opts.ManagedConfigPrefix)
	runtimeKey := conventionRuntimeKey(opts.RuntimeKey, managedPrefix)

	fileProvider := deps.newFileProvider(root)
	runtimeFragment, err := loadRequiredConventionFragment[conventionRuntimeFragment](ctx, fileProvider, runtimeKey)
	if err != nil {
		return Config{}, err
	}
	normalizeConventionRuntime(&runtimeFragment, root, runtimeKey)
	if err := validateConventionRuntime(runtimeFragment, runtimeKey, managedPrefix); err != nil {
		return Config{}, err
	}

	providerName := strings.ToLower(strings.TrimSpace(runtimeFragment.Config.Provider))
	if providerName == "" || providerName == "file" {
		return composeConventionConfig(ctx, service, fileProvider, root, runtimeFragment, managedPrefix, runtimeKey)
	}

	etcdProvider, closer, ok := deps.newEtcdProvider(runtimeFragment.Config)
	if !ok {
		return Config{}, fmt.Errorf("configure etcd provider for %s: etcd config is required", runtimeKey)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	managedProvider := newConventionEtcdProvider(etcdProvider, fileProvider, opts.DisableEtcdAutoSeed)
	managedRuntime, err := loadRequiredConventionFragment[conventionRuntimeFragment](ctx, managedProvider, runtimeKey)
	if err != nil {
		return Config{}, err
	}
	normalizeConventionRuntime(&managedRuntime, root, runtimeKey)
	if err := validateConventionRuntime(managedRuntime, runtimeKey, managedPrefix); err != nil {
		return Config{}, err
	}
	return composeConventionConfig(ctx, service, managedProvider, root, managedRuntime, managedPrefix, runtimeKey)
}

func composeConventionConfig(ctx context.Context, service string, provider runtimeconfig.ConfigProvider, root string, runtimeFragment conventionRuntimeFragment, managedPrefix string, runtimeKey string) (Config, error) {
	cfg := defaultConventionConfig(root, runtimeFragment)

	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "logger.yaml"), &cfg.Logger); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "registry.yaml"), &cfg.Registry); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "infra", "etcd.yaml"), &cfg.Infra.Etcd); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "infra", "mysql.yaml"), &cfg.Infra.MySQL); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "infra", "redis.yaml"), &cfg.Infra.Redis); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "infra", "kafka.yaml"), &cfg.Infra.Kafka); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "infra", "elastic.yaml"), &cfg.Infra.Elastic); err != nil {
		return Config{}, err
	}
	if err := loadOptionalConventionFragment(ctx, provider, path.Join(managedPrefix, "infra", "minio.yaml"), &cfg.Infra.MinIO); err != nil {
		return Config{}, err
	}

	serviceKey, err := defaultConventionServiceKey(service, managedPrefix)
	if err != nil {
		return Config{}, err
	}
	fragment, err := loadRequiredConventionFragment[serviceFragmentConfig](ctx, provider, serviceKey)
	if err != nil {
		return Config{}, err
	}
	if err := mergeServiceFragment(&cfg, service, serviceKey, fragment); err != nil {
		return Config{}, err
	}
	if err := validateConventionConfig(cfg, serviceKey, runtimeKey, managedPrefix); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultConventionConfig(root string, runtimeFragment conventionRuntimeFragment) Config {
	cfg := Config{
		Logger: loggerDefaultConfig(),
		Runtime: RuntimeConfig{
			Config:  runtimeFragment.Config,
			Control: runtimeFragment.Control,
		},
		Metadata: runtimeFragment.Metadata,
		Settings: map[string]any{},
	}
	if cfg.Runtime.Config.Root == "" {
		cfg.Runtime.Config.Root = root
	}
	if cfg.Runtime.Config.Provider == "" {
		cfg.Runtime.Config.Provider = "file"
	}
	return cfg
}

func loggerDefaultConfig() logger.Config {
	return logger.Config{
		Level:        "info",
		Format:       "json",
		EnableStdout: true,
		Caller:       true,
	}
}

func normalizeConventionRuntime(fragment *conventionRuntimeFragment, root string, runtimeKey string) {
	if fragment.Config.Provider == "" {
		fragment.Config.Provider = "file"
	}
	if fragment.Config.Root == "" {
		fragment.Config.Root = root
	}
	if fragment.Config.Key == "" {
		fragment.Config.Key = runtimeKey
	}
}

func validateConventionRuntime(fragment conventionRuntimeFragment, runtimeKey string, managedPrefix string) error {
	provider := strings.ToLower(strings.TrimSpace(fragment.Config.Provider))
	switch provider {
	case "", "file":
	case "etcd":
		if len(fragment.Config.Etcd.Endpoints) == 0 {
			return fmt.Errorf("validate %s: runtime.config.etcd.endpoints is required", runtimeKey)
		}
		if strings.TrimSpace(fragment.Config.Etcd.Prefix) == "" {
			return fmt.Errorf("validate %s: runtime.config.etcd.prefix is required", runtimeKey)
		}
	default:
		return fmt.Errorf("validate %s: unsupported runtime.config.provider %q", runtimeKey, fragment.Config.Provider)
	}
	if !logicalKeyHasPrefix(fragment.Config.Key, managedPrefix) {
		return fmt.Errorf("validate %s: runtime.config.key %q must be under %q", runtimeKey, fragment.Config.Key, managedPrefix)
	}
	return nil
}

func validateConventionConfig(cfg Config, serviceKey string, runtimeKey string, managedPrefix string) error {
	if strings.TrimSpace(cfg.Service.GRPCAddr) == "" {
		return fmt.Errorf("validate %s: grpc_addr is required", serviceKey)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Registry.Provider), "etcd") {
		if len(cfg.Registry.Etcd.Endpoints) == 0 {
			return fmt.Errorf("validate %s: registry.etcd.endpoints is required", path.Join(managedPrefix, "registry.yaml"))
		}
		if strings.TrimSpace(cfg.Registry.Etcd.Prefix) == "" {
			return fmt.Errorf("validate %s: registry.etcd.prefix is required", path.Join(managedPrefix, "registry.yaml"))
		}
	}
	if strings.TrimSpace(cfg.Metadata.RoutesPrefix) == "" {
		return fmt.Errorf("validate %s: metadata.routes_prefix is required", runtimeKey)
	}
	if strings.TrimSpace(cfg.Metadata.DescriptorsPrefix) == "" {
		return fmt.Errorf("validate %s: metadata.descriptors_prefix is required", runtimeKey)
	}
	if err := cfg.Infra.Etcd.Validate(); err != nil {
		return fmt.Errorf("validate %s: %w", path.Join(managedPrefix, "infra", "etcd.yaml"), err)
	}
	if err := cfg.Infra.MySQL.Validate(); err != nil {
		return fmt.Errorf("validate %s: %w", path.Join(managedPrefix, "infra", "mysql.yaml"), err)
	}
	if err := cfg.Infra.Redis.Validate(); err != nil {
		return fmt.Errorf("validate %s: %w", path.Join(managedPrefix, "infra", "redis.yaml"), err)
	}
	if err := cfg.Infra.Kafka.Validate(); err != nil {
		return fmt.Errorf("validate %s: %w", path.Join(managedPrefix, "infra", "kafka.yaml"), err)
	}
	if err := cfg.Infra.Elastic.Validate(); err != nil {
		return fmt.Errorf("validate %s: %w", path.Join(managedPrefix, "infra", "elastic.yaml"), err)
	}
	if err := cfg.Infra.MinIO.Validate(); err != nil {
		return fmt.Errorf("validate %s: %w", path.Join(managedPrefix, "infra", "minio.yaml"), err)
	}
	return nil
}

func mergeServiceFragment(cfg *Config, service string, key string, fragment serviceFragmentConfig) error {
	if fragment.Name != "" && fragment.Name != service {
		return fmt.Errorf("validate %s: service name %q does not match %q", key, fragment.Name, service)
	}
	cfg.Service = fragment.ServiceConfig
	cfg.Service.Name = service
	cfg.Settings = fragment.Settings
	if cfg.Settings == nil {
		cfg.Settings = map[string]any{}
	}
	return nil
}

func loadRequiredConventionFragment[T any](ctx context.Context, provider runtimeconfig.ConfigProvider, key string) (T, error) {
	var out T
	data, err := provider.Load(ctx, key)
	if err != nil {
		return out, fmt.Errorf("load %s: %w", key, err)
	}
	if err := runtimeconfig.DecodeInto(data, &out); err != nil {
		return out, fmt.Errorf("decode %s: %w", key, err)
	}
	return out, nil
}

func loadOptionalConventionFragment(ctx context.Context, provider runtimeconfig.ConfigProvider, key string, out any) error {
	data, err := provider.Load(ctx, key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, runtimeconfig.ErrConfigNotFound) {
			return nil
		}
		return fmt.Errorf("load %s: %w", key, err)
	}
	if err := runtimeconfig.DecodeInto(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", key, err)
	}
	return nil
}

func defaultConventionServiceKey(service string, prefix string) (string, error) {
	service = strings.Trim(strings.TrimSpace(service), "/")
	if service == "" {
		return "", fmt.Errorf("service name is required")
	}
	return path.Join(prefix, "service", service+".yaml"), nil
}

func conventionManagedPrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return defaultConventionPrefix
	}
	return prefix
}

func conventionRuntimeKey(runtimeKey string, prefix string) string {
	runtimeKey = strings.Trim(strings.TrimSpace(runtimeKey), "/")
	if runtimeKey != "" {
		return runtimeKey
	}
	if strings.Trim(strings.TrimSpace(prefix), "/") == defaultConventionPrefix {
		return defaultConventionRuntimeKey
	}
	return path.Join(prefix, "runtime.yaml")
}

type conventionEtcdProvider struct {
	remote          runtimeconfig.ConfigProvider
	local           runtimeconfig.ConfigProvider
	disableAutoSeed bool
}

func newConventionEtcdProvider(remote runtimeconfig.ConfigProvider, local runtimeconfig.ConfigProvider, disableAutoSeed bool) runtimeconfig.ConfigProvider {
	return conventionEtcdProvider{remote: remote, local: local, disableAutoSeed: disableAutoSeed}
}

func (p conventionEtcdProvider) Load(ctx context.Context, key string) ([]byte, error) {
	data, err := p.remote.Load(ctx, key)
	if err == nil {
		return data, nil
	}
	if !errors.Is(err, runtimeconfig.ErrConfigNotFound) || p.disableAutoSeed {
		return nil, fmt.Errorf("load etcd %s: %w", key, err)
	}
	localData, localErr := p.local.Load(ctx, key)
	if localErr != nil {
		if errors.Is(localErr, os.ErrNotExist) {
			return nil, fmt.Errorf("load etcd %s: %w", key, err)
		}
		return nil, fmt.Errorf("load local seed %s: %w", key, localErr)
	}
	seeder, ok := p.remote.(configSeeder)
	if !ok {
		return nil, fmt.Errorf("seed etcd %s: provider does not support PutIfAbsent", key)
	}
	if _, seedErr := seeder.PutIfAbsent(ctx, key, localData); seedErr != nil {
		return nil, fmt.Errorf("seed etcd %s: %w", key, seedErr)
	}
	data, err = p.remote.Load(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load seeded etcd %s: %w", key, err)
	}
	return data, nil
}
