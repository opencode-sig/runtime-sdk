package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Component interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}

// Runtime manages startup, health checks, and shutdown for an ordered component set.
//
// It intentionally avoids a complex state machine. It records started
// components and stops them in reverse order.
type Runtime struct {
	name       string
	mu         sync.Mutex
	components []namedComponent
	started    []namedComponent
	running    bool
}

type namedComponent struct {
	name      string
	component Component
}

// New creates a runtime lifecycle.
func New(name string) *Runtime {
	return &Runtime{name: name}
}

// Add registers a component.
//
// Components can only be added before startup, and names must be unique so
// startup and health errors are easy to locate.
func (r *Runtime) Add(name string, component Component) error {
	if name == "" {
		return fmt.Errorf("component name is required")
	}
	if component == nil {
		return fmt.Errorf("component is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Errorf("runtime %s is running", r.name)
	}
	for _, item := range r.components {
		if item.name == name {
			return fmt.Errorf("component %s already exists", name)
		}
	}
	r.components = append(r.components, namedComponent{name: name, component: component})
	return nil
}

// Start starts all components in registration order.
//
// If one component fails, already-started components are stopped in reverse
// order to avoid partially started processes.
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return nil
	}
	for _, item := range r.components {
		if err := item.component.Start(ctx); err != nil {
			stopErr := r.stopStarted(ctx)
			return errors.Join(fmt.Errorf("start %s: %w", item.name, err), stopErr)
		}
		r.started = append(r.started, item)
	}
	r.running = true
	return nil
}

// Stop stops all started components in reverse startup order.
func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	return r.stopStarted(ctx)
}

// Health runs health checks for started components in order.
//
// Returned errors include the component name for /healthz and log diagnosis.
func (r *Runtime) Health(ctx context.Context) error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return fmt.Errorf("runtime %s is not running", r.name)
	}
	items := append([]namedComponent(nil), r.started...)
	r.mu.Unlock()

	for _, item := range items {
		if err := item.component.Health(ctx); err != nil {
			return fmt.Errorf("health %s: %w", item.name, err)
		}
	}
	return nil
}

// stopStarted stops all started components.
//
// Callers must hold the runtime lock. errors.Join preserves multiple stop errors.
func (r *Runtime) stopStarted(ctx context.Context) error {
	var err error
	for i := len(r.started) - 1; i >= 0; i-- {
		item := r.started[i]
		err = errors.Join(err, item.component.Stop(ctx))
	}
	r.started = nil
	return err
}
