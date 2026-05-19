package component

import (
	"context"
	"errors"
	"time"

	applogger "github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

type RegistrationComponent struct {
	registry     registry.Registry
	instance     registry.ServiceInstance
	registration registry.Registration
	logger       *applogger.Logger
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

// Start registers the instance in registry.
func (c *RegistrationComponent) Start(ctx context.Context) error {
	if c.registry == nil {
		return errors.New("registry is not configured")
	}
	registration, err := c.registry.Register(ctx, c.instance)
	if err != nil {
		return err
	}
	c.registration = registration
	if c.logger != nil {
		c.logger.Info(ctx, "service registered",
			applogger.Event("service_registered"),
			applogger.Module(c.instance.Name),
			applogger.String("address", c.instance.Address),
			applogger.String("instance_id", c.instance.ID),
			applogger.String("hostname", c.instance.Hostname),
		)
	}
	return nil
}

// Stop deregisters the current instance.
//
// Metadata routes are not deleted here; this component only owns instance lifecycle.
func (c *RegistrationComponent) Stop(ctx context.Context) error {
	if c.registration == nil {
		return nil
	}
	err := c.registration.Deregister(ctx)
	c.registration = nil
	return err
}

// Health checks registration validity through Renew.
//
// etcd registries refresh leases; memory registries update LastSeen.
func (c *RegistrationComponent) Health(ctx context.Context) error {
	if c.registration == nil {
		return errors.New("service is not registered")
	}
	return c.registration.Renew(ctx)
}
