package servicekit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/opencode-sig/runtime-sdk/logger"
	"go.uber.org/zap"
)

// DataPlane is one immutable runtime generation managed by servicekit.
type DataPlane interface {
	Generation() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
}

// Builder creates one DataPlane generation from the latest config snapshot.
type Builder func(ctx context.Context) (DataPlane, error)

// Manager serializes DataPlane rebuilds inside one process.
type Manager struct {
	builder Builder
	logger  *logger.Logger

	mu      sync.Mutex
	current DataPlane
	status  ManagerStatus
}

// ManagerStatus is a snapshot of the current DataPlane generation state.
type ManagerStatus struct {
	Running    bool
	Generation string
	LastReason string
	LastError  string
	UpdatedAt  time.Time
}

// RebuildLogContext carries command-plane correlation fields into rebuild logs.
type RebuildLogContext struct {
	CommandID  string
	Command    string
	Module     string
	InstanceID string
}

func NewManager(builder Builder, logger *logger.Logger) *Manager {
	return &Manager{builder: builder, logger: logger}
}

func (m *Manager) Start(ctx context.Context) error {
	_, err := m.Rebuild(ctx, "initial_start")
	return err
}

func (m *Manager) Rebuild(ctx context.Context, reason string) (DataPlane, error) {
	return m.RebuildWithLogContext(ctx, reason, RebuildLogContext{})
}

func (m *Manager) RebuildWithLogContext(ctx context.Context, reason string, logCtx RebuildLogContext) (DataPlane, error) {
	if m == nil || m.builder == nil {
		return nil, fmt.Errorf("data plane builder is required")
	}
	startedAt := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	old := m.current
	oldGeneration := generationOf(old)
	if logCtx.Module == "" {
		logCtx.Module = moduleFromGeneration(oldGeneration)
	}
	if m.logger != nil {
		m.logger.Info(ctx, "data plane rebuild started", rebuildFields(logCtx,
			logger.String("reason", reason),
			logger.String("old_generation", oldGeneration),
		)...)
	}
	next, err := m.builder(ctx)
	if err != nil {
		m.recordLocked(false, oldGeneration, reason, err)
		m.logRebuildFailure(ctx, "data plane build failed", reason, oldGeneration, "", logCtx, err)
		return nil, err
	}
	nextGeneration := next.Generation()
	if logCtx.Module == "" {
		logCtx.Module = moduleFromGeneration(nextGeneration)
	}
	if old != nil {
		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = old.Stop(stopCtx)
		cancel()
		if err != nil {
			m.recordLocked(true, oldGeneration, reason, err)
			m.logRebuildFailure(ctx, "data plane old generation stop failed", reason, oldGeneration, nextGeneration, logCtx, err)
			return nil, err
		}
		m.current = nil
	}
	if err := next.Start(ctx); err != nil {
		m.recordLocked(false, "", reason, err)
		m.logRebuildFailure(ctx, "data plane next generation start failed", reason, oldGeneration, nextGeneration, logCtx, err)
		return nil, err
	}
	m.current = next
	m.recordLocked(true, nextGeneration, reason, nil)
	if m.logger != nil {
		m.logger.Info(ctx, "data plane rebuild completed", rebuildFields(logCtx,
			logger.String("reason", reason),
			logger.String("old_generation", oldGeneration),
			logger.String("generation", nextGeneration),
			logger.Duration("duration", time.Since(startedAt)),
			logger.String("status", "success"),
		)...)
	}
	return next, nil
}

func (m *Manager) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	current := m.current
	m.current = nil
	m.recordLocked(false, "", "stop", nil)
	m.mu.Unlock()
	if current == nil {
		return nil
	}
	return current.Stop(ctx)
}

func (m *Manager) Health(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("data plane manager is not configured")
	}
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	if current == nil {
		return fmt.Errorf("data plane is not running")
	}
	return current.Health(ctx)
}

// Status returns a point-in-time snapshot of the current DataPlane state.
func (m *Manager) Status() ManagerStatus {
	if m == nil {
		return ManagerStatus{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) recordLocked(running bool, generation string, reason string, err error) {
	status := ManagerStatus{
		Running:    running,
		Generation: generation,
		LastReason: reason,
		UpdatedAt:  time.Now().UTC(),
	}
	if err != nil {
		status.LastError = err.Error()
	}
	m.status = status
}

func (m *Manager) logRebuildFailure(ctx context.Context, msg string, reason string, oldGeneration string, nextGeneration string, logCtx RebuildLogContext, err error) {
	if m.logger == nil {
		return
	}
	fields := rebuildFields(logCtx,
		logger.String("reason", reason),
		logger.String("old_generation", oldGeneration),
		logger.String("next_generation", nextGeneration),
		logger.String("status", "failed"),
	)
	fields = append(fields, logger.ErrorFields(err)...)
	m.logger.Error(ctx, msg, fields...)
}

func generationOf(plane DataPlane) string {
	if plane == nil {
		return ""
	}
	return plane.Generation()
}

func rebuildFields(logCtx RebuildLogContext, fields ...zap.Field) []zap.Field {
	base := logger.Fields(
		logger.Event("dataplane_rebuild"),
		logger.Operation("rebuild"),
	)
	if logCtx.Module != "" {
		base = append(base, logger.Module(logCtx.Module))
	}
	if logCtx.CommandID != "" {
		base = append(base, logger.String("command_id", logCtx.CommandID))
	}
	if logCtx.Command != "" {
		base = append(base, logger.String("command", logCtx.Command))
	}
	if logCtx.InstanceID != "" {
		base = append(base, logger.String("target_instance_id", logCtx.InstanceID))
	}
	return append(base, fields...)
}

func moduleFromGeneration(generation string) string {
	generation = strings.TrimSpace(generation)
	if generation == "" {
		return ""
	}
	index := strings.LastIndex(generation, "-")
	if index <= 0 {
		return generation
	}
	return generation[:index]
}
