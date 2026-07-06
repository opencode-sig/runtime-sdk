package mysql

import (
	"fmt"
	"strings"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"
)

const (
	defaultPort             = 3306
	defaultEnsureCharset    = "utf8mb4"
	defaultEnsureCollation  = "utf8mb4_unicode_ci"
	defaultMaxOpenConns     = 50
	defaultMaxIdleConns     = 10
	defaultConnMaxLifetime  = "1h"
	defaultConnMaxIdleTime  = "15m"
	defaultInstanceName     = "default"
	instanceNameSeparator   = "."
	mysqlNetwork            = "tcp"
	anonymousSingleServerID = ""
)

// Config describes structured MySQL servers, databases and pool settings.
//
// DSNs are intentionally not part of the public configuration contract. Runtime
// compiles this structure into driver DSNs internally.
type Config struct {
	Host     string            `json:"host" yaml:"host"`
	Port     int               `json:"port" yaml:"port"`
	Username string            `json:"username" yaml:"username"`
	Password string            `json:"password" yaml:"password"`
	Params   map[string]string `json:"params" yaml:"params"`

	Write EndpointConfig   `json:"write" yaml:"write"`
	Reads []EndpointConfig `json:"reads" yaml:"reads"`

	Databases map[string]DatabaseConfig `json:"databases" yaml:"databases"`
	Servers   map[string]ServerConfig   `json:"servers" yaml:"servers"`

	Pool     PoolConfig `json:"pool" yaml:"pool"`
	ReadPool PoolConfig `json:"read_pool" yaml:"read_pool"`
}

// ServerConfig describes one MySQL server or write/read topology.
type ServerConfig struct {
	Host     string            `json:"host" yaml:"host"`
	Port     int               `json:"port" yaml:"port"`
	Username string            `json:"username" yaml:"username"`
	Password string            `json:"password" yaml:"password"`
	Params   map[string]string `json:"params" yaml:"params"`

	Write EndpointConfig   `json:"write" yaml:"write"`
	Reads []EndpointConfig `json:"reads" yaml:"reads"`

	Databases map[string]DatabaseConfig `json:"databases" yaml:"databases"`

	Pool     PoolConfig `json:"pool" yaml:"pool"`
	ReadPool PoolConfig `json:"read_pool" yaml:"read_pool"`
}

// EndpointConfig describes a concrete MySQL endpoint.
type EndpointConfig struct {
	Host     string            `json:"host" yaml:"host"`
	Port     int               `json:"port" yaml:"port"`
	Username string            `json:"username" yaml:"username"`
	Password string            `json:"password" yaml:"password"`
	Params   map[string]string `json:"params" yaml:"params"`
}

// DatabaseConfig describes a logical database exposed as a MySQL infra instance.
type DatabaseConfig struct {
	Name   string               `json:"name" yaml:"name"`
	Params map[string]string    `json:"params" yaml:"params"`
	Pool   PoolConfig           `json:"pool" yaml:"pool"`
	Ensure EnsureDatabaseConfig `json:"ensure" yaml:"ensure"`
}

// EnsureDatabaseConfig optionally creates a missing database before opening pools.
type EnsureDatabaseConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Charset   string `json:"charset" yaml:"charset"`
	Collation string `json:"collation" yaml:"collation"`
}

// PoolConfig describes MySQL connection pool settings.
type PoolConfig struct {
	MaxOpenConns    int    `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime string `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime string `json:"conn_max_idle_time" yaml:"conn_max_idle_time"`
}

// IsZero reports whether pool config is unset.
func (p PoolConfig) IsZero() bool {
	return p.MaxOpenConns == 0 &&
		p.MaxIdleConns == 0 &&
		p.ConnMaxLifetime == "" &&
		p.ConnMaxIdleTime == ""
}

// IsZero reports whether MySQL config is unset.
func (c Config) IsZero() bool {
	return strings.TrimSpace(c.Host) == "" &&
		c.Port == 0 &&
		strings.TrimSpace(c.Username) == "" &&
		c.Password == "" &&
		len(c.Params) == 0 &&
		c.Write.IsZero() &&
		len(c.Reads) == 0 &&
		len(c.Databases) == 0 &&
		len(c.Servers) == 0 &&
		c.Pool.IsZero() &&
		c.ReadPool.IsZero()
}

// IsZero reports whether server config is unset.
func (s ServerConfig) IsZero() bool {
	return strings.TrimSpace(s.Host) == "" &&
		s.Port == 0 &&
		strings.TrimSpace(s.Username) == "" &&
		s.Password == "" &&
		len(s.Params) == 0 &&
		s.Write.IsZero() &&
		len(s.Reads) == 0 &&
		len(s.Databases) == 0 &&
		s.Pool.IsZero() &&
		s.ReadPool.IsZero()
}

// IsZero reports whether endpoint config is unset.
func (e EndpointConfig) IsZero() bool {
	return strings.TrimSpace(e.Host) == "" &&
		e.Port == 0 &&
		strings.TrimSpace(e.Username) == "" &&
		e.Password == "" &&
		len(e.Params) == 0
}

// IsZero reports whether database config is unset.
func (d DatabaseConfig) IsZero() bool {
	return strings.TrimSpace(d.Name) == "" &&
		len(d.Params) == 0 &&
		d.Pool.IsZero() &&
		d.Ensure.IsZero()
}

// IsZero reports whether ensure config is unset.
func (e EnsureDatabaseConfig) IsZero() bool {
	return !e.Enabled &&
		strings.TrimSpace(e.Charset) == "" &&
		strings.TrimSpace(e.Collation) == ""
}

// Normalize returns a MySQL config copy with defaults applied where possible.
func (c Config) Normalize() Config {
	c.Pool = c.Pool.Normalize()
	if len(c.Reads) > 0 || !c.ReadPool.IsZero() {
		c.ReadPool = c.ReadPool.Normalize()
	}
	if len(c.Databases) > 0 {
		databases := make(map[string]DatabaseConfig, len(c.Databases))
		for name, database := range c.Databases {
			databases[name] = database.Normalize()
		}
		c.Databases = databases
	}
	if len(c.Servers) > 0 {
		servers := make(map[string]ServerConfig, len(c.Servers))
		for name, server := range c.Servers {
			servers[name] = server.Normalize()
		}
		c.Servers = servers
	}
	return c
}

// Normalize returns a server config copy with defaults applied where possible.
func (s ServerConfig) Normalize() ServerConfig {
	s.Pool = s.Pool.Normalize()
	if len(s.Reads) > 0 || !s.ReadPool.IsZero() {
		s.ReadPool = s.ReadPool.Normalize()
	}
	if len(s.Databases) > 0 {
		databases := make(map[string]DatabaseConfig, len(s.Databases))
		for name, database := range s.Databases {
			databases[name] = database.Normalize()
		}
		s.Databases = databases
	}
	return s
}

// Normalize returns a database config copy with defaults applied where possible.
func (d DatabaseConfig) Normalize() DatabaseConfig {
	d.Pool = d.Pool.Normalize()
	d.Ensure = d.Ensure.Normalize()
	return d
}

// Normalize returns an ensure config copy with defaults applied when enabled.
func (e EnsureDatabaseConfig) Normalize() EnsureDatabaseConfig {
	if !e.Enabled {
		return e
	}
	if strings.TrimSpace(e.Charset) == "" {
		e.Charset = defaultEnsureCharset
	}
	if strings.TrimSpace(e.Collation) == "" {
		e.Collation = defaultEnsureCollation
	}
	return e
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

// Validate checks whether MySQL config satisfies runtime constraints.
func (c Config) Validate() error {
	if c.IsZero() {
		return nil
	}
	_, err := c.Compile()
	return err
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
