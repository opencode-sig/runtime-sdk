package registry

import (
	"regexp"
	"strings"
	"testing"
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
