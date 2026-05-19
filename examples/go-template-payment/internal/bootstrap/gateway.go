package bootstrap

import (
	paymentv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/payment/v1"
	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

const ServiceName = "payment"

func GatewayPublication() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
	return gatewaymeta.NewGatewayPublication(gatewaymeta.GatewayPublicationSpec{
		Service: ServiceName,
		File:    paymentv1.File_examples_go_template_payment_protobuf_payment_v1_payment_proto,
		Routes: []gatewaymeta.GatewayRouteSpec{
			gatewaymeta.GET("GetPayment", "/v1/payments/{id}").
				Path("id", "id").
				Timeout("3s"),
			gatewaymeta.POST("CreatePayment", "/v1/payments").
				Body("*").
				Timeout("3s"),
		},
	})
}
