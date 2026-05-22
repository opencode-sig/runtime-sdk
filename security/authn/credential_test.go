package authn

import (
	"errors"
	"net/http"
	"testing"
)

func TestExtractCredential(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		wantType string
		want     string
		wantErr  error
	}{
		{
			name:     "bearer",
			headers:  header("Authorization", "Bearer dev-token"),
			wantType: CredentialTypeBearer,
			want:     "dev-token",
		},
		{
			name:     "basic",
			headers:  header("Authorization", "Basic basic-value"),
			wantType: CredentialTypeBasic,
			want:     "basic-value",
		},
		{
			name:     "raw authorization",
			headers:  header("Authorization", "Custom raw-token"),
			wantType: CredentialTypeAuthorization,
			want:     "Custom raw-token",
		},
		{
			name:     "api key",
			headers:  header("X-API-Key", "api-token"),
			wantType: CredentialTypeAPIKey,
			want:     "api-token",
		},
		{
			name:     "legacy apitoken",
			headers:  header("apitoken", "legacy-api-token"),
			wantType: CredentialTypeAPIKey,
			want:     "legacy-api-token",
		},
		{
			name: "authorization precedence",
			headers: http.Header{
				"Authorization": []string{"Bearer dev-token"},
				"X-API-Key":     []string{"api-token"},
				"apitoken":      []string{"legacy-api-token"},
			},
			wantType: CredentialTypeBearer,
			want:     "dev-token",
		},
		{
			name:    "missing",
			headers: http.Header{},
			wantErr: ErrMissingCredential,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, got, err := ExtractCredential(tt.headers)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if gotType != tt.wantType || got != tt.want {
				t.Fatalf("credential = (%q, %q), want (%q, %q)", gotType, got, tt.wantType, tt.want)
			}
		})
	}
}

func header(key string, value string) http.Header {
	headers := http.Header{}
	headers.Set(key, value)
	return headers
}
