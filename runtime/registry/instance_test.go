package registry

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestInstanceIDUsesServicePrefixAndMD5(t *testing.T) {
	id := InstanceID("order", "127.0.0.1:2002")

	if ok := regexp.MustCompile(`^order-[a-f0-9]{32}$`).MatchString(id); !ok {
		t.Fatalf("instance id format = %q", id)
	}
	if strings.ContainsAny(id, ":/\\ ") {
		t.Fatalf("instance id contains unsanitized separator: %q", id)
	}
}

func TestInstanceIDIsStableInsideProcess(t *testing.T) {
	first := InstanceID("order", "127.0.0.1:2002")
	second := InstanceID("order", "127.0.0.1:2002")
	if first != second {
		t.Fatalf("instance id should be stable in one process: %q != %q", first, second)
	}
}

func TestNewServiceInstanceSetsStartedAt(t *testing.T) {
	instance := NewServiceInstance("order", "127.0.0.1:2002", nil)
	if instance.Hostname == "" {
		t.Fatal("hostname was not set")
	}
	if instance.StartedAt.IsZero() {
		t.Fatal("started_at was not set")
	}
	if instance.LastSeen.IsZero() {
		t.Fatal("last_seen was not set")
	}
}

func TestMarshalInstanceUsesSnakeCaseContract(t *testing.T) {
	startedAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	instance := ServiceInstance{
		ID:                  "order-1",
		Name:                "order",
		Address:             "127.0.0.1:2002",
		Hostname:            "node-a",
		Metadata:            map[string]string{"runtime": "distributed"},
		StartedAt:           startedAt,
		LastSeen:            startedAt.Add(time.Second),
		DataPlaneStartedAt:  startedAt.Add(2 * time.Second),
		DataPlaneGeneration: "order-generation-1",
	}

	data, err := MarshalInstance(instance)
	if err != nil {
		t.Fatalf("marshal instance: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw instance: %v", err)
	}

	for _, key := range []string{
		"id",
		"name",
		"address",
		"hostname",
		"metadata",
		"started_at",
		"last_seen",
		"data_plane_started_at",
		"data_plane_generation",
	} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("expected snake_case key %q in %s", key, data)
		}
	}
	for _, key := range []string{
		"ID",
		"Name",
		"Address",
		"Hostname",
		"Metadata",
		"StartedAt",
		"LastSeen",
		"DataPlaneStartedAt",
		"DataPlaneGeneration",
	} {
		if _, ok := raw[key]; ok {
			t.Fatalf("legacy key %q must not be emitted in %s", key, data)
		}
	}
}

func TestUnmarshalInstanceRequiresSnakeCaseContract(t *testing.T) {
	instance, err := UnmarshalInstance([]byte(`{
		"id": "order-1",
		"name": "order",
		"address": "127.0.0.1:2002",
		"data_plane_generation": "order-generation-1"
	}`))
	if err != nil {
		t.Fatalf("unmarshal snake_case instance: %v", err)
	}
	if instance.ID != "order-1" || instance.Address != "127.0.0.1:2002" || instance.DataPlaneGeneration != "order-generation-1" {
		t.Fatalf("snake_case instance was not decoded: %#v", instance)
	}
}
