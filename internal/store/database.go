// Package store owns the SQLite persistence boundary: opening the
// database, applying embedded Goose migrations, and configuring
// WAL mode and the busy timeout (spec 001-local-first-cli §8.3).
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// busyTimeoutMS bounds how long a command waits on SQLite locks when
// another process (a concurrent CLI invocation) holds the write lock.
const busyTimeoutMS = 5000

// Open resolves the database at path, applies pending embedded
// migrations, and returns a *sql.DB configured for CLI use.
// Opening is idempotent: re-opening an up-to-date database is a no-op.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)", path, busyTimeoutMS)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate applies embedded forward-only SQL migrations via Goose.
func migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// SchemaVersion reports the current applied migration version, for
// doctor reporting (spec §11).
func SchemaVersion(ctx context.Context, db *sql.DB) (int64, error) {
	goose.SetBaseFS(migrationsFS)
	goose.SetDialect("sqlite3")

	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
