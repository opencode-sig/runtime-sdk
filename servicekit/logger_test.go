package servicekit

import (
	"context"
	"testing"

	sdklogger "github.com/opencode-sig/runtime-sdk/logger"
	"github.com/opencode-sig/runtime-sdk/runtime/registry"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggerWithRuntimeIdentityAddsStableInstanceFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	base := sdklogger.Wrap(zap.New(core))

	log := loggerWithRuntimeIdentity(base, Config{
		Service: ServiceConfig{
			Name:              "auth",
			GRPCAddr:          ":2005",
			AdvertiseGRPCAddr: "127.0.0.1:2005",
		},
	}, Spec{Name: "auth"}, "distributed")

	log.Info(context.Background(), "hello")

	fields := logs.All()[0].ContextMap()
	if fields["runtime_service"] != "auth" {
		t.Fatalf("runtime_service = %v", fields["runtime_service"])
	}
	if fields["runtime_mode"] != "distributed" {
		t.Fatalf("runtime_mode = %v", fields["runtime_mode"])
	}
	expectedInstanceID := registry.InstanceID("auth", "127.0.0.1:2005")
	if fields["instance_id"] != expectedInstanceID {
		t.Fatalf("instance_id = %v, want %s", fields["instance_id"], expectedInstanceID)
	}
}

func TestLoggerWithRuntimeIdentityFallsBackToListenAddress(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	base := sdklogger.Wrap(zap.New(core))

	log := loggerWithRuntimeIdentity(base, Config{
		Service: ServiceConfig{
			Name:     "scheduler",
			GRPCAddr: "127.0.0.1:2104",
		},
	}, Spec{Name: "scheduler"}, "monolith")

	log.Info(context.Background(), "hello")

	fields := logs.All()[0].ContextMap()
	expectedInstanceID := registry.InstanceID("scheduler", "127.0.0.1:2104")
	if fields["instance_id"] != expectedInstanceID {
		t.Fatalf("instance_id = %v, want %s", fields["instance_id"], expectedInstanceID)
	}
}
