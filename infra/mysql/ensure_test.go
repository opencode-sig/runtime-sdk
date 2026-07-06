package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"
)

const fakeEnsureDriverName = "runtime_sdk_mysql_ensure_test"

var fakeEnsureState = struct {
	sync.Mutex
	exists  bool
	opened  int
	queries []string
	execs   []string
}{}

func init() {
	sql.Register(fakeEnsureDriverName, fakeEnsureDriver{})
}

func TestEnsureDatabaseDisabledDoesNotOpenConnection(t *testing.T) {
	resetFakeEnsureState(false)
	restore := replaceOpenSQLForEnsureTest()
	defer restore()

	err := ensureDatabase(t.Context(), CompiledInstance{
		Database:  "payment",
		ServerDSN: "server-dsn",
		Ensure:    EnsureDatabaseConfig{Enabled: false},
	})
	if err != nil {
		t.Fatalf("ensure disabled: %v", err)
	}
	fakeEnsureState.Lock()
	defer fakeEnsureState.Unlock()
	if fakeEnsureState.opened != 0 {
		t.Fatalf("opened = %d, want 0", fakeEnsureState.opened)
	}
}

func TestEnsureDatabaseSkipsExistingDatabase(t *testing.T) {
	resetFakeEnsureState(true)
	restore := replaceOpenSQLForEnsureTest()
	defer restore()

	err := ensureDatabase(t.Context(), CompiledInstance{
		Database:  "payment",
		ServerDSN: "server-dsn",
		Ensure: EnsureDatabaseConfig{
			Enabled:   true,
			Charset:   "utf8mb4",
			Collation: "utf8mb4_unicode_ci",
		},
	})
	if err != nil {
		t.Fatalf("ensure existing: %v", err)
	}
	fakeEnsureState.Lock()
	defer fakeEnsureState.Unlock()
	if fakeEnsureState.opened != 1 {
		t.Fatalf("opened = %d, want 1", fakeEnsureState.opened)
	}
	if len(fakeEnsureState.execs) != 0 {
		t.Fatalf("execs = %v, want none", fakeEnsureState.execs)
	}
}

func TestEnsureDatabaseCreatesMissingDatabase(t *testing.T) {
	resetFakeEnsureState(false)
	restore := replaceOpenSQLForEnsureTest()
	defer restore()

	err := ensureDatabase(t.Context(), CompiledInstance{
		Database:  "payment",
		ServerDSN: "server-dsn",
		Ensure: EnsureDatabaseConfig{
			Enabled:   true,
			Charset:   "utf8mb4",
			Collation: "utf8mb4_unicode_ci",
		},
	})
	if err != nil {
		t.Fatalf("ensure missing: %v", err)
	}
	fakeEnsureState.Lock()
	defer fakeEnsureState.Unlock()
	if len(fakeEnsureState.execs) != 1 {
		t.Fatalf("execs = %v, want one create", fakeEnsureState.execs)
	}
	if !strings.Contains(fakeEnsureState.execs[0], "CREATE DATABASE IF NOT EXISTS `payment`") {
		t.Fatalf("create query = %q", fakeEnsureState.execs[0])
	}
}

func replaceOpenSQLForEnsureTest() func() {
	previous := openSQL
	openSQL = func(driverName string, dsn string) (*sql.DB, error) {
		fakeEnsureState.Lock()
		fakeEnsureState.opened++
		fakeEnsureState.Unlock()
		return sql.Open(fakeEnsureDriverName, dsn)
	}
	return func() { openSQL = previous }
}

func resetFakeEnsureState(exists bool) {
	fakeEnsureState.Lock()
	defer fakeEnsureState.Unlock()
	fakeEnsureState.exists = exists
	fakeEnsureState.opened = 0
	fakeEnsureState.queries = nil
	fakeEnsureState.execs = nil
}

type fakeEnsureDriver struct{}

func (fakeEnsureDriver) Open(name string) (driver.Conn, error) {
	return fakeEnsureConn{}, nil
}

type fakeEnsureConn struct{}

func (fakeEnsureConn) Prepare(query string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (fakeEnsureConn) Close() error {
	return nil
}

func (fakeEnsureConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (fakeEnsureConn) Ping(ctx context.Context) error {
	return nil
}

func (fakeEnsureConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	fakeEnsureState.Lock()
	defer fakeEnsureState.Unlock()
	fakeEnsureState.queries = append(fakeEnsureState.queries, query)
	if fakeEnsureState.exists {
		return &fakeEnsureRows{values: [][]driver.Value{{"payment"}}}, nil
	}
	return &fakeEnsureRows{}, nil
}

func (fakeEnsureConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	fakeEnsureState.Lock()
	defer fakeEnsureState.Unlock()
	fakeEnsureState.execs = append(fakeEnsureState.execs, query)
	return driver.RowsAffected(1), nil
}

type fakeEnsureRows struct {
	values [][]driver.Value
	index  int
}

func (r *fakeEnsureRows) Columns() []string {
	return []string{"SCHEMA_NAME"}
}

func (r *fakeEnsureRows) Close() error {
	return nil
}

func (r *fakeEnsureRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
