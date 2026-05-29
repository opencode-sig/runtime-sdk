package config

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestResolveSourceUsesEnvOverrideWithoutPolicyLookup(t *testing.T) {
	dir := t.TempDir()
	bootstrapPath := filepath.Join(dir, "bootstrap.yaml")
	if err := os.WriteFile(bootstrapPath, []byte(`
config_center:
  prefix: /runtime/config
  source_policy_key: system/config-source.yaml
  default_source:
    provider: etcd
    key: configs/runtime.yaml
    etcd:
      endpoints:
        - 10.0.0.10:2379
      prefix: /runtime/config
`), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	env := mapEnv(map[string]string{
		envConfigBootstrapPath: bootstrapPath,
		envConfigProvider:      "etcd",
		envConfigKey:           "configs/runtime.yaml",
	})

	source, _, err := ResolveSource(context.Background(), SourceResolverOptions{
		Env: env,
		NewStore: func(Source, ConfigCenterSource) (Store, io.Closer, error) {
			t.Fatal("store should not be opened when env override is present")
			return nil, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if source.Provider != "etcd" {
		t.Fatalf("provider = %q", source.Provider)
	}
	if len(source.Etcd.Endpoints) != 1 || source.Etcd.Endpoints[0] != "10.0.0.10:2379" {
		t.Fatalf("endpoints = %#v", source.Etcd.Endpoints)
	}
	if source.Etcd.Prefix != "/runtime/config" {
		t.Fatalf("prefix = %q", source.Etcd.Prefix)
	}
}

func TestResolveSourceSearchesParentDirs(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "configs")
	workDir := filepath.Join(root, "cmd", "service")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "bootstrap.yaml"), []byte(`
config_center:
  prefix: /runtime/config
  default_source:
    provider: file
    key: runtime.yaml
`), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	t.Chdir(workDir)

	source, baseDir, err := ResolveSource(context.Background(), SourceResolverOptions{
		Env: mapEnv(map[string]string{
			envConfigProvider: "file",
		}),
	})
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if baseDir != configDir {
		t.Fatalf("base dir = %q, want %q", baseDir, configDir)
	}
	if source.Key != filepath.Join(configDir, "runtime.yaml") {
		t.Fatalf("key = %q", source.Key)
	}
}

func TestResolveSourceSeedsDefaultPolicyWhenMissing(t *testing.T) {
	store := newFakeSourceStore()
	dir := t.TempDir()
	bootstrapPath := filepath.Join(dir, "bootstrap.yaml")
	if err := os.WriteFile(bootstrapPath, []byte(`
config_center:
  prefix: /runtime/config
  source_policy_key: system/config-source.yaml
  default_source:
    provider: file
    key: configs/runtime.yaml
    etcd:
      endpoints:
        - 10.0.0.10:2379
      prefix: /runtime/config
`), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}

	source, _, err := ResolveSource(context.Background(), SourceResolverOptions{
		Env: mapEnv(map[string]string{envConfigBootstrapPath: bootstrapPath}),
		NewStore: func(Source, ConfigCenterSource) (Store, io.Closer, error) {
			return store, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if source.Provider != "file" || source.Key != filepath.Join(dir, "configs", "runtime.yaml") {
		t.Fatalf("source = %#v", source)
	}
	entry, err := store.Get(context.Background(), "system/config-source.yaml")
	if err != nil {
		t.Fatalf("get seeded policy: %v", err)
	}
	if string(entry.Value) != "provider: file\nkey: configs/runtime.yaml\n" {
		t.Fatalf("seeded policy = %q", entry.Value)
	}
}

func TestResolveSourceUsesExistingPolicyWithFallback(t *testing.T) {
	store := newFakeSourceStore()
	if err := store.Put(context.Background(), "system/config-source.yaml", []byte(`
provider: etcd
key: configs/runtime.yaml
`)); err != nil {
		t.Fatalf("put policy: %v", err)
	}
	dir := t.TempDir()
	bootstrapPath := filepath.Join(dir, "bootstrap.yaml")
	if err := os.WriteFile(bootstrapPath, []byte(`
config_center:
  prefix: /runtime/config
  source_policy_key: system/config-source.yaml
  default_source:
    provider: file
    key: configs/runtime.yaml
    etcd:
      endpoints:
        - 10.0.0.10:2379
      prefix: /runtime/config
`), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}

	source, _, err := ResolveSource(context.Background(), SourceResolverOptions{
		Env: mapEnv(map[string]string{envConfigBootstrapPath: bootstrapPath}),
		NewStore: func(Source, ConfigCenterSource) (Store, io.Closer, error) {
			return store, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if source.Provider != "etcd" {
		t.Fatalf("provider = %q", source.Provider)
	}
	if len(source.Etcd.Endpoints) != 1 || source.Etcd.Endpoints[0] != "10.0.0.10:2379" {
		t.Fatalf("endpoints = %#v", source.Etcd.Endpoints)
	}
}

func TestResolveSourceHandlesPutIfAbsentRace(t *testing.T) {
	store := newFakeSourceStore()
	store.putIfAbsentHook = func() {
		store.values["system/config-source.yaml"] = Entry{
			Key:   "system/config-source.yaml",
			Value: []byte("provider: etcd\nkey: configs/runtime.yaml\n"),
		}
	}
	dir := t.TempDir()
	bootstrapPath := filepath.Join(dir, "bootstrap.yaml")
	if err := os.WriteFile(bootstrapPath, []byte(`
config_center:
  prefix: /runtime/config
  source_policy_key: system/config-source.yaml
  default_source:
    provider: file
    key: configs/runtime.yaml
    etcd:
      endpoints:
        - 10.0.0.10:2379
      prefix: /runtime/config
`), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}

	source, _, err := ResolveSource(context.Background(), SourceResolverOptions{
		Env: mapEnv(map[string]string{envConfigBootstrapPath: bootstrapPath}),
		NewStore: func(Source, ConfigCenterSource) (Store, io.Closer, error) {
			return store, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if source.Provider != "etcd" {
		t.Fatalf("provider = %q", source.Provider)
	}
}

func TestSourceNormalizationPublicAPI(t *testing.T) {
	source := Source{
		Provider: " ETCD ",
		Etcd: EtcdSource{
			Prefix: " runtime/config/ ",
		},
	}
	source.ApplyEnv(mapEnv(map[string]string{
		envConfigKey:           "configs/runtime.yaml",
		envConfigEtcdEndpoints: " 127.0.0.1:2379,10.0.0.2:2379 ",
	}))
	source.Normalize()

	if source.Provider != "etcd" {
		t.Fatalf("provider = %q", source.Provider)
	}
	if source.Key != "configs/runtime.yaml" {
		t.Fatalf("key = %q", source.Key)
	}
	if source.Etcd.Prefix != "/runtime/config" {
		t.Fatalf("prefix = %q", source.Etcd.Prefix)
	}
	if len(source.Etcd.Endpoints) != 2 || source.Etcd.Endpoints[0] != "127.0.0.1:2379" || source.Etcd.Endpoints[1] != "10.0.0.2:2379" {
		t.Fatalf("endpoints = %#v", source.Etcd.Endpoints)
	}
}

type fakeSourceStore struct {
	values          map[string]Entry
	putIfAbsentHook func()
}

func newFakeSourceStore() *fakeSourceStore {
	return &fakeSourceStore{values: map[string]Entry{}}
}

func (s *fakeSourceStore) Get(ctx context.Context, key string) (Entry, error) {
	value, ok := s.values[strings.Trim(key, "/")]
	if !ok {
		return Entry{}, ErrConfigNotFound
	}
	return value, nil
}

func (s *fakeSourceStore) Put(ctx context.Context, key string, value []byte) error {
	key = strings.Trim(key, "/")
	s.values[key] = Entry{Key: key, Value: append([]byte(nil), value...)}
	return nil
}

func (s *fakeSourceStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if s.putIfAbsentHook != nil {
		s.putIfAbsentHook()
	}
	key = strings.Trim(key, "/")
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = Entry{Key: key, Value: append([]byte(nil), value...)}
	return true, nil
}

func (s *fakeSourceStore) Delete(ctx context.Context, key string) error {
	delete(s.values, strings.Trim(key, "/"))
	return nil
}

func (s *fakeSourceStore) List(ctx context.Context, prefix string) ([]Entry, error) {
	prefix = strings.Trim(prefix, "/")
	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, s.values[key])
	}
	return entries, nil
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
