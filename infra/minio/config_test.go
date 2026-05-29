package minio

import "testing"

func TestConfigNormalizeDefaultsOptionalValues(t *testing.T) {
	cfg := Config{
		Endpoint:  " 127.0.0.1:9000 ",
		AccessKey: " minio ",
		SecretKey: " secret ",
		Bucket:    " bucket ",
	}.Normalize()
	if cfg.Endpoint != "127.0.0.1:9000" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.Region != defaultRegion {
		t.Fatalf("region = %q", cfg.Region)
	}
	if cfg.AccessKey != "minio" || cfg.SecretKey != "secret" {
		t.Fatalf("credentials = %q/%q", cfg.AccessKey, cfg.SecretKey)
	}
	if cfg.Bucket != "bucket" {
		t.Fatalf("bucket = %q", cfg.Bucket)
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
}

func TestConfigValidateAllowsEmptyConfig(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestConfigValidateRequiresEndpointAndCredentials(t *testing.T) {
	if err := (Config{AccessKey: "minio", SecretKey: "secret"}).Validate(); err == nil {
		t.Fatal("expected endpoint error")
	}
	if err := (Config{Endpoint: "127.0.0.1:9000", SecretKey: "secret"}).Validate(); err == nil {
		t.Fatal("expected access_key error")
	}
	if err := (Config{Endpoint: "127.0.0.1:9000", AccessKey: "minio"}).Validate(); err == nil {
		t.Fatal("expected secret_key error")
	}
}

func TestConfigValidateRejectsBadDurations(t *testing.T) {
	err := (Config{
		Endpoint:       "127.0.0.1:9000",
		AccessKey:      "minio",
		SecretKey:      "secret",
		RequestTimeout: "-1s",
	}).Validate()
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
}
