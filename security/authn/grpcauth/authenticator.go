package grpcauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/opencode-sig/runtime-sdk/observability/tracing"
	securityv1 "github.com/opencode-sig/runtime-sdk/protobuf/security/v1"
	"github.com/opencode-sig/runtime-sdk/security/authn"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

const roundRobinServiceConfig = `{"loadBalancingConfig":[{"round_robin":{}}]}`

type Authenticator struct {
	resolver resolver.Builder
	service  string
	timeout  time.Duration
	mu       sync.Mutex
	conn     *grpc.ClientConn
	client   securityv1.AuthServiceClient
}

func NewAuthenticator(resolver resolver.Builder, service string, timeout time.Duration) *Authenticator {
	service = strings.TrimSpace(service)
	if service == "" {
		service = "auth"
	}
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return &Authenticator{
		resolver: resolver,
		service:  service,
		timeout:  timeout,
	}
}

func (a *Authenticator) Authenticate(ctx context.Context, req authn.Request) (authn.Decision, error) {
	if strings.TrimSpace(req.CredentialType) == "" || strings.TrimSpace(req.Credential) == "" {
		return authn.Decision{}, authn.ErrMissingCredential
	}
	client, err := a.clientConn(ctx)
	if err != nil {
		return authn.Decision{}, errors.Join(authn.ErrUnavailable, err)
	}
	authCtx, cancel := authn.TimeoutContext(ctx, a.timeout)
	defer cancel()
	resp, err := client.Authenticate(authCtx, &securityv1.AuthenticateRequest{
		CredentialType: req.CredentialType,
		Credential:     req.Credential,
		TargetService:  req.TargetService,
		Context: &securityv1.RequestContext{
			RequestId:  req.RequestID,
			RouteId:    req.RouteID,
			HttpMethod: req.HTTPMethod,
			HttpPath:   req.HTTPPath,
			ClientIp:   req.ClientIP,
			UserAgent:  req.UserAgent,
			Headers:    req.Headers,
		},
	})
	if err != nil {
		return authn.Decision{}, mapAuthError(err)
	}
	if !resp.GetAllowed() {
		return authn.Decision{
			Allowed: false,
			Reason:  resp.GetReason(),
		}, nil
	}
	return authn.Decision{
		Allowed:  true,
		Reason:   resp.GetReason(),
		Identity: identityFromProto(resp.GetIdentity()),
	}, nil
}

func (a *Authenticator) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return nil
	}
	err := a.conn.Close()
	a.conn = nil
	a.client = nil
	return err
}

func (a *Authenticator) clientConn(ctx context.Context) (securityv1.AuthServiceClient, error) {
	if a == nil || a.resolver == nil {
		return nil, status.Error(codes.Unavailable, "auth resolver is not configured")
	}
	a.mu.Lock()
	if a.client != nil {
		client := a.client
		a.mu.Unlock()
		return client, nil
	}
	a.mu.Unlock()

	conn, err := a.dial(ctx)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client != nil {
		_ = conn.Close()
		return a.client, nil
	}
	a.conn = conn
	a.client = securityv1.NewAuthServiceClient(conn)
	return a.client, nil
}

func (a *Authenticator) dial(ctx context.Context) (*grpc.ClientConn, error) {
	scheme := strings.TrimSpace(a.resolver.Scheme())
	if scheme == "" {
		return nil, status.Error(codes.Unavailable, "auth resolver scheme is not configured")
	}
	dialCtx, cancel := authn.TimeoutContext(ctx, a.timeout)
	defer cancel()
	return grpc.DialContext(
		dialCtx,
		scheme+":///"+a.service,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor("gateway.auth")),
		grpc.WithDefaultServiceConfig(roundRobinServiceConfig),
		grpc.WithResolvers(a.resolver),
		grpc.WithBlock(),
	)
}

func mapAuthError(err error) error {
	switch status.Code(err) {
	case codes.Unauthenticated:
		return errors.Join(authn.ErrUnauthenticated, err)
	case codes.PermissionDenied:
		return errors.Join(authn.ErrPermissionDenied, err)
	case codes.InvalidArgument:
		return errors.Join(authn.ErrMissingCredential, err)
	default:
		return errors.Join(authn.ErrUnavailable, err)
	}
}

func identityFromProto(identity *securityv1.Identity) authn.Identity {
	if identity == nil {
		return authn.Identity{}
	}
	return authn.Identity{
		Subject:       identity.GetSubject(),
		SubjectType:   identity.GetSubjectType(),
		TenantID:      identity.GetTenantId(),
		DisplayName:   identity.GetDisplayName(),
		Attributes:    identity.GetAttributes(),
		ExpiresAtUnix: identity.GetExpiresAtUnix(),
	}
}
