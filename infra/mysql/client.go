package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/opencode-sig/runtime-sdk/infra/internal/configutil"

	_ "github.com/go-sql-driver/mysql"
)

// DB groups write and optional read pools for MySQL.
type DB struct {
	Writes []*sql.DB
	Reads  []*sql.DB

	nextRead uint64
}

// NewDB creates MySQL database/sql pools from config.
//
// The function intentionally returns *sql.DB based pools instead of GORM
// instances. ORM selection and model naming strategies belong to upper layers.
func NewDB(ctx context.Context, cfg Config) (*DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	if len(cfg.WriteDSNs) == 0 {
		return nil, fmt.Errorf("mysql write_dsns is required")
	}

	db := &DB{}

	for _, dsn := range cfg.WriteDSNs {
		write, err := openDB(ctx, dsn, cfg.WritePool)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		db.Writes = append(db.Writes, write)
	}

	for _, dsn := range cfg.ReadDSNs {
		read, err := openDB(ctx, dsn, cfg.ReadPool)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		db.Reads = append(db.Reads, read)
	}

	return db, nil
}

// Write returns the active write pool.
//
// Multiple write DSNs are initialized so callers can observe and manage all
// pools, but this package intentionally does not round-robin writes. Write
// failover has consistency implications and should be handled by MySQL proxy,
// VIP, orchestration tooling or an upper runtime policy.
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
	db, err := sql.Open("mysql", dsn)
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
