package registry

import (
	"context"
	"errors"
	"time"
)

var ErrInstanceNotFound = errors.New("service instance not found")

type ServiceInstance struct {
	ID        string
	Name      string
	Address   string
	Hostname  string
	Metadata  map[string]string
	StartedAt time.Time
	LastSeen  time.Time
}

// Registration represents one service registration.
//
// Deregister removes the instance, and Renew refreshes lease or health state.
type Registration interface {
	Deregister(ctx context.Context) error
	Renew(ctx context.Context) error
}

// Registry abstracts service registration.
//
// Current implementations include memory and etcd registries.
type Registry interface {
	Register(ctx context.Context, instance ServiceInstance) (Registration, error)
}

// InstanceStore provides instance query and operational delete capabilities.
type InstanceStore interface {
	Services(ctx context.Context) ([]string, error)
	Instances(ctx context.Context, service string) ([]ServiceInstance, error)
	Instance(ctx context.Context, service string, id string) (ServiceInstance, error)
	Delete(ctx context.Context, service string, id string) error
}
