package config

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var ErrConfigNotFound = errors.New("config not found")

type EtcdProvider struct {
	Endpoints []string
	Prefix    string
	client    *clientv3.Client
	owns      bool
	mu        sync.Mutex
}

// NewEtcdProvider creates a config provider that owns its etcd client lifecycle.
func NewEtcdProvider(endpoints []string, prefix string) *EtcdProvider {
	return &EtcdProvider{Endpoints: endpoints, Prefix: cleanPrefix(prefix), owns: true}
}

// NewEtcdProviderWithClient creates a provider using an externally owned etcd client.
//
// The provider will not close the external client, which is useful for tests or
// runtimes that manage the client lifecycle elsewhere.
func NewEtcdProviderWithClient(client *clientv3.Client, prefix string) *EtcdProvider {
	return &EtcdProvider{Prefix: cleanPrefix(prefix), client: client}
}

// Close closes the etcd client created by this provider.
func (p *EtcdProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil || !p.owns {
		return nil
	}
	err := p.client.Close()
	p.client = nil
	return err
}

// Load reads one config key from etcd.
func (p *EtcdProvider) Load(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, errors.New("config key is required")
	}
	client, err := p.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := p.waitReady(ctx, client); err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, p.key(key))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, key)
	}
	return append([]byte(nil), resp.Kvs[0].Value...), nil
}

// Get reads one config entry from etcd with version metadata.
func (p *EtcdProvider) Get(ctx context.Context, key string) (Entry, error) {
	if key == "" {
		return Entry{}, errors.New("config key is required")
	}
	client, err := p.ensureClient()
	if err != nil {
		return Entry{}, err
	}
	if err := p.waitReady(ctx, client); err != nil {
		return Entry{}, err
	}

	resp, err := client.Get(ctx, p.key(key))
	if err != nil {
		return Entry{}, err
	}
	if len(resp.Kvs) == 0 {
		return Entry{}, fmt.Errorf("%w: %s", ErrConfigNotFound, key)
	}
	return entryFromKV(p.logicalKey(string(resp.Kvs[0].Key)), resp.Kvs[0]), nil
}

// Put writes one config entry.
func (p *EtcdProvider) Put(ctx context.Context, key string, value []byte) error {
	if key == "" {
		return errors.New("config key is required")
	}
	client, err := p.ensureClient()
	if err != nil {
		return err
	}
	if err := p.waitReady(ctx, client); err != nil {
		return err
	}
	_, err = client.Put(ctx, p.key(key), string(value))
	return err
}

// PutIfAbsent writes one config entry only when the key does not exist.
func (p *EtcdProvider) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if key == "" {
		return false, errors.New("config key is required")
	}
	client, err := p.ensureClient()
	if err != nil {
		return false, err
	}
	if err := p.waitReady(ctx, client); err != nil {
		return false, err
	}
	fullKey := p.key(key)
	resp, err := client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(fullKey), "=", 0)).
		Then(clientv3.OpPut(fullKey, string(value))).
		Commit()
	if err != nil {
		return false, err
	}
	return resp.Succeeded, nil
}

// Delete removes one config entry.
func (p *EtcdProvider) Delete(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("config key is required")
	}
	client, err := p.ensureClient()
	if err != nil {
		return err
	}
	if err := p.waitReady(ctx, client); err != nil {
		return err
	}
	_, err = client.Delete(ctx, p.key(key))
	return err
}

// List returns config entries under a logical prefix.
func (p *EtcdProvider) List(ctx context.Context, prefix string) ([]Entry, error) {
	client, err := p.ensureClient()
	if err != nil {
		return nil, err
	}
	if err := p.waitReady(ctx, client); err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, p.key(prefix), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		entries = append(entries, entryFromKV(p.logicalKey(string(kv.Key)), kv))
	}
	return entries, nil
}

// ensureClient returns an available etcd client and lazily creates one when needed.
func (p *EtcdProvider) ensureClient() (*clientv3.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		return p.client, nil
	}
	endpoints := p.Endpoints
	if len(endpoints) == 0 {
		endpoints = []string{"127.0.0.1:2379"}
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	p.client = client
	return client, nil
}

func (p *EtcdProvider) waitReady(ctx context.Context, client *clientv3.Client) error {
	endpoints := p.Endpoints
	if len(endpoints) == 0 {
		endpoints = client.Endpoints()
	}
	if len(endpoints) == 0 {
		endpoints = []string{"127.0.0.1:2379"}
	}
	backoff := 500 * time.Millisecond
	for {
		for _, endpoint := range endpoints {
			statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, err := client.Status(statusCtx, endpoint)
			cancel()
			if err == nil {
				return nil
			}
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < 3*time.Second {
			backoff *= 2
			if backoff > 3*time.Second {
				backoff = 3 * time.Second
			}
		}
	}
}

// key converts a logical config key into a full etcd key.
func (p *EtcdProvider) key(key string) string {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "/") {
		return cleanPrefix(key)
	}
	if p.Prefix == "" {
		return strings.Trim(key, "/")
	}
	return p.Prefix + "/" + strings.Trim(key, "/")
}

// logicalKey converts a full etcd key back into a logical config key.
func (p *EtcdProvider) logicalKey(fullKey string) string {
	prefix := p.Prefix + "/"
	if strings.HasPrefix(fullKey, prefix) {
		return strings.TrimPrefix(fullKey, prefix)
	}
	return fullKey
}

// cleanPrefix normalizes the config-center prefix.
func cleanPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	return "/" + strings.Trim(prefix, "/")
}

// entryFromKV converts an etcd KeyValue into a config entry.
func entryFromKV(key string, kv *mvccpb.KeyValue) Entry {
	return Entry{
		Key:     key,
		Value:   append([]byte(nil), kv.Value...),
		Version: strconv.FormatInt(kv.ModRevision, 10),
	}
}
