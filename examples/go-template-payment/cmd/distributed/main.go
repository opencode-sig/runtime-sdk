package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	paymentbootstrap "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/payment/bootstrap"
	userbootstrap "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/user/bootstrap"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

type serviceModule struct {
	name   string
	module func() (servicekit.Spec, error)
}

func main() {
	configRoot := flag.String("config-root", defaultConfigRoot(), "project root that contains configs")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loader := servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
		Root: *configRoot,
	})
	services := []serviceModule{
		{name: "user", module: userbootstrap.Module},
		{name: "payment", module: paymentbootstrap.Module},
	}
	errCh := make(chan error, len(services))
	for _, svc := range services {
		svc := svc
		go func() {
			errCh <- runService(ctx, loader, svc)
			cancel()
		}()
	}

	var firstErr error
	for range services {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	if firstErr != nil {
		panic(firstErr)
	}
}

func defaultConfigRoot() string {
	if _, err := os.Stat("configs/runtime.yaml"); err == nil {
		return "."
	}
	if _, err := os.Stat("examples/go-template-payment/configs/runtime.yaml"); err == nil {
		return "examples/go-template-payment"
	}
	return "."
}

func runService(ctx context.Context, loader servicekit.ConfigLoader, svc serviceModule) error {
	spec, err := svc.module()
	if err != nil {
		return fmt.Errorf("%s module: %w", svc.name, err)
	}
	if err := servicekit.Run(ctx, servicekit.RunOptions{
		Spec:       spec,
		LoadConfig: loader,
	}); err != nil {
		return fmt.Errorf("%s runtime: %w", svc.name, err)
	}
	return nil
}
