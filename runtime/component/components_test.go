package component

import (
	"context"
	"errors"
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func TestCloseComponentClosesOnce(t *testing.T) {
	calls := 0
	component := NewCloseComponent(func() error {
		calls++
		return nil
	})

	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := component.Health(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := component.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := component.Stop(context.Background()); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	if calls != 1 {
		t.Fatalf("close calls = %d", calls)
	}
	if err := component.Health(context.Background()); err == nil {
		t.Fatal("closed component should not be healthy")
	}
}

func TestCloseComponentRequiresCloseFunc(t *testing.T) {
	component := NewCloseComponent(nil)
	err := component.Start(context.Background())
	if err == nil {
		t.Fatal("expected start error")
	}
}

func TestCloseComponentReturnsCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	component := NewCloseComponent(func() error {
		return closeErr
	})

	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := component.Stop(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("stop error = %v", err)
	}
}

func TestRegistrationComponentRecordsDataPlaneInfo(t *testing.T) {
	reg := &fakeRegistry{}
	instance := registry.NewServiceInstance("order", "127.0.0.1:2002", nil)
	component := NewRegistrationComponent(reg, instance, nil).WithDataPlaneGeneration("order-1")

	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if reg.instance.DataPlaneStartedAt.IsZero() {
		t.Fatal("data plane started_at was not set")
	}
	if reg.instance.DataPlaneGeneration != "order-1" {
		t.Fatalf("data plane generation = %q", reg.instance.DataPlaneGeneration)
	}
}

type fakeRegistry struct {
	instance registry.ServiceInstance
}

func (r *fakeRegistry) Register(ctx context.Context, instance registry.ServiceInstance) (registry.Registration, error) {
	r.instance = instance
	return fakeRegistration{}, nil
}

type fakeRegistration struct{}

func (fakeRegistration) Deregister(ctx context.Context) error {
	return nil
}

func (fakeRegistration) Renew(ctx context.Context) error {
	return nil
}
