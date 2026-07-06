package mysql

import (
	"context"
	"database/sql"
	"fmt"
)

func ensureDatabase(ctx context.Context, instance CompiledInstance) error {
	if !instance.Ensure.Enabled {
		return nil
	}
	if err := validateIdentifier(instance.Database); err != nil {
		return fmt.Errorf("validate mysql database name: %w", err)
	}
	db, err := openSQL(mysqlDriverName, instance.ServerDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql server for database ensure: %w", err)
	}
	exists, err := databaseExists(ctx, db, instance.Database)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	databaseName, err := quoteIdentifier(instance.Database)
	if err != nil {
		return fmt.Errorf("quote mysql database name: %w", err)
	}
	if err := validateIdentifier(instance.Ensure.Charset); err != nil {
		return fmt.Errorf("validate mysql database charset: %w", err)
	}
	if err := validateIdentifier(instance.Ensure.Collation); err != nil {
		return fmt.Errorf("validate mysql database collation: %w", err)
	}
	query := "CREATE DATABASE IF NOT EXISTS " + databaseName +
		" DEFAULT CHARACTER SET " + instance.Ensure.Charset +
		" DEFAULT COLLATE " + instance.Ensure.Collation
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create mysql database %q: %w", instance.Database, err)
	}
	return nil
}

func databaseExists(ctx context.Context, db *sql.DB, database string) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx,
		"SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?",
		database,
	).Scan(&name)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, fmt.Errorf("check mysql database %q: %w", database, err)
}
