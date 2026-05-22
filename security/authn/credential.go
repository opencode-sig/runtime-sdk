package authn

import (
	"net/http"
	"strings"
)

const (
	CredentialTypeAPIKey        = "api_key"
	CredentialTypeAuthorization = "authorization"
	CredentialTypeBasic         = "basic"
	CredentialTypeBearer        = "bearer"
)

func ExtractCredential(headers http.Header) (string, string, error) {
	if value := strings.TrimSpace(headers.Get("Authorization")); value != "" {
		parts := strings.Fields(value)
		if len(parts) == 2 {
			switch strings.ToLower(parts[0]) {
			case CredentialTypeBearer:
				return CredentialTypeBearer, parts[1], nil
			case CredentialTypeBasic:
				return CredentialTypeBasic, parts[1], nil
			}
		}
		return CredentialTypeAuthorization, value, nil
	}
	if value := strings.TrimSpace(headers.Get("X-API-Key")); value != "" {
		return CredentialTypeAPIKey, value, nil
	}
	if value := strings.TrimSpace(headers.Get("apitoken")); value != "" {
		return CredentialTypeAPIKey, value, nil
	}
	return "", "", ErrMissingCredential
}
