package discovery

import (
	"context"
	"fmt"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdDiscovery struct {
	client   *clientv3.Client
	keyspace registry.Keyspace
}

// NewEtcdDiscovery creates service discovery backed by the etcd registry keyspace.
func NewEtcdDiscovery(client *clientv3.Client, prefix string) *EtcdDiscovery {
	return &EtcdDiscovery{
		client:   client,
		keyspace: registry.NewKeyspace(prefix),
	}
}

// Resolve reads all current instances for a service.
func (d *EtcdDiscovery) Resolve(ctx context.Context, service string) ([]registry.ServiceInstance, error) {
	if d.client == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	resp, err := d.client.Get(ctx, d.keyspace.ServicePrefix(service), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	instances := make([]registry.ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		instance, err := registry.UnmarshalInstance(kv.Value)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// Watch watches instance changes for a service.
//
// The returned channel first emits added events for current instances and then
// emits incremental etcd watch events.
func (d *EtcdDiscovery) Watch(ctx context.Context, service string) (<-chan DiscoveryEvent, error) {
	if d.client == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	keyPrefix := d.keyspace.ServicePrefix(service)
	current, err := d.client.Get(ctx, keyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	out := make(chan DiscoveryEvent, len(current.Kvs)+8)
	for _, kv := range current.Kvs {
		instance, err := registry.UnmarshalInstance(kv.Value)
		if err != nil {
			return nil, err
		}
		out <- DiscoveryEvent{Type: EventAdded, Instance: instance}
	}

	watch := d.client.Watch(ctx, keyPrefix, clientv3.WithPrefix(), clientv3.WithPrevKV(), clientv3.WithRev(current.Header.Revision+1))
	go func() {
		defer close(out)
		for resp := range watch {
			for _, event := range resp.Events {
				discoveryEvent, err := decodeEvent(event)
				if err != nil {
					continue
				}
				select {
				case out <- discoveryEvent:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// decodeEvent converts an etcd watch event into a DiscoveryEvent.
func decodeEvent(event *clientv3.Event) (DiscoveryEvent, error) {
	switch event.Type {
	case clientv3.EventTypePut:
		instance, err := registry.UnmarshalInstance(event.Kv.Value)
		if err != nil {
			return DiscoveryEvent{}, err
		}
		return DiscoveryEvent{Type: EventAdded, Instance: instance}, nil
	case clientv3.EventTypeDelete:
		if event.PrevKv == nil {
			return DiscoveryEvent{}, fmt.Errorf("delete event has no previous value")
		}
		instance, err := registry.UnmarshalInstance(event.PrevKv.Value)
		if err != nil {
			return DiscoveryEvent{}, err
		}
		return DiscoveryEvent{Type: EventRemoved, Instance: instance}, nil
	default:
		return DiscoveryEvent{}, fmt.Errorf("unsupported event type %v", event.Type)
	}
}
