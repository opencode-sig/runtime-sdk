package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/opencode-sig/runtime-sdk/observability/tracing"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
)

const roundRobinServiceConfig = `{"loadBalancingConfig":[{"round_robin":{}}]}`

// Option customizes Manager.
type Option func(*Manager)

// WithDialTimeout sets the maximum time spent establishing a new gRPC connection.
func WithDialTimeout(timeout time.Duration) Option {
	return func(m *Manager) {
		if timeout > 0 {
			m.dialTimeout = timeout
		}
	}
}

// WithDialOptions appends grpc dial options.
func WithDialOptions(options ...grpc.DialOption) Option {
	return func(m *Manager) {
		m.dialOptions = append(m.dialOptions, options...)
	}
}

// Manager owns grpc ClientConn instances for one runtime process.
type Manager struct {
	resolver    resolver.Builder
	dialTimeout time.Duration
	dialOptions []grpc.DialOption

	mu     sync.Mutex
	conns  map[string]*grpc.ClientConn
	dials  map[string]*dialCall
	closed bool
}

type dialCall struct {
	done chan struct{}
	conn *grpc.ClientConn
	err  error
}

// NewManager creates a gRPC client connection manager.
func NewManager(builder resolver.Builder, options ...Option) *Manager {
	m := &Manager{
		resolver:    builder,
		dialTimeout: 3 * time.Second,
		conns:       make(map[string]*grpc.ClientConn),
		dials:       make(map[string]*dialCall),
	}
	for _, option := range options {
		if option != nil {
			option(m)
		}
	}
	return m
}

// Conn returns a cached ClientConn for service, creating it on first use.
func (m *Manager) Conn(ctx context.Context, service string) (*grpc.ClientConn, error) {
	if m == nil {
		return nil, fmt.Errorf("grpc client manager is required")
	}
	if m.resolver == nil {
		return nil, fmt.Errorf("grpc resolver is required")
	}
	service = strings.TrimSpace(service)
	target, err := m.target(service)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("grpc client manager is closed")
	}
	conn := m.conns[service]
	if conn != nil && conn.GetState() == connectivity.Shutdown {
		delete(m.conns, service)
		conn = nil
	}
	if conn != nil {
		m.mu.Unlock()
		return conn, nil
	}
	if call := m.dials[service]; call != nil {
		m.mu.Unlock()
		return waitDial(ctx, call)
	}
	call := &dialCall{done: make(chan struct{})}
	m.dials[service] = call
	m.mu.Unlock()

	call.conn, call.err = m.dial(ctx, target)

	m.mu.Lock()
	if m.dials[service] == call {
		delete(m.dials, service)
	}
	if call.err == nil {
		if existing := m.conns[service]; existing != nil && existing.GetState() != connectivity.Shutdown {
			_ = call.conn.Close()
			call.conn = existing
		} else if m.closed {
			_ = call.conn.Close()
			call.conn = nil
			call.err = fmt.Errorf("grpc client manager is closed")
		} else {
			m.conns[service] = call.conn
		}
	}
	close(call.done)
	m.mu.Unlock()
	return call.conn, call.err
}

// Health checks that the manager is usable and existing conns are not closed.
func (m *Manager) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if m == nil {
		return fmt.Errorf("grpc client manager is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("grpc client manager is closed")
	}
	for service, conn := range m.conns {
		if conn == nil || conn.GetState() == connectivity.Shutdown {
			return fmt.Errorf("grpc client %s is closed", service)
		}
	}
	return nil
}

// Close closes all cached gRPC connections.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	var err error
	for service, conn := range m.conns {
		err = errors.Join(err, conn.Close())
		delete(m.conns, service)
	}
	return err
}

func waitDial(ctx context.Context, call *dialCall) (*grpc.ClientConn, error) {
	select {
	case <-call.done:
		return call.conn, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Manager) dial(ctx context.Context, target string) (*grpc.ClientConn, error) {
	timeout := m.dialTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor("runtime.grpcclient")),
		grpc.WithDefaultServiceConfig(roundRobinServiceConfig),
		grpc.WithResolvers(m.resolver),
		grpc.WithBlock(),
	}
	options = append(options, m.dialOptions...)
	return grpc.DialContext(dialCtx, target, options...)
}

func (m *Manager) target(service string) (string, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return "", fmt.Errorf("grpc service is required")
	}
	scheme := strings.TrimSpace(m.resolver.Scheme())
	if scheme == "" {
		return "", fmt.Errorf("grpc resolver scheme is required")
	}
	return scheme + ":///" + service, nil
}
