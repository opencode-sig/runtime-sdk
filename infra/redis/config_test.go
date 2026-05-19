package redis

import (
	"context"
	"testing"
)

func TestConfigValidateAllowsZeroConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("validate zero config: %v", err)
	}
}

func TestConfigNormalizeDefaultsOptionalValues(t *testing.T) {
	cfg := Config{
		Addrs: []string{"127.0.0.1:6379"},
	}

	normalized := cfg.Normalize()
	if normalized.Mode != ModeSingle {
		t.Fatalf("mode = %q", normalized.Mode)
	}
	if normalized.DialTimeout != defaultDialTimeout {
		t.Fatalf("dial timeout = %q", normalized.DialTimeout)
	}
	if normalized.ReadTimeout != defaultReadTimeout {
		t.Fatalf("read timeout = %q", normalized.ReadTimeout)
	}
	if normalized.WriteTimeout != defaultWriteTimeout {
		t.Fatalf("write timeout = %q", normalized.WriteTimeout)
	}
	if normalized.PoolSize != defaultPoolSize {
		t.Fatalf("pool size = %d", normalized.PoolSize)
	}
	if normalized.MinIdleConns != defaultMinIdleConns {
		t.Fatalf("min idle conns = %d", normalized.MinIdleConns)
	}
	if normalized.MaxIdleConns != defaultMaxIdleConns {
		t.Fatalf("max idle conns = %d", normalized.MaxIdleConns)
	}
	if normalized.MaxRetries != defaultMaxRetries {
		t.Fatalf("max retries = %d", normalized.MaxRetries)
	}
	if normalized.ConnMaxIdleTime != defaultConnMaxIdleTime {
		t.Fatalf("conn max idle time = %q", normalized.ConnMaxIdleTime)
	}
	if normalized.ConnMaxLifetime != defaultConnMaxLifetime {
		t.Fatalf("conn max lifetime = %q", normalized.ConnMaxLifetime)
	}
}

func TestConfigValidateAllowsZeroOptionalValues(t *testing.T) {
	cfg := Config{
		Addrs: []string{"127.0.0.1:6379"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestConfigValidateRejectsNegativeOptionalValues(t *testing.T) {
	cfg := Config{
		Addrs:    []string{"127.0.0.1:6379"},
		PoolSize: -1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected negative pool size error")
	}
}

func TestConfigValidateRejectsUnsupportedMode(t *testing.T) {
	cfg := Config{
		Mode:  "proxy",
		Addrs: []string{"127.0.0.1:6379"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func TestConfigValidateRejectsSentinelWithoutMasterName(t *testing.T) {
	cfg := Config{
		Mode:  ModeSentinel,
		Addrs: []string{"127.0.0.1:26379"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing sentinel master name error")
	}
}

func TestConfigValidateRejectsInvalidDuration(t *testing.T) {
	cfg := Config{
		Addrs:       []string{"127.0.0.1:6379"},
		DialTimeout: "bad",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestNewClientRejectsZeroConfig(t *testing.T) {
	if _, err := NewClient(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing redis addrs error")
	}
}

func TestUniversalOptionsMapsConfig(t *testing.T) {
	cfg := Config{
		Mode:         ModeSingle,
		Addrs:        []string{"127.0.0.1:6379"},
		Username:     "user",
		Password:     "pass",
		DB:           2,
		PoolSize:     10,
		MinIdleConns: 1,
		MaxIdleConns: 3,
		TLS: TLSConfig{
			Enabled:    true,
			ServerName: "redis.local",
		},
	}.Normalize()

	options := universalOptions(cfg)
	if len(options.Addrs) != 1 || options.Addrs[0] != "127.0.0.1:6379" {
		t.Fatalf("addrs = %#v", options.Addrs)
	}
	if options.Username != "user" || options.Password != "pass" || options.DB != 2 {
		t.Fatalf("auth/db = %q/%q/%d", options.Username, options.Password, options.DB)
	}
	if options.PoolSize != 10 || options.MinIdleConns != 1 || options.MaxIdleConns != 3 {
		t.Fatalf("pool = %d/%d/%d", options.PoolSize, options.MinIdleConns, options.MaxIdleConns)
	}
	if options.TLSConfig == nil || options.TLSConfig.ServerName != "redis.local" {
		t.Fatalf("tls = %#v", options.TLSConfig)
	}
}
