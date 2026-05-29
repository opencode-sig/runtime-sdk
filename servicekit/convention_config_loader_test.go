package servicekit

import (
	"io"
	"slices"
	"strings"
	"testing"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

func TestConventionConfigLoaderFileModeComposesSplitConfig(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml": []byte(`
config:
  provider: file
  etcd:
    endpoints:
      - 127.0.0.1:2379
    prefix: /runtime/config
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
`),
		"configs/logger.yaml": []byte(`
service_name: payment
file_prefix: payment
level: debug
format: console
enable_stdout: true
caller: true
`),
		"configs/registry.yaml": []byte(`
provider: etcd
etcd:
  prefix: /runtime/registry
`),
		"configs/infra/elastic.yaml": []byte(`
addresses:
  - http://127.0.0.1:9200
username: elastic
password: secret
`),
		"configs/infra/minio.yaml": []byte(`
endpoint: 127.0.0.1:9000
access_key: minio
secret_key: secret
bucket: payment
`),
		"configs/service/payment.yaml": []byte(`
grpc_addr: :9001
advertise_grpc_addr: 127.0.0.1:9001
admin_addr: :9101
advertise_admin_addr: 127.0.0.1:9101
settings:
  provider: static
`),
	}}

	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: "."}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider {
			if root != "." {
				t.Fatalf("root = %q, want .", root)
			}
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
	if cfg.Service.GRPCAddr != ":9001" {
		t.Fatalf("grpc addr = %q, want :9001", cfg.Service.GRPCAddr)
	}
	if cfg.Runtime.Config.Root != "." {
		t.Fatalf("runtime root = %q, want .", cfg.Runtime.Config.Root)
	}
	if cfg.Runtime.Config.Key != "configs/runtime.yaml" {
		t.Fatalf("runtime key = %q, want configs/runtime.yaml", cfg.Runtime.Config.Key)
	}
	if cfg.Logger.Level != "debug" || cfg.Logger.Format != "console" {
		t.Fatalf("logger = %+v, want debug console", cfg.Logger)
	}
	if cfg.Registry.Etcd.Prefix != "/runtime/registry" {
		t.Fatalf("registry prefix = %q, want /runtime/registry", cfg.Registry.Etcd.Prefix)
	}
	if len(cfg.Registry.Etcd.Endpoints) != 1 || cfg.Registry.Etcd.Endpoints[0] != "127.0.0.1:2379" {
		t.Fatalf("registry endpoints = %v, want inherited runtime config endpoint", cfg.Registry.Etcd.Endpoints)
	}
	if cfg.Infra.Elastic.Addresses[0] != "http://127.0.0.1:9200" {
		t.Fatalf("elastic addresses = %v", cfg.Infra.Elastic.Addresses)
	}
	if cfg.Infra.MinIO.Bucket != "payment" {
		t.Fatalf("minio bucket = %q, want payment", cfg.Infra.MinIO.Bucket)
	}
	settings, err := DecodeSettings[struct {
		Provider string `json:"provider" yaml:"provider"`
	}](cfg)
	if err != nil {
		t.Fatalf("decode settings error = %v", err)
	}
	if settings.Provider != "static" {
		t.Fatalf("settings provider = %q, want static", settings.Provider)
	}
}

func TestConventionConfigLoaderFileModeOptionalFilesMissing(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml":       conventionRuntimeYAML("file"),
		"configs/service/auth.yaml":  []byte("grpc_addr: :9002\n"),
		"configs/service/user.yaml":  []byte("grpc_addr: :9003\n"),
		"configs/global/unused.yaml": []byte("enabled: true\n"),
	}}
	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: "."}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
	})

	cfg, err := loader(t.Context(), "auth")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.Name != "auth" || cfg.Service.GRPCAddr != ":9002" {
		t.Fatalf("service = %+v, want auth :9002", cfg.Service)
	}
	if slices.Contains(fileProvider.loadCalls, "configs/service/user.yaml") {
		t.Fatalf("loader should not load other service config, calls = %v", fileProvider.loadCalls)
	}
}

func TestConventionConfigLoaderManagedPrefixAppliesToAllConventionKeys(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"tenant/config/runtime.yaml": []byte(`
config:
  provider: file
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
`),
		"tenant/config/logger.yaml":          []byte("level: warn\nformat: json\nenable_stdout: true\n"),
		"tenant/config/service/payment.yaml": []byte("grpc_addr: :9001\n"),
	}}
	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{
		Root:                ".",
		ManagedConfigPrefix: "tenant/config",
	}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Runtime.Config.Key != "tenant/config/runtime.yaml" {
		t.Fatalf("runtime key = %q, want tenant/config/runtime.yaml", cfg.Runtime.Config.Key)
	}
	if cfg.Logger.Level != "warn" {
		t.Fatalf("logger level = %q, want warn", cfg.Logger.Level)
	}
	if !slices.Contains(fileProvider.loadCalls, "tenant/config/logger.yaml") {
		t.Fatalf("load calls = %v, want custom logger key", fileProvider.loadCalls)
	}
}

func TestConventionConfigLoaderServiceFragmentJSON(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml":         conventionRuntimeYAML("file"),
		"configs/service/payment.yaml": []byte(`{"grpc_addr":":9001","settings":{"provider":"json"}}`),
	}}
	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: "."}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.GRPCAddr != ":9001" {
		t.Fatalf("grpc addr = %q, want :9001", cfg.Service.GRPCAddr)
	}
	if cfg.Settings["provider"] != "json" {
		t.Fatalf("settings = %v, want provider json", cfg.Settings)
	}
}

func TestConventionConfigLoaderErrors(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string][]byte
		service   string
		wantError string
	}{
		{
			name: "missing service file",
			data: map[string][]byte{
				"configs/runtime.yaml": conventionRuntimeYAML("file"),
			},
			service:   "payment",
			wantError: "load configs/service/payment.yaml",
		},
		{
			name: "missing grpc addr",
			data: map[string][]byte{
				"configs/runtime.yaml":         conventionRuntimeYAML("file"),
				"configs/service/payment.yaml": []byte("admin_addr: :9101\n"),
			},
			service:   "payment",
			wantError: "validate configs/service/payment.yaml: grpc_addr is required",
		},
		{
			name: "service name mismatch",
			data: map[string][]byte{
				"configs/runtime.yaml":         conventionRuntimeYAML("file"),
				"configs/service/payment.yaml": []byte("name: order\ngrpc_addr: :9001\n"),
			},
			service:   "payment",
			wantError: "service name \"order\" does not match \"payment\"",
		},
		{
			name: "invalid optional fragment",
			data: map[string][]byte{
				"configs/runtime.yaml":         conventionRuntimeYAML("file"),
				"configs/service/payment.yaml": []byte("grpc_addr: :9001\n"),
				"configs/infra/minio.yaml":     []byte("access_key: minio\nsecret_key: secret\n"),
			},
			service:   "payment",
			wantError: "validate configs/infra/minio.yaml: minio endpoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileProvider := &fakeConfigProvider{data: tt.data}
			loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: "."}, conventionConfigLoaderDeps{
				newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
			})

			_, err := loader(t.Context(), tt.service)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestConventionConfigLoaderEtcdModeLoadsExistingKeys(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml": conventionRuntimeYAML("etcd"),
		"configs/service/payment.yaml": []byte(`
grpc_addr: :9001
settings:
  provider: local
`),
	}}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml": conventionRuntimeYAML("etcd"),
		"configs/service/payment.yaml": []byte(`
grpc_addr: :9999
settings:
  provider: remote
`),
	}}
	closer := &fakeCloser{}

	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: "."}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
		newEtcdProvider: func(cfg ConfigSourceConfig) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			if len(cfg.Etcd.Endpoints) != 1 || cfg.Etcd.Prefix != "/runtime/config" {
				t.Fatalf("etcd config = %+v, want bootstrap endpoints and prefix", cfg.Etcd)
			}
			return etcdProvider, closer, true
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.GRPCAddr != ":9999" {
		t.Fatalf("grpc addr = %q, want remote :9999", cfg.Service.GRPCAddr)
	}
	if len(etcdProvider.putCalls) != 0 {
		t.Fatalf("put calls = %v, want none", etcdProvider.putCalls)
	}
	if !closer.closed {
		t.Fatal("etcd provider closer was not called")
	}
}

func TestConventionConfigLoaderEtcdModeSeedsMissingKeys(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml": conventionRuntimeYAML("ETCD"),
		"configs/logger.yaml":  []byte("level: warn\nformat: json\nenable_stdout: true\n"),
		"configs/service/payment.yaml": []byte(`
grpc_addr: :9001
settings:
  provider: seeded
`),
		"configs/service/user.yaml": []byte("grpc_addr: :9002\n"),
	}}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{}}

	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: "."}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
		newEtcdProvider: func(cfg ConfigSourceConfig) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			return etcdProvider, nil, true
		},
	})

	cfg, err := loader(t.Context(), "payment")
	if err != nil {
		t.Fatalf("loader error = %v", err)
	}
	if cfg.Service.GRPCAddr != ":9001" {
		t.Fatalf("grpc addr = %q, want :9001", cfg.Service.GRPCAddr)
	}
	wantPut := []string{"configs/runtime.yaml", "configs/logger.yaml", "configs/service/payment.yaml"}
	if !slices.Equal(etcdProvider.putCalls, wantPut) {
		t.Fatalf("put calls = %v, want %v", etcdProvider.putCalls, wantPut)
	}
	if slices.Contains(etcdProvider.putCalls, "configs/service/user.yaml") {
		t.Fatalf("loader should not seed other service config, put calls = %v", etcdProvider.putCalls)
	}
}

func TestConventionConfigLoaderEtcdModeAutoSeedDisabled(t *testing.T) {
	fileProvider := &fakeConfigProvider{data: map[string][]byte{
		"configs/runtime.yaml":         conventionRuntimeYAML("etcd"),
		"configs/service/payment.yaml": []byte("grpc_addr: :9001\n"),
	}}
	etcdProvider := &fakeConfigProvider{data: map[string][]byte{}}
	loader := newConventionConfigLoaderWithDeps(ConventionConfigLoaderOptions{Root: ".", DisableEtcdAutoSeed: true}, conventionConfigLoaderDeps{
		newFileProvider: func(root string) runtimeconfig.ConfigProvider { return fileProvider },
		newEtcdProvider: func(cfg ConfigSourceConfig) (runtimeconfig.ConfigProvider, io.Closer, bool) {
			return etcdProvider, nil, true
		},
	})

	_, err := loader(t.Context(), "payment")
	if err == nil || !strings.Contains(err.Error(), "load configs/runtime.yaml") {
		t.Fatalf("error = %v, want configs/runtime.yaml load error", err)
	}
	if len(etcdProvider.putCalls) != 0 {
		t.Fatalf("put calls = %v, want none", etcdProvider.putCalls)
	}
}

func conventionRuntimeYAML(provider string) []byte {
	return []byte(`
config:
  provider: ` + provider + `
  key: configs/runtime.yaml
  etcd:
    endpoints:
      - 127.0.0.1:2379
    prefix: /runtime/config
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
`)
}
