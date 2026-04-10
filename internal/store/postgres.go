// Package store provides database access for the Financial Compliance Ledger.
// It manages PostgreSQL connection pooling and schema migrations.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresPool creates a pgxpool connection pool from the given
// database URL. It pings the database to verify connectivity before
// returning. Returns an error if the URL is invalid or the database
// is unreachable.
func NewPostgresPool(databaseURL string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return pool, nil
}

// RunMigrations runs all pending up migrations against the database at
// the given URL. migrationsPath should be a file:// URI pointing to the
// migrations directory (e.g., "file://migrations" from the project root,
// or "file://../../migrations" from a sub-package). If the database is
// already up to date, it returns nil (no error).
func RunMigrations(databaseURL, migrationsPath string) error {
	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
