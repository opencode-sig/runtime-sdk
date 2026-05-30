# go-template payment distributed sample

This example runs two independent managed gRPC services with the shared
`servicekit` runtime:

- `user`, registered as service name `user`
- `payment`, registered as service name `payment`

`payment.GetPayment` uses `servicekit.Clients` to resolve `user` through etcd
registry/discovery and calls `user.v1.UserService/GetUser` before returning.

## Run

Start etcd locally:

```sh
docker run --rm -p 2379:2379 --name runtime-sdk-etcd \
  quay.io/coreos/etcd:v3.5.17 \
  /usr/local/bin/etcd \
  --advertise-client-urls http://0.0.0.0:2379 \
  --listen-client-urls http://0.0.0.0:2379
```

In another terminal, run both services from the repository root with the
distributed launcher:

```sh
go run ./examples/go-template-payment/cmd/distributed
```

For production-like process boundaries, the same services can also be started
independently:

```sh
go run ./examples/go-template-payment/cmd/user -config-root examples/go-template-payment
go run ./examples/go-template-payment/cmd/payment -config-root examples/go-template-payment
```

Call payment with the sample client:

```sh
go run ./examples/go-template-payment/cmd/client
```

Expected result: payment returns a JSON response and the internal user lookup
has succeeded through service discovery.

Admin endpoints:

```text
payment health: http://127.0.0.1:9104/healthz
payment metrics: http://127.0.0.1:9104/metrics
user health:    http://127.0.0.1:9105/healthz
user metrics:   http://127.0.0.1:9105/metrics
```
