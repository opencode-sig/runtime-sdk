package servicekit

import (
	"context"
	"errors"
	"fmt"

	runtimediscovery "github.com/opencode-sig/runtime-sdk/runtime/discovery"
	"github.com/opencode-sig/runtime-sdk/runtime/grpcclient"
	"google.golang.org/grpc"
)

// Clients provides service-name based gRPC connections for managed services.
//
// It hides discovery, resolver, load balancing and connection caching details
// from service modules. Service code only needs the target service name and,
// when desired, the generated protobuf client constructor.
type Clients struct {
	manager *grpcclient.Manager
}

// NewClients creates a service client set backed by runtime discovery.
func NewClients(source runtimediscovery.Discovery, options ...grpcclient.Option) (*Clients, error) {
	if source == nil {
		return nil, fmt.Errorf("service client discovery source is required")
	}
	return &Clients{
		manager: grpcclient.NewManager(grpcclient.NewResolverBuilder(source), options...),
	}, nil
}

// Conn returns a cached gRPC ClientConn for service.
func (c *Clients) Conn(ctx context.Context, service string) (*grpc.ClientConn, error) {
	if c == nil || c.manager == nil {
		return nil, fmt.Errorf("service clients are not configured")
	}
	return c.manager.Conn(ctx, service)
}

// Start implements lifecycle.Component. Connections are created lazily.
func (c *Clients) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Stop implements lifecycle.Component.
func (c *Clients) Stop(ctx context.Context) error {
	return errors.Join(c.Close(), ctx.Err())
}

// Health checks that the underlying client manager is usable.
func (c *Clients) Health(ctx context.Context) error {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.Health(ctx)
}

// Close closes all cached gRPC client connections.
func (c *Clients) Close() error {
	if c == nil || c.manager == nil {
		return nil
	}
	return c.manager.Close()
}

// Client returns a typed protobuf client for service.
func Client[T any](clients *Clients, ctx context.Context, service string, newClient func(grpc.ClientConnInterface) T) (T, error) {
	var zero T
	if newClient == nil {
		return zero, fmt.Errorf("grpc client constructor is required")
	}
	conn, err := clients.Conn(ctx, service)
	if err != nil {
		return zero, err
	}
	return newClient(conn), nil
}
