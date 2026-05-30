package servicekit

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
)

var generationSequence atomic.Int64

// ServiceDataPlane owns one generation of a managed gRPC service runtime.
type ServiceDataPlane struct {
	generation string
	config     Config
	lifecycle  *lifecycle.Runtime
	logger     *logger.Logger
}

func NewServiceDataPlane(ctx context.Context, cfg Config, spec Spec, runtimeMode string, log *logger.Logger) (*ServiceDataPlane, error) {
	generation := NewGeneration(spec.Name)
	app, err := newServiceLifecycle(ctx, cfg, spec, runtimeMode, log, generation)
	if err != nil {
		return nil, err
	}
	return newDataPlaneWithGeneration(generation, cfg, app, log)
}

func NewDataPlane(name string, cfg Config, app *lifecycle.Runtime, log *logger.Logger) (*ServiceDataPlane, error) {
	return newDataPlaneWithGeneration(NewGeneration(name), cfg, app, log)
}

func newDataPlaneWithGeneration(generation string, cfg Config, app *lifecycle.Runtime, log *logger.Logger) (*ServiceDataPlane, error) {
	if app == nil {
		return nil, fmt.Errorf("data plane lifecycle is required")
	}
	return &ServiceDataPlane{
		generation: generation,
		config:     cfg,
		lifecycle:  app,
		logger:     log,
	}, nil
}

// NewGeneration creates a runtime DataPlane generation identifier for service.
//
// The returned value is stable in shape, "<service>-<number>", so registry
// metadata, runtime-admin views, rebuild logs, and custom DataPlane owners such
// as Gateways can share one SDK-defined generation contract. Empty service
// names are normalized to "dataplane".
func NewGeneration(service string) string {
	service = strings.TrimSpace(service)
	if service == "" {
		service = "dataplane"
	}
	sequence := nextGenerationSequence()
	return fmt.Sprintf("%s-%d", service, sequence)
}

func nextGenerationSequence() int64 {
	now := time.Now().UnixNano()
	for {
		last := generationSequence.Load()
		next := now
		if next <= last {
			next = last + 1
		}
		if generationSequence.CompareAndSwap(last, next) {
			return next
		}
	}
}

func (r *ServiceDataPlane) Generation() string {
	if r == nil {
		return ""
	}
	return r.generation
}

func (r *ServiceDataPlane) Config() Config {
	if r == nil {
		return Config{}
	}
	return r.config
}

func (r *ServiceDataPlane) Start(ctx context.Context) error {
	if r == nil || r.lifecycle == nil {
		return fmt.Errorf("data plane is not configured")
	}
	if err := r.lifecycle.Start(ctx); err != nil {
		return err
	}
	if r.logger != nil {
		r.logger.Warn(ctx, "data plane started",
			logger.Event("dataplane_lifecycle"),
			logger.Module(moduleFromGeneration(r.generation)),
			logger.String("generation", r.generation),
		)
	}
	return nil
}

func (r *ServiceDataPlane) Stop(ctx context.Context) error {
	if r == nil || r.lifecycle == nil {
		return nil
	}
	err := r.lifecycle.Stop(ctx)
	if r.logger != nil {
		if err != nil {
			fields := append(logger.Fields(
				logger.Event("dataplane_lifecycle"),
				logger.Module(moduleFromGeneration(r.generation)),
				logger.String("generation", r.generation),
			), logger.ErrorFields(err)...)
			r.logger.Error(ctx, "data plane stopped with error", fields...)
		} else {
			r.logger.Warn(ctx, "data plane stopped",
				logger.Event("dataplane_lifecycle"),
				logger.Module(moduleFromGeneration(r.generation)),
				logger.String("generation", r.generation),
			)
		}
	}
	return err
}

func (r *ServiceDataPlane) Health(ctx context.Context) error {
	if r == nil || r.lifecycle == nil {
		return fmt.Errorf("data plane is not configured")
	}
	return r.lifecycle.Health(ctx)
}
