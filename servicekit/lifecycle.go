package servicekit

import (
	"fmt"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/opencode-sig/runtime-sdk/logger"
	runtimemetrics "github.com/opencode-sig/runtime-sdk/observability/metrics"
	runtimecomponent "github.com/opencode-sig/runtime-sdk/runtime/component"
	"github.com/opencode-sig/runtime-sdk/runtime/lifecycle"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
)

// Component describes a lifecycle-managed service component factory.
type Component interface {
	AddTo(app *lifecycle.Runtime) error
}

// ComponentConfig contains the resources needed to add one service to a
// lifecycle graph.
type ComponentConfig struct {
	Config              Config
	Spec                Spec
	Etcd                *clientv3.Client
	Registry            registry.Registry
	Clients             *Clients
	Infra               Infra
	Configs             *Configs
	RuntimeMode         string
	DataPlaneGeneration string
	Logger              *logger.Logger
	identity            *runtimeIdentityStore
}

// NewComponent returns a component factory for one service spec.
func NewComponent(cfg ComponentConfig) Component {
	return serviceComponent{cfg: cfg}
}

type serviceComponent struct {
	cfg ComponentConfig
}

func (c serviceComponent) AddTo(app *lifecycle.Runtime) error {
	return AddToLifecycle(app, c.cfg)
}

// AddToLifecycle adds one service gRPC server, optional HTTP listener, optional registry entry
// and optional Gateway metadata publisher to a lifecycle graph.
func AddToLifecycle(app *lifecycle.Runtime, cfg ComponentConfig) error {
	if app == nil {
		return fmt.Errorf("runtime is required")
	}
	spec, err := NewSpec(cfg.Spec)
	if err != nil {
		return err
	}
	cfg.Spec = spec
	if strings.TrimSpace(cfg.Config.Service.Name) == "" {
		cfg.Config.Service.Name = spec.Name
	}
	cfg.Logger = loggerWithRuntimeIdentity(cfg.Logger, cfg.Config, spec, cfg.RuntimeMode)
	configs := cfg.Configs
	closeConfigsOnError := false
	defer func() {
		if closeConfigsOnError {
			_ = configs.Close()
		}
	}()
	if configs == nil {
		var err error
		configs, err = NewConfigs(cfg.Config, WithConfigsEtcdClient(cfg.Etcd))
		if err != nil {
			return err
		}
		closeConfigsOnError = true
		if err := app.Add(cfg.Spec.Name+"_configs", runtimecomponent.NewCloseComponent(configs.Close)); err != nil {
			return err
		}
	}
	if cfg.Spec.InitDistributed != nil {
		instanceStore, _ := cfg.Registry.(registry.InstanceStore)
		if err := cfg.Spec.InitDistributed(DistributedContext{
			Config:        cfg.Config,
			Configs:       configs,
			Infra:         cfg.Infra,
			App:           app,
			Etcd:          cfg.Etcd,
			Registry:      cfg.Registry,
			InstanceStore: instanceStore,
			Clients:       cfg.Clients,
			Logger:        cfg.Logger,
		}); err != nil {
			return err
		}
	}
	if cfg.Spec.Init != nil {
		if err := cfg.Spec.Init(RuntimeContext{Config: cfg.Config, Configs: configs, Infra: cfg.Infra, App: app, Logger: cfg.Logger}); err != nil {
			return err
		}
	}
	controlPlane := runtimemetrics.NewControlPlaneMetrics(cfg.Spec.Name)
	grpcService, err := addGRPCService(app, cfg, controlPlane)
	if err != nil {
		return err
	}
	if cfg.Registry != nil {
		if err := addServiceRegistration(app, cfg, controlPlane, grpcService); err != nil {
			return err
		}
	}
	if cfg.Etcd != nil && strings.TrimSpace(cfg.Config.Metadata.RoutesPrefix) != "" {
		if err := addGatewayMetadata(app, cfg, controlPlane); err != nil {
			return err
		}
	}
	closeConfigsOnError = false
	return nil
}
