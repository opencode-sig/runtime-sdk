package servicekit

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

type fakeConfigProvider struct {
	data      map[string][]byte
	loadErr   error
	loadCalls []string
	putErr    error
	putCalls  []string
}

func (p *fakeConfigProvider) Load(ctx context.Context, key string) ([]byte, error) {
	p.loadCalls = append(p.loadCalls, key)
	if p.loadErr != nil {
		return nil, p.loadErr
	}
	data, ok := p.data[key]
	if !ok {
		return nil, runtimeconfig.ErrConfigNotFound
	}
	return data, nil
}

func (p *fakeConfigProvider) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	p.putCalls = append(p.putCalls, key)
	if p.putErr != nil {
		return false, p.putErr
	}
	if p.data == nil {
		p.data = make(map[string][]byte)
	}
	if _, ok := p.data[key]; ok {
		return false, nil
	}
	p.data[key] = append([]byte(nil), value...)
	return true, nil
}

type fakeCloser struct {
	closed bool
}

func (c *fakeCloser) Close() error {
	c.closed = true
	return nil
}

func TestManagedConfigLoaderFileModeYAML(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"service.yaml": []byte(`
runtime:
  config:
    provider: file
    key: service.yaml
service:
  name: payment
`),
		},
	}
	etcdCalled := false

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "service.yaml"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			if root != "configs" {
				t.Fatalf("root = %q, want configs", root)
			}
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			etcdCalled = true
			return nil, nil, false
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.Name != "payment" {
		t.Fatalf("service name = %q, want payment", cfg.Service.Name)
	}
	if cfg.Runtime.Config.Root != "configs" {
		t.Fatalf("root = %q, want configs", cfg.Runtime.Config.Root)
	}
	if etcdCalled {
		t.Fatal("etcd provider should not be used for file config")
	}
}

func TestManagedConfigLoaderFileModeJSON(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"service.json": []byte(`{"runtime":{"config":{"provider":"file","root":"custom","key":"service.json"}},"service":{"name":"payment"}}`),
		},
	}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "service.json"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Runtime.Config.Root != "custom" {
		t.Fatalf("root = %q, want custom", cfg.Runtime.Config.Root)
	}
	if cfg.Service.Name != "payment" {
		t.Fatalf("service name = %q, want payment", cfg.Service.Name)
	}
}

func TestManagedConfigLoaderProviderEmptyDoesNotUseEtcd(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"service.yaml": []byte(`
runtime:
  config:
    key: service.yaml
`),
		},
	}
	etcdCalled := false

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "service.yaml"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			etcdCalled = true
			return nil, nil, false
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Runtime.Config.Root != "configs" {
		t.Fatalf("root = %q, want configs", cfg.Runtime.Config.Root)
	}
	if etcdCalled {
		t.Fatal("etcd provider should not be used for empty provider")
	}
}

func TestManagedConfigLoaderBootstrapLoadError(t *testing.T) {
	wantErr := errors.New("missing file")
	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "missing.yaml"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return &fakeConfigProvider{loadErr: wantErr}
		},
	})

	_, err := loader(t.Context(), "payment")
	if err == nil || !strings.Contains(err.Error(), "load bootstrap config") {
		t.Fatalf("error = %v, want load bootstrap config", err)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error should wrap original error, got %v", err)
	}
}

func TestManagedConfigLoaderBootstrapDecodeError(t *testing.T) {
	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "service.yaml"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return &fakeConfigProvider{data: map[string][]byte{"service.yaml": []byte(`runtime: [`)}}
		},
	})

	_, err := loader(t.Context(), "payment")
	if err == nil || !strings.Contains(err.Error(), "decode bootstrap config") {
		t.Fatalf("error = %v, want decode bootstrap config", err)
	}
}

func TestManagedConfigLoaderEtcdModeLoadsManagedConfig(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"bootstrap.yaml": []byte(`
runtime:
  config:
    provider: ETCD
    key: services/payment.yaml
`),
		},
	}
	etcdProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"services/payment.yaml": []byte(`
runtime:
  config:
    provider: file
service:
  name: payment
`),
		},
	}
	closer := &fakeCloser{}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "bootstrap.yaml"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			if cfg.Runtime.Config.Root != "configs" {
				t.Fatalf("bootstrap root = %q, want configs", cfg.Runtime.Config.Root)
			}
			return etcdProvider, closer, true
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.Name != "payment" {
		t.Fatalf("service name = %q, want payment", cfg.Service.Name)
	}
	if cfg.Runtime.Config.Root != "configs" {
		t.Fatalf("managed root = %q, want configs", cfg.Runtime.Config.Root)
	}
	if len(etcdProvider.loadCalls) != 1 || etcdProvider.loadCalls[0] != "services/payment.yaml" {
		t.Fatalf("etcd load calls = %v, want [services/payment.yaml]", etcdProvider.loadCalls)
	}
	if !closer.closed {
		t.Fatal("etcd provider closer was not called")
	}
}

func TestManagedConfigLoaderDefaultKeyUsesServiceName(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"configs/service/payment.yaml": []byte(`
runtime:
  config:
    provider: file
service:
  name: payment
`),
		},
	}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "."}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.Name != "payment" {
		t.Fatalf("service name = %q, want payment", cfg.Service.Name)
	}
	if len(fileProvider.loadCalls) != 1 || fileProvider.loadCalls[0] != "configs/service/payment.yaml" {
		t.Fatalf("file load calls = %v, want [configs/service/payment.yaml]", fileProvider.loadCalls)
	}
}

func TestManagedConfigLoaderEtcdMissingSeedsLocalConfig(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"configs/service/payment.yaml": []byte(`
runtime:
  config:
    provider: etcd
    etcd:
      endpoints:
        - 127.0.0.1:2379
      prefix: /runtime/config
service:
  name: payment
  grpc_addr: :9001
`),
		},
	}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{}}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "."}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			if cfg.Runtime.Config.Key != "configs/service/payment.yaml" {
				t.Fatalf("managed key = %q, want configs/service/payment.yaml", cfg.Runtime.Config.Key)
			}
			return etcdProvider, nil, true
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.Name != "payment" {
		t.Fatalf("service name = %q, want payment", cfg.Service.Name)
	}
	if cfg.Runtime.Config.Key != "configs/service/payment.yaml" {
		t.Fatalf("returned managed key = %q, want configs/service/payment.yaml", cfg.Runtime.Config.Key)
	}
	if len(etcdProvider.putCalls) != 1 || etcdProvider.putCalls[0] != "configs/service/payment.yaml" {
		t.Fatalf("put calls = %v, want [configs/service/payment.yaml]", etcdProvider.putCalls)
	}
	if len(etcdProvider.loadCalls) != 2 {
		t.Fatalf("etcd load calls = %v, want initial miss and post-seed load", etcdProvider.loadCalls)
	}
}

func TestManagedConfigLoaderEtcdMissingAutoSeedDisabled(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"configs/service/payment.yaml": []byte(`
runtime:
  config:
    provider: etcd
    etcd:
      endpoints:
        - 127.0.0.1:2379
      prefix: /runtime/config
service:
  name: payment
  grpc_addr: :9001
`),
		},
	}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{}}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: ".", DisableEtcdAutoSeed: true}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			return etcdProvider, nil, true
		},
	})

	_, err := loader(t.Context(), "payment")
	if err == nil || !strings.Contains(err.Error(), "load etcd config") {
		t.Fatalf("error = %v, want load etcd config", err)
	}
	if len(etcdProvider.putCalls) != 0 {
		t.Fatalf("put calls = %v, want none", etcdProvider.putCalls)
	}
}

func TestManagedConfigLoaderEtcdSeedRequiresCompleteServiceConfig(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"configs/service/payment.yaml": []byte(`
runtime:
  config:
    provider: etcd
    etcd:
      endpoints:
        - 127.0.0.1:2379
      prefix: /runtime/config
`),
		},
	}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{}}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "."}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			return etcdProvider, nil, true
		},
	})

	_, err := loader(t.Context(), "payment")
	if err == nil || !strings.Contains(err.Error(), "seed etcd config") || !strings.Contains(err.Error(), "service.name is required") {
		t.Fatalf("error = %v, want incomplete service config seed error", err)
	}
	if len(etcdProvider.putCalls) != 0 {
		t.Fatalf("put calls = %v, want none", etcdProvider.putCalls)
	}
}

func TestManagedConfigLoaderEtcdSeedRequiresManagedKeyPrefix(t *testing.T) {
	fileProvider := &fakeConfigProvider{
		data: map[string][]byte{
			"bootstrap.yaml": []byte(`
runtime:
  config:
    provider: etcd
    key: services/payment.yaml
    etcd:
      endpoints:
        - 127.0.0.1:2379
      prefix: /runtime/config
service:
  name: payment
  grpc_addr: :9001
`),
		},
	}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{}}

	loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: ".", Key: "bootstrap.yaml"}, configLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			return fileProvider
		},
		newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			return etcdProvider, nil, true
		},
	})

	_, err := loader(t.Context(), "payment")
	if err == nil || !strings.Contains(err.Error(), "must be under") {
		t.Fatalf("error = %v, want managed key prefix error", err)
	}
	if len(etcdProvider.putCalls) != 0 {
		t.Fatalf("put calls = %v, want none", etcdProvider.putCalls)
	}
}

func TestManagedConfigLoaderManagedConfigRootRules(t *testing.T) {
	tests := []struct {
		name     string
		managed  string
		wantRoot string
	}{
		{
			name: "empty provider fills root",
			managed: `
runtime:
  config: {}
`,
			wantRoot: "configs",
		},
		{
			name: "file provider fills root case insensitive",
			managed: `
runtime:
  config:
    provider: FILE
`,
			wantRoot: "configs",
		},
		{
			name: "existing root is kept",
			managed: `
runtime:
  config:
    provider: file
    root: custom
`,
			wantRoot: "custom",
		},
		{
			name: "etcd provider root is not filled",
			managed: `
runtime:
  config:
    provider: etcd
`,
			wantRoot: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileProvider := &fakeConfigProvider{
				data: map[string][]byte{
					"bootstrap.yaml": []byte(`
runtime:
  config:
    provider: etcd
    key: services/payment.yaml
`),
				},
			}
			etcdProvider := &fakeConfigProvider{
				data: map[string][]byte{"services/payment.yaml": []byte(tt.managed)},
			}

			loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "bootstrap.yaml"}, configLoaderDeps{
				newFileProvider: func(root string) runtimeconfig.ConfigProvider {
					return fileProvider
				},
				newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
					return etcdProvider, nil, true
				},
			})

			cfg, err := loader(t.Context(), "payment")
			if err != nil {
				t.Fatalf("loader error = %v", err)
			}
			if cfg.Runtime.Config.Root != tt.wantRoot {
				t.Fatalf("root = %q, want %q", cfg.Runtime.Config.Root, tt.wantRoot)
			}
		})
	}
}

func TestManagedConfigLoaderEtcdErrors(t *testing.T) {
	tests := []struct {
		name      string
		etcdData  []byte
		etcdErr   error
		wantError string
	}{
		{name: "load error", etcdErr: errors.New("etcd down"), wantError: "load etcd config"},
		{name: "decode error", etcdData: []byte(`runtime: [`), wantError: "decode etcd config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileProvider := &fakeConfigProvider{
				data: map[string][]byte{
					"bootstrap.yaml": []byte(`
runtime:
  config:
    provider: etcd
    key: services/payment.yaml
`),
				},
			}
			etcdProvider := &fakeConfigProvider{
				data:    map[string][]byte{"services/payment.yaml": tt.etcdData},
				loadErr: tt.etcdErr,
			}

			loader := newConfigLoaderWithDeps(ConfigLoaderOptions{Root: "configs", Key: "bootstrap.yaml"}, configLoaderDeps{
				newFileProvider: func(root string) runtimeconfig.ConfigProvider {
					return fileProvider
				},
				newEtcdProvider: func(cfg Config) (runtimeconfig.ConfigProvider, io.Closer, bool) {
					return etcdProvider, nil, true
				},
			})

			_, err := loader(t.Context(), "payment")
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %s", err, tt.wantError)
			}
		})
	}
}
