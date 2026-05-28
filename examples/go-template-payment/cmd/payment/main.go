package main

import (
	"context"
	"flag"

	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/bootstrap"
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
		Spec:       spec,
		LoadConfig: servicekit.ManagedConfigLoader(*configRoot, *configKey),
	}); err != nil {
		panic(err)
	}
}
