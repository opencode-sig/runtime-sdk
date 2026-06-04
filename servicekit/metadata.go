package servicekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/opencode-sig/runtime-sdk/logger"
	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
	"github.com/opencode-sig/runtime-sdk/runtime/defaults"
	gatewaymeta "github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

var metadataReconcileInterval = 5 * time.Minute
var metadataReconcileTimeout = 30 * time.Second

type MetadataPrefixes struct {
	RoutesPrefix      string
	DescriptorsPrefix string
}

// MetadataPublisher publishes service-owned Gateway metadata to etcd.
type MetadataPublisher struct {
	client      *clientv3.Client
	prefixes    MetadataPrefixes
	routes      []gatewaymeta.RouteMeta
	descriptors map[string][]byte
	logger      *logger.Logger
	metrics     *runtimemetrics.ControlPlaneMetrics
	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	published   atomic.Bool
	healthy     atomic.Bool
}

func NewMetadataPublisher(client *clientv3.Client, prefixes MetadataPrefixes, routes []gatewaymeta.RouteMeta, descriptors map[string][]byte) *MetadataPublisher {
	copiedDescriptors := make(map[string][]byte, len(descriptors))
	for id, data := range descriptors {
		copiedDescriptors[id] = append([]byte(nil), data...)
	}
	return &MetadataPublisher{
		client:      client,
		prefixes:    normalizeMetadataPrefixes(prefixes),
		routes:      append([]gatewaymeta.RouteMeta(nil), routes...),
		descriptors: copiedDescriptors,
	}
}

func (p *MetadataPublisher) WithLogger(log *logger.Logger) *MetadataPublisher {
	if p != nil {
		p.logger = log
	}
	return p
}

func (p *MetadataPublisher) WithControlPlaneMetrics(metrics *runtimemetrics.ControlPlaneMetrics) *MetadataPublisher {
	if p != nil {
		p.metrics = metrics
	}
	return p
}

func (p *MetadataPublisher) Start(ctx context.Context) error {
	if p.client == nil {
		return errors.New("etcd client is required")
	}
	if err := p.publish(ctx); err != nil {
		p.markError("publish")
		p.logPublishFailed(ctx, err)
		return err
	}
	p.published.Store(true)
	p.markHealthy()
	p.logPublished(ctx, "gateway metadata published")
	reconcileCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	p.mu.Lock()
	p.cancel = cancel
	p.done = done
	p.mu.Unlock()
	go p.reconcileLoop(reconcileCtx, done)
	return nil
}

func (p *MetadataPublisher) publish(ctx context.Context) error {
	for id, data := range p.descriptors {
		if strings.TrimSpace(id) == "" || len(data) == 0 {
			return fmt.Errorf("descriptor %q is invalid", id)
		}
		if _, err := p.client.Put(ctx, metadataDescriptorKey(p.prefixes, id), string(data)); err != nil {
			return err
		}
	}
	for _, route := range p.routes {
		if err := route.Validate(); err != nil {
			return err
		}
		data, err := json.Marshal(route)
		if err != nil {
			return err
		}
		if _, err := p.client.Put(ctx, metadataRouteKey(p.prefixes, route.ID), string(data)); err != nil {
			return err
		}
	}
	return nil
}

func (p *MetadataPublisher) Stop(ctx context.Context) error {
	p.mu.Lock()
	cancel := p.cancel
	done := p.done
	p.cancel = nil
	p.done = nil
	p.mu.Unlock()
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
	p.published.Store(false)
	p.markUnhealthy()
	return nil
}

func (p *MetadataPublisher) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if !p.published.Load() {
		return errors.New("gateway metadata is not published")
	}
	return nil
}

func (p *MetadataPublisher) reconcileLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(metadataReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			publishCtx, cancel := context.WithTimeout(context.Background(), metadataReconcileTimeout)
			err := p.publish(publishCtx)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				p.markError("publish")
				p.logPublishFailed(ctx, err)
				continue
			}
			p.published.Store(true)
			p.markRecovered("publish")
		}
	}
}

func (p *MetadataPublisher) markHealthy() {
	p.healthy.Store(true)
	p.metrics.SetStatus("gateway_metadata", true)
}

func (p *MetadataPublisher) markUnhealthy() {
	p.healthy.Store(false)
	p.metrics.SetStatus("gateway_metadata", false)
}

func (p *MetadataPublisher) markError(operation string) {
	p.healthy.Store(false)
	p.metrics.SetStatus("gateway_metadata", false)
	p.metrics.RecordError("gateway_metadata", operation)
}

func (p *MetadataPublisher) markRecovered(operation string) {
	if !p.healthy.Swap(true) {
		p.metrics.RecordRecovery("gateway_metadata", operation)
		p.logPublished(context.Background(), "gateway metadata publication recovered")
	}
	p.metrics.SetStatus("gateway_metadata", true)
}

func (p *MetadataPublisher) logPublished(ctx context.Context, msg string) {
	if p.logger == nil {
		return
	}
	p.logger.Info(ctx, msg,
		logger.Event("gateway_metadata_published"),
		logger.String("routes_prefix", p.prefixes.RoutesPrefix),
		logger.String("descriptors_prefix", p.prefixes.DescriptorsPrefix),
	)
}

func (p *MetadataPublisher) logPublishFailed(ctx context.Context, err error) {
	if p.logger == nil {
		return
	}
	fields := append(logger.Fields(
		logger.Event("gateway_metadata_publish_failed"),
		logger.String("routes_prefix", p.prefixes.RoutesPrefix),
		logger.String("descriptors_prefix", p.prefixes.DescriptorsPrefix),
	), logger.ErrorFields(err)...)
	p.logger.Warn(ctx, "gateway metadata publish failed", fields...)
}

func metadataRouteKey(prefixes MetadataPrefixes, id string) string {
	return prefixes.RoutesPrefix + "/" + strings.Trim(id, "/")
}

func metadataDescriptorKey(prefixes MetadataPrefixes, id string) string {
	return prefixes.DescriptorsPrefix + "/" + strings.Trim(id, "/")
}

func normalizeMetadataPrefixes(prefixes MetadataPrefixes) MetadataPrefixes {
	return MetadataPrefixes{
		RoutesPrefix:      defaults.CleanPrefix(prefixes.RoutesPrefix, defaults.RoutesPrefix),
		DescriptorsPrefix: defaults.CleanPrefix(prefixes.DescriptorsPrefix, defaults.DescriptorsPrefix),
	}
}
