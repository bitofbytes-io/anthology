// Package testdb opens the disposable PostgreSQL database used by repository
// regression tests.
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // register the postgres driver

	"anthology/internal/platform/migrate"
)

// EnvVar names the connection string for a dedicated, disposable test database.
const EnvVar = "ANTHOLOGY_TEST_DATABASE_URL"

// migrationLockKey serializes migrations across test binaries, which `go test
// ./...` runs in parallel against the same database.
const migrationLockKey = 0x616e74686f6c6f67 // "anthology"

// Open connects to the test database and applies migrations, skipping the test
// when EnvVar is unset. The connection is closed when the test finishes.
func Open(t testing.TB) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv(EnvVar)
	if dsn == "" {
		t.Skip("set " + EnvVar + " for PostgreSQL regression tests")
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, int64(migrationLockKey)); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, int64(migrationLockKey)) }()

	if err := migrate.Apply(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	return db
}
