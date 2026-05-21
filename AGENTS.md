# AGENTS.md

本文是 `github.com/opencode-sig/runtime-sdk` 的 agent 工作说明和外部服务接入规范。它面向两类读者：

- 在本仓库内工作的自动化 agent 或维护者。
- 希望把独立 Go gRPC 微服务接入 runtime-sdk / go-template 运行时体系的服务开发者。

runtime-sdk 是一个应用无关的公开运行时 SDK。它的职责是承接业务服务不应该重复实现的运行时能力：gRPC 服务容器、admin 端点、健康检查、指标、tracing、日志、配置加载、注册发现、Gateway 元数据发布、控制命令和 DataPlane rebuild。业务服务仍然拥有自己的 protobuf、handler、业务逻辑和部署侧配置加载策略。

## 项目定位

runtime-sdk 的核心接入模型是：

1. 业务服务定义 protobuf service，并生成 Go 代码。
2. 业务服务实现 generated gRPC server。
3. 业务服务在 bootstrap 包里声明 `servicekit.Spec` 和 Gateway 元数据。
4. 进程入口调用 `servicekit.Run`，并提供 `LoadConfig`。
5. SDK 根据 `servicekit.Config` 启动受管理的 DataPlane。

服务进程会按配置组装：

- gRPC server。
- admin HTTP server：`/healthz`、`/metrics`，可选 `/debug/pprof/*`。
- gRPC health service。
- Prometheus metrics。
- OpenTelemetry trace context propagation。
- etcd registry registration。
- Gateway route metadata 和 protobuf descriptor publication。
- service-name based gRPC clients。
- infra client container。
- control command watcher。

其中 control command watcher 是进程级资源，位于 DataPlane generation 之外；这样 rebuild 停止旧 DataPlane 时不会把 watcher 自己停掉。

`servicekit.Run` 适合独立进程的分布式 gRPC 微服务。服务只需要把运行时声明交给 SDK；SDK 不应该知道业务包、业务配置路径、HTTP response envelope 或具体服务名。

## 公开能力总览

公开包以 `docs/zh-CN/api-boundary.md` 为准。没有列为公开能力的包，除非 release note 明确提升，否则应视为实现细节。

### `servicekit`

服务接入首选门面。对外能力包括：

- `Run(ctx, RunOptions)`：启动独立受管理 gRPC 服务进程。
- `Spec`：声明服务名、gRPC 注册函数、Gateway 元数据发布函数，以及初始化 hook。
- `GRPCSpec[T]` / `NewGRPCSpec`：从 generated protobuf registrar 构造 `Spec`。
- `Config`：服务运行时配置契约。
- `RuntimeContext`：普通初始化 hook 可见的上下文。
- `DistributedContext`：分布式运行时初始化 hook 可见的上下文，包含 etcd、registry、discovery-backed clients 等资源。
- `Configs`：按逻辑 key 读取配置中心内容，file / etcd 使用同一套 key 约定，例如 `configs/global/app.yaml`。
- `Infra` / `InfraContainer`：按需创建并托管 MySQL、Redis、Kafka、etcd client。
- `Clients` / `Client[T]`：按服务名获取 gRPC `ClientConn` 或 typed protobuf client。
- `DecodeSettings[T]`：从 `Config.Settings` 解码业务私有配置。

`servicekit` 会接管以下运行时问题：

- gRPC/admin server lifecycle。
- registry registration。
- Gateway metadata publication。
- health/metrics/tracing。
- graceful shutdown。
- control-command driven rebuild。

业务代码不应该在 handler 中手动注册服务实例、发布 Gateway 元数据或创建长期持有的 infra client。需要这些资源时，应通过 `Init` / `InitDistributed` 从 `RuntimeContext` 或 `DistributedContext` 获取。

全局配置应放在 `configs/global/` 命名空间下，并通过 `ctx.Configs.Decode(ctx, "configs/global/app.yaml", &cfg)` 读取。全局配置不归属任何具体服务，不应塞入某个 `configs/service/*.yaml`。

### `runtime/gatewaymeta`

Gateway 动态路由元数据生成和 protobuf descriptor 发布辅助能力。

常用入口：

- `NewGatewayPublication(GatewayPublicationSpec)`。
- `GET(method, path)`、`POST(method, path)`、`HTTP(httpMethod, method, path)`。
- `GatewayRouteSpec.Path(param, field)`。
- `GatewayRouteSpec.Query(param, field)`。
- `GatewayRouteSpec.Body(value)`。
- `GatewayRouteSpec.Timeout(value)`。
- `GatewayRouteSpec.Public()`：显式声明该路由属于认证白名单；Gateway 启用认证时应跳过 Authenticator。
- `GatewayRouteSpec.RawResponse(contentType)`：显式声明该路由返回原始 HTTP 内容，不走 JSON envelope。
- `GatewayRouteSpec.RawBody(field)` / `RawStatus(field)` / `RawHeaders(field)`：按需覆盖 raw response 使用的 protobuf 字段名。
- `GatewayDescriptorSet(files...)`。
- `DescriptorID(file)`。

约定：

- 一个 service proto 文件应只声明一个 gRPC service。
- 服务声明的是显式公网网关路径，例如 `/v1/payments/{id}`。
- SDK 只规范化路径斜杠，不会自动添加 `/{service}` 前缀。
- 如果应用需要 `/payment`、`/admin` 等业务前缀，应在 route path 中显式声明。
- Gateway 应根据 `RouteMeta.GRPC.Service` 和 `RouteMeta.GRPC.FullMethod` 转发，不应从 URL 前缀反推服务名。
- descriptor id 默认使用 proto package，要求 proto package 稳定。
- 路由认证白名单必须通过 `Public()` 写入 route metadata。Gateway 不应维护独立 path whitelist；未声明 public 的动态路由在 Gateway 启用认证时默认需要认证。
- 默认响应策略是不生成 `response` 元数据，并由 Gateway 包标准 JSON envelope。
- 需要 HTML、CSV、PDF、纯文本等浏览器或文件型输出时，服务必须显式调用 `RawResponse(contentType)`，由 Gateway 按 `response.raw` 直接写 HTTP body。
- Gateway 不应根据 response message 中的字段名、方法名或 content-type 猜测 raw 输出。

`NewGatewayPublication` 会从 protobuf descriptor 推导：

- `GRPC.FullMethod`，形如 `/payment.v1.PaymentService/GetPayment`。
- `GRPC.RequestType`。
- `GRPC.ResponseType`。
- `GRPC.DescriptorID`。
- descriptor set bytes，包括当前 proto 文件和 imports。

### `runtime/config`

配置源抽象和文件 / etcd provider。

常用能力：

- `ConfigProvider.Load(ctx, key)`。
- `NewFileProvider(root)`：从本地文件加载配置。
- `NewEtcdProvider(endpoints, prefix)`：从 etcd config center 加载配置。
- `NewEtcdProviderWithClient(client, prefix)`：复用外部 etcd client。
- `Decode[T]` / `DecodeInto`：自动识别 JSON 或 YAML。
- etcd store 能力：`Get`、`Put`、`PutIfAbsent`、`Delete`、`List`。

配置加载策略由接入方负责。推荐模式是本地 bootstrap 配置先启动，再根据 `runtime.config.provider` 决定是否从 etcd 加载最终配置。

### `runtime/control`

控制命令契约。

公开概念：

- `CommandRebuild = "rebuild"`。
- `CommandRestart = "restart"`。
- `Command{Command, Service, InstanceID, Reason, CreatedAt}`。
- `Store.Publish(ctx, command)`。
- `Store.Watch(ctx, service)`。
- `NewEtcdStore(client, prefix)`。

当 `servicekit.Config.Runtime.Config.Provider == "etcd"` 时，`servicekit.Run` 会创建进程级 `ProcessControl`，监听 `runtime.control.commands_prefix` 下的命令。命令可以发给具体 service，也可以发给 `all`。如果填写 `instance_id`，只有匹配实例会执行。

rebuild 语义是 stop-start replacement：

1. 调用 `LoadConfig` 获取最新配置。
2. 创建新的 DataPlane。
3. 停止旧 generation。
4. 启动新 generation。

这不是同进程同端口的零停机热切换。若 etcd endpoints 或 command prefix 变化，应重启进程，让 watcher 移动到新的控制通道。

### `runtime/registry`、`runtime/discovery`、`runtime/grpcclient`

注册、发现和服务名 gRPC client 能力。

`runtime/registry`：

- `Registry.Register(ctx, instance)`。
- `Registration.Renew(ctx)` / `Deregister(ctx)`。
- `InstanceStore.Services` / `Instances` / `Instance` / `Delete`。
- `NewEtcdRegistry(client, prefix)`。
- `NewMemoryRegistry()`。
- `NewServiceInstance(name, address, metadata)`。
- `InstanceID(service, address)`。

`runtime/discovery`：

- `Discovery.Resolve(ctx, service)`。
- `Discovery.Watch(ctx, service)`。
- `NewEtcdDiscovery(client, prefix)`。
- `NewMemoryDiscovery(registry)`。

`runtime/grpcclient`：

- `NewManager(resolverBuilder, options...)`。
- `Manager.Conn(ctx, service)`。
- `WithDialTimeout`。
- `WithDialOptions`。

`servicekit.Clients` 是业务服务侧更常用的门面。它把 discovery、resolver、round_robin、连接缓存、client-side tracing 都隐藏起来。

### `runtime/component` 和 `runtime/lifecycle`

生命周期组件能力。

`runtime/lifecycle`：

- `Component`：`Start`、`Stop`、`Health`。
- `Runtime.Add(name, component)`。
- `Runtime.Start(ctx)`。
- `Runtime.Stop(ctx)`。
- `Runtime.Health(ctx)`。

组件按注册顺序启动，失败时反向停止已启动组件；停止时按反向顺序关闭。

`runtime/component.NewGRPCService` 提供托管 gRPC component：

- 自动安装 gRPC health service。
- 自动安装 tracing server interceptor。
- 自动安装 metrics server interceptor。
- 可启动 admin HTTP server。
- admin HTTP server 只提供 `/healthz`、`/metrics` 和可选 pprof，不承载业务 HTTP 路由。

### `observability`

观测能力：

- `observability/health`：健康检查聚合器和标准 JSON `/healthz` handler。
- `observability/metrics`：独立 Prometheus registry、HTTP/gRPC request count 和 latency、默认 gRPC server 指标、custom collector 注册。
- `observability/tracing`：noop tracer provider 初始化、gRPC client/server trace context propagation interceptors。

SDK 默认保持 tracing 边界和上下文传播可用，但不强制外部 collector。需要真实 exporter 时，由上层应用或运行时替换 OpenTelemetry provider。

默认 gRPC server 指标由 SDK 的 gRPC component 统一注入，业务服务不需要重复实现。指标包括：

- `runtime_service_info{service}`：服务指标注册信息，值恒为 1。
- `grpc_server_started_total{service,grpc_type,grpc_service,grpc_method}`：gRPC 请求开始总数。
- `grpc_server_handled_total{service,grpc_type,grpc_service,grpc_method,grpc_code}`：gRPC 请求完成总数。
- `grpc_server_handling_seconds{service,grpc_type,grpc_service,grpc_method,grpc_code}`：gRPC 请求处理耗时。
- `grpc_server_msg_received_total{service,grpc_type,grpc_service,grpc_method}`：gRPC 请求消息接收总数。
- `grpc_server_msg_sent_total{service,grpc_type,grpc_service,grpc_method}`：gRPC 响应消息发送总数。
- `grpc_server_requests_total{service,method,code}`：gRPC 请求总数。
- `grpc_server_request_duration_seconds{service,method,code}`：gRPC 请求耗时。
- `grpc_server_inflight_requests{service,method}`：当前正在处理的 gRPC 请求数。
- `grpc_server_panics_total{service,method}`：handler panic 次数。
- `grpc_server_deadline_exceeded_total{service,method}`：DeadlineExceeded 次数。
- `grpc_server_request_message_bytes{service,method}`：请求 protobuf 消息大小。
- `grpc_server_response_message_bytes{service,method,code}`：响应 protobuf 消息大小。

为兼容旧监控面板，`runtime_grpc_requests_total` 和 `runtime_grpc_request_duration_seconds` 暂时保留；新看板应优先使用 `grpc_server_started_total`、`grpc_server_handled_total`、`grpc_server_handling_seconds` 这一组通用指标。

### `logger`

基于 zap 的结构化日志能力：

- `New(service)` / `NewWithConfig(config)`。
- `NewContext(service)` / `NewContextWithConfig(config)`。
- `Logger.Debug/Info/Warn/Error/DPanic/Panic/Fatal(ctx, msg, fields...)`。
- `Logger.WithModule(module)`、`Named(name)`、`With(fields...)`。
- 上下文字段：request id、user id、client ip、trace id、span id。
- 标准字段 helper：`Event`、`Module`、`Operation`、`Summary`、`Duration`、`StatusCode`、`ErrorCode`、`ErrorFields` 等。
- 事件 builder：`InfoEvent(...).WithOperation(...).WithError(...).Emit()`。
- stdout / file 输出，JSON / console 编码，按天轮转和保留期。

日志消息应保持稳定，便于聚合；变化较大的可读说明放到 `logger.Summary`。

### `rpcerror` 和 `apperror`

`rpcerror` 用于 gRPC 业务错误：

- `Business(code, message)`。
- `InvalidArgument(code, message)`。
- `NotFound(code, message)`。
- `Conflict(code, message)`。
- `WithStatus(grpcCode, code, message)`。
- `BusinessCode(status)`。

业务 code 会写入 `google.rpc.ErrorInfo` metadata，Gateway 可以提取后映射到 HTTP response envelope。

`apperror` 用于应用内错误包装：

- `New` / `Errorf`。
- `Wrap` / `Wrapf`。
- `FrameOf(err)`。

它会记录创建或包装位置，并保持 `errors.Is` / `errors.As` 可用。

### `infra`

可选基础设施 client 配置和构造能力：

- `infra/etcd`：etcd client、`NewClientAndWait`、`WaitReady`。
- `infra/mysql`：write/read pools、single/read_write mode、`Ping`、`Close`。
- `infra/redis`：single/sentinel/cluster、TLS、`Ping`。
- `infra/kafka`：producer、consumer、topic policy、TLS/SASL、connectivity `Check`。

统一规则：

- `Config.IsZero()` 表示组件未配置。
- `Config.Normalize()` 填充默认值，不返回错误。
- `Config.Validate()` 允许零配置，对非零配置严格校验。
- `New*` 构造 client 或 pool，但不强制连通。
- `Ping` 或 `Check` 执行显式连通性检查。

这让服务可以在可选 infra 未启用或暂时不可达时启动，由 readiness 或业务初始化决定何时检查。

## 推荐服务接入结构

外部服务建议使用以下结构：

```text
payment-service/
  cmd/payment/main.go
  configs/service.yaml
  internal/bootstrap/gateway.go
  internal/bootstrap/module.go
  internal/handler/handler.go
  internal/service/service.go
  protobuf/payment/v1/payment.proto
  protobuf/payment/v1/payment.pb.go
  protobuf/payment/v1/payment_grpc.pb.go
  go.mod
```

职责边界：

- `internal/service`：纯业务逻辑，不知道 registry、Gateway、control watcher。
- `internal/handler`：protobuf request/response 与业务模型的适配层。
- `internal/bootstrap`：唯一知道 runtime-sdk 接入细节的包。
- `cmd/<service>`：解析启动参数，加载配置，调用 `servicekit.Run`。
- `configs`：本地 bootstrap 配置。最终配置也可以在 etcd。

## 接入步骤

### 1. 初始化模块并依赖 SDK

```sh
go mod init github.com/acme/payment-service
go get github.com/opencode-sig/runtime-sdk
go get google.golang.org/grpc
go get google.golang.org/protobuf
```

### 2. 定义 protobuf service

建议一个 service proto 文件只放一个 gRPC service，便于 `runtime/gatewaymeta` 推导路由元数据。

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

生成代码后，接入层通常需要：

- `paymentv1.RegisterPaymentServiceServer`。
- `paymentv1.File_<path>_<name>_proto`。
- generated request / response message 类型。

### 3. 实现业务 service 和 gRPC handler

业务 service 保持普通 Go 代码。业务错误需要被 Gateway 识别时，优先使用 `rpcerror`：

```go
if req.GetId() == "" {
    return nil, rpcerror.InvalidArgument(10001, "payment id is required")
}
```

普通内部错误可以用 `apperror.Wrap` 保留调用位置：

```go
if err != nil {
    return apperror.Wrap(err, "load payment")
}
```

### 4. 声明 Gateway 元数据

`internal/bootstrap/gateway.go`：

```go
package bootstrap

import (
    paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
    "github.com/opencode-sig/runtime-sdk/runtime/gatewaymeta"
)

const ServiceName = "payment"

func GatewayPublication() ([]gatewaymeta.RouteMeta, map[string][]byte, error) {
    return gatewaymeta.NewGatewayPublication(gatewaymeta.GatewayPublicationSpec{
        Service: ServiceName,
        File:    paymentv1.File_payment_v1_payment_proto,
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
```

绑定规则：

- `Path("id", "id")`：把 HTTP path 参数 `{id}` 写入 request 的 `id` 字段。
- `Query("page", "page")`：把 query 参数 `page` 写入 request 的 `page` 字段。
- `Body("*")`：把完整 JSON body 映射到 request message。
- `Timeout("3s")`：设置单路由上游 gRPC 调用超时。为空时由 Gateway fallback，SDK 默认 fallback 为 3s。
- `Public()`：声明该路由是认证白名单。Gateway 启用认证时跳过 Authenticator；未声明 public 的动态路由默认需要认证。

路由 ID 默认由 service 和 RPC method 推导，例如 `payment.get`。如果路由已经对外发布并被外部系统依赖，可以显式设置 `GatewayRouteSpec.ID` 以维持兼容。

### 5. 声明服务模块

`internal/bootstrap/module.go`：

```go
package bootstrap

import (
    "context"

    "github.com/acme/payment-service/internal/handler"
    "github.com/acme/payment-service/internal/service"
    paymentv1 "github.com/acme/payment-service/protobuf/payment/v1"
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
        Init: func(ctx servicekit.RuntimeContext) error {
            settings, err := servicekit.DecodeSettings[service.Settings](ctx.Config)
            if err != nil {
                return err
            }
            paymentService.ApplySettings(settings)
            return nil
        },
        InitDistributed: func(ctx servicekit.DistributedContext) error {
            if ctx.Clients != nil {
                paymentService.SetUserClientFactory(func(reqCtx context.Context) (userv1.UserServiceClient, error) {
                    return servicekit.Client(ctx.Clients, reqCtx, "user", userv1.NewUserServiceClient)
                })
            }
            return nil
        },
    })
}
```

hook 使用建议：

- `Init`：读取 `Config.Settings`、注册纯进程内资源、给业务 service 注入配置。
- `InitDistributed`：使用 etcd registry、discovery-backed clients、分布式 infra 等资源。
- hook 不要启动自己的长期 goroutine，除非它也被封装为 lifecycle component 并能随 DataPlane 停止。
- hook 不要缓存旧 generation 的 infra client 到全局变量。rebuild 后旧 infra 会关闭。

### 6. 实现进程入口

`cmd/payment/main.go`：

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "strings"

    "github.com/acme/payment-service/internal/bootstrap"
    runtimeconfig "github.com/opencode-sig/runtime-sdk/runtime/config"
    "github.com/opencode-sig/runtime-sdk/servicekit"
)

func main() {
    configRoot := flag.String("config-root", "configs", "directory that contains config")
    configKey := flag.String("config-key", "service.yaml", "config key")
    flag.Parse()

    spec, err := bootstrap.Module()
    if err != nil {
        panic(err)
    }

    if err := servicekit.Run(context.Background(), servicekit.RunOptions{
        Spec: spec,
        LoadConfig: func(ctx context.Context, service string) (servicekit.Config, error) {
            return loadConfig(ctx, *configRoot, *configKey)
        },
    }); err != nil {
        panic(err)
    }
}

func loadConfig(ctx context.Context, root string, key string) (servicekit.Config, error) {
    fileProvider := runtimeconfig.NewFileProvider(root)
    data, err := fileProvider.Load(ctx, key)
    if err != nil {
        return servicekit.Config{}, fmt.Errorf("load config: %w", err)
    }
    cfg, err := runtimeconfig.Decode[servicekit.Config](data)
    if err != nil {
        return servicekit.Config{}, fmt.Errorf("decode config: %w", err)
    }
    if cfg.Runtime.Config.Root == "" {
        cfg.Runtime.Config.Root = root
    }
    if !strings.EqualFold(strings.TrimSpace(cfg.Runtime.Config.Provider), "etcd") {
        return cfg, nil
    }

    etcdProvider, ok := cfg.EtcdConfigStore()
    if !ok {
        return cfg, nil
    }
    defer func() { _ = etcdProvider.Close() }()

    data, err = etcdProvider.Load(ctx, cfg.Runtime.Config.Key)
    if err != nil {
        return servicekit.Config{}, fmt.Errorf("load etcd config: %w", err)
    }
    managed, err := runtimeconfig.Decode[servicekit.Config](data)
    if err != nil {
        return servicekit.Config{}, fmt.Errorf("decode etcd config: %w", err)
    }
    if managed.Runtime.Config.Root == "" &&
        (managed.Runtime.Config.Provider == "" || strings.EqualFold(strings.TrimSpace(managed.Runtime.Config.Provider), "file")) {
        managed.Runtime.Config.Root = root
    }
    return managed, nil
}
```

`LoadConfig` 会在初次启动和每次 rebuild 时调用。它必须返回完整的 `servicekit.Config` 快照。

## `servicekit.Config` 配置规范

最小常用 YAML：

```yaml
logger:
  service_name: payment
  file_prefix: payment
  level: info
  stacktrace_level: error
  format: json
  enable_stdout: true
  enable_file: false
  caller: true

runtime:
  config:
    provider: file
    root: configs
    key: service.yaml
  control:
    commands_prefix: /runtime/control/commands

service:
  name: payment
  grpc_addr: :9004
  advertise_grpc_addr: 127.0.0.1:9004
  admin_addr: :9104
  advertise_admin_addr: 127.0.0.1:9104
  enable_pprof: false

registry:
  provider: etcd
  etcd:
    endpoints:
      - 127.0.0.1:2379
    prefix: /runtime/registry

metadata:
  routes_prefix: /runtime/gateway/routes
  descriptors_prefix: /runtime/gateway/descriptors

infra: {}

settings: {}
```

字段说明：

- `logger`：SDK logger 配置契约。`logger.NewContextWithConfig` 可按该结构创建 logger；`servicekit.Run` 在 `RunOptions.Logger` 为空时会创建基于 `Spec.Name` 的默认 logger。
- `runtime.config.provider`：`file` 或 `etcd`。只有 `etcd` 会启用 process control watcher。
- `runtime.config.root`：文件配置根目录。本地 bootstrap 使用 `--config-root configs` 时可写成 `configs`；`servicekit.Configs` 会把 `configs` 目录规范化为项目根目录，使 `configs/global/app.yaml` 这类逻辑 key 在 file / etcd 下保持一致。
- `runtime.config.key`：配置逻辑 key。本地文件模式下是文件名或绝对路径；etcd 模式下是 etcd prefix 下的逻辑 key。
- `runtime.config.etcd`：etcd 配置中心 endpoints 和 prefix。
- `runtime.control.commands_prefix`：控制命令前缀，默认语义为 `/runtime/control/commands/<service>/<timestamp>`。
- `service.name`：服务名，应与 `servicekit.Spec.Name` 和 Gateway `Service` 保持一致。
- `service.grpc_addr`：本进程监听地址，必填。
- `service.advertise_grpc_addr`：注册中心和控制命令 instance id 使用的地址；为空时回退到 `grpc_addr`。
- `service.admin_addr`：admin HTTP 监听地址；为空则不启动 admin HTTP。
- `service.enable_pprof`：是否在 admin server 上挂载 pprof。
- `registry.provider`：当前分布式注册中心支持 `etcd`。
- `registry.etcd.prefix`：服务实例注册前缀。
- `metadata.routes_prefix`：Gateway route metadata 发布前缀，默认 `/runtime/gateway/routes`。
- `metadata.descriptors_prefix`：Gateway descriptor set 发布前缀，默认 `/runtime/gateway/descriptors`。
- `infra`：可选 infra 配置，由 `servicekit.Infra` 按需创建。
- `settings`：业务私有配置，SDK 不解释，业务用 `DecodeSettings[T]` 解码。

etcd bootstrap 示例：

```yaml
runtime:
  config:
    provider: etcd
    key: services/payment/service.yaml
    etcd:
      endpoints:
        - 127.0.0.1:2379
      prefix: /runtime/config
  control:
    commands_prefix: /runtime/control/commands
```

这种模式下，本地文件只需要包含连接配置中心所需的最小信息；最终完整配置由 etcd 中的 `/runtime/config/services/payment/service.yaml` 提供。

## Gateway 元数据发布规范

服务接入 Gateway 时必须提供 `GatewayPublication`。推荐只使用 `runtime/gatewaymeta` 的 builder，不要手写 `RouteMeta`，除非在做兼容迁移。

必须满足：

- `GatewayPublicationSpec.Service` 非空。
- `GatewayPublicationSpec.File` 是 generated proto file descriptor。
- `GatewayPublicationSpec.Routes` 至少一条。
- 每条 route 的 RPC method 必须存在于 proto service 中。
- HTTP path 必须以 `/` 开头，或者能被 builder 规范化为 `/` 开头。
- route timeout 若配置必须是 Go duration，例如 `500ms`、`3s`、`1m`。

发布后的 route metadata 形态：

```json
{
  "id": "payment.get",
  "enabled": true,
  "http": {
    "method": "GET",
    "path": "/v1/payments/{id}"
  },
  "grpc": {
    "service": "payment",
    "full_method": "/payment.v1.PaymentService/GetPayment",
    "request_type": "payment.v1.GetPaymentRequest",
    "response_type": "payment.v1.PaymentResponse",
    "descriptor_id": "payment.v1"
  },
  "binding": {
    "path": {
      "id": "id"
    }
  },
  "timeout": "3s"
}
```

调用 `Public()` 的路由会额外发布：

```json
{
  "auth": {
    "public": true
  }
}
```

Gateway 消费方应：

- 从 `metadata.routes_prefix` 加载 route metadata。
- 从 `metadata.descriptors_prefix/<descriptor_id>` 加载 descriptor set。
- 使用 `full_method` 和 dynamicpb 发起泛化 gRPC 调用。
- 使用 `binding` 把 HTTP path/query/body 映射到 protobuf request。
- 使用 discovery 解析 `grpc.service` 对应实例。
- 使用 `auth.public` 判断路由是否属于认证白名单。Gateway 启用认证时，只有 `auth.public=true` 的路由跳过认证，其他动态路由默认需要认证。

## Registry 和 Discovery 约定

启用 etcd registry 时，DataPlane 会注册自己的 advertised gRPC address。

实例 metadata 包含：

- `runtime`：`distributed` 或 `monolith`。
- `admin_addr`。
- `advertise_admin_addr`。

业务服务不应该直接依赖 registry key 结构。需要访问其他服务时，用 `servicekit.Clients`：

```go
userClient, err := servicekit.Client(ctx.Clients, reqCtx, "user", userv1.NewUserServiceClient)
if err != nil {
    return err
}
resp, err := userClient.GetUser(reqCtx, &userv1.GetUserRequest{Id: userID})
```

`Clients` 内部会使用 runtime discovery、gRPC resolver、round_robin 和连接缓存。

## Control Command 规范

发布 rebuild 命令示例：

```go
store := runtimecontrol.NewEtcdStore(etcdClient, "/runtime/control/commands")
err := store.Publish(ctx, runtimecontrol.Command{
    Command: runtimecontrol.CommandRebuild,
    Service: "payment",
    Reason:  "config updated",
})
```

发布到所有服务：

```go
err := store.Publish(ctx, runtimecontrol.Command{
    Command: runtimecontrol.CommandRestart,
    Service: "all",
    Reason:  "rotate credentials",
})
```

定向到单实例时填写 `InstanceID`。实例 ID 由 `registry.InstanceID(service, advertiseAddress)` 生成，`servicekit.ControlConfigForService` 使用同一规则。

注意：

- `rebuild` 和 `restart` 在当前实现中都会触发 DataPlane rebuild。
- rebuild 会重新调用 `LoadConfig`。
- process control watcher 不在 DataPlane generation 内，避免 rebuild 时停止自己。
- file-configured 进程不会启动 watcher。

## Infra 使用规范

业务服务需要 infra client 时，在 `Init` / `InitDistributed` 中通过 `ctx.Infra` 获取，并注入业务 service。

```go
Init: func(ctx servicekit.RuntimeContext) error {
    redisClient, err := ctx.Infra.Redis()
    if err != nil {
        return err
    }
    paymentService.SetRedis(redisClient)
    return nil
}
```

约束：

- 当前 `InfraContainer` 只支持默认实例名。传空、`default` 或不传名称均表示默认实例。
- 不要把 infra client 放入 package-level global。
- 不要跨 rebuild generation 复用旧 client。
- 如果业务需要多个命名实例，应先扩展 `servicekit.InfraConfig` 和 `InfraContainer`，并补测试。

示例 infra 配置片段：

```yaml
infra:
  redis:
    mode: single
    addrs:
      - 127.0.0.1:6379
    db: 0
  mysql:
    mode: single
    write_dsns:
      - user:pass@tcp(127.0.0.1:3306)/payment?parseTime=true
  kafka:
    brokers:
      - 127.0.0.1:9092
    client_id: payment
```

## 日志、错误和观测规范

日志：

- 使用 `logger.Logger` 而不是裸 `zap.Logger`，这样会自动附加 request、trace 等上下文字段。
- 事件名用 `logger.Event("payment_created")` 或事件 builder。
- 模块名用 `logger.Module("payment")`。
- 错误字段用 `logger.ErrorFields(err)`，保留 `apperror` frame。
- 动态说明放在 `logger.Summary`，稳定聚合维度放在结构化字段。

错误：

- 参数错误：`rpcerror.InvalidArgument(code, message)`。
- 资源不存在：`rpcerror.NotFound(code, message)`。
- 资源冲突：`rpcerror.Conflict(code, message)`。
- 普通业务失败：`rpcerror.Business(code, message)`。
- 内部错误链：`apperror.Wrap(err, "operation")`。

观测：

- admin `/healthz` 返回 JSON，失败时 HTTP 503。
- admin `/metrics` 暴露 Prometheus metrics。
- gRPC server/client 默认传播 W3C trace context。
- pprof 只在 `service.enable_pprof: true` 时开启。

## Agent 修改本仓库时的边界规则

维护 runtime-sdk 时必须保持应用无关：

- 不要导入任何外部项目的 `internal` 包。
- 不要硬编码应用名、服务名、本地配置路径、业务路由前缀或部署环境。
- `servicekit` 不应依赖 protobuf 生成包、Gin、Gateway response envelope 或具体平台应用；它可以聚合公开 optional infra 配置和 client facade，方便服务接入。
- `runtime/*` 包不要反向依赖顶层 `servicekit` 门面。
- core runtime 包不要依赖可选 MySQL、Redis、Kafka 包。
- `logger`、`rpcerror`、`apperror` 不要依赖 runtime 或 infra 包。
- 服务声明 Gateway 路由时写显式公网网关路径；`runtime/gatewaymeta` 不会自动添加服务名前缀。
- 新增公开能力时同步更新 README、中文 README、相关 docs 和本文件。
- 改动 public API 时补充或更新测试，并考虑 release note。

代码风格：

- 贴合现有包边界和命名风格。
- 优先使用标准库和已引入依赖，不为小问题引入新依赖。
- 配置结构体需要同时带 `json` 和 `yaml` tag。
- 配置默认值放在 `Normalize`，严格校验放在 `Validate`。
- 构造函数不应偷偷阻塞外部依赖连通性，除非函数名明确表示等待，例如 `NewClientAndWait`。
- lifecycle component 必须实现 `Start`、`Stop`、`Health`，并能重复安全停止。
- 涉及 goroutine 的组件必须在 `Stop` 中可退出。

## 验证命令

常规修改后至少运行：

```sh
go test ./...
```

发布前运行：

```sh
make verify
make race
```

`make verify` 包含：

- gofmt 检查。
- go module tidy 检查。
- 单元测试。
- `go vet`。
- API 边界检查。

涉及 etcd、registry、discovery、Gateway metadata 或外部 go-template 接入时，按影响范围补充：

```sh
make integration
make smoke-consumer
```

## 参考文件

- [README.zh-CN.md](README.zh-CN.md)
- [docs/zh-CN/go-template-service-example.md](docs/zh-CN/go-template-service-example.md)
- [docs/zh-CN/api-boundary.md](docs/zh-CN/api-boundary.md)
- [docs/zh-CN/release.md](docs/zh-CN/release.md)
- [examples/go-template-payment](examples/go-template-payment)
