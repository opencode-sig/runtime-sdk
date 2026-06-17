package servicekit

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"

	gatewaymeta "github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
)

func TestServiceDataPlaneRuntimeIdentityMatchesPortZeroRegistration(t *testing.T) {
	reg := &captureRegistry{}
	identity := newRuntimeIdentityStore()
	cfg := Config{
		Service: ServiceConfig{
			GRPCAddr: "127.0.0.1:0",
			HTTPAddr: "127.0.0.1:0",
		},
	}
	app := lifecycle.New("payment")
	if err := AddToLifecycle(app, ComponentConfig{
		Config: cfg,
		Spec: Spec{
			Name:               "payment",
			RegisterGRPC:       func(grpc.ServiceRegistrar) {},
			GatewayPublication: func() ([]gatewaymeta.RouteMeta, map[string][]byte, error) { return nil, nil, nil },
		},
		Registry:            reg,
		RuntimeMode:         "distributed",
		DataPlaneGeneration: "payment-test",
		identity:            identity,
	}); err != nil {
		t.Fatalf("add lifecycle: %v", err)
	}
	plane, err := newDataPlaneWithGeneration("payment-test", cfg, app, nil, identity)
	if err != nil {
		t.Fatalf("new data plane: %v", err)
	}
	manager := NewManager(func(context.Context) (DataPlane, error) {
		return plane, nil
	}, nil)
	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Stop(context.Background())
	})

	host, port, err := net.SplitHostPort(reg.instance.Address)
	if err != nil {
		t.Fatalf("split registered address %q: %v", reg.instance.Address, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("registered host = %q, want 127.0.0.1", host)
	}
	if port == "" || port == "0" {
		t.Fatalf("registered port = %q, want actual port", port)
	}
	if plane.RuntimeIdentity().InstanceID != reg.instance.ID {
		t.Fatalf("plane identity instance id = %q, want registry id %q", plane.RuntimeIdentity().InstanceID, reg.instance.ID)
	}
	if manager.RuntimeIdentity().InstanceID != reg.instance.ID {
		t.Fatalf("manager identity instance id = %q, want registry id %q", manager.RuntimeIdentity().InstanceID, reg.instance.ID)
	}
}
