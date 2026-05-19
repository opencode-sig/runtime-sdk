package bootstrap

import (
	"context"

	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/handler"
	"github.com/opencode-sig/runtime-sdk/examples/go-template-payment/internal/service"
	paymentv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/payment/v1"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func Module() (servicekit.Spec, error) {
	paymentService := service.New()
	paymentHandler := handler.New(paymentService)

	return servicekit.NewGRPCSpec(servicekit.GRPCSpec[paymentv1.PaymentServiceServer]{
		Name:               ServiceName,
		Server:             paymentHandler,
		Register:           paymentv1.RegisterPaymentServiceServer,
		GatewayPublication: GatewayPublication,
		InitDistributed: func(ctx servicekit.DistributedContext) error {
			if ctx.Clients != nil {
				paymentService.SetUserProbe(userProbe{clients: ctx.Clients})
			}
			return nil
		},
		Init: func(ctx servicekit.RuntimeContext) error {
			settings, err := servicekit.DecodeSettings[service.Settings](ctx.Config)
			if err != nil {
				return err
			}
			paymentService.ApplySettings(settings)
			return nil
		},
	})
}

type userProbe struct {
	clients *servicekit.Clients
}

func (p userProbe) CheckUser(ctx context.Context) error {
	_, err := p.clients.Conn(ctx, "user")
	return err
}
