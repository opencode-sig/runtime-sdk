package grpcclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	runtimediscovery "github.com/opencode-sig/runtime-sdk/runtime/discovery"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

const ResolverScheme = "runtime"

// ResolverBuilder adapts runtime discovery to grpc-go resolver.Builder.
type ResolverBuilder struct {
	source runtimediscovery.Discovery
}

// NewResolverBuilder creates a grpc-go resolver builder backed by runtime discovery.
func NewResolverBuilder(source runtimediscovery.Discovery) *ResolverBuilder {
	return &ResolverBuilder{source: source}
}

// Scheme returns the grpc target scheme.
func (b *ResolverBuilder) Scheme() string {
	return ResolverScheme
}

// Build creates one resolver for one service target.
func (b *ResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	service := strings.TrimSpace(target.Endpoint())
	if service == "" {
		return nil, fmt.Errorf("grpc resolver service is required")
	}
	if b.source == nil {
		return nil, fmt.Errorf("grpc resolver discovery source is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &serviceResolver{
		source:    b.source,
		service:   service,
		cc:        cc,
		ctx:       ctx,
		cancel:    cancel,
		instances: make(map[string]registry.ServiceInstance),
	}
	go r.run()
	return r, nil
}

type serviceResolver struct {
	source  runtimediscovery.Discovery
	service string
	cc      resolver.ClientConn
	ctx     context.Context
	cancel  context.CancelFunc

	mu        sync.Mutex
	instances map[string]registry.ServiceInstance
}

func (r *serviceResolver) ResolveNow(options resolver.ResolveNowOptions) {
	go r.refresh()
}

func (r *serviceResolver) Close() {
	r.cancel()
}

func (r *serviceResolver) run() {
	r.refresh()

	events, err := r.source.Watch(r.ctx, r.service)
	if err != nil {
		r.report(err)
		return
	}

	for {
		select {
		case <-r.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				r.report(status.Errorf(codes.Unavailable, "watch service %q stopped", r.service))
				return
			}
			r.apply(event)
		}
	}
}

func (r *serviceResolver) refresh() {
	ctx, cancel := context.WithTimeout(r.ctx, 3*time.Second)
	defer cancel()

	instances, err := r.source.Resolve(ctx, r.service)
	if err != nil {
		r.report(err)
		return
	}
	r.replace(instances)
}

func (r *serviceResolver) replace(instances []registry.ServiceInstance) {
	next := make(map[string]registry.ServiceInstance, len(instances))
	for _, instance := range instances {
		if strings.TrimSpace(instance.Address) == "" {
			continue
		}
		next[instanceKey(instance)] = instance
	}

	r.mu.Lock()
	r.instances = next
	addresses := r.addressesLocked()
	r.mu.Unlock()
	r.update(addresses)
}

func (r *serviceResolver) apply(event runtimediscovery.DiscoveryEvent) {
	if strings.TrimSpace(event.Instance.Address) == "" {
		return
	}

	r.mu.Lock()
	switch event.Type {
	case runtimediscovery.EventAdded, runtimediscovery.EventUpdated:
		r.instances[instanceKey(event.Instance)] = event.Instance
	case runtimediscovery.EventRemoved:
		delete(r.instances, instanceKey(event.Instance))
	}
	addresses := r.addressesLocked()
	r.mu.Unlock()
	r.update(addresses)
}

func (r *serviceResolver) addressesLocked() []resolver.Address {
	keys := make([]string, 0, len(r.instances))
	for key := range r.instances {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	addresses := make([]resolver.Address, 0, len(keys))
	for _, key := range keys {
		addresses = append(addresses, resolver.Address{Addr: r.instances[key].Address})
	}
	return addresses
}

func (r *serviceResolver) update(addresses []resolver.Address) {
	if len(addresses) == 0 {
		r.report(status.Errorf(codes.Unavailable, "no instances for service %q", r.service))
	}
	if err := r.cc.UpdateState(resolver.State{Addresses: addresses}); err != nil {
		r.report(err)
	}
}

func (r *serviceResolver) report(err error) {
	select {
	case <-r.ctx.Done():
		return
	default:
	}
	r.cc.ReportError(err)
}

func instanceKey(instance registry.ServiceInstance) string {
	if strings.TrimSpace(instance.ID) != "" {
		return instance.ID
	}
	return strings.TrimSpace(instance.Name) + "/" + strings.TrimSpace(instance.Address)
}
