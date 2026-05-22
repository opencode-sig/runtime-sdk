package servicekit

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	runtimecomponent "github.com/opencode-sig/runtime-sdk/runtime/component"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func addGRPCService(app *lifecycle.Runtime, cfg ComponentConfig) error {
	service := cfg.Config.Service
	if strings.TrimSpace(service.GRPCAddr) == "" {
		return fmt.Errorf("service %s grpc_addr is required", cfg.Spec.Name)
	}
	server := runtimecomponent.NewGRPCService(runtimecomponent.GRPCConfig{
		Name:        cfg.Spec.Name,
		GRPCAddr:    service.GRPCAddr,
		AdminAddr:   service.AdminAddr,
		EnablePprof: service.EnablePprof,
		HealthChecks: map[string]func(context.Context) error{
			"runtime": app.Health,
		},
		Register: func(server *grpc.Server) {
			cfg.Spec.RegisterGRPC(server)
		},
	}, cfg.Logger)
	return app.Add(cfg.Spec.Name+"_grpc", server)
}

func addServiceRegistration(app *lifecycle.Runtime, cfg ComponentConfig) error {
	service := cfg.Config.Service
	address := serviceAddress(cfg.Config)
	if address == "" {
		return fmt.Errorf("service %s advertise grpc addr is required", cfg.Spec.Name)
	}
	instance := registry.NewServiceInstance(cfg.Spec.Name, address, map[string]string{
		"runtime":              strings.TrimSpace(cfg.RuntimeMode),
		"admin_addr":           service.AdminAddr,
		"advertise_admin_addr": service.AdvertiseAdminAddr,
	})
	return app.Add(cfg.Spec.Name+"_registry", runtimecomponent.NewRegistrationComponent(cfg.Registry, instance, cfg.Logger).WithDataPlaneGeneration(cfg.DataPlaneGeneration))
}

func addGatewayMetadata(app *lifecycle.Runtime, cfg ComponentConfig) error {
	routes, descriptors, err := cfg.Spec.GatewayMetadata()
	if err != nil {
		return err
	}
	return app.Add(cfg.Spec.Name+"_gateway_metadata", NewMetadataPublisher(cfg.Etcd, MetadataPrefixes{
		RoutesPrefix:      cfg.Config.Metadata.RoutesPrefix,
		DescriptorsPrefix: cfg.Config.Metadata.DescriptorsPrefix,
	}, routes, descriptors))
}
