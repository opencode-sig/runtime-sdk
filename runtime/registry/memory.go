package registry

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type EventType string

const (
	EventAdded   EventType = "added"
	EventRemoved EventType = "removed"
	EventUpdated EventType = "updated"
)

type Event struct {
	Type     EventType
	Instance ServiceInstance
}

type MemoryRegistry struct {
	mu        sync.RWMutex
	instances map[string]map[string]ServiceInstance
	watchers  map[string][]chan Event
}

// NewMemoryRegistry creates an in-process registry.
//
// Monolith mode can use it to run registration and discovery without etcd.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		instances: make(map[string]map[string]ServiceInstance),
		watchers:  make(map[string][]chan Event),
	}
}

// Register registers a service instance and notifies watchers.
func (r *MemoryRegistry) Register(ctx context.Context, instance ServiceInstance) (Registration, error) {
	if err := validate(instance); err != nil {
		return nil, err
	}
	instance.ID = strings.TrimSpace(instance.ID)
	instance.Name = strings.TrimSpace(instance.Name)
	instance.Address = strings.TrimSpace(instance.Address)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	r.mu.Lock()
	if r.instances[instance.Name] == nil {
		r.instances[instance.Name] = make(map[string]ServiceInstance)
	}
	if instance.LastSeen.IsZero() {
		instance.LastSeen = time.Now().UTC()
	}
	if instance.StartedAt.IsZero() {
		instance.StartedAt = instance.LastSeen
	}
	instance = cloneInstance(instance)
	r.instances[instance.Name][instance.ID] = instance
	r.publishLocked(Event{Type: EventAdded, Instance: instance})
	r.mu.Unlock()

	return &memoryRegistration{registry: r, name: instance.Name, id: instance.ID}, nil
}

// Services lists registered service names.
func (r *MemoryRegistry) Services(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	services := make([]string, 0, len(r.instances))
	for service := range r.instances {
		services = append(services, service)
	}
	sort.Strings(services)
	return services, nil
}

// Instances returns an instance snapshot for a service.
func (r *MemoryRegistry) Instances(ctx context.Context, service string) ([]ServiceInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service = strings.TrimSpace(service)
	if service == "" {
		return nil, fmt.Errorf("service name is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	source := r.instances[service]
	instances := make([]ServiceInstance, 0, len(source))
	for _, instance := range source {
		instances = append(instances, cloneInstance(instance))
	}
	return instances, nil
}

// Instance returns one service instance.
func (r *MemoryRegistry) Instance(ctx context.Context, service string, id string) (ServiceInstance, error) {
	if err := ctx.Err(); err != nil {
		return ServiceInstance{}, err
	}
	service = strings.TrimSpace(service)
	id = strings.TrimSpace(id)
	if service == "" {
		return ServiceInstance{}, fmt.Errorf("service name is required")
	}
	if id == "" {
		return ServiceInstance{}, fmt.Errorf("service instance id is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, ok := r.instances[service][id]
	if !ok {
		return ServiceInstance{}, fmt.Errorf("%w: %s/%s", ErrInstanceNotFound, service, id)
	}
	return cloneInstance(instance), nil
}

// Delete removes one service instance.
func (r *MemoryRegistry) Delete(ctx context.Context, service string, id string) error {
	return r.deregister(ctx, service, id)
}

// Watch watches instance changes for a service.
//
// New watchers first receive added events for existing instances and then later
// incremental events.
func (r *MemoryRegistry) Watch(ctx context.Context, service string) (<-chan Event, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return nil, fmt.Errorf("service name is required")
	}
	r.mu.Lock()
	snapshot := make([]ServiceInstance, 0, len(r.instances[service]))
	for _, instance := range r.instances[service] {
		snapshot = append(snapshot, cloneInstance(instance))
	}
	ch := make(chan Event, len(snapshot)+8)
	for _, instance := range snapshot {
		ch <- Event{Type: EventAdded, Instance: instance}
	}
	r.watchers[service] = append(r.watchers[service], ch)
	r.mu.Unlock()

	go func() {
		<-ctx.Done()
		r.removeWatcher(service, ch)
		close(ch)
	}()

	return ch, nil
}

// renew refreshes LastSeen for a memory instance and publishes an updated event.
func (r *MemoryRegistry) renew(ctx context.Context, name string, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	instance, ok := r.instances[name][id]
	if !ok {
		return fmt.Errorf("%w: %s/%s", ErrInstanceNotFound, name, id)
	}
	instance.LastSeen = time.Now().UTC()
	instance = cloneInstance(instance)
	r.instances[name][id] = instance
	r.publishLocked(Event{Type: EventUpdated, Instance: instance})
	return nil
}

// deregister removes an instance and publishes a removed event.
func (r *MemoryRegistry) deregister(ctx context.Context, name string, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name == "" {
		return fmt.Errorf("service name is required")
	}
	if id == "" {
		return fmt.Errorf("service instance id is required")
	}

	instance, ok := r.instances[name][id]
	if !ok {
		return nil
	}
	delete(r.instances[name], id)
	if len(r.instances[name]) == 0 {
		delete(r.instances, name)
	}
	r.publishLocked(Event{Type: EventRemoved, Instance: instance})
	return nil
}

// publishLocked broadcasts an event to current service watchers.
//
// Full watcher channels drop events so slow consumers cannot block registry writes.
func (r *MemoryRegistry) publishLocked(event Event) {
	event.Instance = cloneInstance(event.Instance)
	for _, watcher := range r.watchers[event.Instance.Name] {
		select {
		case watcher <- event:
		default:
		}
	}
}

// removeWatcher removes and closes a watcher.
func (r *MemoryRegistry) removeWatcher(service string, ch chan Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	watchers := r.watchers[service]
	for i, watcher := range watchers {
		if watcher == ch {
			r.watchers[service] = append(watchers[:i], watchers[i+1:]...)
			break
		}
	}
	if len(r.watchers[service]) == 0 {
		delete(r.watchers, service)
	}
}

// validate checks whether a service instance contains required registry fields.
func validate(instance ServiceInstance) error {
	if strings.TrimSpace(instance.ID) == "" {
		return fmt.Errorf("service instance id is required")
	}
	if strings.TrimSpace(instance.Name) == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.TrimSpace(instance.Address) == "" {
		return fmt.Errorf("service address is required")
	}
	return nil
}

type memoryRegistration struct {
	registry *MemoryRegistry
	name     string
	id       string
}

// Deregister removes a memory instance.
func (r *memoryRegistration) Deregister(ctx context.Context) error {
	return r.registry.deregister(ctx, r.name, r.id)
}

// Renew refreshes memory instance state.
func (r *memoryRegistration) Renew(ctx context.Context) error {
	return r.registry.renew(ctx, r.name, r.id)
}
