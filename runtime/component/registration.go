package component

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	applogger "github.com/opencode-sig/runtime-sdk/logger"
	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

var registrationRenewInterval = 30 * time.Second
var registrationRenewTimeout = 15 * time.Second

type RegistrationComponent struct {
	registry            registry.Registry
	instance            registry.ServiceInstance
	instanceFactory     func(context.Context) (registry.ServiceInstance, error)
	registration        registry.Registration
	logger              *applogger.Logger
	metrics             *runtimemetrics.ControlPlaneMetrics
	dataPlaneGeneration string
	mu                  sync.Mutex
	renewMu             sync.Mutex
	healthy             atomic.Bool
	cancelRenew         context.CancelFunc
	renewDone           chan struct{}
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

// NewDynamicRegistrationComponent wraps service instance registration with an
// instance factory evaluated when the component starts.
func NewDynamicRegistrationComponent(reg registry.Registry, factory func(context.Context) (registry.ServiceInstance, error), logger *applogger.Logger) *RegistrationComponent {
	return &RegistrationComponent{
		registry:        reg,
		instanceFactory: factory,
		logger:          logger,
	}
}

// WithDataPlaneGeneration attaches the owning DataPlane generation to the
// instance registration. The registration start time is recorded when Start is
// called, because that is when this generation becomes visible in registry.
func (c *RegistrationComponent) WithDataPlaneGeneration(generation string) *RegistrationComponent {
	if c != nil {
		c.mu.Lock()
		c.dataPlaneGeneration = strings.TrimSpace(generation)
		c.instance.DataPlaneGeneration = strings.TrimSpace(generation)
		c.mu.Unlock()
	}
	return c
}

func (c *RegistrationComponent) WithControlPlaneMetrics(metrics *runtimemetrics.ControlPlaneMetrics) *RegistrationComponent {
	if c != nil {
		c.metrics = metrics
	}
	return c
}

// Start registers the instance in registry.
func (c *RegistrationComponent) Start(ctx context.Context) error {
	if c.registry == nil {
		return errors.New("registry is not configured")
	}
	if err := c.refreshInstance(ctx); err != nil {
		return err
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
	c.markHealthy()
	go c.renewLoop(renewCtx, done)
	c.logRegistered(ctx, "service registered")
	return nil
}

func (c *RegistrationComponent) refreshInstance(ctx context.Context) error {
	if c.instanceFactory == nil {
		return nil
	}
	instance, err := c.instanceFactory(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.dataPlaneGeneration != "" {
		instance.DataPlaneGeneration = c.dataPlaneGeneration
	}
	c.instance = instance
	c.mu.Unlock()
	return nil
}

func (c *RegistrationComponent) register(ctx context.Context, markDataPlaneStart bool) (registry.Registration, error) {
	now := time.Now().UTC()
	c.mu.Lock()
	c.instance.LastSeen = now
	if c.dataPlaneGeneration != "" {
		c.instance.DataPlaneGeneration = c.dataPlaneGeneration
	}
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
	c.cancelRenew = nil
	c.renewDone = nil
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
	c.mu.Lock()
	registration := c.registration
	c.registration = nil
	c.mu.Unlock()
	if registration == nil {
		return nil
	}
	c.markUnhealthy()
	return registration.Deregister(ctx)
}

// Health checks whether this component has established a local registration.
//
// Registry lease renewal and recovery run in the background so service
// liveness does not depend on transient registry connectivity.
func (c *RegistrationComponent) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	registered := c.registration != nil
	c.mu.Unlock()
	if !registered {
		return errors.New("service is not registered")
	}
	return nil
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
			if ctx.Err() != nil {
				return
			}
			renewCtx, cancel := context.WithTimeout(context.Background(), registrationRenewTimeout)
			err := c.renewOrRegister(renewCtx)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				c.markError("renew")
				c.mu.Lock()
				instance := c.instance
				c.mu.Unlock()
				if c.logger != nil {
					fields := append(applogger.Fields(
						applogger.Event("service_registration_renew_failed"),
						applogger.Module(instance.Name),
						applogger.String("address", instance.Address),
						applogger.String("instance_id", instance.ID),
					), applogger.ErrorFields(err)...)
					c.logger.Warn(ctx, "service registration renew failed", fields...)
				}
				continue
			}
			c.markRecovered("renew")
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
	c.healthy.Store(true)
	c.metrics.SetStatus("registry", true)
	c.metrics.RecordRecovery("registry", "re_register")
	c.logRegistered(ctx, "service registration recovered")
	return nil
}

func (c *RegistrationComponent) markHealthy() {
	c.healthy.Store(true)
	c.metrics.SetStatus("registry", true)
}

func (c *RegistrationComponent) markUnhealthy() {
	c.healthy.Store(false)
	c.metrics.SetStatus("registry", false)
}

func (c *RegistrationComponent) markError(operation string) {
	c.healthy.Store(false)
	c.metrics.SetStatus("registry", false)
	c.metrics.RecordError("registry", operation)
}

func (c *RegistrationComponent) markRecovered(operation string) {
	if !c.healthy.Swap(true) {
		c.metrics.RecordRecovery("registry", operation)
	}
	c.metrics.SetStatus("registry", true)
}
