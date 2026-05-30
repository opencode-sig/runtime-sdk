package component

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	applogger "github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

var registrationRenewInterval = 5 * time.Second
var registrationRenewTimeout = 3 * time.Second

type RegistrationComponent struct {
	registry     registry.Registry
	instance     registry.ServiceInstance
	registration registry.Registration
	logger       *applogger.Logger
	mu           sync.Mutex
	renewMu      sync.Mutex
	cancelRenew  context.CancelFunc
	renewDone    chan struct{}
}

// NewRegistrationComponent wraps service instance registration as a lifecycle component.
//
// Service processes and single-process runtimes can use the same lifecycle path.
func NewRegistrationComponent(reg registry.Registry, instance registry.ServiceInstance, logger *applogger.Logger) *RegistrationComponent {
	if instance.LastSeen.IsZero() {
		instance.LastSeen = time.Now().UTC()
	}
	return &RegistrationComponent{
		registry: reg,
		instance: instance,
		logger:   logger,
	}
}

// WithDataPlaneGeneration attaches the owning DataPlane generation to the
// instance registration. The registration start time is recorded when Start is
// called, because that is when this generation becomes visible in registry.
func (c *RegistrationComponent) WithDataPlaneGeneration(generation string) *RegistrationComponent {
	if c != nil {
		c.mu.Lock()
		c.instance.DataPlaneGeneration = strings.TrimSpace(generation)
		c.mu.Unlock()
	}
	return c
}

// Start registers the instance in registry.
func (c *RegistrationComponent) Start(ctx context.Context) error {
	if c.registry == nil {
		return errors.New("registry is not configured")
	}
	registration, err := c.register(ctx, true)
	if err != nil {
		return err
	}
	renewCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.mu.Lock()
	c.registration = registration
	c.cancelRenew = cancel
	c.renewDone = done
	c.mu.Unlock()
	go c.renewLoop(renewCtx, done)
	c.logRegistered(ctx, "service registered")
	return nil
}

func (c *RegistrationComponent) register(ctx context.Context, markDataPlaneStart bool) (registry.Registration, error) {
	now := time.Now().UTC()
	c.mu.Lock()
	c.instance.LastSeen = now
	if markDataPlaneStart || c.instance.DataPlaneStartedAt.IsZero() {
		c.instance.DataPlaneStartedAt = now
	}
	instance := c.instance
	c.mu.Unlock()
	return c.registry.Register(ctx, instance)
}

func (c *RegistrationComponent) logRegistered(ctx context.Context, msg string) {
	if c.logger != nil {
		c.mu.Lock()
		instance := c.instance
		c.mu.Unlock()
		c.logger.Info(ctx, msg,
			applogger.Event("service_registered"),
			applogger.Module(instance.Name),
			applogger.String("address", instance.Address),
			applogger.String("instance_id", instance.ID),
			applogger.String("hostname", instance.Hostname),
		)
	}
}

// Stop deregisters the current instance.
//
// Metadata routes are not deleted here; this component only owns instance lifecycle.
func (c *RegistrationComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancelRenew
	done := c.renewDone
	registration := c.registration
	c.cancelRenew = nil
	c.renewDone = nil
	c.registration = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if registration == nil {
		return nil
	}
	return registration.Deregister(ctx)
}

// Health checks registration validity through Renew.
//
// etcd registries refresh leases; memory registries update LastSeen.
func (c *RegistrationComponent) Health(ctx context.Context) error {
	return c.renewOrRegister(ctx)
}

func (c *RegistrationComponent) renewLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(registrationRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.renewOrRegister(ctx); err != nil && c.logger != nil {
				c.mu.Lock()
				instance := c.instance
				c.mu.Unlock()
				fields := append(applogger.Fields(
					applogger.Event("service_registration_renew_failed"),
					applogger.Module(instance.Name),
					applogger.String("address", instance.Address),
					applogger.String("instance_id", instance.ID),
				), applogger.ErrorFields(err)...)
				c.logger.Warn(ctx, "service registration renew failed", fields...)
			}
		}
	}
}

func (c *RegistrationComponent) renewOrRegister(ctx context.Context) error {
	c.renewMu.Lock()
	defer c.renewMu.Unlock()

	c.mu.Lock()
	registration := c.registration
	c.mu.Unlock()
	if registration == nil {
		return errors.New("service is not registered")
	}
	renewCtx, cancel := context.WithTimeout(ctx, registrationRenewTimeout)
	err := registration.Renew(renewCtx)
	cancel()
	if err == nil {
		return nil
	}
	if !errors.Is(err, registry.ErrRegistrationExpired) {
		return err
	}
	registerCtx, registerCancel := context.WithTimeout(ctx, registrationRenewTimeout)
	next, registerErr := c.register(registerCtx, false)
	registerCancel()
	if registerErr != nil {
		return err
	}
	c.mu.Lock()
	c.registration = next
	c.mu.Unlock()
	c.logRegistered(ctx, "service registration recovered")
	return nil
}
