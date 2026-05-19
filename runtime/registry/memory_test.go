package registry

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRegistryRegisterRenewDeregister(t *testing.T) {
	reg := NewMemoryRegistry()
	ctx := context.Background()

	lease, err := reg.Register(ctx, ServiceInstance{ID: "one", Name: "user", Address: "127.0.0.1:9001"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if got, err := reg.Instances(ctx, "user"); err != nil || len(got) != 1 {
		t.Fatalf("got %d instances, want 1", len(got))
	}
	if err := lease.Renew(ctx); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := lease.Deregister(ctx); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if got, err := reg.Instances(ctx, "user"); err != nil || len(got) != 0 {
		t.Fatalf("got %d instances, want 0", len(got))
	}
}

func TestMemoryRegistryImplementsInstanceStore(t *testing.T) {
	reg := NewMemoryRegistry()
	ctx := context.Background()
	metadata := map[string]string{"zone": "a"}
	_, err := reg.Register(ctx, ServiceInstance{ID: "one", Name: "service", Address: "127.0.0.1:9001", Metadata: metadata})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	metadata["zone"] = "mutated"

	services, err := reg.Services(ctx)
	if err != nil {
		t.Fatalf("services: %v", err)
	}
	if len(services) != 1 || services[0] != "service" {
		t.Fatalf("services = %#v", services)
	}

	instance, err := reg.Instance(ctx, "service", "one")
	if err != nil {
		t.Fatalf("instance: %v", err)
	}
	if instance.Metadata["zone"] != "a" {
		t.Fatalf("metadata was not copied: %#v", instance.Metadata)
	}
	instance.Metadata["zone"] = "caller-mutated"

	instance, err = reg.Instance(ctx, "service", "one")
	if err != nil {
		t.Fatalf("second instance: %v", err)
	}
	if instance.Metadata["zone"] != "a" {
		t.Fatalf("stored metadata was mutated: %#v", instance.Metadata)
	}

	if err := reg.Delete(ctx, "service", "one"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := reg.Instance(ctx, "service", "one"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("missing instance error = %v", err)
	}
}

func TestMemoryRegistryWatchSendsLargeInitialSnapshot(t *testing.T) {
	reg := NewMemoryRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 16; i++ {
		id := string(rune('a' + i))
		if _, err := reg.Register(ctx, ServiceInstance{ID: id, Name: "service", Address: "127.0.0.1:9" + id}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	events, err := reg.Watch(ctx, "service")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	for i := 0; i < 16; i++ {
		event := <-events
		if event.Type != EventAdded {
			t.Fatalf("event type = %s", event.Type)
		}
	}
}
