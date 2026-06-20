package servicekit

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
	runtimecomponent "github.com/opencode-sig/runtime-sdk/runtime/component"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func addGRPCService(app *lifecycle.Runtime, cfg ComponentConfig, controlPlane *runtimemetrics.ControlPlaneMetrics) (*runtimecomponent.GRPCService, error) {
	service := cfg.Config.Service
	if strings.TrimSpace(service.GRPCAddr) == "" {
		return nil, fmt.Errorf("service %s grpc_addr is required", cfg.Spec.Name)
	}
	server := runtimecomponent.NewGRPCService(runtimecomponent.GRPCConfig{
		Name:            cfg.Spec.Name,
		GRPCAddr:        service.GRPCAddr,
		HTTPAddr:        service.HTTPAddr,
		EnablePprof:     service.EnablePprof,
		ReadinessChecks: cfg.Spec.ReadinessChecks,
		ControlPlane:    controlPlane,
		Register: func(server *grpc.Server) {
			cfg.Spec.RegisterGRPC(server)
		},
		RegisterHTTP: cfg.Spec.RegisterHTTP,
	}, cfg.Logger)
	if err := app.Add(cfg.Spec.Name+"_grpc", server); err != nil {
		return nil, err
	}
	return server, nil
}

func addServiceRegistration(app *lifecycle.Runtime, cfg ComponentConfig, controlPlane *runtimemetrics.ControlPlaneMetrics, grpcServices ...*runtimecomponent.GRPCService) error {
	service := cfg.Config.Service
	var grpcService *runtimecomponent.GRPCService
	if len(grpcServices) > 0 {
		grpcService = grpcServices[0]
	}
	instanceFactory := func(ctx context.Context) (registry.ServiceInstance, error) {
		var bound serviceAdvertiseAddresses
		if grpcService != nil {
			bound.GRPC = grpcService.BoundGRPCAddr()
			bound.HTTP = grpcService.BoundHTTPAddr()
		}
		addresses, err := resolveServiceAddressesWithBound(ctx, cfg.Config, bound)
		if err != nil {
			return registry.ServiceInstance{}, err
		}
		if addresses.GRPC == "" {
			return registry.ServiceInstance{}, fmt.Errorf("service %s advertise grpc addr is required", cfg.Spec.Name)
		}
		instance := registry.NewServiceInstance(cfg.Spec.Name, addresses.GRPC, map[string]string{
			"runtime":             strings.TrimSpace(cfg.RuntimeMode),
			"http_addr":           service.HTTPAddr,
			"advertise_http_addr": addresses.HTTP,
		})
		cfg.identity.Set(RuntimeIdentity{
			Service:    instance.Name,
			Address:    instance.Address,
			InstanceID: instance.ID,
		})
		return instance, nil
	}
	return app.Add(cfg.Spec.Name+"_registry", runtimecomponent.NewDynamicRegistrationComponent(cfg.Registry, instanceFactory, cfg.Logger).
		WithDataPlaneGeneration(cfg.DataPlaneGeneration).
		WithControlPlaneMetrics(controlPlane))
}

func addBoundAddressReporter(app *lifecycle.Runtime, cfg ComponentConfig, grpcService *runtimecomponent.GRPCService) error {
	if cfg.OnBound == nil {
		return nil
	}
	snapshot := func(ctx context.Context) (BoundAddresses, error) {
		var bound serviceAdvertiseAddresses
		if grpcService != nil {
			bound.GRPC = grpcService.BoundGRPCAddr()
			bound.HTTP = grpcService.BoundHTTPAddr()
		}
		addresses, err := resolveServiceAddressesWithBound(ctx, cfg.Config, bound)
		if err != nil {
			return BoundAddresses{}, err
		}
		if addresses.GRPC == "" {
			return BoundAddresses{}, fmt.Errorf("service %s advertise grpc addr is required", cfg.Spec.Name)
		}
		return BoundAddresses{
			Service:           cfg.Spec.Name,
			Generation:        cfg.DataPlaneGeneration,
			GRPCListenAddr:    bound.GRPC,
			HTTPListenAddr:    bound.HTTP,
			AdvertiseGRPCAddr: addresses.GRPC,
			AdvertiseHTTPAddr: addresses.HTTP,
		}, nil
	}
	return app.Add(cfg.Spec.Name+"_bound_addresses", newBoundAddressComponent(snapshot, cfg.OnBound))
}

func addGatewayMetadata(app *lifecycle.Runtime, cfg ComponentConfig, controlPlane *runtimemetrics.ControlPlaneMetrics) error {
	routes, descriptors, err := cfg.Spec.GatewayMetadata()
	if err != nil {
		return err
	}
	return app.Add(cfg.Spec.Name+"_gateway_metadata", NewMetadataPublisher(cfg.Etcd, MetadataPrefixes{
		RoutesPrefix:      cfg.Config.Metadata.RoutesPrefix,
		DescriptorsPrefix: cfg.Config.Metadata.DescriptorsPrefix,
	}, routes, descriptors).WithLogger(cfg.Logger).WithControlPlaneMetrics(controlPlane))
}
