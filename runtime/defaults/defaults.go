// Package defaults defines runtime namespace defaults shared by runtime-sdk
// consumers.
package defaults

import "strings"

const (
	ConfigPrefix      = "/runtime/config"
	RegistryPrefix    = "/runtime/registry"
	RoutesPrefix      = "/runtime/gateway/routes"
	DescriptorsPrefix = "/runtime/gateway/descriptors"
	CommandsPrefix    = "/runtime/control/commands"
)

// CleanPrefix normalizes a runtime key prefix. Empty input returns fallback.
func CleanPrefix(prefix string, fallback string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return fallback
	}
	return "/" + strings.Trim(prefix, "/")
}
