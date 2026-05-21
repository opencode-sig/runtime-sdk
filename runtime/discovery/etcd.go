package discovery

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const etcdWatchRecoverMinDelay = 200 * time.Millisecond
const etcdWatchRecoverMaxDelay = 3 * time.Second

type EtcdDiscovery struct {
	client   *clientv3.Client
	keyspace registry.Keyspace
	mu       sync.Mutex
	statuses map[string]WatchStatus
}

// NewEtcdDiscovery creates service discovery backed by the etcd registry keyspace.
func NewEtcdDiscovery(client *clientv3.Client, prefix string) *EtcdDiscovery {
	return &EtcdDiscovery{
		client:   client,
		keyspace: registry.NewKeyspace(prefix),
		statuses: make(map[string]WatchStatus),
	}
}

// Resolve reads all current instances for a service.
func (d *EtcdDiscovery) Resolve(ctx context.Context, service string) ([]registry.ServiceInstance, error) {
	if d.client == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	snapshot, _, err := d.loadSnapshot(ctx, service)
	if err != nil {
		return nil, err
	}
	keys := sortedSnapshotKeys(snapshot)
	instances := make([]registry.ServiceInstance, 0, len(keys))
	for _, key := range keys {
		instances = append(instances, snapshot[key])
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
	service = strings.TrimSpace(service)
	snapshot, revision, err := d.loadSnapshot(ctx, service)
	if err != nil {
		return nil, err
	}

	out := make(chan DiscoveryEvent, len(snapshot)+8)
	for _, key := range sortedSnapshotKeys(snapshot) {
		out <- DiscoveryEvent{Type: EventAdded, Instance: snapshot[key]}
	}

	go d.watchLoop(ctx, service, out, snapshot, revision)
	return out, nil
}

// Status returns the latest known watch status for a service.
func (d *EtcdDiscovery) Status(service string) WatchStatus {
	if d == nil {
		return WatchStatus{}
	}
	service = strings.TrimSpace(service)
	d.mu.Lock()
	defer d.mu.Unlock()
	status := d.statuses[service]
	status.Healthy = status.Running && !status.Stale && status.LastError == ""
	return status
}

func (d *EtcdDiscovery) loadSnapshot(ctx context.Context, service string) (map[string]registry.ServiceInstance, int64, error) {
	service = strings.TrimSpace(service)
	if strings.TrimSpace(service) == "" {
		return nil, 0, fmt.Errorf("service name is required")
	}
	resp, err := d.client.Get(ctx, d.keyspace.ServicePrefix(service), clientv3.WithPrefix())
	if err != nil {
		return nil, 0, err
	}

	instances := make(map[string]registry.ServiceInstance, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		instance, err := registry.UnmarshalInstance(kv.Value)
		if err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(instance.Address) == "" {
			continue
		}
		instances[instanceKey(instance)] = instance
	}
	return instances, resp.Header.Revision, nil
}

func (d *EtcdDiscovery) watchLoop(ctx context.Context, service string, out chan<- DiscoveryEvent, snapshot map[string]registry.ServiceInstance, revision int64) {
	defer close(out)
	d.markWatchLoaded(service, snapshot, revision)
	defer d.markWatchStopped(service)

	delay := etcdWatchRecoverMinDelay
	for ctx.Err() == nil {
		nextRevision, err := d.watchOnce(ctx, service, out, snapshot, revision)
		if nextRevision > revision {
			revision = nextRevision
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			d.markWatchError(service, err)
			if !sleepWithContext(ctx, delay) {
				return
			}
			if delay < etcdWatchRecoverMaxDelay {
				delay *= 2
				if delay > etcdWatchRecoverMaxDelay {
					delay = etcdWatchRecoverMaxDelay
				}
			}
		}

		var nextSnapshot map[string]registry.ServiceInstance
		var loadedRevision int64
		for ctx.Err() == nil {
			var loadErr error
			nextSnapshot, loadedRevision, loadErr = d.loadSnapshot(ctx, service)
			if loadErr == nil {
				break
			}
			d.markWatchError(service, loadErr)
			if !sleepWithContext(ctx, delay) {
				return
			}
			if delay < etcdWatchRecoverMaxDelay {
				delay *= 2
				if delay > etcdWatchRecoverMaxDelay {
					delay = etcdWatchRecoverMaxDelay
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		if !emitSnapshotDiff(ctx, out, snapshot, nextSnapshot) {
			return
		}
		snapshot = nextSnapshot
		revision = loadedRevision
		d.markWatchLoaded(service, snapshot, revision)
		delay = etcdWatchRecoverMinDelay
	}
}

func (d *EtcdDiscovery) watchOnce(ctx context.Context, service string, out chan<- DiscoveryEvent, snapshot map[string]registry.ServiceInstance, revision int64) (int64, error) {
	keyPrefix := d.keyspace.ServicePrefix(service)
	opts := []clientv3.OpOption{clientv3.WithPrefix(), clientv3.WithPrevKV()}
	if revision > 0 {
		opts = append(opts, clientv3.WithRev(revision+1))
	}
	latestRevision := revision
	watch := d.client.Watch(ctx, keyPrefix, opts...)
	for resp := range watch {
		if err := resp.Err(); err != nil {
			return latestRevision, err
		}
		latestRevision = maxRevision(latestRevision, resp.Header.GetRevision())
		for _, event := range resp.Events {
			discoveryEvent, err := decodeEvent(event)
			if err != nil {
				continue
			}
			latestRevision = maxRevision(latestRevision, eventRevision(event))
			applySnapshotEvent(snapshot, discoveryEvent)
			d.markWatchEvent(service, snapshot, latestRevision)
			if !sendEvent(ctx, out, discoveryEvent) {
				return latestRevision, ctx.Err()
			}
		}
	}
	if ctx.Err() != nil {
		return latestRevision, ctx.Err()
	}
	return latestRevision, fmt.Errorf("watch service %q stopped", service)
}

func (d *EtcdDiscovery) markWatchLoaded(service string, snapshot map[string]registry.ServiceInstance, revision int64) {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	status := d.statuses[service]
	status.Service = service
	status.Running = true
	status.Healthy = true
	status.Stale = false
	status.LastSyncedAt = now
	status.LastError = ""
	status.Revision = revision
	status.InstanceCount = len(snapshot)
	d.statuses[service] = status
}

func (d *EtcdDiscovery) markWatchEvent(service string, snapshot map[string]registry.ServiceInstance, revision int64) {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	status := d.statuses[service]
	status.Service = service
	status.Running = true
	status.Healthy = true
	status.Stale = false
	status.LastEventAt = now
	status.LastSyncedAt = now
	status.LastError = ""
	status.Revision = maxRevision(status.Revision, revision)
	status.InstanceCount = len(snapshot)
	d.statuses[service] = status
}

func (d *EtcdDiscovery) markWatchError(service string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	status := d.statuses[service]
	status.Service = service
	status.Running = true
	status.Healthy = false
	status.Stale = true
	status.Reconnects++
	if err != nil {
		status.LastError = err.Error()
	}
	d.statuses[service] = status
}

func (d *EtcdDiscovery) markWatchStopped(service string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	status := d.statuses[service]
	status.Service = service
	status.Running = false
	status.Healthy = false
	d.statuses[service] = status
}

// decodeEvent converts an etcd watch event into a DiscoveryEvent.
func decodeEvent(event *clientv3.Event) (DiscoveryEvent, error) {
	switch event.Type {
	case clientv3.EventTypePut:
		instance, err := registry.UnmarshalInstance(event.Kv.Value)
		if err != nil {
			return DiscoveryEvent{}, err
		}
		eventType := EventAdded
		if event.IsModify() {
			eventType = EventUpdated
		}
		return DiscoveryEvent{Type: eventType, Instance: instance}, nil
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

func emitSnapshotDiff(ctx context.Context, out chan<- DiscoveryEvent, old map[string]registry.ServiceInstance, next map[string]registry.ServiceInstance) bool {
	oldKeys := sortedSnapshotKeys(old)
	for _, key := range oldKeys {
		if _, ok := next[key]; ok {
			continue
		}
		if !sendEvent(ctx, out, DiscoveryEvent{Type: EventRemoved, Instance: old[key]}) {
			return false
		}
	}
	nextKeys := sortedSnapshotKeys(next)
	for _, key := range nextKeys {
		oldInstance, ok := old[key]
		if !ok {
			if !sendEvent(ctx, out, DiscoveryEvent{Type: EventAdded, Instance: next[key]}) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(oldInstance, next[key]) {
			if !sendEvent(ctx, out, DiscoveryEvent{Type: EventUpdated, Instance: next[key]}) {
				return false
			}
		}
	}
	return true
}

func applySnapshotEvent(snapshot map[string]registry.ServiceInstance, event DiscoveryEvent) {
	if snapshot == nil || strings.TrimSpace(event.Instance.Address) == "" {
		return
	}
	key := instanceKey(event.Instance)
	switch event.Type {
	case EventAdded, EventUpdated:
		snapshot[key] = event.Instance
	case EventRemoved:
		delete(snapshot, key)
	}
}

func sendEvent(ctx context.Context, out chan<- DiscoveryEvent, event DiscoveryEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func sortedSnapshotKeys(snapshot map[string]registry.ServiceInstance) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func instanceKey(instance registry.ServiceInstance) string {
	if id := strings.TrimSpace(instance.ID); id != "" {
		return id
	}
	return strings.TrimSpace(instance.Name) + "/" + strings.TrimSpace(instance.Address)
}

func eventRevision(event *clientv3.Event) int64 {
	if event == nil {
		return 0
	}
	if event.Kv != nil {
		return event.Kv.ModRevision
	}
	if event.PrevKv != nil {
		return event.PrevKv.ModRevision
	}
	return 0
}

func maxRevision(left int64, right int64) int64 {
	if right > left {
		return right
	}
	return left
}
