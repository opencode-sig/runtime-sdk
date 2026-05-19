package servicekit

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/opencode-sig/runtime-sdk/logger"
)

// ConfigLoader loads the latest servicekit config snapshot.
type ConfigLoader func(ctx context.Context, service string) (Config, error)

// RunOptions configures a standalone servicekit process.
type RunOptions struct {
	Spec       Spec
	LoadConfig ConfigLoader
	Logger     *logger.Logger
}

// Run starts a standalone managed gRPC service process.
//
// It is intended for external services that want to join the platform without
// importing internal application packages. The caller owns the config loader so
// each deployment can decide whether config comes from local files, etcd, or a
// custom bootstrap layer.
func Run(ctx context.Context, opts RunOptions) error {
	spec, err := NewSpec(opts.Spec)
	if err != nil {
		return err
	}
	if opts.LoadConfig == nil {
		return ErrConfigLoaderRequired
	}
	log := opts.Logger
	if log == nil {
		log, err = logger.NewContext(spec.Name)
		if err != nil {
			return err
		}
		defer func() { _ = log.Sync() }()
	}

	initial, err := opts.LoadConfig(ctx, spec.Name)
	if err != nil {
		return err
	}
	useInitial := true
	manager := NewManager(func(ctx context.Context) (DataPlane, error) {
		cfg := initial
		if useInitial {
			useInitial = false
		} else {
			var err error
			cfg, err = opts.LoadConfig(ctx, spec.Name)
			if err != nil {
				return nil, err
			}
		}
		return NewServiceDataPlane(ctx, cfg, spec, RuntimeModeDistributed, log)
	}, log)
	if err := manager.Start(ctx); err != nil {
		return err
	}

	watcherCfg, err := ControlConfigForService(initial)
	if err != nil {
		_ = manager.Stop(ctx)
		return err
	}
	control, err := NewProcessControl(ctx, initial, watcherCfg, manager, log)
	if err != nil {
		_ = manager.Stop(ctx)
		return err
	}
	if err := control.Start(ctx); err != nil {
		_ = manager.Stop(ctx)
		return err
	}

	signalCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-signalCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := control.Stop(shutdownCtx); err != nil {
		log.Error(shutdownCtx, "stop process control failed", logger.ErrorFields(err)...)
	}
	return manager.Stop(shutdownCtx)
}
