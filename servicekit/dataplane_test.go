package servicekit

import (
	"context"
	"errors"
	"testing"
)

func TestManagerRebuildReplacesGeneration(t *testing.T) {
	var first, second fakeDataPlane
	planes := []*fakeDataPlane{&first, &second}
	index := 0

	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		plane := planes[index]
		index++
		return plane, nil
	}, nil)

	if _, err := manager.Rebuild(context.Background(), "first"); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if !first.started || first.stopped {
		t.Fatalf("first plane state = started:%v stopped:%v", first.started, first.stopped)
	}

	if _, err := manager.Rebuild(context.Background(), "second"); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if !first.stopped {
		t.Fatal("first generation was not stopped")
	}
	if !second.started || second.stopped {
		t.Fatalf("second plane state = started:%v stopped:%v", second.started, second.stopped)
	}
	status := manager.Status()
	if !status.Running || status.Generation != "fake" || status.LastReason != "second" || status.LastError != "" {
		t.Fatalf("unexpected status after rebuild: %+v", status)
	}
}

func TestManagerRebuildStartFailureClearsCurrent(t *testing.T) {
	startErr := errors.New("start failed")
	first := &fakeDataPlane{}
	second := &fakeDataPlane{startErr: startErr}
	planes := []*fakeDataPlane{first, second}
	index := 0

	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		plane := planes[index]
		index++
		return plane, nil
	}, nil)

	if _, err := manager.Rebuild(context.Background(), "first"); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if _, err := manager.Rebuild(context.Background(), "second"); !errors.Is(err, startErr) {
		t.Fatalf("second rebuild error = %v", err)
	}
	if !first.stopped {
		t.Fatal("old generation was not stopped")
	}
	if err := manager.Health(context.Background()); err == nil {
		t.Fatal("health should fail after failed replacement")
	}
	status := manager.Status()
	if status.Running || status.Generation != "" || status.LastError == "" {
		t.Fatalf("unexpected status after failed rebuild: %+v", status)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("stop should be idempotent after failed replacement: %v", err)
	}
}

func TestManagerRuntimeIdentityUsesCurrentDataPlane(t *testing.T) {
	planes := []*fakeIdentifiedDataPlane{
		{identity: RuntimeIdentity{Service: "order", Address: "127.0.0.1:2001", InstanceID: "instance-a"}},
		{identity: RuntimeIdentity{Service: "order", Address: "127.0.0.1:2002", InstanceID: "instance-b"}},
	}
	index := 0
	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		plane := planes[index]
		index++
		return plane, nil
	}, nil)

	if identity := manager.RuntimeIdentity(); identity.InstanceID != "" {
		t.Fatalf("identity before start = %+v, want empty", identity)
	}
	if _, err := manager.Rebuild(context.Background(), "first"); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if identity := manager.RuntimeIdentity(); identity.InstanceID != "instance-a" {
		t.Fatalf("identity after first rebuild = %+v, want instance-a", identity)
	}
	if _, err := manager.Rebuild(context.Background(), "second"); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if identity := manager.RuntimeIdentity(); identity.InstanceID != "instance-b" {
		t.Fatalf("identity after second rebuild = %+v, want instance-b", identity)
	}
}

type fakeDataPlane struct {
	started  bool
	stopped  bool
	startErr error
}

func (p *fakeDataPlane) Generation() string {
	return "fake"
}

func (p *fakeDataPlane) Start(ctx context.Context) error {
	p.started = true
	return p.startErr
}

func (p *fakeDataPlane) Stop(ctx context.Context) error {
	p.stopped = true
	return nil
}

func (p *fakeDataPlane) Health(ctx context.Context) error {
	return nil
}

type fakeIdentifiedDataPlane struct {
	fakeDataPlane
	identity RuntimeIdentity
}

func (p *fakeIdentifiedDataPlane) RuntimeIdentity() RuntimeIdentity {
	return p.identity
}
