package control

import (
	"context"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestCleanPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default", in: "", want: "/runtime/control/commands"},
		{name: "trims spaces", in: "  runtime/control  ", want: "/runtime/control"},
		{name: "normalizes slashes", in: "///runtime/control///", want: "/runtime/control"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanPrefix(tt.in); got != tt.want {
				t.Fatalf("cleanPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCommandFromEvent(t *testing.T) {
	createdAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	event := &clientv3.Event{
		Type: mvccpb.PUT,
		Kv: &mvccpb.KeyValue{
			Value: []byte(`{"command":"rebuild","service":"payment","instance_id":"payment-1","reason":"config changed","created_at":"` + createdAt.Format(time.RFC3339Nano) + `"}`),
		},
	}

	command, ok := commandFromEvent(event)
	if !ok {
		t.Fatal("commandFromEvent returned false")
	}
	if command.Command != CommandRebuild || command.Service != "payment" || command.InstanceID != "payment-1" {
		t.Fatalf("unexpected command: %+v", command)
	}
	if !command.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", command.CreatedAt, createdAt)
	}
}

func TestCommandFromEventIgnoresInvalidEvents(t *testing.T) {
	tests := []struct {
		name  string
		event *clientv3.Event
	}{
		{name: "nil", event: nil},
		{name: "delete", event: &clientv3.Event{Type: mvccpb.DELETE, Kv: &mvccpb.KeyValue{Value: []byte(`{"command":"rebuild"}`)}}},
		{name: "missing key value", event: &clientv3.Event{Type: mvccpb.PUT}},
		{name: "invalid json", event: &clientv3.Event{Type: mvccpb.PUT, Kv: &mvccpb.KeyValue{Value: []byte(`{`)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if command, ok := commandFromEvent(tt.event); ok {
				t.Fatalf("commandFromEvent returned (%+v, true), want false", command)
			}
		})
	}
}

func TestEtcdStoreRequiresClient(t *testing.T) {
	store := NewEtcdStore(nil, "")

	if err := store.Publish(context.Background(), Command{Command: CommandRebuild, Service: "payment"}); err == nil {
		t.Fatal("Publish error = nil, want client error")
	}
	if _, err := store.Watch(context.Background(), "payment"); err == nil {
		t.Fatal("Watch error = nil, want client error")
	}
}
