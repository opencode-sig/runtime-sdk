package etcd

import "testing"

func TestConfigValidateAllowsZeroConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("validate zero config: %v", err)
	}
}

func TestConfigNormalizeDefaultsOptionalValues(t *testing.T) {
	normalized := (Config{}).Normalize()
	if len(normalized.Endpoints) != 1 || normalized.Endpoints[0] != defaultEndpoint {
		t.Fatalf("endpoints = %#v", normalized.Endpoints)
	}
	if normalized.DialTimeout != defaultDialTimeout {
		t.Fatalf("dial timeout = %q", normalized.DialTimeout)
	}
}

func TestConfigNormalizeDropsEmptyEndpoints(t *testing.T) {
	cfg := Config{
		Endpoints: []string{"", " 127.0.0.1:2379 ", " "},
	}

	normalized := cfg.Normalize()
	if len(normalized.Endpoints) != 1 {
		t.Fatalf("endpoints = %#v", normalized.Endpoints)
	}
	if normalized.Endpoints[0] != "127.0.0.1:2379" {
		t.Fatalf("endpoint = %q", normalized.Endpoints[0])
	}
}

func TestConfigValidateRejectsInvalidDialTimeout(t *testing.T) {
	cfg := Config{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: "bad",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid dial timeout error")
	}
}

func TestConfigValidateRejectsNonPositiveDialTimeout(t *testing.T) {
	cfg := Config{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: "0s",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected non-positive dial timeout error")
	}
}
