package component

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	applogger "github.com/opencode-sig/runtime-sdk/logger"
)

type HTTPComponent struct {
	server  *http.Server
	logger  *applogger.Logger
	mu      sync.Mutex
	running bool
}

// NewHTTPComponent adapts an http.Server to a lifecycle Component.
func NewHTTPComponent(server *http.Server, logger *applogger.Logger) *HTTPComponent {
	return &HTTPComponent{server: server, logger: logger}
}

// Start listens on the HTTP address and starts Serve in the background.
//
// Listen happens before the goroutine starts, so address errors are returned
// synchronously to lifecycle.
func (c *HTTPComponent) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c.server == nil {
		return errors.New("http server is required")
	}
	if c.server.Addr == "" {
		return errors.New("http server addr is required")
	}

	listener, err := net.Listen("tcp", c.server.Addr)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	go func() {
		if c.logger != nil {
			c.logger.Info(context.Background(), "http server started", applogger.String("addr", c.server.Addr))
		}
		if err := c.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if c.logger != nil {
				c.logger.Error(context.Background(), "http server stopped unexpectedly", applogger.ErrorFields(err)...)
			}
		}
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()
	return nil
}

// Stop gracefully shuts down with http.Server.Shutdown.
//
// Shutdown stops accepting new connections and waits for in-flight requests
// until ctx expires.
func (c *HTTPComponent) Stop(ctx context.Context) error {
	if c.server == nil {
		return nil
	}
	c.mu.Lock()
	running := c.running
	c.running = false
	c.mu.Unlock()
	if !running {
		return nil
	}
	return c.server.Shutdown(ctx)
}

// Health reports whether the HTTP server is running.
func (c *HTTPComponent) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return errors.New("http server is not running")
	}
	return nil
}

// CloseComponent wraps a simple close function as a lifecycle Component.
//
// It is useful for resources that only need release during Stop.
type CloseComponent struct {
	close func() error
	mu    sync.Mutex
	done  bool
}

// NewCloseComponent creates a close function adapter.
func NewCloseComponent(close func() error) *CloseComponent {
	return &CloseComponent{close: close}
}

// Start validates that a close function exists.
func (c *CloseComponent) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c.close == nil {
		return errors.New("close function is required")
	}
	return nil
}

// Stop idempotently executes the close function.
//
// The done flag prevents repeated lifecycle Stop calls from closing twice.
func (c *CloseComponent) Stop(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		return nil
	}
	c.done = true
	closeFn := c.close
	c.mu.Unlock()
	return closeFn()
}

// Health reports healthy while the resource is not closed.
func (c *CloseComponent) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return errors.New("component is closed")
	}
	return nil
}
