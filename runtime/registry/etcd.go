package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultLeaseTTL = 15

// EtcdRegistry manages service instance registration with etcd leases.
type EtcdRegistry struct {
	client   *clientv3.Client
	keyspace Keyspace
	ttl      int64
}

// NewEtcdRegistry creates an etcd registry.
func NewEtcdRegistry(client *clientv3.Client, prefix string) *EtcdRegistry {
	return &EtcdRegistry{
		client:   client,
		keyspace: NewKeyspace(prefix),
		ttl:      defaultLeaseTTL,
	}
}

// Register registers a service instance and creates a keepalive lease.
//
// Instance data is written to /prefix/service/id. The key disappears when the
// lease expires or the registration is explicitly deregistered.
func (r *EtcdRegistry) Register(ctx context.Context, instance ServiceInstance) (Registration, error) {
	if r.client == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if err := validate(instance); err != nil {
		return nil, err
	}
	if instance.LastSeen.IsZero() {
		instance.LastSeen = time.Now().UTC()
	}
	if instance.StartedAt.IsZero() {
		instance.StartedAt = instance.LastSeen
	}

	lease, err := r.client.Grant(ctx, r.ttl)
	if err != nil {
		return nil, err
	}

	key := r.keyspace.ServiceKey(instance.Name, instance.ID)
	data, err := MarshalInstance(instance)
	if err != nil {
		_, _ = r.client.Revoke(context.Background(), lease.ID)
		return nil, err
	}

	if _, err := r.client.Put(ctx, key, string(data), clientv3.WithLease(lease.ID)); err != nil {
		_, _ = r.client.Revoke(context.Background(), lease.ID)
		return nil, err
	}

	keepCtx, cancel := context.WithCancel(context.Background())
	ch, err := r.client.KeepAlive(keepCtx, lease.ID)
	if err != nil {
		cancel()
		_, _ = r.client.Revoke(context.Background(), lease.ID)
		return nil, err
	}

	reg := &etcdRegistration{
		client: r.client,
		key:    key,
		lease:  lease.ID,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go reg.drainKeepAlive(ch)
	return reg, nil
}

// Services lists service names currently present in registry.
func (r *EtcdRegistry) Services(ctx context.Context) ([]string, error) {
	if r.client == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	resp, err := r.client.Get(ctx, r.keyspace.ServicesPrefix(), clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, kv := range resp.Kvs {
		service, ok := r.keyspace.ServiceFromKey(string(kv.Key))
		if ok {
			seen[service] = struct{}{}
		}
	}
	services := make([]string, 0, len(seen))
	for service := range seen {
		services = append(services, service)
	}
	sort.Strings(services)
	return services, nil
}

// Instances returns all instances for a service.
func (r *EtcdRegistry) Instances(ctx context.Context, service string) ([]ServiceInstance, error) {
	if r.client == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if strings.TrimSpace(service) == "" {
		return nil, fmt.Errorf("service name is required")
	}
	resp, err := r.client.Get(ctx, r.keyspace.ServicePrefix(service), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	instances := make([]ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		instance, err := UnmarshalInstance(kv.Value)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// Instance returns one service instance.
func (r *EtcdRegistry) Instance(ctx context.Context, service string, id string) (ServiceInstance, error) {
	if r.client == nil {
		return ServiceInstance{}, fmt.Errorf("etcd client is required")
	}
	if strings.TrimSpace(service) == "" {
		return ServiceInstance{}, fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(id) == "" {
		return ServiceInstance{}, fmt.Errorf("service instance id is required")
	}
	resp, err := r.client.Get(ctx, r.keyspace.ServiceKey(service, id))
	if err != nil {
		return ServiceInstance{}, err
	}
	if len(resp.Kvs) == 0 {
		return ServiceInstance{}, fmt.Errorf("%w: %s/%s", ErrInstanceNotFound, service, id)
	}
	return UnmarshalInstance(resp.Kvs[0].Value)
}

// Delete removes one service instance.
func (r *EtcdRegistry) Delete(ctx context.Context, service string, id string) error {
	if r.client == nil {
		return fmt.Errorf("etcd client is required")
	}
	if strings.TrimSpace(service) == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("service instance id is required")
	}
	_, err := r.client.Delete(ctx, r.keyspace.ServiceKey(service, id))
	return err
}

// etcdRegistration stores the key and lease for one etcd registration.
type etcdRegistration struct {
	client *clientv3.Client
	key    string
	lease  clientv3.LeaseID
	cancel context.CancelFunc
	done   chan struct{}
}

// Renew actively refreshes the lease once.
func (r *etcdRegistration) Renew(ctx context.Context) error {
	_, err := r.client.KeepAliveOnce(ctx, r.lease)
	return err
}

// Deregister stops keepalive, deletes the instance key, and revokes the lease.
func (r *etcdRegistration) Deregister(ctx context.Context) error {
	r.cancel()
	select {
	case <-r.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, deleteErr := r.client.Delete(ctx, r.key)
	_, revokeErr := r.client.Revoke(ctx, r.lease)
	return errors.Join(deleteErr, revokeErr)
}

// drainKeepAlive consumes keepalive responses until the channel closes.
//
// The etcd client expects this channel to be consumed continuously.
func (r *etcdRegistration) drainKeepAlive(ch <-chan *clientv3.LeaseKeepAliveResponse) {
	defer close(r.done)
	for range ch {
	}
}
