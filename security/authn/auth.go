package authn

import (
	"context"
	"time"
)

type Authenticator interface {
	Authenticate(ctx context.Context, req Request) (Decision, error)
}

type Request struct {
	CredentialType string
	Credential     string
	TargetService  string
	RouteID        string
	HTTPMethod     string
	HTTPPath       string
	ClientIP       string
	UserAgent      string
	RequestID      string
	Headers        map[string]string
}

type Decision struct {
	Allowed  bool
	Reason   string
	Identity Identity
}

type Identity struct {
	Subject       string
	SubjectType   string
	TenantID      string
	DisplayName   string
	Attributes    map[string]string
	ExpiresAtUnix int64
}

type NoopAuthenticator struct{}

func (NoopAuthenticator) Authenticate(ctx context.Context, _ Request) (Decision, error) {
	select {
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	default:
	}
	return Decision{Allowed: true}, nil
}

func TimeoutContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return context.WithTimeout(ctx, timeout)
}
