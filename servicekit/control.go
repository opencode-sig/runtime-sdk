package servicekit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/opencode-sig/runtime-sdk/logger"
	runtimecontrol "github.com/opencode-sig/runtime-sdk/runtime/control"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

// ControlWatcher listens to runtime-admin commands and rebuilds the DataPlane.
//
// It intentionally lives outside DataPlane generations. A watcher may trigger
// Manager.Rebuild, and that rebuild stops the old generation; keeping the
// watcher outside prevents it from stopping itself.
type ControlWatcher struct {
	store      runtimecontrol.Store
	manager    *Manager
	service    string
	instanceID string
	logger     *logger.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

type ControlWatcherConfig struct {
	Service    string
	InstanceID string
}

func NewControlWatcher(store runtimecontrol.Store, manager *Manager, cfg ControlWatcherConfig, logger *logger.Logger) (*ControlWatcher, error) {
	service := strings.TrimSpace(cfg.Service)
	if service == "" {
		return nil, fmt.Errorf("control watcher service is required")
	}
	if store == nil {
		return nil, fmt.Errorf("control command store is required")
	}
	if manager == nil {
		return nil, fmt.Errorf("data plane manager is required")
	}
	return &ControlWatcher{
		store:      store,
		manager:    manager,
		service:    service,
		instanceID: strings.TrimSpace(cfg.InstanceID),
		logger:     logger,
	}, nil
}

func (w *ControlWatcher) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return nil
	}
	watchCtx, cancel := context.WithCancel(context.Background())
	commands, err := w.store.Watch(watchCtx, w.service)
	if err != nil {
		cancel()
		w.mu.Unlock()
		return err
	}
	done := make(chan struct{})
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()

	go w.run(watchCtx, commands, done)
	return nil
}

func (w *ControlWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.cancel = nil
	w.done = nil
	w.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *ControlWatcher) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel == nil {
		return errors.New("control watcher is not running")
	}
	select {
	case <-w.done:
		return errors.New("control watcher is stopped")
	default:
		return nil
	}
}

func (w *ControlWatcher) run(ctx context.Context, commands <-chan runtimecontrol.Command, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case command, ok := <-commands:
			if !ok {
				return
			}
			w.handle(ctx, command)
		case <-ctx.Done():
			return
		}
	}
}

func (w *ControlWatcher) handle(ctx context.Context, command runtimecontrol.Command) {
	if !w.accept(command) {
		return
	}
	switch strings.ToLower(strings.TrimSpace(command.Command)) {
	case runtimecontrol.CommandRebuild, runtimecontrol.CommandRestart:
		if command.CreatedAt.IsZero() {
			command.CreatedAt = time.Now().UTC()
		}
		reason := strings.TrimSpace(command.Reason)
		if reason == "" {
			reason = command.Command
		}
		logCtx := RebuildLogContext{
			CommandID:  commandID(command),
			Command:    strings.ToLower(strings.TrimSpace(command.Command)),
			Module:     firstNonEmpty(strings.TrimSpace(command.Service), w.service),
			InstanceID: strings.TrimSpace(command.InstanceID),
		}
		rebuildCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := w.manager.RebuildWithLogContext(rebuildCtx, reason, logCtx); err != nil {
			if w.logger != nil {
				w.logger.Error(ctx, "data plane command failed", append(logger.Fields(
					logger.Event("dataplane_command"),
					logger.String("command", command.Command),
					logger.Module(logCtx.Module),
					logger.String("command_id", logCtx.CommandID),
					logger.String("instance_id", logCtx.InstanceID),
				), logger.ErrorFields(err)...)...)
			}
			return
		}
		if w.logger != nil {
			w.logger.Debug(ctx, "data plane command applied",
				logger.Event("dataplane_command"),
				logger.String("command", command.Command),
				logger.Module(logCtx.Module),
				logger.String("command_id", logCtx.CommandID),
				logger.String("instance_id", logCtx.InstanceID),
				logger.String("reason", reason),
			)
		}
	default:
		if w.logger != nil {
			w.logger.Warn(ctx, "runtime command ignored",
				logger.Event("dataplane_command"),
				logger.String("command", command.Command),
				logger.Module(command.Service),
			)
		}
	}
}

func (w *ControlWatcher) accept(command runtimecontrol.Command) bool {
	service := strings.TrimSpace(command.Service)
	if service != "" && service != w.service && service != "all" {
		return false
	}
	instanceID := strings.TrimSpace(command.InstanceID)
	return instanceID == "" || instanceID == w.instanceID
}

func commandID(command runtimecontrol.Command) string {
	if !command.CreatedAt.IsZero() {
		return fmt.Sprintf("%s-%d", strings.TrimSpace(command.Service), command.CreatedAt.UnixNano())
	}
	return strings.TrimSpace(command.Service) + "-" + strings.TrimSpace(command.Command)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// ControlConfigForService returns the command watcher routing identity for a
// service config.
func ControlConfigForService(cfg Config) (ControlWatcherConfig, error) {
	service, address, err := ServiceIdentity(cfg)
	if err != nil {
		return ControlWatcherConfig{}, err
	}
	return ControlWatcherConfig{
		Service:    service,
		InstanceID: registry.InstanceID(service, address),
	}, nil
}
