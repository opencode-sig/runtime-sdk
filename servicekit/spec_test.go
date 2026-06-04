package servicekit

import (
	"context"
	"errors"
	"testing"

	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
	"google.golang.org/grpc"
)

func TestNewSpecNormalizesReadinessChecks(t *testing.T) {
	checkErr := errors.New("db unavailable")
	check := func(context.Context) error { return checkErr }
	original := map[string]func(context.Context) error{
		" db ": check,
		"":     check,
		"nil":  nil,
	}

	spec, err := NewSpec(Spec{
		Name:               " payment ",
		RegisterGRPC:       func(grpc.ServiceRegistrar) {},
		GatewayPublication: emptyGatewayPublication,
		ReadinessChecks:    original,
	})
	if err != nil {
		t.Fatalf("new spec: %v", err)
	}
	original["db"] = func(context.Context) error { return nil }

	if spec.Name != "payment" {
		t.Fatalf("name = %q, want payment", spec.Name)
	}
	if len(spec.ReadinessChecks) != 1 {
		t.Fatalf("readiness checks = %d, want 1", len(spec.ReadinessChecks))
	}
	if err := spec.ReadinessChecks["db"](context.Background()); !errors.Is(err, checkErr) {
		t.Fatalf("readiness check error = %v, want %v", err, checkErr)
	}
}

func TestNewGRPCSpecCarriesReadinessChecks(t *testing.T) {
	check := func(context.Context) error { return nil }
	spec, err := NewGRPCSpec(GRPCSpec[struct{}]{
		Name:               "payment",
		Server:             struct{}{},
		Register:           func(grpc.ServiceRegistrar, struct{}) {},
		GatewayPublication: emptyGatewayPublication,
		ReadinessChecks: map[string]func(context.Context) error{
			"db": check,
		},
	})
	if err != nil {
		t.Fatalf("new grpc spec: %v", err)
	}
	if spec.ReadinessChecks["db"] == nil {
		t.Fatal("readiness check was not carried")
	}
}

func emptyGatewayPublication() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
	return nil, nil, nil
}
