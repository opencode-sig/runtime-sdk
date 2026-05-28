package servicekit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

func TestSeedServiceConfigCreatesServiceConfig(t *testing.T) {
	root := writeServiceConfig(t, "echo", "grpc_addr: :2301\nsettings: {}\n")
	store := newMemoryConfigStore()

	result, err := SeedServiceConfig(context.Background(), SeedServiceConfigOptions{
		Service:   "echo",
		ConfigDir: root,
		Store:     store,
	})
	if err != nil {
		t.Fatalf("SeedServiceConfig returned error: %v", err)
	}
	if !result.Created || result.Updated || result.Unchanged {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Key != "configs/service/echo.yaml" {
		t.Fatalf("key = %q", result.Key)
	}
	got := string(store.values["configs/service/echo.yaml"])
	if !strings.Contains(got, "grpc_addr: :2301") {
		t.Fatalf("stored config = %q", got)
	}
	if len(store.values) != 1 {
		t.Fatalf("stored keys = %#v", store.values)
	}
}

func TestSeedServiceConfigDoesNotOverwriteByDefault(t *testing.T) {
	root := writeServiceConfig(t, "echo", "grpc_addr: :2301\n")
	store := newMemoryConfigStore()
	store.values["configs/service/echo.yaml"] = []byte("grpc_addr: :old\n")

	result, err := SeedServiceConfig(context.Background(), SeedServiceConfigOptions{
		Service:   "echo",
		ConfigDir: root,
		Store:     store,
	})
	if err != nil {
		t.Fatalf("SeedServiceConfig returned error: %v", err)
	}
	if !result.Unchanged || result.Created || result.Updated {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := string(store.values["configs/service/echo.yaml"]); got != "grpc_addr: :old\n" {
		t.Fatalf("stored config = %q", got)
	}
}

func TestSeedServiceConfigOverwritesWhenRequested(t *testing.T) {
	root := writeServiceConfig(t, "echo", "grpc_addr: :2301\n")
	store := newMemoryConfigStore()
	store.values["configs/service/echo.yaml"] = []byte("grpc_addr: :old\n")

	result, err := SeedServiceConfig(context.Background(), SeedServiceConfigOptions{
		Service:   "echo",
		ConfigDir: root,
		Store:     store,
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf("SeedServiceConfig returned error: %v", err)
	}
	if !result.Updated || result.Created || result.Unchanged {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got := string(store.values["configs/service/echo.yaml"]); got != "grpc_addr: :2301\n" {
		t.Fatalf("stored config = %q", got)
	}
}

func TestSeedServiceConfigRejectsInvalidInput(t *testing.T) {
	root := writeServiceConfig(t, "echo", "grpc_addr: :2301\n")
	store := newMemoryConfigStore()
	tests := []struct {
		name string
		opts SeedServiceConfigOptions
		want error
	}{
		{name: "missing service", opts: SeedServiceConfigOptions{ConfigDir: root, Store: store}, want: ErrServiceRequired},
		{name: "invalid service", opts: SeedServiceConfigOptions{Service: "../echo", ConfigDir: root, Store: store}, want: ErrInvalidServiceName},
		{name: "missing config dir", opts: SeedServiceConfigOptions{Service: "echo", Store: store}, want: ErrConfigDirRequired},
		{name: "missing store", opts: SeedServiceConfigOptions{Service: "echo", ConfigDir: root}, want: ErrConfigStoreRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SeedServiceConfig(context.Background(), tt.opts)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSeedServiceConfigRejectsMissingOrInvalidFile(t *testing.T) {
	root := t.TempDir()
	store := newMemoryConfigStore()
	if _, err := SeedServiceConfig(context.Background(), SeedServiceConfigOptions{
		Service:   "echo",
		ConfigDir: root,
		Store:     store,
	}); err == nil || !strings.Contains(err.Error(), "read service config") {
		t.Fatalf("missing file error = %v", err)
	}

	root = writeServiceConfig(t, "echo", "settings: {}\n")
	if _, err := SeedServiceConfig(context.Background(), SeedServiceConfigOptions{
		Service:   "echo",
		ConfigDir: root,
		Store:     store,
	}); err == nil || !strings.Contains(err.Error(), "grpc_addr is required") {
		t.Fatalf("invalid config error = %v", err)
	}
}

func writeServiceConfig(t *testing.T, service string, data string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, service+".yaml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type memoryConfigStore struct {
	values map[string][]byte
}

func newMemoryConfigStore() *memoryConfigStore {
	return &memoryConfigStore{values: make(map[string][]byte)}
}

func (s *memoryConfigStore) Get(context.Context, string) (runtimeconfig.Entry, error) {
	return runtimeconfig.Entry{}, runtimeconfig.ErrConfigNotFound
}

func (s *memoryConfigStore) Put(_ context.Context, key string, value []byte) error {
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *memoryConfigStore) PutIfAbsent(_ context.Context, key string, value []byte) (bool, error) {
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = append([]byte(nil), value...)
	return true, nil
}

func (s *memoryConfigStore) Delete(context.Context, string) error {
	return nil
}

func (s *memoryConfigStore) List(context.Context, string) ([]runtimeconfig.Entry, error) {
	return nil, nil
}
