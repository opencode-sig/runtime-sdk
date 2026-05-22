package grpcauth

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	securityv1 "github.com/opencode-sig/runtime-sdk/protobuf/security/v1"
	"github.com/opencode-sig/runtime-sdk/security/authn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
)

func TestAuthenticatorAllowed(t *testing.T) {
	server := startAuthServer(t, fakeAuthServer{
		resp: &securityv1.AuthenticateResponse{
			Allowed: true,
			Identity: &securityv1.Identity{
				Subject:     "user:1",
				SubjectType: "user",
				TenantId:    "tenant-1",
				DisplayName: "Developer",
				Attributes:  map[string]string{"username": "developer"},
			},
		},
	})

	authenticator := NewAuthenticator(staticResolver(server.addr), "auth", time.Second)
	defer func() { _ = authenticator.Close() }()

	decision, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "dev-token",
	})
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if !decision.Allowed || decision.Identity.Subject != "user:1" || decision.Identity.Attributes["username"] != "developer" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestAuthenticatorDenied(t *testing.T) {
	server := startAuthServer(t, fakeAuthServer{
		resp: &securityv1.AuthenticateResponse{Allowed: false, Reason: "unauthenticated"},
	})

	authenticator := NewAuthenticator(staticResolver(server.addr), "auth", time.Second)
	defer func() { _ = authenticator.Close() }()

	decision, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "bad-token",
	})
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if decision.Allowed || decision.Reason != "unauthenticated" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestAuthenticatorUnavailable(t *testing.T) {
	authenticator := NewAuthenticator(nil, "auth", time.Second)
	_, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "dev-token",
	})
	if !errors.Is(err, authn.ErrUnavailable) {
		t.Fatalf("error = %v, want unavailable", err)
	}
}

func TestAuthenticatorTimeout(t *testing.T) {
	server := startAuthServer(t, fakeAuthServer{delay: 100 * time.Millisecond})

	authenticator := NewAuthenticator(staticResolver(server.addr), "auth", 20*time.Millisecond)
	defer func() { _ = authenticator.Close() }()

	_, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "dev-token",
	})
	if !errors.Is(err, authn.ErrUnavailable) {
		t.Fatalf("error = %v, want unavailable", err)
	}
}

type fakeAuthServer struct {
	securityv1.UnimplementedAuthServiceServer
	resp  *securityv1.AuthenticateResponse
	delay time.Duration
}

func (s fakeAuthServer) Authenticate(ctx context.Context, _ *securityv1.AuthenticateRequest) (*securityv1.AuthenticateResponse, error) {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &securityv1.AuthenticateResponse{Allowed: true}, nil
}

type authServerHandle struct {
	addr string
}

func startAuthServer(t *testing.T, srv securityv1.AuthServiceServer) authServerHandle {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	securityv1.RegisterAuthServiceServer(server, srv)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	return authServerHandle{addr: listener.Addr().String()}
}

type staticResolverBuilder struct {
	addr string
}

func staticResolver(addr string) resolver.Builder {
	return staticResolverBuilder{addr: addr}
}

func (b staticResolverBuilder) Scheme() string {
	return "static-auth"
}

func (b staticResolverBuilder) Build(_ resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	if err := cc.UpdateState(resolver.State{Addresses: []resolver.Address{{Addr: b.addr}}}); err != nil {
		return nil, err
	}
	return staticResolverInstance{}, nil
}

type staticResolverInstance struct{}

func (staticResolverInstance) ResolveNow(resolver.ResolveNowOptions) {}

func (staticResolverInstance) Close() {}
