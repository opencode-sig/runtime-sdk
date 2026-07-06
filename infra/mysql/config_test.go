package mysql

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestConfigValidateAllowsZeroConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("validate zero config: %v", err)
	}
}

func TestConfigCompileSingleServer(t *testing.T) {
	cfg := testStructuredConfig()

	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(compiled.Instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(compiled.Instances))
	}

	defaultInstance, err := compiled.Resolve()
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if defaultInstance.Name != "default" || defaultInstance.Database != "payment" {
		t.Fatalf("default instance = %+v, want default/payment", defaultInstance)
	}
	if !strings.Contains(defaultInstance.WriteDSN, "/payment?") {
		t.Fatalf("default write dsn = %q, want payment database", defaultInstance.WriteDSN)
	}
	if !strings.Contains(defaultInstance.WriteDSN, "parseTime=true") {
		t.Fatalf("default write dsn = %q, want parseTime param", defaultInstance.WriteDSN)
	}
	if defaultInstance.Pool.MaxOpenConns != 10 {
		t.Fatalf("default max open = %d, want global pool", defaultInstance.Pool.MaxOpenConns)
	}

	reportInstance, err := compiled.Resolve("report")
	if err != nil {
		t.Fatalf("resolve report: %v", err)
	}
	if reportInstance.Database != "payment_report" {
		t.Fatalf("report database = %q, want payment_report", reportInstance.Database)
	}
	if reportInstance.Pool.MaxOpenConns != 20 {
		t.Fatalf("report max open = %d, want database pool override", reportInstance.Pool.MaxOpenConns)
	}
}

func TestConfigCompileMultiServer(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"main": {
				Host:     "mysql-main",
				Username: "app",
				Password: "secret",
				Databases: map[string]DatabaseConfig{
					"default": {Name: "payment"},
					"report":  {Name: "payment_report"},
				},
			},
			"analytics": {
				Host:     "mysql-analytics",
				Username: "analytics",
				Password: "secret",
				Databases: map[string]DatabaseConfig{
					"events": {Name: "analytics_events"},
				},
			},
		},
	}

	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	mainReport, err := compiled.Resolve("main.report")
	if err != nil {
		t.Fatalf("resolve main.report: %v", err)
	}
	if mainReport.Name != "main.report" || mainReport.Server != "main" || mainReport.Database != "payment_report" {
		t.Fatalf("main.report = %+v", mainReport)
	}

	events, err := compiled.Resolve("events")
	if err != nil {
		t.Fatalf("resolve unique alias events: %v", err)
	}
	if events.Name != "analytics.events" {
		t.Fatalf("events alias resolved to %q, want analytics.events", events.Name)
	}
}

func TestConfigCompileAmbiguousAlias(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"main": {
				Host:     "mysql-main",
				Username: "app",
				Databases: map[string]DatabaseConfig{
					"report": {Name: "payment_report"},
				},
			},
			"analytics": {
				Host:     "mysql-analytics",
				Username: "analytics",
				Databases: map[string]DatabaseConfig{
					"report": {Name: "analytics_report"},
				},
			},
		},
	}

	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := compiled.Resolve("report"); err == nil || !strings.Contains(err.Error(), `mysql instance "report" is ambiguous`) {
		t.Fatalf("resolve ambiguous error = %v", err)
	}
}

func TestConfigCompileReadEndpoints(t *testing.T) {
	cfg := Config{
		Write: EndpointConfig{
			Host:     "mysql-primary",
			Username: "app",
		},
		Reads: []EndpointConfig{
			{Host: "mysql-replica-1", Username: "app_ro"},
			{Host: "mysql-replica-2", Username: "app_ro"},
		},
		Databases: map[string]DatabaseConfig{
			"default": {Name: "payment"},
		},
	}

	compiled, err := cfg.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	instance, err := compiled.Resolve()
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if len(instance.ReadDSNs) != 2 {
		t.Fatalf("read dsns = %d, want 2", len(instance.ReadDSNs))
	}
	if !strings.Contains(instance.ReadDSNs[0], "mysql-replica-1") {
		t.Fatalf("read dsn 0 = %q", instance.ReadDSNs[0])
	}
}

func TestConfigValidateErrors(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantError string
	}{
		{
			name: "mix servers and single server fields",
			cfg: Config{
				Host: "127.0.0.1",
				Servers: map[string]ServerConfig{
					"main": {Host: "mysql-main", Username: "app", Databases: map[string]DatabaseConfig{"default": {}}},
				},
			},
			wantError: "mysql cannot mix servers",
		},
		{
			name: "missing username",
			cfg: Config{
				Host:      "127.0.0.1",
				Databases: map[string]DatabaseConfig{"default": {Name: "payment"}},
			},
			wantError: "mysql.write.username is required",
		},
		{
			name: "server name contains dot",
			cfg: Config{
				Servers: map[string]ServerConfig{
					"main.primary": {Host: "mysql-main", Username: "app", Databases: map[string]DatabaseConfig{"default": {}}},
				},
			},
			wantError: `mysql server name "main.primary" must not contain "."`,
		},
		{
			name: "database name contains dot",
			cfg: Config{
				Host:     "127.0.0.1",
				Username: "app",
				Databases: map[string]DatabaseConfig{
					"main.default": {Name: "payment"},
				},
			},
			wantError: `mysql.databases name "main.default" must not contain "."`,
		},
		{
			name: "ensure invalid identifier",
			cfg: Config{
				Host:     "127.0.0.1",
				Username: "app",
				Databases: map[string]DatabaseConfig{
					"default": {
						Name:   "payment-report",
						Ensure: EnsureDatabaseConfig{Enabled: true},
					},
				},
			},
			wantError: `mysql.databases.default.name: identifier "payment-report" contains unsupported character '-'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validate error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestIdentifierQuote(t *testing.T) {
	quoted, err := quoteIdentifier("payment_2026")
	if err != nil {
		t.Fatalf("quote identifier: %v", err)
	}
	if quoted != "`payment_2026`" {
		t.Fatalf("quoted = %q", quoted)
	}
	if _, err := quoteIdentifier("payment-report"); err == nil {
		t.Fatal("expected invalid identifier error")
	}
}

func TestNewDBRejectsZeroConfig(t *testing.T) {
	if _, err := NewDB(context.Background(), Config{}); err == nil {
		t.Fatal("expected missing mysql config error")
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

func testStructuredConfig() Config {
	return Config{
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "payment",
		Password: "secret",
		Params: map[string]string{
			"parseTime": "true",
			"charset":   "utf8mb4",
		},
		Pool: PoolConfig{MaxOpenConns: 10},
		Databases: map[string]DatabaseConfig{
			"default": {Name: "payment"},
			"report": {
				Name: "payment_report",
				Params: map[string]string{
					"loc": "Local",
				},
				Pool: PoolConfig{MaxOpenConns: 20},
			},
		},
	}
}
