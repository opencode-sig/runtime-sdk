package servicekit

import (
	"context"
	"fmt"
)

// BoundAddresses reports the concrete service listener addresses selected for
// one DataPlane generation, together with the addresses published to registry.
type BoundAddresses struct {
	Service           string
	Generation        string
	GRPCListenAddr    string
	HTTPListenAddr    string
	AdvertiseGRPCAddr string
	AdvertiseHTTPAddr string
}

// BoundAddressHandler receives concrete listener addresses after a DataPlane
// has bound its service listeners and resolved registry advertise addresses.
type BoundAddressHandler func(context.Context, BoundAddresses)

type boundAddressComponent struct {
	snapshot func(context.Context) (BoundAddresses, error)
	handler  BoundAddressHandler
}

func newBoundAddressComponent(snapshot func(context.Context) (BoundAddresses, error), handler BoundAddressHandler) *boundAddressComponent {
	return &boundAddressComponent{snapshot: snapshot, handler: handler}
}

func (c *boundAddressComponent) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c == nil || c.handler == nil {
		return fmt.Errorf("bound address handler is required")
	}
	if c.snapshot == nil {
		return fmt.Errorf("bound address snapshot is required")
	}
	addresses, err := c.snapshot(ctx)
	if err != nil {
		return err
	}
	c.handler(ctx, addresses)
	return nil
}

func (c *boundAddressComponent) Stop(ctx context.Context) error {
	return nil
}

func (c *boundAddressComponent) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}
