// Package dbtest provides a shared real-Postgres connection for
// integration tests (files tagged "//go:build integration"). Not used by
// any unit test, so it's safe to always compile.
package dbtest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JRedCodes/rollout-controller/internal/db"
)

// Pool connects to a real Postgres database for integration tests and
// applies migrations, returning a ready-to-use pool that's closed
// automatically at test cleanup. Requires DATABASE_URL to point at a real,
// disposable database (e.g. the docker-compose postgres service) --
// deliberately does not fall back to any default, so an integration test
// can never silently run against a developer's real local dev database.
// Skips (doesn't fail) when DATABASE_URL is unset or unreachable, so
// `go test -tags=integration ./...` degrades gracefully without a live
// Postgres.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pgURL := os.Getenv("DATABASE_URL")
	if pgURL == "" {
		t.Skip("skipping integration test: DATABASE_URL not set (point it at a disposable Postgres, e.g. docker-compose's postgres service)")
	}

	migrateURL := "pgx5://" + strings.TrimPrefix(pgURL, "postgres://")
	if err := db.RunMigrations(migrateURL, migrationsPath()); err != nil {
		t.Skipf("skipping integration test: run migrations: %v", err)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, pgURL)
	if err != nil {
		t.Skipf("skipping integration test: connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// migrationsPath resolves the migrations directory relative to this file
// (not the caller's working directory), so Pool() works the same no
// matter which package's test invokes it.
func migrationsPath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}
