package minio

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

// Client wraps the official MinIO client with a stable infra boundary.
type Client struct {
	*minio.Client
	DefaultBucket string
	transport     *http.Transport
}

// NewClient creates a MinIO/S3-compatible client.
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
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		Secure:    cfg.UseSSL || cfg.TLS.Enabled,
		Region:    cfg.Region,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}
	return &Client{Client: client, DefaultBucket: cfg.Bucket, transport: transport}, nil
}

// Ping verifies the configured default bucket is reachable.
//
// Empty DefaultBucket intentionally makes Ping a no-op because not every
// service has a single bucket.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Client == nil || c.DefaultBucket == "" {
		return nil
	}
	ok, err := c.BucketExists(ctx, c.DefaultBucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("minio bucket %q does not exist", c.DefaultBucket)
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
		return nil, fmt.Errorf("read minio tls ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("parse minio tls ca_file: no certificates found")
	}
	return pool, nil
}
