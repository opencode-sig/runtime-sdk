package discovery

import (
	"context"
	"fmt"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

type MemoryDiscovery struct {
	registry *registry.MemoryRegistry
}

// NewMemoryDiscovery creates service discovery backed by MemoryRegistry.
func NewMemoryDiscovery(reg *registry.MemoryRegistry) *MemoryDiscovery {
	return &MemoryDiscovery{registry: reg}
}

// Resolve returns service instances from the in-memory registry.
func (d *MemoryDiscovery) Resolve(ctx context.Context, service string) ([]registry.ServiceInstance, error) {
	if d.registry == nil {
		return nil, fmt.Errorf("memory registry is required")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return d.registry.Instances(ctx, service)
}

// Watch converts memory registry events into discovery events.
func (d *MemoryDiscovery) Watch(ctx context.Context, service string) (<-chan DiscoveryEvent, error) {
	if d.registry == nil {
		return nil, fmt.Errorf("memory registry is required")
	}
	source, err := d.registry.Watch(ctx, service)
	if err != nil {
		return nil, err
	}

	out := make(chan DiscoveryEvent, 8)
	go func() {
		defer close(out)
		for event := range source {
			select {
			case out <- DiscoveryEvent{Type: EventType(event.Type), Instance: event.Instance}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
