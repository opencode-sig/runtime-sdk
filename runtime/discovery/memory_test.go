package discovery

import (
	"context"
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func TestMemoryDiscoveryResolve(t *testing.T) {
	reg := registry.NewMemoryRegistry()
	_, err := reg.Register(context.Background(), registry.ServiceInstance{ID: "one", Name: "user", Address: "127.0.0.1:9001"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	discovery := NewMemoryDiscovery(reg)
	instances, err := discovery.Resolve(context.Background(), "user")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != "one" {
		t.Fatalf("unexpected instances: %#v", instances)
	}
}
