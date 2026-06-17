package servicekit

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// RuntimeIdentity is the effective service identity used at runtime for
// registry instance ids and targeted control commands.
type RuntimeIdentity struct {
	Service    string
	Address    string
	InstanceID string
}

type runtimeIdentityStore struct {
	mu       sync.RWMutex
	identity RuntimeIdentity
}

func newRuntimeIdentityStore() *runtimeIdentityStore {
	return &runtimeIdentityStore{}
}

func (s *runtimeIdentityStore) Set(identity RuntimeIdentity) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identity = identity
}

func (s *runtimeIdentityStore) Get() RuntimeIdentity {
	if s == nil {
		return RuntimeIdentity{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.identity
}

// runtimeIdentifiedDataPlane is implemented by DataPlanes that can report the
// effective registry identity selected after startup.
type runtimeIdentifiedDataPlane interface {
	RuntimeIdentity() RuntimeIdentity
}

// ServiceIdentity returns the service name and advertised gRPC address used by
// control commands and registry instance ids.
func ServiceIdentity(cfg Config) (string, string, error) {
	name := strings.TrimSpace(cfg.Service.Name)
	if name == "" {
		return "", "", fmt.Errorf("service name is required")
	}
	address, err := resolveServiceAddress(context.Background(), cfg)
	if err != nil {
		return "", "", err
	}
	if address == "" {
		return "", "", fmt.Errorf("service %s advertise grpc addr is required", name)
	}
	return name, address, nil
}
