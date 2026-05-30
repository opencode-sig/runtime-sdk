package registry

import (
	"context"
	"errors"
	"time"
)

var ErrInstanceNotFound = errors.New("service instance not found")

type ServiceInstance struct {
	ID                  string            `json:"id" yaml:"id"`
	Name                string            `json:"name" yaml:"name"`
	Address             string            `json:"address" yaml:"address"`
	Hostname            string            `json:"hostname" yaml:"hostname"`
	Metadata            map[string]string `json:"metadata" yaml:"metadata"`
	StartedAt           time.Time         `json:"started_at" yaml:"started_at"`
	LastSeen            time.Time         `json:"last_seen" yaml:"last_seen"`
	DataPlaneStartedAt  time.Time         `json:"data_plane_started_at" yaml:"data_plane_started_at"`
	DataPlaneGeneration string            `json:"data_plane_generation" yaml:"data_plane_generation"`
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
