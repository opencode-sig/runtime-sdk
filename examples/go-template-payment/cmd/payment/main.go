package main

import (
	"context"
	"flag"

	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/bootstrap"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func main() {
	configRoot := flag.String("config-root", ".", "project root that contains configs/service")
	configKey := flag.String("config-key", "", "bootstrap config key; empty means configs/service/<service>.yaml")
	flag.Parse()

	ctx := context.Background()
	spec, err := bootstrap.Module()
	if err != nil {
		panic(err)
	}
	if err := servicekit.Run(ctx, servicekit.RunOptions{
		Spec: spec,
		LoadConfig: servicekit.NewConfigLoader(servicekit.ConfigLoaderOptions{
			Root: *configRoot,
			Key:  *configKey,
		}),
	}); err != nil {
		panic(err)
	}
}
