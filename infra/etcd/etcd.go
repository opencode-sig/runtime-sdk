package etcd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	defaultEndpoint    = "127.0.0.1:2379"
	defaultDialTimeout = "3s"
)

type Config struct {
	Endpoints   []string `json:"endpoints" yaml:"endpoints"`
	DialTimeout string   `json:"dial_timeout,omitempty" yaml:"dial_timeout,omitempty"`
}

// IsZero reports whether etcd config is unset.
func (c Config) IsZero() bool {
	return len(c.Endpoints) == 0 &&
		strings.TrimSpace(c.DialTimeout) == ""
}

// Normalize returns an etcd config copy with defaults applied.
func (c Config) Normalize() Config {
	c.Endpoints = cleanEndpoints(c.Endpoints)
	if len(c.Endpoints) == 0 {
		c.Endpoints = []string{defaultEndpoint}
	}
	if strings.TrimSpace(c.DialTimeout) == "" {
		c.DialTimeout = defaultDialTimeout
	}
	return c
}

// Validate checks whether etcd config satisfies basic connection constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	c = c.Normalize()
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("etcd endpoints is required")
	}
	if _, err := configutil.PositiveDuration("etcd dial_timeout", c.DialTimeout); err != nil {
		return err
	}
	return nil
}

// NewClient creates an etcd v3 client.
//
// Empty endpoints and dial_timeout use common local defaults so minimal config
// can still start.
func NewClient(cfg Config) (*clientv3.Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	dialTimeout, err := configutil.PositiveDuration("etcd dial_timeout", cfg.DialTimeout)
	if err != nil {
		return nil, err
	}

	return clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: dialTimeout,
	})
}

// NewClientAndWait creates an etcd client and blocks until at least one endpoint is reachable.
func NewClientAndWait(ctx context.Context, cfg Config) (*clientv3.Client, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if err := WaitReady(ctx, client, cfg.Normalize().Endpoints); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// WaitReady waits until etcd answers a Status request.
func WaitReady(ctx context.Context, client *clientv3.Client, endpoints []string) error {
	if client == nil {
		return fmt.Errorf("etcd client is required")
	}
	endpoints = cleanEndpoints(endpoints)
	if len(endpoints) == 0 {
		endpoints = []string{defaultEndpoint}
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

// cleanEndpoints removes blank endpoints before passing them to the etcd client.
func cleanEndpoints(endpoints []string) []string {
	cleaned := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint != "" {
			cleaned = append(cleaned, endpoint)
		}
	}
	return cleaned
}
