package discovery

import (
	"context"
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	mvccpb "go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdDiscoverySnapshotDiffEmitsRemovedAddedAndUpdated(t *testing.T) {
	old := map[string]registry.ServiceInstance{
		"removed": {ID: "removed", Name: "user", Address: "127.0.0.1:9001"},
		"updated": {ID: "updated", Name: "user", Address: "127.0.0.1:9002",
			Metadata: map[string]string{"version": "old"}},
	}
	next := map[string]registry.ServiceInstance{
		"added": {ID: "added", Name: "user", Address: "127.0.0.1:9003"},
		"updated": {ID: "updated", Name: "user", Address: "127.0.0.1:9002",
			Metadata: map[string]string{"version": "new"}},
	}

	out := make(chan DiscoveryEvent, 4)
	if !emitSnapshotDiff(context.Background(), out, old, next) {
		t.Fatal("emit diff returned false")
	}
	close(out)

	events := drainEvents(out)
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	assertEvent(t, events[0], EventRemoved, "removed")
	assertEvent(t, events[1], EventAdded, "added")
	assertEvent(t, events[2], EventUpdated, "updated")
}

func TestEtcdDiscoveryDecodeEventUsesUpdatedForModify(t *testing.T) {
	data := marshalDiscoveryInstance(t, registry.ServiceInstance{
		ID:      "one",
		Name:    "user",
		Address: "127.0.0.1:9001",
	})
	event, err := decodeEvent(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:            []byte("/registry/user/one"),
			Value:          data,
			CreateRevision: 1,
			ModRevision:    2,
		},
	})
	if err != nil {
		t.Fatalf("decode modify event: %v", err)
	}
	if event.Type != EventUpdated || event.Instance.ID != "one" {
		t.Fatalf("event = %#v", event)
	}
}

func TestEtcdDiscoveryApplySnapshotEvent(t *testing.T) {
	snapshot := map[string]registry.ServiceInstance{}
	instance := registry.ServiceInstance{ID: "one", Name: "user", Address: "127.0.0.1:9001"}
	applySnapshotEvent(snapshot, DiscoveryEvent{Type: EventAdded, Instance: instance})
	if len(snapshot) != 1 {
		t.Fatalf("snapshot after add = %#v", snapshot)
	}
	applySnapshotEvent(snapshot, DiscoveryEvent{Type: EventRemoved, Instance: instance})
	if len(snapshot) != 0 {
		t.Fatalf("snapshot after remove = %#v", snapshot)
	}
}

func TestEtcdDiscoveryWatchStatusTransitions(t *testing.T) {
	discovery := NewEtcdDiscovery(nil, "/registry")
	snapshot := map[string]registry.ServiceInstance{
		"one": {ID: "one", Name: "user", Address: "127.0.0.1:9001"},
	}

	discovery.markWatchLoaded("user", snapshot, 10)
	status := discovery.Status("user")
	if !status.Running || !status.Healthy || status.Stale || status.InstanceCount != 1 || status.Revision != 10 {
		t.Fatalf("loaded status = %#v", status)
	}

	snapshot["two"] = registry.ServiceInstance{ID: "two", Name: "user", Address: "127.0.0.1:9002"}
	discovery.markWatchEvent("user", snapshot, 11)
	status = discovery.Status("user")
	if !status.Healthy || status.InstanceCount != 2 || status.Revision != 11 || status.LastEventAt.IsZero() {
		t.Fatalf("event status = %#v", status)
	}

	discovery.markWatchError("user", context.DeadlineExceeded)
	status = discovery.Status("user")
	if status.Healthy || !status.Stale || status.Reconnects != 1 || status.LastError == "" {
		t.Fatalf("error status = %#v", status)
	}

	discovery.markWatchStopped("user")
	status = discovery.Status("user")
	if status.Running || status.Healthy {
		t.Fatalf("stopped status = %#v", status)
	}
}

func drainEvents(ch <-chan DiscoveryEvent) []DiscoveryEvent {
	var events []DiscoveryEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func assertEvent(t *testing.T, event DiscoveryEvent, eventType EventType, id string) {
	t.Helper()
	if event.Type != eventType || event.Instance.ID != id {
		t.Fatalf("event = %#v, want %s %s", event, eventType, id)
	}
}

func marshalDiscoveryInstance(t *testing.T, instance registry.ServiceInstance) []byte {
	t.Helper()
	data, err := registry.MarshalInstance(instance)
	if err != nil {
		t.Fatalf("marshal instance: %v", err)
	}
	return data
}
