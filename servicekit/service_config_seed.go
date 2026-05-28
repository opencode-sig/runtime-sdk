package servicekit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

var (
	ErrServiceRequired     = errors.New("service is required")
	ErrInvalidServiceName  = errors.New("invalid service name")
	ErrConfigDirRequired   = errors.New("config dir is required")
	ErrConfigStoreRequired = errors.New("config store is required")
)

var serviceConfigNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// SeedServiceConfigOptions controls publishing one service-owned config file.
type SeedServiceConfigOptions struct {
	Service   string
	ConfigDir string
	Store     runtimeconfig.Store
	Overwrite bool
}

// SeedServiceConfigResult describes the config-center write outcome.
type SeedServiceConfigResult struct {
	Key       string
	Created   bool
	Updated   bool
	Unchanged bool
}

// SeedServiceConfig publishes only configs/service/<service>.yaml.
func SeedServiceConfig(ctx context.Context, opts SeedServiceConfigOptions) (SeedServiceConfigResult, error) {
	service, err := normalizeServiceConfigName(opts.Service)
	if err != nil {
		return SeedServiceConfigResult{}, err
	}
	configDir := strings.TrimSpace(opts.ConfigDir)
	if configDir == "" {
		return SeedServiceConfigResult{}, ErrConfigDirRequired
	}
	if opts.Store == nil {
		return SeedServiceConfigResult{}, ErrConfigStoreRequired
	}

	key := serviceConfigKey(service)
	path := filepath.Join(configDir, "service", service+".yaml")
	value, err := os.ReadFile(path)
	if err != nil {
		return SeedServiceConfigResult{}, fmt.Errorf("read service config %s: %w", path, err)
	}
	if err := validateServiceConfigFile(path, value); err != nil {
		return SeedServiceConfigResult{}, err
	}

	result := SeedServiceConfigResult{Key: key}
	if opts.Overwrite {
		if err := opts.Store.Put(ctx, key, value); err != nil {
			return SeedServiceConfigResult{}, fmt.Errorf("seed service config %s: %w", key, err)
		}
		result.Updated = true
		return result, nil
	}

	created, err := opts.Store.PutIfAbsent(ctx, key, value)
	if err != nil {
		return SeedServiceConfigResult{}, fmt.Errorf("seed service config %s: %w", key, err)
	}
	if created {
		result.Created = true
	} else {
		result.Unchanged = true
	}
	return result, nil
}

func normalizeServiceConfigName(service string) (string, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return "", ErrServiceRequired
	}
	if !serviceConfigNamePattern.MatchString(service) {
		return "", fmt.Errorf("%w: %s", ErrInvalidServiceName, service)
	}
	return service, nil
}

func serviceConfigKey(service string) string {
	return "configs/service/" + service + ".yaml"
}

func validateServiceConfigFile(path string, data []byte) error {
	var cfg serviceConfigFile
	if err := runtimeconfig.DecodeInto(data, &cfg); err != nil {
		return fmt.Errorf("decode service config %s: %w", path, err)
	}
	if strings.TrimSpace(cfg.GRPCAddr) == "" {
		return fmt.Errorf("decode service config %s: grpc_addr is required", path)
	}
	return nil
}

type serviceConfigFile struct {
	GRPCAddr           string         `json:"grpc_addr" yaml:"grpc_addr"`
	AdvertiseGRPCAddr  string         `json:"advertise_grpc_addr" yaml:"advertise_grpc_addr"`
	AdminAddr          string         `json:"admin_addr" yaml:"admin_addr"`
	AdvertiseAdminAddr string         `json:"advertise_admin_addr" yaml:"advertise_admin_addr"`
	Settings           map[string]any `json:"settings" yaml:"settings"`
}
