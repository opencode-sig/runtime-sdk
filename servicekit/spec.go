package servicekit

import (
	"fmt"
	"strings"

	"google.golang.org/grpc"

	gatewaymeta "github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

// Spec declares one managed gRPC service.
//
// The spec is the public contract shared by internal and external services:
// business code provides protobuf registration and Gateway metadata, while the
// runtime owns transport, lifecycle, registry and management-plane wiring.
type Spec struct {
	Name               string
	RegisterGRPC       func(grpc.ServiceRegistrar)
	GatewayPublication func() ([]gatewaymeta.RouteMeta, map[string][]byte, error)
	Init               func(RuntimeContext) error
	InitDistributed    func(DistributedContext) error
}

// GRPCSpec describes a service backed by a generated protobuf registrar.
type GRPCSpec[T any] struct {
	Name               string
	Server             T
	Register           func(grpc.ServiceRegistrar, T)
	GatewayPublication func() ([]gatewaymeta.RouteMeta, map[string][]byte, error)
	Init               func(RuntimeContext) error
	InitDistributed    func(DistributedContext) error
}

// NewGRPCSpec creates a Spec from a generated protobuf registration function.
func NewGRPCSpec[T any](spec GRPCSpec[T]) (Spec, error) {
	if spec.Register == nil {
		return Spec{}, fmt.Errorf("service %s grpc registration is required", strings.TrimSpace(spec.Name))
	}
	return NewSpec(Spec{
		Name: spec.Name,
		RegisterGRPC: func(registrar grpc.ServiceRegistrar) {
			spec.Register(registrar, spec.Server)
		},
		GatewayPublication: spec.GatewayPublication,
		Init:               spec.Init,
		InitDistributed:    spec.InitDistributed,
	})
}

// NewSpec validates and normalizes one service spec.
func NewSpec(spec Spec) (Spec, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return Spec{}, fmt.Errorf("service name is required")
	}
	if spec.RegisterGRPC == nil {
		return Spec{}, fmt.Errorf("service %s grpc registration is required", name)
	}
	if spec.GatewayPublication == nil {
		return Spec{}, fmt.Errorf("service %s gateway publication is required", name)
	}
	spec.Name = name
	return spec, nil
}

func (s Spec) GatewayMetadata() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
	if s.GatewayPublication == nil {
		return nil, nil, fmt.Errorf("service %s gateway publication is required", s.Name)
	}
	return s.GatewayPublication()
}
