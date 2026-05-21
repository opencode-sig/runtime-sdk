package discovery

import (
	"context"
	"time"

	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

type EventType string

const (
	EventAdded   EventType = "added"
	EventRemoved EventType = "removed"
	EventUpdated EventType = "updated"
)

type DiscoveryEvent struct {
	Type     EventType
	Instance registry.ServiceInstance
}

type WatchStatus struct {
	Service       string
	Running       bool
	Healthy       bool
	Stale         bool
	LastSyncedAt  time.Time
	LastEventAt   time.Time
	LastError     string
	Revision      int64
	InstanceCount int
	Reconnects    uint64
}

// Discovery abstracts service discovery.
//
// Resolve provides the current snapshot, and Watch provides later changes.
// Gateway resolvers depend on both.
type Discovery interface {
	Resolve(ctx context.Context, service string) ([]registry.ServiceInstance, error)
	Watch(ctx context.Context, service string) (<-chan DiscoveryEvent, error)
}
