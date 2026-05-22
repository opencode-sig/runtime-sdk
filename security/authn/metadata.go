package authn

import (
	"net/http"
	"sort"
	"strings"
	"unicode"
)

const (
	AttributeMetadataPrefix = "x-auth-attr-"
	MaxAttributeCount       = 32
	MaxAttributeValueBytes  = 512
	MaxAttributeTotalBytes  = 8 * 1024
)

var identityHeaders = []string{
	"X-Auth-Subject",
	"X-Auth-Subject-Type",
	"X-Auth-Display-Name",
	"X-User-ID",
	"X-Tenant-ID",
	"X-User-Roles",
	"X-User-Scopes",
}

func StripIdentityHeaders(headers http.Header) {
	for _, header := range identityHeaders {
		headers.Del(header)
	}
	for header := range headers {
		name := strings.ToLower(header)
		if strings.HasPrefix(name, "x-auth-") || strings.HasPrefix(name, "x-user-") || name == "x-tenant-id" {
			headers.Del(header)
		}
	}
}

func IdentityMetadata(identity Identity) []string {
	pairs := make([]string, 0, 8)
	appendPair := func(key string, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			pairs = append(pairs, key, value)
		}
	}
	appendPair("x-auth-subject", identity.Subject)
	appendPair("x-auth-subject-type", identity.SubjectType)
	appendPair("x-tenant-id", identity.TenantID)
	appendPair("x-auth-display-name", identity.DisplayName)
	appendAttributePairs(&pairs, identity.Attributes)
	return pairs
}

func appendAttributePairs(pairs *[]string, attributes map[string]string) {
	if len(attributes) == 0 {
		return
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		if attributeMetadataKey(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	totalBytes := 0
	count := 0
	for _, key := range keys {
		if count >= MaxAttributeCount {
			return
		}
		metadataKey := attributeMetadataKey(key)
		value := strings.TrimSpace(attributes[key])
		if metadataKey == "" || value == "" || isSensitiveAttributeKey(key) || len(value) > MaxAttributeValueBytes {
			continue
		}
		totalBytes += len(metadataKey) + len(value)
		if totalBytes > MaxAttributeTotalBytes {
			return
		}
		*pairs = append(*pairs, metadataKey, value)
		count++
	}
}

func attributeMetadataKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			builder.WriteByte('-')
		default:
			return ""
		}
	}
	normalized := strings.Trim(builder.String(), "-")
	if normalized == "" {
		return ""
	}
	return AttributeMetadataPrefix + normalized
}

func isSensitiveAttributeKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || unicode.IsSpace(r)
	})
	for _, part := range parts {
		switch part {
		case "password", "passwd", "pwd", "token", "secret", "credential", "private", "key":
			return true
		}
	}
	return false
}
