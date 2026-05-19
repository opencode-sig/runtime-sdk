package registry

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var processStartedAt = time.Now().UnixNano()

// NewServiceInstance builds a service instance for registry storage.
//
// The instance ID is service-md5(service-hostname-mac-address-pid-startedAt).
// Keeping startedAt process-scoped makes registration and control watcher IDs
// stable inside one process while still changing after process restarts.
func NewServiceInstance(name string, address string, metadata map[string]string) ServiceInstance {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	hostname := hostname()
	return ServiceInstance{
		ID:        InstanceID(name, address),
		Name:      name,
		Address:   address,
		Hostname:  hostname,
		Metadata:  cloneMetadata(metadata),
		StartedAt: time.Unix(0, processStartedAt).UTC(),
		LastSeen:  time.Now().UTC(),
	}
}

// InstanceID builds the stable unique id used by registry for a service instance.
func InstanceID(service string, address string) string {
	service = strings.TrimSpace(service)
	value := service + "-" + hostname() + "-" + hardwareAddress() + "-" + strings.TrimSpace(address) + "-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(processStartedAt, 10)
	sum := md5.Sum([]byte(value))
	return sanitizeInstanceIDPart(service) + "-" + hex.EncodeToString(sum[:])
}

func hostname() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "local"
	}
	return strings.TrimSpace(hostname)
}

func sanitizeInstanceIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "service"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
}

func hardwareAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "nomac"
	}
	for _, item := range interfaces {
		if item.Flags&net.FlagLoopback != 0 || len(item.HardwareAddr) == 0 {
			continue
		}
		return item.HardwareAddr.String()
	}
	return "nomac"
}

// cloneInstance returns a defensive instance copy.
func cloneInstance(instance ServiceInstance) ServiceInstance {
	instance.Metadata = cloneMetadata(instance.Metadata)
	return instance
}

// cloneMetadata returns a defensive metadata copy.
func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return make(map[string]string)
	}
	copied := make(map[string]string, len(metadata))
	for key, value := range metadata {
		copied[key] = value
	}
	return copied
}
