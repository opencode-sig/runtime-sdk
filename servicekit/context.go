package servicekit

import (
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

// RuntimeContext exposes common runtime assembly state to service modules.
type RuntimeContext struct {
	Config  Config
	Configs *Configs
	Infra   Infra
	App     *lifecycle.Runtime
	Logger  *logger.Logger
}

// DistributedContext exposes distributed runtime resources to service modules.
type DistributedContext struct {
	Config        Config
	Configs       *Configs
	Infra         Infra
	App           *lifecycle.Runtime
	Etcd          *clientv3.Client
	Registry      registry.Registry
	InstanceStore registry.InstanceStore
	Clients       *Clients
	Logger        *logger.Logger
}
