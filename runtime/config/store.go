package config

import "context"

type Entry struct {
	Key     string
	Value   []byte
	Version string
}

type Store interface {
	Get(ctx context.Context, key string) (Entry, error)
	Put(ctx context.Context, key string, value []byte) error
	PutIfAbsent(ctx context.Context, key string, value []byte) (created bool, err error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]Entry, error)
}
