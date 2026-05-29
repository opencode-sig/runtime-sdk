package discovery

import (
	"context"
	"fmt"
	"strings"

	infraetcd "github.com/opencode-sig/runtime-sdk/infra/etcd"
	"github.com/opencode-sig/runtime-sdk/runtime/defaults"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// RegistryDiscoveryConfig describes a registry/discovery backend.
type RegistryDiscoveryConfig struct {
	Provider string
	Etcd     EtcdConfig
}

// EtcdConfig describes the etcd entrypoint and keyspace for registry/discovery.
type EtcdConfig struct {
	Endpoints []string
	Prefix    string
}

// RegistryDiscovery groups a registry, discovery source, and owned clients.
type RegistryDiscovery struct {
	Registry      registry.Registry
	Discovery     Discovery
	InstanceStore registry.InstanceStore
	EtcdClient    *clientv3.Client
}

// RegistryEnabled reports whether provider requires external registry/discovery.
func RegistryEnabled(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), "etcd")
}

// NewRegistryDiscovery creates a registry/discovery pair for the configured provider.
func NewRegistryDiscovery(ctx context.Context, cfg RegistryDiscoveryConfig) (*RegistryDiscovery, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "", "none", "memory":
		memoryRegistry := registry.NewMemoryRegistry()
		return &RegistryDiscovery{
			Registry:      memoryRegistry,
			Discovery:     NewMemoryDiscovery(memoryRegistry),
			InstanceStore: memoryRegistry,
		}, nil
	case "etcd":
		etcdCfg := normalizeEtcdConfig(cfg.Etcd)
		if len(etcdCfg.Endpoints) == 0 {
			return nil, fmt.Errorf("registry etcd endpoints are required")
		}
		client, err := infraetcd.NewClientAndWait(ctx, infraetcd.Config{Endpoints: etcdCfg.Endpoints})
		if err != nil {
			return nil, err
		}
		reg := registry.NewEtcdRegistry(client, etcdCfg.Prefix)
		return &RegistryDiscovery{
			Registry:      reg,
			Discovery:     NewEtcdDiscovery(client, etcdCfg.Prefix),
			InstanceStore: reg,
			EtcdClient:    client,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported registry provider %q", cfg.Provider)
	}
}

// Close releases clients owned by the registry/discovery bundle.
func (r *RegistryDiscovery) Close() error {
	if r == nil || r.EtcdClient == nil {
		return nil
	}
	return r.EtcdClient.Close()
}

func normalizeEtcdConfig(cfg EtcdConfig) EtcdConfig {
	out := EtcdConfig{
		Endpoints: append([]string(nil), cfg.Endpoints...),
		Prefix:    defaults.CleanPrefix(cfg.Prefix, defaults.RegistryPrefix),
	}
	for i, endpoint := range out.Endpoints {
		out.Endpoints[i] = strings.TrimSpace(endpoint)
	}
	out.Endpoints = compactStrings(out.Endpoints)
	return out
}

func compactStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
