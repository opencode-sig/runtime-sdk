package servicekit

import (
	"context"
	"fmt"
	"time"

	"github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
)

// ServiceDataPlane owns one generation of a managed gRPC service runtime.
type ServiceDataPlane struct {
	generation string
	config     Config
	lifecycle  *lifecycle.Runtime
	logger     *logger.Logger
}

func NewServiceDataPlane(ctx context.Context, cfg Config, spec Spec, runtimeMode string, log *logger.Logger) (*ServiceDataPlane, error) {
	app, err := NewServiceLifecycle(ctx, cfg, spec, runtimeMode, log)
	if err != nil {
		return nil, err
	}
	return NewDataPlane(spec.Name, cfg, app, log)
}

func NewDataPlane(name string, cfg Config, app *lifecycle.Runtime, log *logger.Logger) (*ServiceDataPlane, error) {
	if app == nil {
		return nil, fmt.Errorf("data plane lifecycle is required")
	}
	return &ServiceDataPlane{
		generation: fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
		config:     cfg,
		lifecycle:  app,
		logger:     log,
	}, nil
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
