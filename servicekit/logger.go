package servicekit

import (
	"strings"

	"github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

// loggerWithRuntimeIdentity returns a service-scoped logger that attaches the
// runtime service identity to every log entry emitted by this service instance.
func loggerWithRuntimeIdentity(base *logger.Logger, cfg Config, spec Spec, runtimeMode string) *logger.Logger {
	if base == nil {
		return nil
	}
	serviceName := strings.TrimSpace(spec.Name)
	if serviceName == "" {
		serviceName = strings.TrimSpace(cfg.Service.Name)
	}
	address := serviceAddress(cfg)
	fields := logger.Fields(logger.String("runtime_service", serviceName))
	if mode := strings.TrimSpace(runtimeMode); mode != "" {
		fields = append(fields, logger.String("runtime_mode", mode))
	}
	if serviceName != "" && address != "" {
		fields = append(fields, logger.String("instance_id", registry.InstanceID(serviceName, address)))
	}
	return base.With(fields...)
}

func serviceAddress(cfg Config) string {
	address := strings.TrimSpace(cfg.Service.AdvertiseGRPCAddr)
	if address == "" {
		address = strings.TrimSpace(cfg.Service.GRPCAddr)
	}
	return address
}
