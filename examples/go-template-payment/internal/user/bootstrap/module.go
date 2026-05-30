package bootstrap

import (
	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/user/handler"
	userservice "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/user/service"
	userv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/user/v1"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func Module() (servicekit.Spec, error) {
	userService := userservice.New()
	userHandler := handler.New(userService)

	return servicekit.NewGRPCSpec(servicekit.GRPCSpec[userv1.UserServiceServer]{
		Name:               ServiceName,
		Server:             userHandler,
		Register:           userv1.RegisterUserServiceServer,
		GatewayPublication: GatewayPublication,
	})
}
