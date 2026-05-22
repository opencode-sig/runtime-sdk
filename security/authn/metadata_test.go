package authn

import (
	"net/http"
	"strings"
	"testing"
)

func TestStripIdentityHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Auth-Subject", "spoofed")
	headers.Set("X-Auth-Attr-Department-ID", "spoofed")
	headers.Set("X-User-ID", "spoofed")
	headers.Set("X-Tenant-ID", "spoofed")
	headers.Set("Authorization", "Bearer token")

	StripIdentityHeaders(headers)

	for _, key := range []string{"X-Auth-Subject", "X-Auth-Attr-Department-ID", "X-User-ID", "X-Tenant-ID"} {
		if got := headers.Get(key); got != "" {
			t.Fatalf("%s was not stripped: %q", key, got)
		}
	}
	if got := headers.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestIdentityMetadata(t *testing.T) {
	pairs := IdentityMetadata(Identity{
		Subject:     "user:1",
		SubjectType: "user",
		TenantID:    "tenant-1",
		DisplayName: "Developer",
		Attributes: map[string]string{
			"department_id": "dept-1",
			"org.id":        "org-1",
			"bad space":     "ignored",
			"token":         "ignored",
			"empty":         " ",
		},
	})
	md := pairMap(pairs)
	if md["x-auth-subject"] != "user:1" {
		t.Fatalf("subject = %q", md["x-auth-subject"])
	}
	if md["x-auth-subject-type"] != "user" {
		t.Fatalf("subject type = %q", md["x-auth-subject-type"])
	}
	if md["x-tenant-id"] != "tenant-1" {
		t.Fatalf("tenant = %q", md["x-tenant-id"])
	}
	if md["x-auth-display-name"] != "Developer" {
		t.Fatalf("display name = %q", md["x-auth-display-name"])
	}
	if md["x-auth-attr-department-id"] != "dept-1" {
		t.Fatalf("department attr = %q", md["x-auth-attr-department-id"])
	}
	if md["x-auth-attr-org-id"] != "org-1" {
		t.Fatalf("org attr = %q", md["x-auth-attr-org-id"])
	}
	if _, ok := md["x-auth-attr-token"]; ok {
		t.Fatal("sensitive token attribute was forwarded")
	}
	if _, ok := md["x-auth-attr-bad-space"]; ok {
		t.Fatal("invalid attribute key was forwarded")
	}
}

func TestIdentityMetadataLimits(t *testing.T) {
	attributes := map[string]string{}
	for i := 0; i < MaxAttributeCount+10; i++ {
		attributes[string(rune('a'+i%26))+string(rune('a'+(i/26)%26))] = "value"
	}
	attributes["large"] = strings.Repeat("x", MaxAttributeValueBytes+1)

	pairs := IdentityMetadata(Identity{Attributes: attributes})
	md := pairMap(pairs)
	count := 0
	for key := range md {
		if strings.HasPrefix(key, AttributeMetadataPrefix) {
			count++
		}
	}
	if count > MaxAttributeCount {
		t.Fatalf("attribute count = %d, want <= %d", count, MaxAttributeCount)
	}
	if _, ok := md["x-auth-attr-large"]; ok {
		t.Fatal("oversized attribute value was forwarded")
	}
}

func pairMap(pairs []string) map[string]string {
	out := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = pairs[i+1]
	}
	return out
}
