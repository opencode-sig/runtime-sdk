package servicekit

import (
	"context"

	"github.com/opencode-sig/runtime-sdk/infra/etcd"
	"github.com/opencode-sig/runtime-sdk/logger"
	runtimecomponent "github.com/opencode-sig/runtime-sdk/runtime/component"
	runtimediscovery "github.com/opencode-sig/runtime-sdk/runtime/discovery"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

// NewServiceLifecycle builds the lifecycle graph for one service DataPlane.
func NewServiceLifecycle(ctx context.Context, cfg Config, spec Spec, runtimeMode string, log *logger.Logger) (*lifecycle.Runtime, error) {
	return newServiceLifecycle(ctx, cfg, spec, runtimeMode, log, "", nil, nil)
}

func newServiceLifecycle(ctx context.Context, cfg Config, spec Spec, runtimeMode string, log *logger.Logger, dataPlaneGeneration string, identity *runtimeIdentityStore, onBound BoundAddressHandler) (*lifecycle.Runtime, error) {
	validSpec, err := NewSpec(spec)
	if err != nil {
		return nil, err
	}
	app := lifecycle.New(validSpec.Name)
	infra := NewInfraContainer(cfg.Infra)
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = infra.Close()
		}
	}()

	if !cfg.RegistryEnabled() {
		if err := app.Add("infra", runtimecomponent.NewCloseComponent(infra.Close)); err != nil {
			return nil, err
		}
		if err := AddToLifecycle(app, ComponentConfig{
			Config:              cfg,
			Spec:                validSpec,
			Infra:               infra,
			RuntimeMode:         runtimeMode,
			DataPlaneGeneration: dataPlaneGeneration,
			OnBound:             onBound,
			Logger:              log,
			identity:            identity,
		}); err != nil {
			return nil, err
		}
		closeOnError = false
		return app, nil
	}

	etcdClient, err := etcd.NewClientAndWait(ctx, etcd.Config{Endpoints: cfg.Registry.Etcd.Endpoints})
	if err != nil {
		return nil, err
	}
	if err := app.Add("etcd_client", runtimecomponent.NewCloseComponent(etcdClient.Close)); err != nil {
		_ = etcdClient.Close()
		return nil, err
	}
	reg := registry.NewEtcdRegistry(etcdClient, cfg.Registry.Etcd.Prefix)
	clients, err := NewClients(runtimediscovery.NewEtcdDiscovery(etcdClient, cfg.Registry.Etcd.Prefix))
	if err != nil {
		_ = etcdClient.Close()
		return nil, err
	}
	if err := app.Add("grpc_clients", clients); err != nil {
		_ = clients.Close()
		_ = etcdClient.Close()
		return nil, err
	}
	if err := app.Add("infra", runtimecomponent.NewCloseComponent(infra.Close)); err != nil {
		_ = clients.Close()
		_ = etcdClient.Close()
		return nil, err
	}
	if err := AddToLifecycle(app, ComponentConfig{
		Config:              cfg,
		Spec:                validSpec,
		Etcd:                etcdClient,
		Registry:            reg,
		Clients:             clients,
		Infra:               infra,
		RuntimeMode:         runtimeMode,
		DataPlaneGeneration: dataPlaneGeneration,
		OnBound:             onBound,
		Logger:              log,
		identity:            identity,
	}); err != nil {
		_ = clients.Close()
		_ = etcdClient.Close()
		return nil, err
	}
	closeOnError = false
	return app, nil
}
