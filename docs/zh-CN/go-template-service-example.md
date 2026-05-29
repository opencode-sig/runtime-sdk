# 将 gRPC 微服务接入 go-template

本文说明外部 Go gRPC 微服务如何通过 `github.com/opencode-sig/runtime-sdk` 接入 go-template 运行时集群。

目标是保持业务代码不感知运行时细节：

- 业务代码只实现 protobuf handler 和应用逻辑；
- `servicekit` 负责 gRPC transport、生命周期、注册中心、Gateway 元数据发布和 control-command rebuild；
- go-template Gateway 通过注册中心和元数据发现服务，不需要为每个新服务写 Gateway 代码。

本文示例服务名为 `payment`。

## 运行流程

服务启动后：

1. 进程从本地文件、etcd 或其他 bootstrap source 加载 `servicekit.Config`。
2. `servicekit.Run` 启动受管理的 DataPlane。
3. DataPlane 启动 gRPC server 和 admin server。
4. 如果启用 etcd registry，服务注册自己的 advertised gRPC address。
5. 如果配置了 Gateway metadata，服务发布 route metadata 和 protobuf descriptors。
6. 如果配置了 etcd config 和 control command，服务监听 rebuild 命令，并用最新配置重建 DataPlane。

之后 go-template Gateway 会通过动态路由和服务发现把 HTTP 请求转发到该服务。

## 推荐项目结构

```text
payment-service/
  cmd/payment/main.go
  configs/runtime.yaml
  configs/logger.yaml
  configs/registry.yaml
  configs/infra/etcd.yaml
  configs/service/payment.yaml
  internal/bootstrap/gateway.go
  internal/bootstrap/module.go
  internal/handler/handler.go
  internal/service/service.go
  protobuf/payment/v1/payment.proto
  protobuf/payment/v1/payment.pb.go
  protobuf/payment/v1/payment_grpc.pb.go
  go.mod
```

边界约定：

- `internal/service`：业务逻辑；
- `internal/handler`：把 protobuf request 适配到业务逻辑；
- `internal/bootstrap`：唯一知道 runtime-sdk 注册和 Gateway 发布细节的包；
- `cmd/payment`：只负责把 service spec 和 config loader 交给 `servicekit.Run`。

## 初始化模块

```sh
go mod init github.com/acme/payment-service
go get github.com/opencode-sig/runtime-sdk
go get google.golang.org/grpc
go get google.golang.org/protobuf
```

## Protobuf 契约

`protobuf/payment/v1/payment.proto`

```proto
syntax = "proto3";

package payment.v1;

option go_package = "github.com/acme/payment-service/protobuf/payment/v1;paymentv1";

service PaymentService {
  rpc GetPayment(GetPaymentRequest) returns (PaymentResponse);
  rpc CreatePayment(CreatePaymentRequest) returns (PaymentResponse);
}

message GetPaymentRequest {
  string id = 1;
}

message CreatePaymentRequest {
  string order_id = 1;
  int64 amount = 2;
  string currency = 3;
}

message PaymentResponse {
  string id = 1;
  string order_id = 2;
  int64 amount = 3;
  string currency = 4;
  string status = 5;
}
```

生成 Go 代码后，包中应包含：

- `paymentv1.RegisterPaymentServiceServer`
- `paymentv1.File_payment_v1_payment_proto`
- request/response message 类型

## 业务服务

`internal/service/service.go`

```go
package service

import (
	"context"
	"fmt"
)

type Payment struct {
	ID       string
	OrderID  string
	Amount   int64
	Currency string
	Status   string
}

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) GetPayment(ctx context.Context, id string) (Payment, error) {
	if id == "" {
		return Payment{}, fmt.Errorf("payment id is required")
	}
	return Payment{
		ID:       id,
		OrderID:  "order-1001",
		Amount:   9900,
		Currency: "CNY",
		Status:   "paid",
	}, nil
}
```

## gRPC Handler

`internal/handler/handler.go`

```go
package handler

import (
	"context"

	"github.com/acme/payment-service/internal/service"
	paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
)

type Handler struct {
	paymentv1.UnimplementedPaymentServiceServer

	service *service.Service
}

func New(service *service.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := h.service.GetPayment(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &paymentv1.PaymentResponse{
		Id:       payment.ID,
		OrderId:  payment.OrderID,
		Amount:   payment.Amount,
		Currency: payment.Currency,
		Status:   payment.Status,
	}, nil
}
```

## Gateway 元数据发布

`internal/bootstrap/gateway.go`

```go
package bootstrap

import (
	paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
	"github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

func GatewayPublication() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
	return gatewaymeta.NewGatewayPublication(gatewaymeta.GatewayPublicationSpec{
		Service: "payment",
		File:    paymentv1.File_payment_v1_payment_proto,
		Routes: []gatewaymeta.GatewayRouteSpec{
			gatewaymeta.GET("GetPayment", "/v1/payments/{id}").
				Path("id", "id"),
			gatewaymeta.POST("CreatePayment", "/v1/payments").
				Body("*"),
		},
	})
}
```

服务只声明 HTTP method/path、RPC method 和参数绑定关系。`runtime-sdk` 会从 protobuf descriptor 推导 full method、request type、response type 和 descriptor id。
服务代码声明的是显式公网网关路径，例如 `/v1/payments/{id}`；SDK 只规范化斜杠，不会自动添加服务名前缀。如果应用需要 `/payment` 这类前缀，应在 route path 中显式声明。

如果 Gateway 启用认证，动态路由默认需要认证。确实属于认证白名单的路由
必须在服务自己的路由声明中显式 opt out：

```go
gatewaymeta.POST("Authenticate", "/v1/auth/authenticate").
	Body("*").
	Public()
```

这会在 route metadata 中发布 `auth.public=true`。白名单归属于服务路由契约，
不要在 Gateway 配置里维护独立 path 列表。

普通路由默认走 Gateway JSON response envelope。如果服务需要返回 HTML、CSV、
PDF、纯文本或其他浏览器/文件型响应，应显式声明 raw output：

```go
gatewaymeta.POST("RenderHTML", "/v1/payments/html/render").
	Body("*").
	RawResponse("text/html; charset=utf-8")
```

response message 可以使用默认 raw 字段名：

```proto
message RenderHTMLResponse {
  string body = 1;
  string content_type = 2;
  int32 status = 3;
  map<string, string> headers = 4;
}
```

`RawResponse` 会让 Gateway 直接把配置的 `body` 字段写入 HTTP response，
而不是包 JSON envelope。route metadata 中的 `content_type` 优先级高于
response message 字段。`status` 默认是 `200`，`headers` 是可选字段。
如果服务使用不同的 response 字段名，可以显式覆盖：

```go
gatewaymeta.POST("RenderHTML", "/v1/payments/html/render").
	Body("*").
	RawResponse("text/html; charset=utf-8").
	RawBody("html").
	RawStatus("http_status").
	RawHeaders("response_headers")
```

Gateway 实现应在加载路由时基于 protobuf descriptor 静态编译
`response.raw` 元数据，不能根据方法名、URL path 或仅仅存在
`body` / `content_type` 字段来猜测 raw 输出。

## 服务模块

`internal/bootstrap/module.go`

```go
package bootstrap

import (
	"google.golang.org/grpc"

	"github.com/acme/payment-service/internal/handler"
	"github.com/acme/payment-service/internal/service"
	paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func Module() (servicekit.Spec, error) {
	svc := service.New()
	h := handler.New(svc)

	return servicekit.NewSpec(servicekit.Spec{
		Name: "payment",
		RegisterGRPC: func(registrar grpc.ServiceRegistrar) {
			paymentv1.RegisterPaymentServiceServer(registrar, h)
		},
		GatewayPublication: GatewayPublication,
	})
}
```

## 进程入口

`cmd/payment/main.go`

```go
package main

import (
	"context"
	"flag"

	"github.com/acme/payment-service/internal/bootstrap"
	"github.com/opencode-sig/runtime-sdk/servicekit"
)

func main() {
	configRoot := flag.String("config-root", ".", "project root that contains configs")
	flag.Parse()

	ctx := context.Background()
	spec, err := bootstrap.Module()
	if err != nil {
		panic(err)
	}
	if err := servicekit.Run(ctx, servicekit.RunOptions{
		Spec: spec,
		LoadConfig: servicekit.NewConventionConfigLoader(servicekit.ConventionConfigLoaderOptions{
			Root: *configRoot,
		}),
	}); err != nil {
		panic(err)
	}
}
```

约定式 loader 默认读取 `configs/runtime.yaml`，然后从固定拆分 key 合成完整
`servicekit.Config`。`configs/runtime.yaml` 和 `configs/service/<service>.yaml`
必填；logger、registry 和 infra 片段可选。旧的
`servicekit.NewConfigLoader` 仍保留给单文件完整 `servicekit.Config` 使用。

## 本地文件配置

约定式配置与 go-template 主项目保持同一套目录规则。公共运行时配置放在共享文件中，`configs/service/payment.yaml` 只描述 payment 服务自己的配置片段。

`configs/runtime.yaml`

```yaml
config:
  provider: file
  key: configs/runtime.yaml
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
```

`configs/logger.yaml`

```yaml
service_name: payment
file_prefix: payment
level: info
stacktrace_level: error
format: json
enable_stdout: true
enable_file: false
caller: true
```

`configs/registry.yaml`

```yaml
provider: etcd
etcd:
  endpoints:
    - 127.0.0.1:2379
  prefix: /runtime/registry
```

`configs/infra/etcd.yaml`

```yaml
endpoints:
  - 127.0.0.1:2379
dial_timeout: 3s
```

`configs/service/payment.yaml`

```yaml
grpc_addr: :9004
advertise_grpc_addr: 127.0.0.1:9004
admin_addr: :9104
advertise_admin_addr: 127.0.0.1:9104
settings:
  payment_provider: sandbox
```

使用 etcd 配置中心时，在 `configs/runtime.yaml` 中切换 provider：

```yaml
config:
  provider: etcd
  key: configs/runtime.yaml
  etcd:
    endpoints:
      - 127.0.0.1:2379
    prefix: /runtime/config
control:
  commands_prefix: /runtime/control/commands
metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors
```

etcd 模式会从配置中心读取同名逻辑 key。如果 key 不存在且本地存在同名文件，SDK 会通过 `PutIfAbsent` 自动 seed；已有 etcd 值不会被覆盖。服务进程只会 seed 当前服务的 `configs/service/payment.yaml`，不会扫描或上传其他服务文件。服务进程只允许读配置中心时，可设置 `DisableEtcdAutoSeed` 关闭自动 seed。

## 接收 rebuild 命令

如果 `configs/runtime.yaml` 中的 `config.provider` 为 `etcd`，并配置了 `control.commands_prefix`，`servicekit.Run` 会启动 control watcher。合成后的 `servicekit.Config` 会把这些值放到 `runtime.config` 和 `runtime.control`。runtime-admin 或其他管理端可以发布 `rebuild` 或 `restart` 命令，服务收到后会重建 DataPlane。

rebuild 语义是 stop-start replacement：创建新 DataPlane，停止旧 generation，再启动新 generation。同一进程复用相同 gRPC/admin 端口时，不承诺零停机。

## 通过服务名访问其他 gRPC 服务

服务可以在 `InitDistributed` 中保存 `ctx.Clients`，然后按服务名获取连接：

```go
conn, err := ctx.Clients.Conn(ctx, "user")
if err != nil {
	return err
}
userClient := userv1.NewUserServiceClient(conn)
```

更顺手的泛型 helper：

```go
userClient, err := servicekit.Client(ctx.Clients, ctx, "user", userv1.NewUserServiceClient)
```

## 运行验证

在外部服务项目中运行：

```sh
go test ./...
go build ./cmd/payment
```

在 go-template 项目中运行 Gateway 和相关服务后，可以通过 Gateway HTTP 路由访问 `payment` 服务发布的接口。

runtime-sdk 仓库内也提供了可运行示例：

```text
examples/go-template-payment
```
