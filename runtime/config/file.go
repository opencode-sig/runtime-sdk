package config

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
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

// Get reads one local file config entry.
func (p *FileProvider) Get(ctx context.Context, key string) (Entry, error) {
	data, err := p.Load(ctx, key)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Key: filepath.ToSlash(filepath.Clean(key)), Value: data}, nil
}

// List reads local file config entries below prefix.
func (p *FileProvider) List(ctx context.Context, prefix string) ([]Entry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	prefix = strings.Trim(filepath.ToSlash(filepath.Clean(prefix)), "/")
	root := p.path(prefix)
	stat, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !stat.IsDir() {
		entry, err := p.Get(ctx, prefix)
		if err != nil {
			return nil, err
		}
		return []Entry{entry}, nil
	}
	out := make([]Entry, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		baseRoot := p.root
		if strings.TrimSpace(baseRoot) == "" {
			baseRoot = "."
		}
		rel, err := filepath.Rel(baseRoot, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(strings.Trim(key, "/"), prefix) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, Entry{Key: key, Value: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
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
