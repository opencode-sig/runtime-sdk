package registry

import (
	"encoding/json"
	"strings"
)

// Keyspace defines the etcd key layout used by registry and discovery.
type Keyspace struct {
	prefix string
}

// NewKeyspace creates a normalized registry keyspace.
func NewKeyspace(prefix string) Keyspace {
	return Keyspace{prefix: normalizePrefix(prefix)}
}

// Prefix returns the normalized registry root prefix.
func (k Keyspace) Prefix() string {
	return k.prefix
}

// ServicesPrefix returns the prefix that contains all service instance keys.
func (k Keyspace) ServicesPrefix() string {
	return k.prefix + "/"
}

// ServicePrefix returns the etcd key prefix for one service's instances.
func (k Keyspace) ServicePrefix(service string) string {
	return k.ServicesPrefix() + strings.Trim(strings.TrimSpace(service), "/") + "/"
}

// ServiceKey returns the etcd key for one service instance.
func (k Keyspace) ServiceKey(service string, id string) string {
	return k.ServicePrefix(service) + strings.Trim(strings.TrimSpace(id), "/")
}

// ServiceFromKey extracts the service name from a full registry key.
func (k Keyspace) ServiceFromKey(key string) (string, bool) {
	rest := strings.TrimPrefix(key, k.ServicesPrefix())
	if rest == key || rest == "" {
		return "", false
	}
	service, _, _ := strings.Cut(rest, "/")
	if service == "" {
		return "", false
	}
	return service, true
}

// MarshalInstance encodes a registry instance for storage.
func MarshalInstance(instance ServiceInstance) ([]byte, error) {
	return json.Marshal(instance)
}

// UnmarshalInstance decodes a registry instance from storage.
func UnmarshalInstance(data []byte) (ServiceInstance, error) {
	var instance ServiceInstance
	if err := json.Unmarshal(data, &instance); err != nil {
		return ServiceInstance{}, err
	}
	return instance, nil
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	return "/" + strings.Trim(prefix, "/")
}
