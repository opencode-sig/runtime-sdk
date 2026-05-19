package mysql

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestConfigValidateAllowsZeroConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("validate zero config: %v", err)
	}
}

func TestConfigValidateAllowsZeroOptionalValues(t *testing.T) {
	cfg := Config{
		WriteDSNs: []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestConfigNormalizeDefaultsOptionalValues(t *testing.T) {
	cfg := Config{
		WriteDSNs: []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
	}

	normalized := cfg.Normalize()
	if normalized.Mode != ModeSingle {
		t.Fatalf("mode = %q", normalized.Mode)
	}
	if normalized.WritePool.MaxOpenConns != 50 {
		t.Fatalf("write max open conns = %d", normalized.WritePool.MaxOpenConns)
	}
	if normalized.WritePool.MaxIdleConns != 10 {
		t.Fatalf("write max idle conns = %d", normalized.WritePool.MaxIdleConns)
	}
	if normalized.WritePool.ConnMaxLifetime != "1h" {
		t.Fatalf("write conn max lifetime = %q", normalized.WritePool.ConnMaxLifetime)
	}
	if normalized.WritePool.ConnMaxIdleTime != "15m" {
		t.Fatalf("write conn max idle time = %q", normalized.WritePool.ConnMaxIdleTime)
	}
	if normalized.ConnTimeout != "3s" {
		t.Fatalf("conn timeout = %q", normalized.ConnTimeout)
	}
	if normalized.ReadTimeout != "3s" {
		t.Fatalf("read timeout = %q", normalized.ReadTimeout)
	}
	if normalized.WriteTimeout != "3s" {
		t.Fatalf("write timeout = %q", normalized.WriteTimeout)
	}
	if normalized.SlowQueryThreshold != "500ms" {
		t.Fatalf("slow query threshold = %q", normalized.SlowQueryThreshold)
	}
}

func TestConfigValidateRejectsUnsupportedMode(t *testing.T) {
	cfg := Config{
		Mode:      "proxy",
		WriteDSNs: []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func TestConfigValidateRejectsReadWriteWithoutReadDSNs(t *testing.T) {
	cfg := Config{
		Mode:      ModeReadWrite,
		WriteDSNs: []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing read DSNs error")
	}
}

func TestConfigValidateRejectsNegativePoolValues(t *testing.T) {
	cfg := Config{
		WriteDSNs: []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
		WritePool: PoolConfig{
			MaxOpenConns: -1,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected negative pool value error")
	}
}

func TestConfigValidateRejectsMaxIdleGreaterThanMaxOpen(t *testing.T) {
	cfg := Config{
		WriteDSNs: []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
		WritePool: PoolConfig{
			MaxOpenConns: 5,
			MaxIdleConns: 6,
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected max idle greater than max open error")
	}
}

func TestConfigValidateRejectsInvalidDuration(t *testing.T) {
	cfg := Config{
		WriteDSNs:   []string{"user:pass@tcp(127.0.0.1:3306)/app?parseTime=true"},
		ConnTimeout: "bad",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestNewDBRejectsZeroConfig(t *testing.T) {
	if _, err := NewDB(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing write DSNs error")
	}
}

func TestDBReadRoundRobinAndFallback(t *testing.T) {
	write1, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/app")
	if err != nil {
		t.Fatalf("open write1 db: %v", err)
	}
	write2, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3309)/app")
	if err != nil {
		t.Fatalf("open write2 db: %v", err)
	}
	read1, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3307)/app")
	if err != nil {
		t.Fatalf("open read1 db: %v", err)
	}
	read2, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3308)/app")
	if err != nil {
		t.Fatalf("open read2 db: %v", err)
	}
	db := &DB{Writes: []*sql.DB{write1, write2}, Reads: []*sql.DB{read1, read2}}
	defer db.Close()

	if got := db.Write(); got != write1 {
		t.Fatal("active write db was not first write pool")
	}
	if got := db.Read(); got != read1 {
		t.Fatal("first read db was not read1")
	}
	if got := db.Read(); got != read2 {
		t.Fatal("second read db was not read2")
	}

	writeOnly := &DB{Writes: []*sql.DB{write1, write2}}
	if got := writeOnly.Read(); got != write1 {
		t.Fatal("write fallback was not returned")
	}
}

func TestApplyPool(t *testing.T) {
	db, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/app")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	applyPool(db, PoolConfig{
		MaxOpenConns:    7,
		MaxIdleConns:    3,
		ConnMaxLifetime: "30m",
		ConnMaxIdleTime: "5m",
	})

	stats := db.Stats()
	if stats.MaxOpenConnections != 7 {
		t.Fatalf("max open = %d", stats.MaxOpenConnections)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
}
