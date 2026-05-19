package config

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type FileProvider struct {
	root string
}

// NewFileProvider creates a local filesystem config provider.
func NewFileProvider(root string) *FileProvider {
	return &FileProvider{root: root}
}

// Load reads the file content for key.
//
// key may be absolute or relative to root.
func (p *FileProvider) Load(ctx context.Context, key string) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if key == "" {
		return nil, errors.New("config key is required")
	}
	return os.ReadFile(p.path(key))
}

// DecodeJSON decodes JSON bytes into a typed target value.
func DecodeJSON[T any](data []byte) (T, error) {
	var value T
	err := json.Unmarshal(data, &value)
	return value, err
}

// Decode decodes JSON or YAML bytes into a typed target value.
//
// Data starting with "{" or "[" is decoded as JSON; other data is decoded as
// YAML. Keeping JSON support lets remote config centers store structured JSON.
func Decode[T any](data []byte) (T, error) {
	var value T
	err := DecodeInto(data, &value)
	return value, err
}

// DecodeInto decodes JSON or YAML bytes into the provided target object.
func DecodeInto(data []byte, out any) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return json.Unmarshal(data, out)
	}
	return yaml.Unmarshal(data, out)
}

// path returns the local file path for key.
func (p *FileProvider) path(key string) string {
	if filepath.IsAbs(key) {
		return filepath.Clean(key)
	}
	return filepath.Join(p.root, key)
}
