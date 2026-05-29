package discovery

import (
	"context"
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

func TestRegistryEnabled(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{provider: "", want: false},
		{provider: "none", want: false},
		{provider: "memory", want: false},
		{provider: "etcd", want: true},
		{provider: " ETCD ", want: true},
	}
	for _, tt := range tests {
		if got := RegistryEnabled(tt.provider); got != tt.want {
			t.Fatalf("RegistryEnabled(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}

func TestNewRegistryDiscoveryMemory(t *testing.T) {
	bundle, err := NewRegistryDiscovery(context.Background(), RegistryDiscoveryConfig{Provider: "memory"})
	if err != nil {
		t.Fatalf("new registry discovery: %v", err)
	}
	if bundle.Registry == nil || bundle.Discovery == nil || bundle.InstanceStore == nil {
		t.Fatalf("bundle is incomplete: %#v", bundle)
	}
	if bundle.EtcdClient != nil {
		t.Fatal("memory provider should not create etcd client")
	}
	instance := registry.NewServiceInstance("user", "127.0.0.1:2001", nil)
	if _, err := bundle.Registry.Register(context.Background(), instance); err != nil {
		t.Fatalf("register memory instance: %v", err)
	}
	instances, err := bundle.Discovery.Resolve(context.Background(), "user")
	if err != nil {
		t.Fatalf("resolve memory instance: %v", err)
	}
	if len(instances) != 1 || instances[0].Address != "127.0.0.1:2001" {
		t.Fatalf("instances = %#v", instances)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("close memory bundle: %v", err)
	}
}

func TestNewRegistryDiscoveryRejectsUnsupportedProvider(t *testing.T) {
	if _, err := NewRegistryDiscovery(context.Background(), RegistryDiscoveryConfig{Provider: "consul"}); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestNewRegistryDiscoveryRequiresEtcdEndpoints(t *testing.T) {
	_, err := NewRegistryDiscovery(context.Background(), RegistryDiscoveryConfig{
		Provider: "etcd",
		Etcd:     EtcdConfig{Prefix: "/runtime/registry"},
	})
	if err == nil {
		t.Fatal("expected missing endpoints error")
	}
}

func TestNormalizeEtcdConfig(t *testing.T) {
	cfg := normalizeEtcdConfig(EtcdConfig{
		Endpoints: []string{" 127.0.0.1:2379 ", "", "127.0.0.1:2380"},
		Prefix:    "runtime/registry/",
	})
	if cfg.Prefix != "/runtime/registry" {
		t.Fatalf("prefix = %q", cfg.Prefix)
	}
	if len(cfg.Endpoints) != 2 || cfg.Endpoints[0] != "127.0.0.1:2379" || cfg.Endpoints[1] != "127.0.0.1:2380" {
		t.Fatalf("endpoints = %#v", cfg.Endpoints)
	}
}
