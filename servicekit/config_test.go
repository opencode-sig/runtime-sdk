package servicekit

import (
	"strings"
	"testing"
	"time"

	inframysql "github.com/opencode-sig/runtime-sdk/infra/mysql"
)

func TestEtcdConfigStore(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "provider etcd", cfg: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "etcd"}}}, want: true},
		{name: "file provider with etcd endpoints", cfg: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "file", Etcd: EtcdConfig{Endpoints: []string{"127.0.0.1:2379"}}}}}, want: true},
		{name: "file provider with etcd prefix", cfg: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "file", Etcd: EtcdConfig{Prefix: "/runtime/config"}}}}, want: true},
		{name: "file provider without etcd", cfg: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "file"}}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, ok := tt.cfg.EtcdConfigStore()
			if ok != tt.want {
				t.Fatalf("EtcdConfigStore ok = %v, want %v", ok, tt.want)
			}
			if ok && store == nil {
				t.Fatal("EtcdConfigStore returned nil store")
			}
			if store != nil {
				_ = store.Close()
			}
		})
	}
}

func TestProcessControlEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "provider etcd", cfg: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "etcd"}}}, want: true},
		{name: "provider file with etcd endpoints", cfg: Config{Runtime: RuntimeConfig{Config: ConfigSourceConfig{Provider: "file", Etcd: EtcdConfig{Endpoints: []string{"127.0.0.1:2379"}}}}}, want: false},
		{name: "provider empty", cfg: Config{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.ProcessControlEnabled(); got != tt.want {
				t.Fatalf("ProcessControlEnabled = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestControlConfigCommandTTLDuration(t *testing.T) {
	duration, err := (ControlConfig{CommandTTL: "30m"}).CommandTTLDuration()
	if err != nil {
		t.Fatalf("CommandTTLDuration error = %v", err)
	}
	if duration != 30*time.Minute {
		t.Fatalf("duration = %s, want 30m", duration)
	}

	duration, err = (ControlConfig{}).CommandTTLDuration()
	if err != nil {
		t.Fatalf("empty CommandTTLDuration error = %v", err)
	}
	if duration != 0 {
		t.Fatalf("empty duration = %s, want 0", duration)
	}

	if _, err := (ControlConfig{CommandTTL: "bad"}).CommandTTLDuration(); err == nil {
		t.Fatal("invalid CommandTTLDuration error = nil")
	}
}

func TestMySQLConfigsResolve(t *testing.T) {
	cfgs := MySQLConfigs{
		"default": testMySQLConfig("app"),
		"report":  testMySQLConfig("report"),
	}

	name, cfg, err := cfgs.Resolve()
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if name != "default" || cfg.WriteDSNs[0] != testMySQLDSN("app") {
		t.Fatalf("default = %q %#v, want default app", name, cfg)
	}

	name, cfg, err = cfgs.Resolve(" report ")
	if err != nil {
		t.Fatalf("resolve report: %v", err)
	}
	if name != "report" || cfg.WriteDSNs[0] != testMySQLDSN("report") {
		t.Fatalf("report = %q %#v, want report config", name, cfg)
	}

	if _, _, err := cfgs.Resolve("missing"); err == nil || !strings.Contains(err.Error(), `mysql instance "missing" is not configured`) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestMySQLConfigsResolveSingleInstanceAsDefault(t *testing.T) {
	cfgs := MySQLConfigs{"analytics": testMySQLConfig("analytics")}

	name, cfg, err := cfgs.Resolve()
	if err != nil {
		t.Fatalf("resolve single instance: %v", err)
	}
	if name != "analytics" || cfg.WriteDSNs[0] != testMySQLDSN("analytics") {
		t.Fatalf("single default = %q %#v, want analytics", name, cfg)
	}
}

func TestMySQLConfigsValidate(t *testing.T) {
	if err := (MySQLConfigs{}).Validate(); err != nil {
		t.Fatalf("empty configs validate: %v", err)
	}

	valid := MySQLConfigs{
		"default": testMySQLConfig("app"),
		"report":  testMySQLConfig("report"),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid configs validate: %v", err)
	}

	tests := []struct {
		name      string
		cfg       MySQLConfigs
		wantError string
	}{
		{
			name:      "empty instance name",
			cfg:       MySQLConfigs{"": testMySQLConfig("app")},
			wantError: "mysql instance name is required",
		},
		{
			name:      "whitespace instance name",
			cfg:       MySQLConfigs{" report ": testMySQLConfig("report")},
			wantError: `mysql instance " report " must not contain surrounding whitespace`,
		},
		{
			name:      "empty instance config",
			cfg:       MySQLConfigs{"report": {}},
			wantError: `mysql instance "report" is empty`,
		},
		{
			name:      "invalid instance config",
			cfg:       MySQLConfigs{"report": {Mode: inframysql.ModeReadWrite, WriteDSNs: []string{testMySQLDSN("report")}}},
			wantError: `mysql instance "report": mysql read_dsns is required in read_write mode`,
		},
		{
			name: "multiple instances without default",
			cfg: MySQLConfigs{
				"report": testMySQLConfig("report"),
				"audit":  testMySQLConfig("audit"),
			},
			wantError: "mysql default instance is required when multiple mysql instances are configured",
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

func testMySQLConfig(database string) inframysql.Config {
	return inframysql.Config{WriteDSNs: []string{testMySQLDSN(database)}}
}

func testMySQLDSN(database string) string {
	return "user:pass@tcp(127.0.0.1:3306)/" + database + "?parseTime=true"
}
