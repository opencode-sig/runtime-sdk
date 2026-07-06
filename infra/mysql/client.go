package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"

	_ "github.com/go-sql-driver/mysql"
)

const mysqlDriverName = "mysql"

var openSQL = sql.Open

// DB groups write and optional read pools for MySQL.
type DB struct {
	Writes []*sql.DB
	Reads  []*sql.DB

	nextRead uint64
}

// NewDB creates MySQL database/sql pools from structured config.
//
// The function intentionally returns *sql.DB based pools instead of ORM
// instances. ORM selection and model naming strategies belong to upper layers.
func NewDB(ctx context.Context, cfg Config, name ...string) (*DB, error) {
	compiled, err := cfg.Compile()
	if err != nil {
		return nil, err
	}
	instance, err := compiled.Resolve(name...)
	if err != nil {
		return nil, err
	}
	return NewDBFromCompiled(ctx, instance)
}

// NewDBFromCompiled creates MySQL pools for one compiled instance.
func NewDBFromCompiled(ctx context.Context, instance CompiledInstance) (*DB, error) {
	if strings.TrimSpace(instance.WriteDSN) == "" {
		return nil, fmt.Errorf("mysql instance %q write dsn is required", instance.Name)
	}
	if err := ensureDatabase(ctx, instance); err != nil {
		return nil, err
	}

	db := &DB{}

	write, err := openDB(ctx, instance.WriteDSN, instance.Pool)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	db.Writes = append(db.Writes, write)

	for _, dsn := range instance.ReadDSNs {
		read, err := openDB(ctx, dsn, instance.ReadPool)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		db.Reads = append(db.Reads, read)
	}

	return db, nil
}

// Write returns the active write pool.
func (db *DB) Write() *sql.DB {
	if db == nil || len(db.Writes) == 0 {
		return nil
	}
	return db.Writes[0]
}

// Read returns a read pool using round-robin, falling back to Write when no read pool exists.
func (db *DB) Read() *sql.DB {
	if db == nil {
		return nil
	}
	if len(db.Reads) == 0 {
		return db.Write()
	}
	index := atomic.AddUint64(&db.nextRead, 1)
	return db.Reads[(int(index)-1)%len(db.Reads)]
}

// Ping verifies all configured MySQL pools are reachable.
func (db *DB) Ping(ctx context.Context) error {
	if db == nil {
		return nil
	}
	var errs []error
	for _, write := range db.Writes {
		if write != nil {
			errs = append(errs, write.PingContext(ctx))
		}
	}
	for _, read := range db.Reads {
		if read != nil {
			errs = append(errs, read.PingContext(ctx))
		}
	}
	return errors.Join(errs...)
}

// Close closes all configured MySQL pools.
func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	var errs []error
	for _, write := range db.Writes {
		if write != nil {
			errs = append(errs, write.Close())
		}
	}
	for _, read := range db.Reads {
		if read != nil {
			errs = append(errs, read.Close())
		}
	}
	return errors.Join(errs...)
}

func openDB(ctx context.Context, dsn string, pool PoolConfig) (*sql.DB, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	db, err := openSQL(mysqlDriverName, dsn)
	if err != nil {
		return nil, err
	}
	applyPool(db, pool.Normalize())
	return db, nil
}

func applyPool(db *sql.DB, pool PoolConfig) {
	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(configutil.MustDuration(pool.ConnMaxLifetime))
	db.SetConnMaxIdleTime(configutil.MustDuration(pool.ConnMaxIdleTime))
}
