package servicekit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infraelastic "github.com/opencode-sig/runtime-sdk/infra/elastic"
	infraetcd "github.com/opencode-sig/runtime-sdk/infra/etcd"
	infrakafka "github.com/opencode-sig/runtime-sdk/infra/kafka"
	inframinio "github.com/opencode-sig/runtime-sdk/infra/minio"
	inframysql "github.com/opencode-sig/runtime-sdk/infra/mysql"
	infraredis "github.com/opencode-sig/runtime-sdk/infra/redis"
	"github.com/opencode-sig/runtime-sdk/logger"
	runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
)

// Config contains the public runtime configuration required by one managed
// gRPC service. It intentionally avoids importing any internal application
// package so external services can use the same contract.
type Config struct {
	Logger   logger.Config  `json:"logger" yaml:"logger"`
	Runtime  RuntimeConfig  `json:"runtime" yaml:"runtime"`
	Service  ServiceConfig  `json:"service" yaml:"service"`
	Registry RegistryConfig `json:"registry" yaml:"registry"`
	Metadata MetadataConfig `json:"metadata" yaml:"metadata"`
	Infra    InfraConfig    `json:"infra" yaml:"infra"`
	Settings map[string]any `json:"settings" yaml:"settings"`
}

// InfraConfig contains shared infrastructure configuration for one managed
// service generation. Runtime loads these values from file or config center and
// passes them through; services decide which dependencies they actually use.
type InfraConfig struct {
	Etcd    infraetcd.Config    `json:"etcd" yaml:"etcd"`
	MySQL   inframysql.Config   `json:"mysql" yaml:"mysql"`
	Redis   infraredis.Config   `json:"redis" yaml:"redis"`
	Kafka   infrakafka.Config   `json:"kafka" yaml:"kafka"`
	Elastic infraelastic.Config `json:"elastic" yaml:"elastic"`
	MinIO   inframinio.Config   `json:"minio" yaml:"minio"`
}

type RuntimeConfig struct {
	Config  ConfigSourceConfig `json:"config" yaml:"config"`
	Control ControlConfig      `json:"control" yaml:"control"`
}

type ConfigSourceConfig struct {
	Provider string     `json:"provider" yaml:"provider"`
	Root     string     `json:"root,omitempty" yaml:"root,omitempty"`
	Key      string     `json:"key" yaml:"key"`
	Etcd     EtcdConfig `json:"etcd" yaml:"etcd"`
}

type ControlConfig struct {
	CommandsPrefix string `json:"commands_prefix" yaml:"commands_prefix"`
	CommandTTL     string `json:"command_ttl" yaml:"command_ttl"`
}

type ServiceConfig struct {
	Name               string `json:"name" yaml:"name"`
	GRPCAddr           string `json:"grpc_addr" yaml:"grpc_addr"`
	AdvertiseGRPCAddr  string `json:"advertise_grpc_addr" yaml:"advertise_grpc_addr"`
	AdminAddr          string `json:"admin_addr" yaml:"admin_addr"`
	AdvertiseAdminAddr string `json:"advertise_admin_addr" yaml:"advertise_admin_addr"`
	EnablePprof        bool   `json:"enable_pprof" yaml:"enable_pprof"`
}

type RegistryConfig struct {
	Provider string     `json:"provider" yaml:"provider"`
	Etcd     EtcdConfig `json:"etcd" yaml:"etcd"`
}

type MetadataConfig struct {
	RoutesPrefix      string `json:"routes_prefix" yaml:"routes_prefix"`
	DescriptorsPrefix string `json:"descriptors_prefix" yaml:"descriptors_prefix"`
}

type EtcdConfig struct {
	Endpoints []string `json:"endpoints" yaml:"endpoints"`
	Prefix    string   `json:"prefix" yaml:"prefix"`
}

// RegistryEnabled reports whether the service should use an external registry.
func (c Config) RegistryEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(c.Registry.Provider), "etcd")
}

// EtcdConfigStore creates an etcd config-center store when etcd config is present.
func (c Config) EtcdConfigStore() (*runtimeconfig.EtcdProvider, bool) {
	if !strings.EqualFold(strings.TrimSpace(c.Runtime.Config.Provider), "etcd") && !hasEtcdConfig(c.Runtime.Config.Etcd) {
		return nil, false
	}
	return runtimeconfig.NewEtcdProvider(c.Runtime.Config.Etcd.Endpoints, c.Runtime.Config.Etcd.Prefix), true
}

func hasEtcdConfig(cfg EtcdConfig) bool {
	if strings.TrimSpace(cfg.Prefix) != "" {
		return true
	}
	for _, endpoint := range cfg.Endpoints {
		if strings.TrimSpace(endpoint) != "" {
			return true
		}
	}
	return false
}

// CommandTTLDuration returns the command retention TTL used by etcd-backed
// command stores. Empty values keep the runtime control package default.
func (c ControlConfig) CommandTTLDuration() (time.Duration, error) {
	value := strings.TrimSpace(c.CommandTTL)
	if value == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse control command_ttl: %w", err)
	}
	if duration < 0 {
		return 0, fmt.Errorf("control command_ttl must not be negative")
	}
	return duration, nil
}

// DecodeSettings decodes the service private settings into a strong type.
func DecodeSettings[T any](cfg Config) (T, error) {
	var out T
	if len(cfg.Settings) == 0 {
		return out, nil
	}
	data, err := json.Marshal(cfg.Settings)
	if err != nil {
		return out, fmt.Errorf("marshal service settings: %w", err)
	}
	if err := runtimeconfig.DecodeInto(data, &out); err != nil {
		return out, fmt.Errorf("decode service settings: %w", err)
	}
	return out, nil
}
