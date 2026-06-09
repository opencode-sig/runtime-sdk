package servicekit

import (
	"context"
	"testing"

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
