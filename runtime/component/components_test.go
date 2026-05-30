package component

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	t.Cleanup(func() {
		_ = component.Stop(context.Background())
	})
	if reg.instance.DataPlaneStartedAt.IsZero() {
		t.Fatal("data plane started_at was not set")
	}
	if reg.instance.DataPlaneGeneration != "order-1" {
		t.Fatalf("data plane generation = %q", reg.instance.DataPlaneGeneration)
	}
}

func TestRegistrationComponentHealthReRegistersAfterExpiredRegistration(t *testing.T) {
	reg := &fakeRegistry{
		renewErrors: []error{registry.ErrRegistrationExpired},
	}
	component := NewRegistrationComponent(reg, registry.NewServiceInstance("order", "127.0.0.1:2002", nil), nil)
	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = component.Stop(context.Background())
	})

	if err := component.Health(context.Background()); err != nil {
		t.Fatalf("health should recover registration: %v", err)
	}
	if got := reg.registerCalls(); got != 2 {
		t.Fatalf("register calls = %d, want 2", got)
	}
	instances := reg.registeredInstances()
	if got, want := instances[1].DataPlaneStartedAt, instances[0].DataPlaneStartedAt; !got.Equal(want) {
		t.Fatalf("recovered data_plane_started_at = %s, want %s", got, want)
	}
}

func TestRegistrationComponentHealthDoesNotReRegisterTransientRenewFailure(t *testing.T) {
	renewErr := errors.New("etcd unavailable")
	reg := &fakeRegistry{
		renewErrors: []error{renewErr},
	}
	component := NewRegistrationComponent(reg, registry.NewServiceInstance("order", "127.0.0.1:2002", nil), nil)
	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = component.Stop(context.Background())
	})

	if err := component.Health(context.Background()); !errors.Is(err, renewErr) {
		t.Fatalf("health error = %v, want %v", err, renewErr)
	}
	if got := reg.registerCalls(); got != 1 {
		t.Fatalf("register calls = %d, want 1", got)
	}
}

func TestRegistrationComponentConcurrentHealthReRegistersOnce(t *testing.T) {
	reg := &fakeRegistry{
		renewErrors: []error{registry.ErrRegistrationExpired},
	}
	component := NewRegistrationComponent(reg, registry.NewServiceInstance("order", "127.0.0.1:2002", nil), nil)
	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = component.Stop(context.Background())
	})

	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	for i := 0; i < cap(errCh); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- component.Health(context.Background())
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("health should recover registration: %v", err)
		}
	}
	if got := reg.registerCalls(); got != 2 {
		t.Fatalf("register calls = %d, want 2", got)
	}
}

func TestRegistrationComponentRenewLoopReRegistersAfterExpiredRegistration(t *testing.T) {
	oldInterval := registrationRenewInterval
	oldTimeout := registrationRenewTimeout
	registrationRenewInterval = 10 * time.Millisecond
	registrationRenewTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		registrationRenewInterval = oldInterval
		registrationRenewTimeout = oldTimeout
	})

	reg := &fakeRegistry{
		renewErrors: []error{registry.ErrRegistrationExpired},
	}
	component := NewRegistrationComponent(reg, registry.NewServiceInstance("order", "127.0.0.1:2002", nil), nil)
	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = component.Stop(context.Background())
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if reg.registerCalls() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("register calls = %d, want at least 2", reg.registerCalls())
}

type fakeRegistry struct {
	mu          sync.Mutex
	instance    registry.ServiceInstance
	instances   []registry.ServiceInstance
	calls       int
	renewErrors []error
}

func (r *fakeRegistry) Register(ctx context.Context, instance registry.ServiceInstance) (registry.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instance = instance
	r.instances = append(r.instances, instance)
	r.calls++
	registration := &fakeRegistration{}
	if len(r.renewErrors) > 0 {
		registration.renewErr = r.renewErrors[0]
		r.renewErrors = r.renewErrors[1:]
	}
	return registration, nil
}

func (r *fakeRegistry) registerCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *fakeRegistry) registeredInstances() []registry.ServiceInstance {
	r.mu.Lock()
	defer r.mu.Unlock()
	instances := make([]registry.ServiceInstance, len(r.instances))
	copy(instances, r.instances)
	return instances
}

type fakeRegistration struct {
	renewErr error
}

func (r *fakeRegistration) Deregister(ctx context.Context) error {
	return nil
}

func (r *fakeRegistration) Renew(ctx context.Context) error {
	if r.renewErr != nil {
		err := r.renewErr
		r.renewErr = nil
		return err
	}
	return nil
}
