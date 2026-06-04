package servicekit

import (
	"fmt"
	"strings"

	"google.golang.org/grpc"

	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
	runtimecomponent "github.com/opencode-sig/runtime-sdk/runtime/component"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func addGRPCService(app *lifecycle.Runtime, cfg ComponentConfig, controlPlane *runtimemetrics.ControlPlaneMetrics) error {
	service := cfg.Config.Service
	if strings.TrimSpace(service.GRPCAddr) == "" {
		return fmt.Errorf("service %s grpc_addr is required", cfg.Spec.Name)
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
	return app.Add(cfg.Spec.Name+"_grpc", server)
}

func addServiceRegistration(app *lifecycle.Runtime, cfg ComponentConfig, controlPlane *runtimemetrics.ControlPlaneMetrics) error {
	service := cfg.Config.Service
	address := serviceAddress(cfg.Config)
	if address == "" {
		return fmt.Errorf("service %s advertise grpc addr is required", cfg.Spec.Name)
	}
	instance := registry.NewServiceInstance(cfg.Spec.Name, address, map[string]string{
		"runtime":             strings.TrimSpace(cfg.RuntimeMode),
		"http_addr":           service.HTTPAddr,
		"advertise_http_addr": service.AdvertiseHTTPAddr,
	})
	return app.Add(cfg.Spec.Name+"_registry", runtimecomponent.NewRegistrationComponent(cfg.Registry, instance, cfg.Logger).
		WithDataPlaneGeneration(cfg.DataPlaneGeneration).
		WithControlPlaneMetrics(controlPlane))
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
