package mysql

import (
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

const (
	ModeSingle    = "single"
	ModeReadWrite = "read_write"

	defaultConnTimeout        = "3s"
	defaultReadTimeout        = "3s"
	defaultWriteTimeout       = "3s"
	defaultSlowQueryThreshold = "500ms"
	defaultMaxOpenConns       = 50
	defaultMaxIdleConns       = 10
	defaultConnMaxLifetime    = "1h"
	defaultConnMaxIdleTime    = "15m"
)

// Config describes MySQL connection and pool configuration.
//
// It only carries configuration. ORM or database/sql initialization belongs to
// the application infrastructure layer.
type Config struct {
	Mode                  string     `json:"mode" yaml:"mode"`
	WriteDSNs             []string   `json:"write_dsns" yaml:"write_dsns"`
	ReadDSNs              []string   `json:"read_dsns,omitempty" yaml:"read_dsns,omitempty"`
	WritePool             PoolConfig `json:"write_pool" yaml:"write_pool"`
	ReadPool              PoolConfig `json:"read_pool,omitempty" yaml:"read_pool,omitempty"`
	ConnTimeout           string     `json:"conn_timeout,omitempty" yaml:"conn_timeout,omitempty"`
	ReadTimeout           string     `json:"read_timeout,omitempty" yaml:"read_timeout,omitempty"`
	WriteTimeout          string     `json:"write_timeout,omitempty" yaml:"write_timeout,omitempty"`
	SlowQueryThreshold    string     `json:"slow_query_threshold,omitempty" yaml:"slow_query_threshold,omitempty"`
	RejectUnsafePlaintext bool       `json:"reject_unsafe_plaintext,omitempty" yaml:"reject_unsafe_plaintext,omitempty"`
}

// PoolConfig describes MySQL connection pool settings.
type PoolConfig struct {
	MaxOpenConns    int    `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime string `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime string `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
}

// IsZero reports whether MySQL config is unset.
func (c Config) IsZero() bool {
	return strings.TrimSpace(c.Mode) == "" &&
		len(c.WriteDSNs) == 0 &&
		len(c.ReadDSNs) == 0 &&
		c.WritePool.IsZero() &&
		c.ReadPool.IsZero() &&
		c.ConnTimeout == "" &&
		c.ReadTimeout == "" &&
		c.WriteTimeout == "" &&
		c.SlowQueryThreshold == "" &&
		!c.RejectUnsafePlaintext
}

// IsZero reports whether pool config is unset.
func (p PoolConfig) IsZero() bool {
	return p.MaxOpenConns == 0 &&
		p.MaxIdleConns == 0 &&
		p.ConnMaxLifetime == "" &&
		p.ConnMaxIdleTime == ""
}

// Normalize returns a MySQL config copy with defaults applied.
func (c Config) Normalize() Config {
	if c.Mode == "" {
		c.Mode = ModeSingle
	}
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	c.WritePool = c.WritePool.Normalize()
	if !c.ReadPool.IsZero() || len(c.ReadDSNs) > 0 {
		c.ReadPool = c.ReadPool.Normalize()
	}
	if c.ConnTimeout == "" {
		c.ConnTimeout = defaultConnTimeout
	}
	if c.ReadTimeout == "" {
		c.ReadTimeout = defaultReadTimeout
	}
	if c.WriteTimeout == "" {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.SlowQueryThreshold == "" {
		c.SlowQueryThreshold = defaultSlowQueryThreshold
	}
	return c
}

// Normalize returns a pool config copy with defaults applied.
func (p PoolConfig) Normalize() PoolConfig {
	if p.MaxOpenConns == 0 {
		p.MaxOpenConns = defaultMaxOpenConns
	}
	if p.MaxIdleConns == 0 {
		p.MaxIdleConns = defaultMaxIdleConns
	}
	if p.ConnMaxLifetime == "" {
		p.ConnMaxLifetime = defaultConnMaxLifetime
	}
	if p.ConnMaxIdleTime == "" {
		p.ConnMaxIdleTime = defaultConnMaxIdleTime
	}
	return p
}

// Validate checks whether MySQL config satisfies production baseline constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	c = c.Normalize()
	switch c.Mode {
	case ModeSingle, ModeReadWrite:
	default:
		return fmt.Errorf("unsupported mysql mode %q", c.Mode)
	}
	if len(c.WriteDSNs) == 0 {
		return fmt.Errorf("mysql write_dsns is required")
	}
	if c.Mode == ModeReadWrite && len(c.ReadDSNs) == 0 {
		return fmt.Errorf("mysql read_dsns is required in read_write mode")
	}
	if err := c.WritePool.Validate("mysql write_pool"); err != nil {
		return err
	}
	if !c.ReadPool.IsZero() || len(c.ReadDSNs) > 0 {
		if err := c.ReadPool.Validate("mysql read_pool"); err != nil {
			return err
		}
	}
	if _, err := configutil.PositiveDuration("mysql conn_timeout", c.ConnTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("mysql read_timeout", c.ReadTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("mysql write_timeout", c.WriteTimeout); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration("mysql slow_query_threshold", c.SlowQueryThreshold); err != nil {
		return err
	}
	return nil
}

// Validate checks whether pool config is reasonable.
func (p PoolConfig) Validate(name string) error {
	p = p.Normalize()
	if p.MaxOpenConns < 0 || p.MaxIdleConns < 0 {
		return fmt.Errorf("%s connection counts must be non-negative", name)
	}
	if p.MaxOpenConns > 0 && p.MaxIdleConns > p.MaxOpenConns {
		return fmt.Errorf("%s max_idle_conns must not exceed max_open_conns", name)
	}
	if _, err := configutil.PositiveDuration(name+" conn_max_lifetime", p.ConnMaxLifetime); err != nil {
		return err
	}
	if _, err := configutil.PositiveDuration(name+" conn_max_idle_time", p.ConnMaxIdleTime); err != nil {
		return err
	}
	return nil
}
