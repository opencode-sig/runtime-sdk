package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/bootstrap"
	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func main() {
	configRoot := flag.String("config-root", "configs", "directory that contains the bootstrap config")
	configKey := flag.String("config-key", "service.yaml", "bootstrap config key")
	flag.Parse()

	ctx := context.Background()
	spec, err := bootstrap.Module()
	if err != nil {
		panic(err)
	}
	if err := servicekit.Run(ctx, servicekit.RunOptions{
		Spec: spec,
		LoadConfig: func(ctx context.Context, service string) (servicekit.Config, error) {
			return loadConfig(ctx, *configRoot, *configKey)
		},
	}); err != nil {
		panic(err)
	}
}

func loadConfig(ctx context.Context, root string, key string) (servicekit.Config, error) {
	fileProvider := runtimeconfig.NewFileProvider(root)
	data, err := fileProvider.Load(ctx, key)
	if err != nil {
		return servicekit.Config{}, fmt.Errorf("load bootstrap config: %w", err)
	}
	cfg, err := runtimeconfig.Decode[servicekit.Config](data)
	if err != nil {
		return servicekit.Config{}, fmt.Errorf("decode bootstrap config: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Runtime.Config.Provider), "etcd") {
		return cfg, nil
	}

	etcdProvider, ok := cfg.EtcdConfigStore()
	if !ok {
		return cfg, nil
	}
	defer func() { _ = etcdProvider.Close() }()

	data, err = etcdProvider.Load(ctx, cfg.Runtime.Config.Key)
	if err != nil {
		return servicekit.Config{}, fmt.Errorf("load etcd config: %w", err)
	}
	managed, err := runtimeconfig.Decode[servicekit.Config](data)
	if err != nil {
		return servicekit.Config{}, fmt.Errorf("decode etcd config: %w", err)
	}
	return managed, nil
}
