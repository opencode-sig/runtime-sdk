package servicekit

import (
	"context"
	"errors"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/opencode-sig/runtime-sdk/infra/etcd"
	"github.com/opencode-sig/runtime-sdk/logger"
	runtimecontrol "github.com/opencode-sig/runtime-sdk/runtime/control"
)

// ProcessControl owns the long-lived control-plane resources for one process.
type ProcessControl struct {
	watcher *ControlWatcher
	etcd    *clientv3.Client
}

// NewProcessControl creates a control watcher when runtime config is sourced
// from etcd. File-configured processes return nil.
func NewProcessControl(ctx context.Context, cfg Config, watcherCfg ControlWatcherConfig, manager *Manager, logger *logger.Logger) (*ProcessControl, error) {
	if !cfg.ProcessControlEnabled() {
		return nil, nil
	}
	client, err := etcd.NewClientAndWait(ctx, etcd.Config{Endpoints: cfg.Runtime.Config.Etcd.Endpoints})
	if err != nil {
		return nil, err
	}
	ttl, err := cfg.Runtime.Control.CommandTTLDuration()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	store := runtimecontrol.NewEtcdStore(client, cfg.Runtime.Control.CommandsPrefix)
	if ttl > 0 {
		store.WithTTL(ttl)
	}
	watcher, err := NewControlWatcher(store, manager, watcherCfg, logger)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &ProcessControl{watcher: watcher, etcd: client}, nil
}

func (c *ProcessControl) Start(ctx context.Context) error {
	if c == nil || c.watcher == nil {
		return nil
	}
	return c.watcher.Start(ctx)
}

func (c *ProcessControl) Stop(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var errs []error
	if c.watcher != nil {
		errs = append(errs, c.watcher.Stop(ctx))
	}
	if c.etcd != nil {
		errs = append(errs, c.etcd.Close())
		c.etcd = nil
	}
	return errors.Join(errs...)
}
