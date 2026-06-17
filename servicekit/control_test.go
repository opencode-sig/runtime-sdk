package servicekit

import (
	"context"
	"testing"
	"time"

	runtimecontrol "github.com/opencode-sig/runtime-sdk/runtime/control"
)

func TestControlWatcherRebuildsOnCommand(t *testing.T) {
	store := newFakeControlStore()
	rebuilt := make(chan string, 1)
	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		rebuilt <- "rebuilt"
		return &fakeDataPlane{}, nil
	}, nil)

	watcher, err := NewControlWatcher(store, manager, ControlWatcherConfig{Service: "order", InstanceID: "instance-a"}, nil)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = watcher.Stop(stopCtx)
	}()

	store.commands <- runtimecontrol.Command{Command: runtimecontrol.CommandRebuild, Service: "order", InstanceID: "instance-a"}
	select {
	case <-rebuilt:
	case <-time.After(time.Second):
		t.Fatal("watcher did not rebuild data plane")
	}
}

func TestControlWatcherIgnoresOtherInstance(t *testing.T) {
	store := newFakeControlStore()
	rebuilt := make(chan string, 1)
	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		rebuilt <- "rebuilt"
		return &fakeDataPlane{}, nil
	}, nil)

	watcher, err := NewControlWatcher(store, manager, ControlWatcherConfig{Service: "order", InstanceID: "instance-a"}, nil)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = watcher.Stop(stopCtx)
	}()

	store.commands <- runtimecontrol.Command{Command: runtimecontrol.CommandRebuild, Service: "order", InstanceID: "instance-b"}
	select {
	case <-rebuilt:
		t.Fatal("watcher rebuilt for another instance")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestControlWatcherUsesRuntimeIdentity(t *testing.T) {
	store := newFakeControlStore()
	rebuilt := make(chan string, 2)
	planes := []DataPlane{
		&fakeIdentifiedDataPlane{identity: RuntimeIdentity{Service: "order", InstanceID: "instance-current"}},
		&fakeIdentifiedDataPlane{identity: RuntimeIdentity{Service: "order", InstanceID: "instance-next"}},
	}
	index := 0
	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		rebuilt <- "rebuilt"
		plane := planes[index]
		index++
		return plane, nil
	}, nil)
	if _, err := manager.Rebuild(context.Background(), "initial"); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	<-rebuilt

	watcher, err := NewControlWatcher(store, manager, ControlWatcherConfig{Service: "order", InstanceID: "fallback-instance"}, nil)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = watcher.Stop(stopCtx)
	}()

	store.commands <- runtimecontrol.Command{Command: runtimecontrol.CommandRebuild, Service: "order", InstanceID: "instance-current"}
	select {
	case <-rebuilt:
	case <-time.After(time.Second):
		t.Fatal("watcher did not rebuild for current runtime identity")
	}
}

func TestControlWatcherRuntimeIdentityUpdatesAfterRebuild(t *testing.T) {
	store := newFakeControlStore()
	rebuilt := make(chan string, 3)
	planes := []DataPlane{
		&fakeIdentifiedDataPlane{identity: RuntimeIdentity{Service: "order", InstanceID: "instance-a"}},
		&fakeIdentifiedDataPlane{identity: RuntimeIdentity{Service: "order", InstanceID: "instance-b"}},
		&fakeIdentifiedDataPlane{identity: RuntimeIdentity{Service: "order", InstanceID: "instance-c"}},
	}
	index := 0
	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		rebuilt <- "rebuilt"
		plane := planes[index]
		index++
		return plane, nil
	}, nil)
	if _, err := manager.Rebuild(context.Background(), "initial"); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	<-rebuilt

	watcher, err := NewControlWatcher(store, manager, ControlWatcherConfig{Service: "order", InstanceID: "fallback-instance"}, nil)
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	if err := watcher.Start(context.Background()); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = watcher.Stop(stopCtx)
	}()

	store.commands <- runtimecontrol.Command{Command: runtimecontrol.CommandRebuild, Service: "order", InstanceID: "instance-a"}
	select {
	case <-rebuilt:
	case <-time.After(time.Second):
		t.Fatal("watcher did not rebuild for initial runtime identity")
	}

	store.commands <- runtimecontrol.Command{Command: runtimecontrol.CommandRebuild, Service: "order", InstanceID: "instance-a"}
	select {
	case <-rebuilt:
		t.Fatal("watcher rebuilt for stale runtime identity")
	case <-time.After(50 * time.Millisecond):
	}

	store.commands <- runtimecontrol.Command{Command: runtimecontrol.CommandRebuild, Service: "order", InstanceID: "instance-b"}
	select {
	case <-rebuilt:
	case <-time.After(time.Second):
		t.Fatal("watcher did not rebuild for updated runtime identity")
	}
}

type fakeControlStore struct {
	commands chan runtimecontrol.Command
}

func newFakeControlStore() *fakeControlStore {
	return &fakeControlStore{commands: make(chan runtimecontrol.Command, 1)}
}

func (s *fakeControlStore) Publish(ctx context.Context, command runtimecontrol.Command) error {
	s.commands <- command
	return nil
}

func (s *fakeControlStore) Watch(ctx context.Context, service string) (<-chan runtimecontrol.Command, error) {
	return s.commands, nil
}
