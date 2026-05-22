package servicekit

import (
	"fmt"
	"strings"
)

// ServiceIdentity returns the service name and advertised gRPC address used by
// control commands and registry instance ids.
func ServiceIdentity(cfg Config) (string, string, error) {
	name := strings.TrimSpace(cfg.Service.Name)
	if name == "" {
		return "", "", fmt.Errorf("service name is required")
	}
	address := serviceAddress(cfg)
	if address == "" {
		return "", "", fmt.Errorf("service %s advertise grpc addr is required", name)
	}
	return name, address, nil
}
