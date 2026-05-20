package servicekit

import (
	"testing"
	"time"
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
