package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencode-sig/runtime-sdk/runtime/defaults"
)

const (
	DefaultBootstrapPath   = "configs/bootstrap.yaml"
	DefaultRuntimeKey      = "configs/runtime.yaml"
	DefaultSourcePolicyKey = "system/config-source.yaml"
	DefaultEtcdEndpoint    = "127.0.0.1:2379"
	envConfigBootstrapPath = "CONFIG_BOOTSTRAP_PATH"
	envConfigRoot          = "CONFIG_ROOT"
	envConfigProvider      = "CONFIG_PROVIDER"
	envConfigPath          = "CONFIG_PATH"
	envConfigKey           = "CONFIG_KEY"
	envConfigEtcdEndpoints = "CONFIG_ETCD_ENDPOINTS"
	envEtcdEndpoints       = "ETCD_ENDPOINTS"
	envConfigEtcdPrefix    = "CONFIG_ETCD_PREFIX"
)

// Source describes where runtime config is loaded from.
type Source struct {
	Provider string     `json:"provider" yaml:"provider"`
	Root     string     `json:"root,omitempty" yaml:"root,omitempty"`
	Key      string     `json:"key" yaml:"key"`
	Etcd     EtcdSource `json:"etcd" yaml:"etcd"`
}

// EtcdSource describes the config-center etcd entrypoint.
type EtcdSource struct {
	Endpoints []string `json:"endpoints" yaml:"endpoints"`
	Prefix    string   `json:"prefix" yaml:"prefix"`
}

// Bootstrap describes the small local config read before the runtime source is known.
type Bootstrap struct {
	Config       Source             `json:"config" yaml:"config"`
	ConfigCenter ConfigCenterSource `json:"config_center" yaml:"config_center"`
}

// ConfigCenterSource configures source-policy lookup.
type ConfigCenterSource struct {
	Prefix          string `json:"prefix" yaml:"prefix"`
	SourcePolicyKey string `json:"source_policy_key" yaml:"source_policy_key"`
	DefaultSource   Source `json:"default_source" yaml:"default_source"`
}

// SourceResolverOptions customizes ResolveSource.
type SourceResolverOptions struct {
	BootstrapPath string
	Env           func(string) string
	NewStore      func(Source, ConfigCenterSource) (Store, io.Closer, error)
}

// ResolveSource resolves the effective runtime config source from bootstrap,
// source policy, and operational environment overrides.
func ResolveSource(ctx context.Context, opts SourceResolverOptions) (Source, string, error) {
	bootstrap, baseDir, err := loadSourceBootstrap(opts)
	if err != nil {
		return Source{}, "", err
	}
	env := sourceEnv(opts.Env)
	center := bootstrap.configCenter()
	source := center.defaultSource()

	if hasSourceEnvOverride(env) {
		source.ApplyEnv(env)
		source.Normalize()
		source.ResolveBaseDir(baseDir)
		return source, baseDir, nil
	}

	newStore := opts.NewStore
	if newStore == nil {
		newStore = defaultSourceStore
	}
	store, closer, err := newStore(source, center)
	if err != nil {
		return Source{}, "", err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	source, err = resolvePolicySource(ctx, store, center.SourcePolicyKey, source)
	if err != nil {
		return Source{}, "", err
	}
	source.ApplyEnv(env)
	source.Normalize()
	source.ResolveBaseDir(baseDir)
	return source, baseDir, nil
}

func defaultSourceStore(source Source, center ConfigCenterSource) (Store, io.Closer, error) {
	store := NewEtcdProvider(source.Etcd.Endpoints, center.Prefix)
	return store, store, nil
}

func loadSourceBootstrap(opts SourceResolverOptions) (Bootstrap, string, error) {
	var bootstrap Bootstrap
	env := sourceEnv(opts.Env)
	baseDir := ""
	path := strings.TrimSpace(opts.BootstrapPath)
	if path == "" {
		path = strings.TrimSpace(firstNonEmpty(env(envConfigBootstrapPath), DefaultBootstrapPath))
	}
	explicit := strings.TrimSpace(opts.BootstrapPath) != "" || strings.TrimSpace(env(envConfigBootstrapPath)) != ""
	resolvedPath, found, err := resolveBootstrapPath(path, explicit, env)
	if err != nil {
		return Bootstrap{}, "", err
	}
	if !found {
		return bootstrap, "", nil
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return Bootstrap{}, "", fmt.Errorf("load bootstrap config %s: %w", resolvedPath, err)
	}
	if err := DecodeInto(data, &bootstrap); err != nil {
		return Bootstrap{}, "", fmt.Errorf("decode bootstrap config %s: %w", resolvedPath, err)
	}
	baseDir = filepath.Dir(resolvedPath)
	return bootstrap, baseDir, nil
}

func resolveBootstrapPath(path string, explicit bool, env func(string) string) (string, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false, nil
	}
	if root := strings.TrimSpace(env(envConfigRoot)); root != "" && !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) && !explicit {
				return "", false, nil
			}
			return "", false, fmt.Errorf("load bootstrap config %s: %w", path, err)
		}
		return path, true, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	for {
		candidate := filepath.Join(dir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true, nil
		} else if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("load bootstrap config %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if explicit {
		return "", false, fmt.Errorf("load bootstrap config %s: %w", path, os.ErrNotExist)
	}
	return "", false, nil
}

func resolvePolicySource(ctx context.Context, store Store, key string, fallback Source) (Source, error) {
	if store == nil {
		return Source{}, fmt.Errorf("config source policy store is required")
	}
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" {
		key = DefaultSourcePolicyKey
	}
	entry, err := store.Get(ctx, key)
	if err == nil {
		return decodePolicySource(entry.Value, fallback)
	}
	if !errors.Is(err, ErrConfigNotFound) {
		return Source{}, fmt.Errorf("get config source policy %s: %w", key, err)
	}
	value, err := encodePolicySource(fallback)
	if err != nil {
		return Source{}, err
	}
	created, err := store.PutIfAbsent(ctx, key, value)
	if err != nil {
		return Source{}, fmt.Errorf("put default config source policy %s: %w", key, err)
	}
	if created {
		return fallback, nil
	}
	entry, err = store.Get(ctx, key)
	if err != nil {
		return Source{}, fmt.Errorf("get config source policy %s after put-if-absent: %w", key, err)
	}
	return decodePolicySource(entry.Value, fallback)
}

func decodePolicySource(data []byte, fallback Source) (Source, error) {
	source, err := Decode[Source](data)
	if err != nil {
		return Source{}, fmt.Errorf("decode config source policy: %w", err)
	}
	source.mergeFallback(fallback)
	source.Normalize()
	return source, nil
}

func encodePolicySource(source Source) ([]byte, error) {
	source.Normalize()
	return []byte(fmt.Sprintf("provider: %s\nkey: %s\n", source.Provider, source.Key)), nil
}

func (b Bootstrap) configCenter() ConfigCenterSource {
	center := b.ConfigCenter
	if strings.TrimSpace(center.Prefix) == "" && !b.Config.isZero() {
		center.Prefix = b.Config.Etcd.Prefix
	}
	if center.DefaultSource.isZero() && !b.Config.isZero() {
		center.DefaultSource = b.Config
	}
	center.normalize()
	return center
}

func (c *ConfigCenterSource) normalize() {
	c.Prefix = defaults.CleanPrefix(c.Prefix, defaults.ConfigPrefix)
	c.SourcePolicyKey = strings.Trim(strings.TrimSpace(c.SourcePolicyKey), "/")
	if c.SourcePolicyKey == "" {
		c.SourcePolicyKey = DefaultSourcePolicyKey
	}
	if c.DefaultSource.isZero() {
		c.DefaultSource = Source{
			Provider: "file",
			Key:      DefaultRuntimeKey,
			Etcd: EtcdSource{
				Endpoints: []string{DefaultEtcdEndpoint},
				Prefix:    c.Prefix,
			},
		}
	}
	c.DefaultSource.Etcd.Prefix = c.Prefix
	if len(c.DefaultSource.Etcd.Endpoints) == 0 {
		c.DefaultSource.Etcd.Endpoints = []string{DefaultEtcdEndpoint}
	}
	c.DefaultSource.Normalize()
}

func (c ConfigCenterSource) defaultSource() Source {
	source := c.DefaultSource
	source.Etcd.Prefix = c.Prefix
	source.Normalize()
	return source
}

// ApplyEnv applies runtime config source environment overrides.
func (s *Source) ApplyEnv(env func(string) string) {
	env = sourceEnv(env)
	s.Provider = firstNonEmpty(env(envConfigProvider), s.Provider)
	if key := env(envConfigKey); key != "" {
		s.Key = key
	} else if !strings.EqualFold(s.Provider, "etcd") {
		s.Key = firstNonEmpty(env(envConfigPath), s.Key)
	}
	if endpoints := splitList(firstNonEmpty(env(envEtcdEndpoints), env(envConfigEtcdEndpoints))); len(endpoints) > 0 {
		s.Etcd.Endpoints = endpoints
	}
	s.Etcd.Prefix = firstNonEmpty(env(envConfigEtcdPrefix), s.Etcd.Prefix)
}

// Normalize applies SDK default values and canonical formatting to Source.
func (s *Source) Normalize() {
	s.Provider = strings.ToLower(strings.TrimSpace(s.Provider))
	if s.Provider == "" {
		s.Provider = "file"
	}
	s.Key = strings.TrimSpace(s.Key)
	switch s.Provider {
	case "file":
		if s.Key == "" {
			s.Key = DefaultRuntimeKey
		}
	case "etcd":
		if s.Key == "" {
			s.Key = DefaultRuntimeKey
		}
		if len(s.Etcd.Endpoints) == 0 {
			s.Etcd.Endpoints = []string{DefaultEtcdEndpoint}
		}
		s.Etcd.Prefix = defaults.CleanPrefix(s.Etcd.Prefix, defaults.ConfigPrefix)
	}
}

// ResolveBaseDir binds a relative file key to the resolved bootstrap directory.
func (s *Source) ResolveBaseDir(baseDir string) {
	if strings.ToLower(strings.TrimSpace(s.Provider)) != "file" {
		return
	}
	key := strings.TrimSpace(s.Key)
	baseDir = strings.TrimSpace(baseDir)
	if key == "" || filepath.IsAbs(key) || baseDir == "" {
		return
	}
	cleanKey := filepath.Clean(key)
	if filepath.Dir(cleanKey) == "." {
		s.Key = filepath.Join(baseDir, cleanKey)
		return
	}
	if firstPathElement(cleanKey) == filepath.Base(baseDir) {
		s.Key = filepath.Join(filepath.Dir(baseDir), cleanKey)
		return
	}
	s.Key = filepath.Join(baseDir, cleanKey)
}

func (s Source) Validate() error {
	s.Normalize()
	switch s.Provider {
	case "file", "etcd":
	default:
		return fmt.Errorf("unsupported config provider %q", s.Provider)
	}
	if s.Key == "" {
		return fmt.Errorf("runtime.config.key is required")
	}
	if s.Provider == "etcd" && len(s.Etcd.Endpoints) == 0 {
		return fmt.Errorf("runtime.config.etcd.endpoints is required")
	}
	return nil
}

func (s Source) isZero() bool {
	return s.Provider == "" &&
		s.Root == "" &&
		s.Key == "" &&
		len(s.Etcd.Endpoints) == 0 &&
		s.Etcd.Prefix == ""
}

func (s *Source) mergeFallback(fallback Source) {
	if s.Provider == "" {
		s.Provider = fallback.Provider
	}
	if s.Key == "" {
		s.Key = fallback.Key
	}
	if len(s.Etcd.Endpoints) == 0 {
		s.Etcd.Endpoints = fallback.Etcd.Endpoints
	}
	if s.Etcd.Prefix == "" {
		s.Etcd.Prefix = fallback.Etcd.Prefix
	}
}

func hasSourceEnvOverride(env func(string) string) bool {
	return env(envConfigProvider) != "" ||
		env(envConfigKey) != "" ||
		env(envConfigPath) != ""
}

func sourceEnv(env func(string) string) func(string) string {
	if env != nil {
		return env
	}
	return os.Getenv
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstPathElement(path string) string {
	path = strings.Trim(filepath.Clean(path), string(os.PathSeparator))
	if path == "" || path == "." {
		return ""
	}
	if index := strings.IndexRune(path, os.PathSeparator); index >= 0 {
		return path[:index]
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
