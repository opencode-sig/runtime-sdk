package redis

import (
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

const (
	ModeSingle   = "single"
	ModeSentinel = "sentinel"
	ModeCluster  = "cluster"

	defaultDialTimeout     = "2s"
	defaultReadTimeout     = "1s"
	defaultWriteTimeout    = "1s"
	defaultPoolSize        = 20
	defaultMinIdleConns    = 2
	defaultMaxIdleConns    = 10
	defaultConnMaxIdleTime = "5m"
	defaultConnMaxLifetime = "1h"
	defaultMaxRetries      = 3
)

// Config describes Redis connection and pool configuration.
//
// It only expresses infrastructure configuration. NewClient creates the client
// without forcing a Ping.
type Config struct {
	Mode            string    `json:"mode" yaml:"mode"`
	Addrs           []string  `json:"addrs" yaml:"addrs"`
	Username        string    `json:"username,omitempty" yaml:"username,omitempty"`
	Password        string    `json:"password,omitempty" yaml:"password,omitempty"`
	DB              int       `json:"db" yaml:"db"`
	MasterName      string    `json:"master_name,omitempty" yaml:"master_name,omitempty"`
	DialTimeout     string    `json:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout     string    `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    string    `json:"write_timeout" yaml:"write_timeout"`
	PoolSize        int       `json:"pool_size,omitempty" yaml:"pool_size,omitempty"`
	MinIdleConns    int       `json:"min_idle_conns,omitempty" yaml:"min_idle_conns,omitempty"`
	MaxIdleConns    int       `json:"max_idle_conns,omitempty" yaml:"max_idle_conns,omitempty"`
	ConnMaxIdleTime string    `json:"conn_max_idle_time,omitempty" yaml:"conn_max_idle_time,omitempty"`
	ConnMaxLifetime string    `json:"conn_max_lifetime,omitempty" yaml:"conn_max_lifetime,omitempty"`
	MaxRetries      int       `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	TLS             TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

// TLSConfig describes Redis TLS connection settings.
type TLSConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	ServerName         string `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
}

// IsZero reports whether Redis config is unset.
func (c Config) IsZero() bool {
	return strings.TrimSpace(c.Mode) == "" &&
		len(c.Addrs) == 0 &&
		c.Username == "" &&
		c.Password == "" &&
		c.DB == 0 &&
		c.MasterName == "" &&
		c.DialTimeout == "" &&
		c.ReadTimeout == "" &&
		c.WriteTimeout == "" &&
		c.PoolSize == 0 &&
		c.MinIdleConns == 0 &&
		c.MaxIdleConns == 0 &&
		c.ConnMaxIdleTime == "" &&
		c.ConnMaxLifetime == "" &&
		c.MaxRetries == 0 &&
		!c.TLS.Enabled
}

// Normalize returns a Redis config copy with defaults applied.
func (c Config) Normalize() Config {
	if c.Mode == "" {
		c.Mode = ModeSingle
	}
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.DialTimeout == "" {
		c.DialTimeout = defaultDialTimeout
	}
	if c.ReadTimeout == "" {
		c.ReadTimeout = defaultReadTimeout
	}
	if c.WriteTimeout == "" {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.PoolSize == 0 {
		c.PoolSize = defaultPoolSize
	}
	if c.MinIdleConns == 0 {
		c.MinIdleConns = defaultMinIdleConns
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = defaultMaxIdleConns
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.ConnMaxIdleTime == "" {
		c.ConnMaxIdleTime = defaultConnMaxIdleTime
	}
	if c.ConnMaxLifetime == "" {
		c.ConnMaxLifetime = defaultConnMaxLifetime
	}
	return c
}

// Validate checks whether Redis config satisfies production baseline constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	c = c.Normalize()
	if len(c.Addrs) == 0 {
		return fmt.Errorf("redis addrs is required")
	}
	switch c.Mode {
	case ModeSingle, ModeCluster:
	case ModeSentinel:
		if strings.TrimSpace(c.MasterName) == "" {
			return fmt.Errorf("redis master_name is required in sentinel mode")
		}
	default:
		return fmt.Errorf("unsupported redis mode %q", c.Mode)
	}
	if c.DB < 0 {
		return fmt.Errorf("redis db must be non-negative")
	}
	if c.PoolSize < 0 || c.MinIdleConns < 0 || c.MaxIdleConns < 0 || c.MaxRetries < 0 {
		return fmt.Errorf("redis pool and retry values must be non-negative")
	}
	if _, err := configutil.PositiveDuration("redis dial_timeout", c.DialTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("redis read_timeout", c.ReadTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("redis write_timeout", c.WriteTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("redis conn_max_idle_time", c.ConnMaxIdleTime); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("redis conn_max_lifetime", c.ConnMaxLifetime); err != nil {
		return err
	}
	return nil
}
