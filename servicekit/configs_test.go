package servicekit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gatewaymeta "github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"google.golang.org/grpc"
)

func TestConfigsDecodeFileRootConfig(t *testing.T) {
	root := t.TempDir()
	globalDir := filepath.Join(root, "configs", "global")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "app.yaml"), []byte("app_name: rooted\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	configs, err := NewConfigs(Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{
		Provider: "file",
		Root:     root,
		Key:      "configs/runtime.yaml",
	}}})
	if err != nil {
		t.Fatalf("new configs: %v", err)
	}

	var app struct {
		AppName string `yaml:"app_name"`
	}
	if err := configs.Decode(context.Background(), "configs/global/app.yaml", &app); err != nil {
		t.Fatalf("decode global config: %v", err)
	}
	if app.AppName != "rooted" {
		t.Fatalf("app = %#v", app)
	}
}

func TestConfigsDecodeFileRootConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "configs")
	globalDir := filepath.Join(configDir, "global")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "app.yaml"), []byte("app_name: configdir\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	configs, err := NewConfigs(Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{
		Provider: "file",
		Root:     configDir,
		Key:      "service.yaml",
	}}})
	if err != nil {
		t.Fatalf("new configs: %v", err)
	}

	var app struct {
		AppName string `yaml:"app_name"`
	}
	if err := configs.Decode(context.Background(), "configs/global/app.yaml", &app); err != nil {
		t.Fatalf("decode global config: %v", err)
	}
	if app.AppName != "configdir" {
		t.Fatalf("app = %#v", app)
	}
}

func TestConfigsDecodeFileRootOption(t *testing.T) {
	root := t.TempDir()
	globalDir := filepath.Join(root, "configs", "global")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "app.yaml"), []byte("app_name: option\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	configs, err := NewConfigs(Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{
		Provider: "file",
		Key:      "configs/runtime.yaml",
	}}}, WithConfigsFileRoot(root))
	if err != nil {
		t.Fatalf("new configs: %v", err)
	}

	var app struct {
		AppName string `yaml:"app_name"`
	}
	if err := configs.Decode(context.Background(), "configs/global/app.yaml", &app); err != nil {
		t.Fatalf("decode global config: %v", err)
	}
	if app.AppName != "option" {
		t.Fatalf("app = %#v", app)
	}
}

func TestConfigsDecodeFileGlobalConfig(t *testing.T) {
	root := t.TempDir()
	globalDir := filepath.Join(root, "configs", "global")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "app.yaml"), []byte("app_name: demo\nenvironment: test\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	configs, err := NewConfigs(Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{
		Provider: "file",
		Key:      filepath.Join(root, "configs", "runtime.yaml"),
	}}})
	if err != nil {
		t.Fatalf("new configs: %v", err)
	}

	var app struct {
		AppName     string `yaml:"app_name"`
		Environment string `yaml:"environment"`
	}
	if err := configs.Decode(context.Background(), "configs/global/app.yaml", &app); err != nil {
		t.Fatalf("decode global config: %v", err)
	}
	if app.AppName != "demo" || app.Environment != "test" {
		t.Fatalf("app = %#v", app)
	}

	entries, err := configs.List(context.Background(), "configs/global/")
	if err != nil {
		t.Fatalf("list global configs: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "configs/global/app.yaml" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestAddToLifecycleProvidesConfigs(t *testing.T) {
	var seen *Configs
	app := lifecycle.New("demo")
	err := AddToLifecycle(app, ComponentConfig{
		Config: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "file", Key: "configs/runtime.yaml"}}, Service: ServiceConfig{Name: "demo", GRPCAddr: ":0"}},
		Spec: Spec{
			Name:               "demo",
			RegisterGRPC:       func(grpc.ServiceRegistrar) {},
			GatewayPublication: func() ([]gatewaymeta.RouteMeta, map[string][]byte, error) { return nil, nil, nil },
			Init: func(ctx RuntimeContext) error {
				seen = ctx.Configs
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("add lifecycle: %v", err)
	}
	if seen == nil {
		t.Fatal("configs accessor was not provided")
	}
}

func TestConfigsCloseClosesProvider(t *testing.T) {
	want := errors.New("close failed")
	provider := &closableConfigProvider{err: want}
	configs := &Configs{provider: provider}

	if err := configs.Close(); !errors.Is(err, want) {
		t.Fatalf("close error = %v, want %v", err, want)
	}
	if !provider.closed {
		t.Fatal("provider was not closed")
	}
}

type closableConfigProvider struct {
	closed bool
	err    error
}

func (p *closableConfigProvider) Load(ctx context.Context, key string) ([]byte, error) {
	return nil, nil
}

func (p *closableConfigProvider) Close() error {
	p.closed = true
	return p.err
}
