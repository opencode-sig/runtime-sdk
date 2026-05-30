package bootstrap

import (
	userv1 "github.com/opencode-sig/runtime-sdk/examples/go-template-payment/protobuf/user/v1"
	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

const ServiceName = "user"

func GatewayPublication() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
	return gatewaymeta.NewGatewayPublication(gatewaymeta.GatewayPublicationSpec{
		Service: ServiceName,
		File:    userv1.File_examples_go_template_payment_protobuf_user_v1_user_proto,
		Routes: []gatewaymeta.GatewayRouteSpec{
			gatewaymeta.GET("GetUser", "/v1/users/{id}").
				Path("id", "id").
				Timeout("3s"),
		},
	})
}
