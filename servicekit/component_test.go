package servicekit

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"

	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func TestAddServiceRegistrationPublishesHTTPMetadata(t *testing.T) {
	reg := &captureRegistry{}
	app := lifecycle.New("payment")

	if err := addServiceRegistration(app, ComponentConfig{
		Config: Config{
			Service: ServiceConfig{
				GRPCAddr:          ":9001",
				AdvertiseGRPCAddr: "127.0.0.1:9001",
				HTTPAddr:          ":9101",
				AdvertiseHTTPAddr: "127.0.0.1:9101",
			},
		},
		Spec:        Spec{Name: "payment"},
		Registry:    reg,
		RuntimeMode: "distributed",
	}, runtimemetrics.NewControlPlaneMetrics("payment")); err != nil {
		t.Fatalf("add registration: %v", err)
	}
	if err := app.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Stop(context.Background())
	})

	if reg.instance.Name != "payment" {
		t.Fatalf("instance name = %q, want payment", reg.instance.Name)
	}
	if reg.instance.Address != "127.0.0.1:9001" {
		t.Fatalf("instance address = %q, want grpc advertise address", reg.instance.Address)
	}
	if reg.instance.Metadata["http_addr"] != ":9101" {
		t.Fatalf("http_addr metadata = %q", reg.instance.Metadata["http_addr"])
	}
	if reg.instance.Metadata["advertise_http_addr"] != "127.0.0.1:9101" {
		t.Fatalf("advertise_http_addr metadata = %q", reg.instance.Metadata["advertise_http_addr"])
	}
}

func TestAddServiceRegistrationFallsBackToConcreteHTTPListenAddress(t *testing.T) {
	reg := &captureRegistry{}
	app := lifecycle.New("payment")

	if err := addServiceRegistration(app, ComponentConfig{
		Config: Config{
			Service: ServiceConfig{
				GRPCAddr:          ":9001",
				AdvertiseGRPCAddr: "127.0.0.1:9001",
				HTTPAddr:          "127.0.0.1:9101",
			},
		},
		Spec:        Spec{Name: "payment"},
		Registry:    reg,
		RuntimeMode: "distributed",
	}, runtimemetrics.NewControlPlaneMetrics("payment")); err != nil {
		t.Fatalf("add registration: %v", err)
	}
	if err := app.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Stop(context.Background())
	})

	if reg.instance.Metadata["advertise_http_addr"] != "127.0.0.1:9101" {
		t.Fatalf("advertise_http_addr metadata = %q", reg.instance.Metadata["advertise_http_addr"])
	}
}

func TestAddServiceRegistrationUsesBoundPortWhenListenPortIsZero(t *testing.T) {
	reg := &captureRegistry{}
	app := lifecycle.New("payment")
	identity := newRuntimeIdentityStore()
	var boundEvent BoundAddresses
	cfg := ComponentConfig{
		Config: Config{
			Service: ServiceConfig{
				GRPCAddr: "127.0.0.1:0",
				HTTPAddr: "127.0.0.1:0",
			},
		},
		Spec:        Spec{Name: "payment", RegisterGRPC: func(grpc.ServiceRegistrar) {}},
		Registry:    reg,
		RuntimeMode: "distributed",
		identity:    identity,
		OnBound: func(ctx context.Context, addresses BoundAddresses) {
			boundEvent = addresses
		},
	}
	controlPlane := runtimemetrics.NewControlPlaneMetrics("payment")
	grpcService, err := addGRPCService(app, cfg, controlPlane)
	if err != nil {
		t.Fatalf("add grpc service: %v", err)
	}
	if err := addServiceRegistration(app, cfg, controlPlane, grpcService); err != nil {
		t.Fatalf("add registration: %v", err)
	}
	if err := addBoundAddressReporter(app, cfg, grpcService); err != nil {
		t.Fatalf("add bound address reporter: %v", err)
	}
	if err := app.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Stop(context.Background())
	})

	host, port, err := net.SplitHostPort(reg.instance.Address)
	if err != nil {
		t.Fatalf("split grpc address %q: %v", reg.instance.Address, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("grpc address host = %q, want 127.0.0.1", host)
	}
	if port == "" || port == "0" {
		t.Fatalf("grpc address port = %q, want actual port", port)
	}
	httpHost, httpPort, err := net.SplitHostPort(reg.instance.Metadata["advertise_http_addr"])
	if err != nil {
		t.Fatalf("split http address %q: %v", reg.instance.Metadata["advertise_http_addr"], err)
	}
	if httpHost != "127.0.0.1" {
		t.Fatalf("http address host = %q, want 127.0.0.1", httpHost)
	}
	if httpPort == "" || httpPort == "0" {
		t.Fatalf("http address port = %q, want actual port", httpPort)
	}
	runtimeIdentity := identity.Get()
	if runtimeIdentity.InstanceID != reg.instance.ID {
		t.Fatalf("runtime identity instance id = %q, want registry instance id %q", runtimeIdentity.InstanceID, reg.instance.ID)
	}
	if runtimeIdentity.Address != reg.instance.Address {
		t.Fatalf("runtime identity address = %q, want registry address %q", runtimeIdentity.Address, reg.instance.Address)
	}
	if boundEvent.Service != "payment" {
		t.Fatalf("bound event service = %q, want payment", boundEvent.Service)
	}
	if boundEvent.GRPCListenAddr == "" || boundEvent.HTTPListenAddr == "" {
		t.Fatalf("bound event listen addresses = %+v, want non-empty", boundEvent)
	}
	if boundEvent.AdvertiseGRPCAddr != reg.instance.Address {
		t.Fatalf("bound event grpc advertise = %q, want %q", boundEvent.AdvertiseGRPCAddr, reg.instance.Address)
	}
	if boundEvent.AdvertiseHTTPAddr != reg.instance.Metadata["advertise_http_addr"] {
		t.Fatalf("bound event http advertise = %q, want %q", boundEvent.AdvertiseHTTPAddr, reg.instance.Metadata["advertise_http_addr"])
	}
}

func TestAddServiceRegistrationKeepsExplicitAdvertiseAddressWhenListenPortIsZero(t *testing.T) {
	reg := &captureRegistry{}
	app := lifecycle.New("payment")
	cfg := ComponentConfig{
		Config: Config{
			Service: ServiceConfig{
				GRPCAddr:          "127.0.0.1:0",
				AdvertiseGRPCAddr: "payment:9001",
				HTTPAddr:          "127.0.0.1:0",
				AdvertiseHTTPAddr: "payment-http:9101",
			},
		},
		Spec:        Spec{Name: "payment", RegisterGRPC: func(grpc.ServiceRegistrar) {}},
		Registry:    reg,
		RuntimeMode: "distributed",
	}
	controlPlane := runtimemetrics.NewControlPlaneMetrics("payment")
	grpcService, err := addGRPCService(app, cfg, controlPlane)
	if err != nil {
		t.Fatalf("add grpc service: %v", err)
	}
	if err := addServiceRegistration(app, cfg, controlPlane, grpcService); err != nil {
		t.Fatalf("add registration: %v", err)
	}
	if err := app.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Stop(context.Background())
	})

	if reg.instance.Address != "payment:9001" {
		t.Fatalf("instance address = %q, want payment:9001", reg.instance.Address)
	}
	if reg.instance.Metadata["advertise_http_addr"] != "payment-http:9101" {
		t.Fatalf("advertise_http_addr metadata = %q, want payment-http:9101", reg.instance.Metadata["advertise_http_addr"])
	}
}

type captureRegistry struct {
	instance registry.ServiceInstance
}

func (r *captureRegistry) Register(ctx context.Context, instance registry.ServiceInstance) (registry.Registration, error) {
	r.instance = instance
	return captureRegistration{}, nil
}

type captureRegistration struct{}

func (captureRegistration) Renew(ctx context.Context) error {
	return nil
}

func (captureRegistration) Deregister(ctx context.Context) error {
	return nil
}
