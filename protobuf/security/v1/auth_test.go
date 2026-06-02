package securityv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestAuthenticateRequestTargetServiceRoundTrip(t *testing.T) {
	original := &AuthenticateRequest{
		CredentialType: "bearer",
		Credential:     "dev-token",
		TargetService:  "oidc-auth",
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}

	var decoded AuthenticateRequest
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if decoded.GetTargetService() != "oidc-auth" {
		t.Fatalf("target_service = %q, want oidc-auth", decoded.GetTargetService())
	}
}
