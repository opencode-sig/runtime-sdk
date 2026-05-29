package elastic

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	elasticsearch "github.com/elastic/go-elasticsearch/v8"
	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

// Client wraps the official Elasticsearch client with a stable infra boundary.
type Client struct {
	*elasticsearch.Client
	transport *http.Transport
}

// NewClient creates an Elasticsearch client.
//
// The constructor validates config, applies defaults and creates the underlying
// client. It does not force connectivity during startup.
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

	transport, err := newTransport(cfg)
	if err != nil {
		return nil, err
	}
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
		CloudID:   cfg.CloudID,
		APIKey:    cfg.APIKey,
		Transport: transport,
		RetryBackoff: func(attempt int) time.Duration {
			return configutil.MustDuration(cfg.RetryBackoff)
		},
		MaxRetries: cfg.MaxRetries,
	})
	if err != nil {
		return nil, err
	}
	return &Client{Client: client, transport: transport}, nil
}

// Ping verifies the Elasticsearch cluster is reachable.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return nil
	}
	resp, err := c.Info(c.Info.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("elastic info failed: %s", resp.Status())
	}
	return nil
}

// Close releases idle transport resources owned by the client.
func (c *Client) Close() error {
	if c == nil || c.transport == nil {
		return nil
	}
	c.transport.CloseIdleConnections()
	return nil
}

func newTransport(cfg Config) (*http.Transport, error) {
	tlsCfg, err := newTLSConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   configutil.MustDuration(cfg.DialTimeout),
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       tlsCfg,
		ResponseHeaderTimeout: configutil.MustDuration(cfg.RequestTimeout),
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}, nil
}

func newTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	tlsCfg := &tls.Config{
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
	if cfg.CAFile != "" {
		pool, err := certPool(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}
	return tlsCfg, nil
}

func certPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read elastic tls ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("parse elastic tls ca_file: no certificates found")
	}
	return pool, nil
}
