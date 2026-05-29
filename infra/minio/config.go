package minio

import (
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

const (
	defaultRegion         = "us-east-1"
	defaultDialTimeout    = "3s"
	defaultRequestTimeout = "10s"
	defaultMaxRetries     = 3
)

// Config describes MinIO/S3-compatible object storage configuration.
//
// It only carries infrastructure configuration. Bucket layout and object key
// semantics belong to service-owned application code.
type Config struct {
	Endpoint       string    `json:"endpoint" yaml:"endpoint"`
	Region         string    `json:"region,omitempty" yaml:"region,omitempty"`
	AccessKey      string    `json:"access_key,omitempty" yaml:"access_key,omitempty"`
	SecretKey      string    `json:"secret_key,omitempty" yaml:"secret_key,omitempty"`
	SessionToken   string    `json:"session_token,omitempty" yaml:"session_token,omitempty"`
	UseSSL         bool      `json:"use_ssl" yaml:"use_ssl"`
	Bucket         string    `json:"bucket,omitempty" yaml:"bucket,omitempty"`
	DialTimeout    string    `json:"dial_timeout,omitempty" yaml:"dial_timeout,omitempty"`
	RequestTimeout string    `json:"request_timeout,omitempty" yaml:"request_timeout,omitempty"`
	MaxRetries     int       `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	TLS            TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

// TLSConfig describes MinIO/S3 TLS connection settings.
type TLSConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	CAFile             string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`
	ServerName         string `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
}

// IsZero reports whether MinIO config is unset.
func (c Config) IsZero() bool {
	return c.Endpoint == "" &&
		c.Region == "" &&
		c.AccessKey == "" &&
		c.SecretKey == "" &&
		c.SessionToken == "" &&
		!c.UseSSL &&
		c.Bucket == "" &&
		c.DialTimeout == "" &&
		c.RequestTimeout == "" &&
		c.MaxRetries == 0 &&
		c.TLS.IsZero()
}

// IsZero reports whether TLS config is unset.
func (c TLSConfig) IsZero() bool {
	return !c.Enabled &&
		c.CAFile == "" &&
		c.ServerName == "" &&
		!c.InsecureSkipVerify
}

// Normalize returns a MinIO config copy with defaults applied.
func (c Config) Normalize() Config {
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.Region = strings.TrimSpace(c.Region)
	if c.Region == "" {
		c.Region = defaultRegion
	}
	c.AccessKey = strings.TrimSpace(c.AccessKey)
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	c.SessionToken = strings.TrimSpace(c.SessionToken)
	c.Bucket = strings.TrimSpace(c.Bucket)
	if c.DialTimeout == "" {
		c.DialTimeout = defaultDialTimeout
	}
	if c.RequestTimeout == "" {
		c.RequestTimeout = defaultRequestTimeout
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultMaxRetries
	}
	return c
}

// Validate checks whether MinIO config satisfies production baseline constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	c = c.Normalize()
	if c.Endpoint == "" {
		return fmt.Errorf("minio endpoint is required")
	}
	if c.AccessKey == "" {
		return fmt.Errorf("minio access_key is required")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("minio secret_key is required")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("minio max_retries must be non-negative")
	}
	if _, err := configutil.PositiveDuration("minio dial_timeout", c.DialTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("minio request_timeout", c.RequestTimeout); err != nil {
		return err
	}
	return nil
}
