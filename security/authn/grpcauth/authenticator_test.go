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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
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

func TestAuthenticatorPassesTargetService(t *testing.T) {
	received := make(chan *securityv1.AuthenticateRequest, 1)
	server := startAuthServer(t, fakeAuthServer{received: received})

	authenticator := NewAuthenticator(staticResolver(server.addr), "auth", time.Second)
	defer func() { _ = authenticator.Close() }()

	_, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "dev-token",
		TargetService:  "legacy-auth",
	})
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	req := <-received
	if req.GetTargetService() != "legacy-auth" {
		t.Fatalf("target_service = %q, want legacy-auth", req.GetTargetService())
	}
}

func TestAuthenticatorEmptyTargetServiceKeepsDefaultRequest(t *testing.T) {
	received := make(chan *securityv1.AuthenticateRequest, 1)
	server := startAuthServer(t, fakeAuthServer{received: received})

	authenticator := NewAuthenticator(staticResolver(server.addr), "auth", time.Second)
	defer func() { _ = authenticator.Close() }()

	decision, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "dev-token",
	})
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("decision = %#v, want allowed", decision)
	}
	req := <-received
	if req.GetTargetService() != "" {
		t.Fatalf("target_service = %q, want empty", req.GetTargetService())
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

func TestAuthenticatorPermissionDeniedError(t *testing.T) {
	server := startAuthServer(t, fakeAuthServer{
		err: status.Error(codes.PermissionDenied, "forbidden"),
	})

	authenticator := NewAuthenticator(staticResolver(server.addr), "auth", time.Second)
	defer func() { _ = authenticator.Close() }()

	_, err := authenticator.Authenticate(context.Background(), authn.Request{
		CredentialType: "bearer",
		Credential:     "dev-token",
	})
	if !errors.Is(err, authn.ErrPermissionDenied) {
		t.Fatalf("error = %v, want permission denied", err)
	}
}

type fakeAuthServer struct {
	securityv1.UnimplementedAuthServiceServer
	resp     *securityv1.AuthenticateResponse
	err      error
	delay    time.Duration
	received chan *securityv1.AuthenticateRequest
}

func (s fakeAuthServer) Authenticate(ctx context.Context, req *securityv1.AuthenticateRequest) (*securityv1.AuthenticateResponse, error) {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	if s.received != nil {
		s.received <- req
	}
	if s.err != nil {
		return nil, s.err
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
