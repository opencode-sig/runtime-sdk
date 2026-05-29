package elastic

import "testing"

func TestConfigNormalizeDefaultsOptionalValues(t *testing.T) {
	cfg := Config{Addresses: []string{"", " http://127.0.0.1:9200 "}}.Normalize()
	if len(cfg.Addresses) != 1 || cfg.Addresses[0] != "http://127.0.0.1:9200" {
		t.Fatalf("addresses = %#v", cfg.Addresses)
	}
	if cfg.DialTimeout != defaultDialTimeout {
		t.Fatalf("dial timeout = %q", cfg.DialTimeout)
	}
	if cfg.RequestTimeout != defaultRequestTimeout {
		t.Fatalf("request timeout = %q", cfg.RequestTimeout)
	}
	if cfg.MaxRetries != defaultMaxRetries {
		t.Fatalf("max retries = %d", cfg.MaxRetries)
	}
	if cfg.RetryBackoff != defaultRetryBackoff {
		t.Fatalf("retry backoff = %q", cfg.RetryBackoff)
	}
	if cfg.Observability.SlowQueryThreshold != defaultSlowQueryThreshold {
		t.Fatalf("slow query threshold = %q", cfg.Observability.SlowQueryThreshold)
	}
}

func TestConfigValidateAllowsEmptyConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestConfigValidateRequiresEndpointOrCloudID(t *testing.T) {
	if err := (Config{Username: "elastic", Password: "secret"}).Validate(); err == nil {
		t.Fatal("expected missing endpoint error")
	}
}

func TestConfigValidateRejectsMixedAuth(t *testing.T) {
	err := (Config{
		Addresses: []string{"http://127.0.0.1:9200"},
		Username:  "elastic",
		Password:  "secret",
		APIKey:    "key",
	}).Validate()
	if err == nil {
		t.Fatal("expected mixed auth error")
	}
}

func TestConfigValidateRejectsBadDurations(t *testing.T) {
	err := (Config{
		Addresses:      []string{"http://127.0.0.1:9200"},
		RequestTimeout: "-1s",
	}).Validate()
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
}
