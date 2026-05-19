package redis

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps go-redis UniversalClient with a small stable infra boundary.
type Client struct {
	goredis.UniversalClient
}

// NewClient creates a Redis client for single, sentinel or cluster mode.
//
// The constructor validates config, applies defaults and creates the underlying
// go-redis client. It does not ping Redis during startup; connectivity is a
// data-plane concern and go-redis reconnects on later commands.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	if len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("redis addrs is required")
	}
	client := goredis.NewUniversalClient(universalOptions(cfg))
	return &Client{UniversalClient: client}, nil
}

// Ping verifies the Redis connection is usable.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.UniversalClient == nil {
		return nil
	}
	return c.UniversalClient.Ping(ctx).Err()
}

func universalOptions(cfg Config) *goredis.UniversalOptions {
	options := &goredis.UniversalOptions{
		Addrs:           cfg.Addrs,
		Username:        cfg.Username,
		Password:        cfg.Password,
		DB:              cfg.DB,
		MasterName:      cfg.MasterName,
		DialTimeout:     configutil.MustDuration(cfg.DialTimeout),
		ReadTimeout:     configutil.MustDuration(cfg.ReadTimeout),
		WriteTimeout:    configutil.MustDuration(cfg.WriteTimeout),
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxIdleTime: configutil.MustDuration(cfg.ConnMaxIdleTime),
		ConnMaxLifetime: configutil.MustDuration(cfg.ConnMaxLifetime),
		MaxRetries:      cfg.MaxRetries,
	}
	if cfg.TLS.Enabled {
		options.TLSConfig = &tls.Config{
			ServerName:         cfg.TLS.ServerName,
			InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
		}
	}
	return options
}
