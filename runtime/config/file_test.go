package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileProviderLoadAndDecode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.yaml")
	if err := os.WriteFile(path, []byte("name: runtime\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	provider := NewFileProvider(dir)
	data, err := provider.Load(context.Background(), "runtime.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	var cfg struct {
		Name string `json:"name" yaml:"name"`
	}
	cfg, err = Decode[struct {
		Name string `json:"name" yaml:"name"`
	}](data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Name != "runtime" {
		t.Fatalf("got %q, want runtime", cfg.Name)
	}
}

func TestDecodeJSON(t *testing.T) {
	cfg, err := DecodeJSON[struct {
		Name string `json:"name"`
	}]([]byte(`{"name":"runtime"}`))
	if err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if cfg.Name != "runtime" {
		t.Fatalf("got %q, want runtime", cfg.Name)
	}
}
