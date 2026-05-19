package servicekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"

	gatewaymeta "github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

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

func (p *MetadataPublisher) Start(ctx context.Context) error {
	if p.client == nil {
		return errors.New("etcd client is required")
	}
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
	return nil
}

func (p *MetadataPublisher) Health(ctx context.Context) error {
	if p.client == nil {
		return errors.New("etcd client is required")
	}
	for _, route := range p.routes {
		resp, err := p.client.Get(ctx, metadataRouteKey(p.prefixes, route.ID))
		if err != nil {
			return err
		}
		if len(resp.Kvs) == 0 {
			return fmt.Errorf("route metadata %s is not published", route.ID)
		}
	}
	return nil
}

func metadataRouteKey(prefixes MetadataPrefixes, id string) string {
	return prefixes.RoutesPrefix + "/" + strings.Trim(id, "/")
}

func metadataDescriptorKey(prefixes MetadataPrefixes, id string) string {
	return prefixes.DescriptorsPrefix + "/" + strings.Trim(id, "/")
}

func normalizeMetadataPrefixes(prefixes MetadataPrefixes) MetadataPrefixes {
	return MetadataPrefixes{
		RoutesPrefix:      cleanPrefix(prefixes.RoutesPrefix, "/runtime/gateway/routes"),
		DescriptorsPrefix: cleanPrefix(prefixes.DescriptorsPrefix, "/runtime/gateway/descriptors"),
	}
}

func cleanPrefix(prefix string, fallback string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return fallback
	}
	return "/" + strings.Trim(prefix, "/")
}
